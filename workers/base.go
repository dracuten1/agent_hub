// Package workers implements the polling worker framework for AgentHub.
package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// API is the AgentHub API client.
type API struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// NewAPI creates a new AgentHub API client.
func NewAPI(baseURL, token string) *API {
	return &API{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Token:   token,
		Client:  http.DefaultClient,
	}
}

func (a *API) do(req *http.Request) (*http.Response, error) {
	if a.Token != "" {
		req.Header.Set("Authorization", "Bearer "+a.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	return a.Client.Do(req)
}

func (a *API) get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", a.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *API) patchJSON(path string, body interface{}) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest("PATCH", a.BaseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return a.do(req)
}

func (a *API) postJSON(path string, body interface{}) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest("POST", a.BaseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return a.do(req)
}

func (a *API) readBody(res *http.Response) ([]byte, error) {
	if res == nil {
		return nil, fmt.Errorf("nil response")
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func (a *API) ReadJSON(res *http.Response, v interface{}) error {
	body, err := a.readBody(res)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// WorkerFunc is the function signature for a worker processor.
// It receives the raw payload JSON and returns a result or error.
type WorkerFunc func(t *Task) Result

// Result is the outcome of a worker's processing.
type Result struct {
	Success bool           `json:"success"`
	Output  string         `json:"output,omitempty"`
	Error   string         `json:"error,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// Worker handles polling and processing for a specific task type.
type Worker struct {
	Name         string
	API          *API
	PollPath     string
	ClaimPath    string
	Process      WorkerFunc
	PollInterval time.Duration
	MaxRetries   int

	// Internal state for graceful shutdown
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewWorker creates a new worker with defaults.
func NewWorker(name string, api *API, pollPath, claimPath string, process WorkerFunc) *Worker {
	return &Worker{
		Name:         name,
		API:          api,
		PollPath:     pollPath,
		ClaimPath:    claimPath,
		Process:      process,
		PollInterval: getPollInterval(),
		stopCh:       make(chan struct{}),
	}
}

// getPollInterval reads POLL_INTERVAL from env, defaults to 10s.
func getPollInterval() time.Duration {
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		// Try as bare seconds
		var s int
		if _, err := fmt.Sscanf(v, "%d", &s); err == nil && s > 0 {
			return time.Duration(s) * time.Second
		}
	}
	return 10 * time.Second
}

// Run starts the worker loop with graceful shutdown and heartbeat.
// On SIGINT/SIGTERM: stops polling, waits for current task, then exits.
// Sends heartbeat every 5 minutes while running.
func (w *Worker) Run() error {
	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Backoff state
	backoff := w.PollInterval
	maxBackoff := 5 * time.Minute

	// Heartbeat ticker (every 5 min)
	heartbeatTick := time.NewTicker(5 * time.Minute)
	defer heartbeatTick.Stop()

	log.Printf("[%s] Starting worker (poll=%s, path=%s)", w.Name, w.PollInterval, w.PollPath)

	// Send initial heartbeat
	w.sendHeartbeat("working")

	// Track in-flight task for graceful shutdown
	var inFlight sync.WaitGroup

	for {
		select {
		case sig := <-sigCh:
			log.Printf("[%s] Received %v, shutting down gracefully...", w.Name, sig)
			w.sendHeartbeat("stopping")
			inFlight.Wait()
			log.Printf("[%s] Shutdown complete", w.Name)
			return nil

		case <-heartbeatTick.C:
			w.sendHeartbeat("working")

		default:
		}

		task, err := w.poll()
		if err != nil {
			log.Printf("[%s] Poll error: %v", w.Name, err)
			// Exponential backoff on error
			time.Sleep(backoff)
			backoff = time.Duration(float64(backoff) * 2)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reset backoff on success
		backoff = w.PollInterval

		if task == nil {
			// No task available, wait and retry
			time.Sleep(w.PollInterval)
			continue
		}

		log.Printf("[%s] Claiming task %s (title=%s)", w.Name, task.ID, task.Title)
		claimed, err := w.claim(task.ID)
		if err != nil || !claimed {
			log.Printf("[%s] Failed to claim task %s: %v", w.Name, task.ID, err)
			time.Sleep(w.PollInterval)
			continue
		}

		// Mark task as in-flight for graceful shutdown
		inFlight.Add(1)

		log.Printf("[%s] Processing task %s...", w.Name, task.ID)

		// Send progress update: 10%
		w.reportProgress(task.ID, 10)

		result := w.Process(task)

		if result.Success {
			log.Printf("[%s] Task %s complete, reporting success", w.Name, task.ID)
			if err := w.reportComplete(task.ID, result); err != nil {
				log.Printf("[%s] Failed to report complete for task %s: %v", w.Name, task.ID, err)
			}
		} else {
			log.Printf("[%s] Task %s failed: %s", w.Name, task.ID, result.Error)
			if err := w.reportFail(task.ID, result.Error); err != nil {
				log.Printf("[%s] Failed to report fail for task %s: %v", w.Name, task.ID, err)
			}
		}

		// Task complete
		inFlight.Done()
	}
}

// poll fetches tasks from the queue.
func (w *Worker) poll() (*Task, error) {
	res, err := w.API.get(w.PollPath)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotFound {
		return nil, nil // no task
	}
	if res.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized — check AGENTHUB_TOKEN")
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, fmt.Errorf("poll returned %d: %s", res.StatusCode, string(body))
	}

	// The queue endpoint returns {"tasks":[...],"capacity":{...}}
	var queueResp struct {
		Tasks []Task `json:"tasks"`
	}
	if err := w.API.ReadJSON(res, &queueResp); err != nil {
		return nil, err
	}
	if len(queueResp.Tasks) == 0 {
		return nil, nil
	}
	return &queueResp.Tasks[0], nil
}

// claim tries to claim a task by ID.
func (w *Worker) claim(id string) (bool, error) {
	res, err := w.API.postJSON(fmt.Sprintf(w.ClaimPath, id), map[string]any{"note": "claimed by worker"})
	if err != nil {
		return false, err
	}
	if res.StatusCode == http.StatusConflict {
		return false, nil // already claimed by another agent
	}
	if res.StatusCode == http.StatusNotFound {
		return false, fmt.Errorf("task not found")
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return false, fmt.Errorf("claim returned %d: %s", res.StatusCode, string(body))
	}
	return true, nil
}

// reportComplete marks a task as done.
func (w *Worker) reportComplete(id string, result Result) error {
	body := map[string]any{
		"status": "done",
		"notes":  result.Output,
	}
	if result.Data != nil {
		body["data"] = result.Data
	}
	res, err := w.API.postJSON(fmt.Sprintf("/api/agent/tasks/%s/complete", id), body)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return fmt.Errorf("complete returned %d", res.StatusCode)
	}
	return nil
}

// reportFail marks a task as failed.
func (w *Worker) reportFail(id string, errMsg string) error {
	body := map[string]any{
		"status": "failed",
		"notes":  errMsg,
	}
	res, err := w.API.postJSON(fmt.Sprintf("/api/agent/tasks/%s/complete", id), body)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return fmt.Errorf("fail returned %d", res.StatusCode)
	}
	return nil
}

// reportProgress updates task progress percentage.
func (w *Worker) reportProgress(id string, pct int) {
	body := map[string]any{"progress": pct}
	res, err := w.API.patchJSON(fmt.Sprintf("/api/agent/tasks/%s/progress", id), body)
	if err != nil {
		log.Printf("[%s] Failed to report progress for %s: %v", w.Name, id, err)
		return
	}
	res.Body.Close()
}

// reportReview submits a review verdict for a task.
func (w *Worker) reportReview(id string, verdict string, severity string, issues []string) error {
	body := map[string]any{
		"verdict":  verdict,
		"severity": severity,
		"issues":   issues,
	}
	res, err := w.API.postJSON(fmt.Sprintf("/api/agent/tasks/%s/review", id), body)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return fmt.Errorf("review returned %d", res.StatusCode)
	}
	return nil
}

// reportTest submits a test verdict for a task.
func (w *Worker) reportTest(id string, verdict string, issues []string) error {
	body := map[string]any{
		"verdict": verdict,
		"issues":  issues,
	}
	res, err := w.API.postJSON(fmt.Sprintf("/api/agent/tasks/%s/test", id), body)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return fmt.Errorf("test returned %d", res.StatusCode)
	}
	return nil
}

// sendHeartbeat sends a heartbeat to the agent hub.
func (w *Worker) sendHeartbeat(status string) {
	body := map[string]any{"status": status}
	res, err := w.API.postJSON("/api/agent/heartbeat", body)
	if err != nil {
		log.Printf("[%s] Heartbeat failed: %v", w.Name, err)
		return
	}
	res.Body.Close()
}

// Task represents a task from the AgentHub queue.
type Task struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Priority    string          `json:"priority"`
	Assignee    string          `json:"assignee"`
	MatchScore  float64         `json:"match_score"`
	Skills      []string        `json:"required_skills"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	CreatedAt   string          `json:"created_at"`

	// Workflow context
	WorkflowID         string `json:"workflow_id"`
	WorkflowPhase      string `json:"workflow_phase"`
	WorkflowPhaseIndex int    `json:"workflow_phase_index"`
}

// RunOpenCode runs an OpenCode command and returns the output.
func RunOpenCode(command string) (string, string, error) {
	apiKey := os.Getenv("DAODUC_API_KEY")
	if apiKey == "" {
		return "", "", fmt.Errorf("DAODUC_API_KEY not set")
	}

	cmd := exec.Command("opencode", "run", command)
	cmd.Env = append(os.Environ(), "DAODUC_API_KEY="+apiKey)
	cmd.Dir = "/root/.openclaw/workspace-pm"

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// RunShell runs a shell command and returns output.
func RunShell(command string, dir string, timeout time.Duration) (string, string, error) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// getEnv returns an env var or fallback.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func init() {
	http.DefaultClient.Timeout = 30 * time.Second
}
