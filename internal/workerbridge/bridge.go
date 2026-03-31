package workerbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"time"

	"github.com/tuyen/agenthub/internal/worker"
)

// Bridge connects AgentHub queue to OpenClaw agent execution.
type Bridge struct {
	cfg    *Config
	client *http.Client
	agent  *Agent
}

// NewBridge creates a new bridge with the given config.
func NewBridge(cfg *Config) *Bridge {
	return &Bridge{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
		agent:  NewAgent(cfg),
	}
}

// Run starts the main poll → claim → delegate → wait → verify → report loop.
func (b *Bridge) Run(ctx context.Context) error {
	log.Printf("[Bridge] Starting (role=%s, agent=%s, poll=%ds, timeout=%ds)",
		b.cfg.TaskType, b.cfg.AgentID, b.cfg.PollInterval, b.cfg.TaskTimeout)

	ticker := time.NewTicker(time.Duration(b.cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[Bridge] Shutting down")
			return nil
		case <-ticker.C:
			if err := b.pollAndProcess(ctx); err != nil {
				log.Printf("[Bridge] Error: %v", err)
			}
		}
	}
}

func (b *Bridge) pollAndProcess(ctx context.Context) error {
	// 1. Poll queue
	tasks, err := b.pollQueue()
	if err != nil {
		return fmt.Errorf("poll: %w", err)
	}
	if len(tasks) == 0 {
		return nil
	}

	task := tasks[0]
	log.Printf("[Bridge] Task: %s (%s, %s)", task.ID, task.Title, task.Priority)

	// 2. Claim
	if err := b.claimTask(task.ID); err != nil {
		return fmt.Errorf("claim %s: %w", task.ID, err)
	}
	log.Printf("[Bridge] Claimed: %s", task.ID)

	// 3. Progress
	b.updateProgress(task.ID, 10, "delegating to agent")

	// 4. Delegate to agent
	result := b.agent.Run(ctx, task)

	// 5. Verify build
	buildOK := true
	if result.Success {
		b.updateProgress(task.ID, 80, "agent done, verifying build")
		buildOK = b.verifyBuild()
	}

	// 6. Report
	b.updateProgress(task.ID, 90, "reporting")
	if err := b.reportResult(task.ID, result, buildOK); err != nil {
		log.Printf("[Bridge] Report failed: %v", err)
	}

	log.Printf("[Bridge] Done: %s (ok=%v build=%v)", task.ID, result.Success, buildOK)
	return nil
}

func (b *Bridge) pollQueue() ([]worker.Task, error) {
	url := fmt.Sprintf("%s/api/agent/tasks/queue?task_type=%s", b.cfg.APIURL, b.cfg.TaskType)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+b.cfg.AgentToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("queue returned %d", resp.StatusCode)
	}

	var result struct {
		Tasks []worker.Task `json:"tasks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

func (b *Bridge) claimTask(taskID string) error {
	url := fmt.Sprintf("%s/api/agent/tasks/%s/claim", b.cfg.APIURL, taskID)
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Set("Authorization", "Bearer "+b.cfg.AgentToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("claim returned %d", resp.StatusCode)
	}
	return nil
}

func (b *Bridge) updateProgress(taskID string, progress int, note string) {
	url := fmt.Sprintf("%s/api/agent/tasks/%s/progress", b.cfg.APIURL, taskID)
	payload := map[string]interface{}{"progress": progress, "note": note}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.cfg.AgentToken)

	resp, err := b.client.Do(req)
	if err != nil {
		log.Printf("[Bridge] Progress failed: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

func (b *Bridge) verifyBuild() bool {
	projectDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	log.Printf("[Bridge] Running go build ./...")
	cmd := exec.Command("/bin/bash", "-c", "export PATH=$PATH:/usr/local/go/bin && go build ./...")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[Bridge] BUILD FAILED:\n%s", string(out))
		return false
	}
	log.Printf("[Bridge] Build OK")
	return true
}

func (b *Bridge) reportResult(taskID string, result *TaskResult, buildOK bool) error {
	status := "done"
	notes := result.Output

	if !result.Success {
		status = "failed"
		notes = "Agent error: " + result.Error
	} else if !buildOK {
		status = "failed"
		notes = "Build verification failed"
	}

	// Truncate notes if too long
	if len(notes) > 2000 {
		notes = notes[:2000] + "...(truncated)"
	}

	url := fmt.Sprintf("%s/api/agent/tasks/%s/complete", b.cfg.APIURL, taskID)
	payload := map[string]interface{}{"status": status, "notes": notes}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.cfg.AgentToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("complete returned %d", resp.StatusCode)
	}
	log.Printf("[Bridge] Reported: %s → %s", taskID, status)
	return nil
}

// Helper to avoid unused import
