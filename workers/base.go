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
type WorkerFunc func(payload json.RawMessage) Result

// Result is the outcome of a worker's processing.
type Result struct {
	Success bool            `json:"success"`
	Output  string          `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    map[string]any  `json:"data,omitempty"`
}

// TaskPayload represents a polled task payload.
type TaskPayload struct {
	Type    string          `json:"type"`
	Project string          `json:"project"`
	Path    string          `json:"path"`
	Command string          `json:"command,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

// ReviewPayload represents a polled review payload.
type ReviewPayload struct {
	Type    string          `json:"type"`
	Project string          `json:"project"`
	PRURL   string          `json:"pr_url,omitempty"`
	Payload json.RawMessage `json:"payload"`
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
		// Try as seconds
		if secs, err := fmt.Sscanf(v, "%d", new(int)); secs == 1 && err == nil {
			var s int
			fmt.Sscanf(v, "%d", &s)
			return time.Duration(s) * time.Second
		}
	}
	return 10 * time.Second
}

// Run starts the worker loop with graceful shutdown.
// On SIGINT/SIGTERM: stops polling, waits for current task, then exits.
func (w *Worker) Run() error {
	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Backoff state
	backoff := w.PollInterval
	maxBackoff := 5 * time.Minute

	log.Printf("[%s] Starting worker (poll=%s, path=%s)", w.Name, w.PollInterval, w.PollPath)

	// Track in-flight task for graceful shutdown
	var inFlight sync.WaitGroup

	for {
		select {
		case sig := <-sigCh:
			log.Printf("[%s] Received %v, shutting down gracefully...", w.Name, sig)
			// Wait for in-flight task to complete
			inFlight.Wait()
			log.Printf("[%s] Shutdown complete", w.Name)
			return nil

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

		log.Printf("[%s] Claiming task %d (type=%s, project=%s)", w.Name, task.ID, task.Type, task.Project)
		claimed, err := w.claim(task.ID)
		if err != nil || !claimed {
			log.Printf("[%s] Failed to claim task %d: %v", w.Name, task.ID, err)
			time.Sleep(w.PollInterval)
			continue
		}

		// Mark task as in-flight for graceful shutdown
		inFlight.Add(1)

		log.Printf("[%s] Processing task %d...", w.Name, task.ID)
		result := w.Process(task.Payload)

		if result.Success {
			log.Printf("[%s] Task %d complete, reporting success", w.Name, task.ID)
			if err := w.reportComplete(task.ID, result); err != nil {
				log.Printf("[%s] Failed to report complete for task %d: %v", w.Name, task.ID, err)
			}
		} else {
			log.Printf("[%s] Task %d failed: %s", w.Name, task.ID, result.Error)
			if err := w.reportFail(task.ID, result.Error); err != nil {
				log.Printf("[%s] Failed to report fail for task %d: %v", w.Name, task.ID, err)
			}
		}

		// Task complete
		inFlight.Done()
	}
}

func (w *Worker) poll() (*Task, error) {
	res, err := w.API.get(w.PollPath)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotFound {
		return nil, nil // no task
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, fmt.Errorf("poll returned %d: %s", res.StatusCode, string(body))
	}

	var task Task
	if err := w.API.ReadJSON(res, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (w *Worker) claim(id int64) (bool, error) {
	res, err := w.API.postJSON(fmt.Sprintf(w.ClaimPath, id), nil)
	if err != nil {
		return false, err
	}
	if res.StatusCode == http.StatusConflict {
		return false, nil // already claimed
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return false, fmt.Errorf("claim returned %d: %s", res.StatusCode, string(body))
	}
	return true, nil
}

func (w *Worker) reportComplete(id int64, result Result) error {
	body := map[string]any{
		"success": true,
		"output":  result.Output,
	}
	if result.Data != nil {
		body["data"] = result.Data
	}
	res, err := w.API.postJSON(fmt.Sprintf("/tasks/%d/complete", id), body)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("complete returned %d", res.StatusCode)
	}
	return nil
}

func (w *Worker) reportFail(id int64, errMsg string) error {
	body := map[string]any{
		"success": false,
		"error":   errMsg,
	}
	res, err := w.API.postJSON(fmt.Sprintf("/tasks/%d/fail", id), body)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("fail returned %d", res.StatusCode)
	}
	return nil
}

// Task represents a polled task from the queue.
type Task struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Project   string          `json:"project"`
	Path      string          `json:"path"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
}

// Review represents a polled review from the queue.
type Review struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Project   string          `json:"project"`
	PRURL     string          `json:"pr_url"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
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

func init() {
	// Ensure DefaultClient has a timeout
	http.DefaultClient.Timeout = 30 * time.Second
}
