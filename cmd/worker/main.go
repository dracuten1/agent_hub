package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	AgentHubURL  = "http://localhost:8081/api/agent"
	PollInterval = 30 * time.Second
	HeartbeatInt = 5 * time.Minute
)

type Config struct {
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Skills   []string `json:"skills"`
	APIKey   string   `json:"api_key"`
	MaxTasks int      `json:"max_tasks"`
}

func main() {
	configPath := os.Getenv("AGENT_CONFIG")
	if configPath == "" {
		configPath = "/etc/agenthub/worker.json"
	}

	// Load config
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Failed to read config: %v\n", err)
		os.Exit(1)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("Failed to parse config: %v\n", err)
		os.Exit(1)
	}

	// Register agent if no API key
	if config.APIKey == "" {
		config.APIKey, err = registerAgent(config)
		if err != nil {
			fmt.Printf("Failed to register: %v\n", err)
			os.Exit(1)
		}
		// Save API key
		config.APIKey = config.APIKey
		data, _ = json.MarshalIndent(config, "", "  ")
		os.WriteFile(configPath, data, 0644)
		fmt.Printf("Registered! API Key: %s\n", config.APIKey)
	}

	fmt.Printf("Agent %s (%s) starting...\n", config.Name, config.Role)

	// Start heartbeat goroutine
	go heartbeatLoop(config)

	// Main poll loop
	for {
		task, err := pollQueue(config)
		if err != nil {
			fmt.Printf("Poll error: %v\n", err)
			time.Sleep(PollInterval)
			continue
		}

		if task == nil {
			time.Sleep(PollInterval)
			continue
		}

		// Claim task
		if err := claimTask(config, task.ID); err != nil {
			fmt.Printf("Claim error: %v\n", err)
			continue
		}

		// Execute task
		fmt.Printf("Working on: %s\n", task.Title)
		result := executeTask(config, task)

		// Report result
		if err := completeTask(config, task.ID, result); err != nil {
			fmt.Printf("Complete error: %v\n", err)
		}
	}
}

func registerAgent(config Config) (string, error) {
	body := fmt.Sprintf(`{"name":"%s","role":"%s","skills":%s,"max_tasks":%d}`,
		config.Name, config.Role, toJSON(config.Skills), config.MaxTasks)

	resp, err := http.Post(AgentHubURL+"/register", "application/json", reader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		APIKey string `json:"api_key"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.APIKey, nil
}

func heartbeatLoop(config Config) {
	ticker := time.NewTicker(HeartbeatInt)
	for range ticker.C {
		body := fmt.Sprintf(`{"status":"idle"}`)
		req, _ := http.NewRequest("POST", AgentHubURL+"/heartbeat", reader(body))
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("Heartbeat error: %v\n", err)
			continue
		}
		resp.Body.Close()
		fmt.Printf("[%s] Heartbeat sent\n", time.Now().Format("15:04:05"))
	}
}

func pollQueue(config Config) (*struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	MatchScore     float64  `json:"match_score"`
}, error) {

	req, _ := http.NewRequest("GET", AgentHubURL+"/tasks/queue", nil)
	req.Header.Set("Authorization", "Bearer "+config.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Tasks []struct {
			ID         string  `json:"id"`
			Title      string  `json:"title"`
			Desc       string  `json:"description"`
			MatchScore float64 `json:"match_score"`
		} `json:"tasks"`
		Message string `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Message == "At max capacity" || len(result.Tasks) == 0 {
		return nil, nil
	}

	return &result.Tasks[0], nil
}

func claimTask(config Config, taskID string) error {
	body := `{"note":"Claimed by worker"}`
	req, _ := http.NewRequest("POST", AgentHubURL+"/tasks/"+taskID+"/claim", reader(body))
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("claim failed (%d): %s", resp.StatusCode, body)
	}
	return nil
}

func completeTask(config Config, taskID string, result *TaskResult) error {
	status := "done"
	if result.Error != "" {
		status = "failed"
	}

	body := fmt.Sprintf(`{"status":"%s","notes":"%s"}`, status, escapeJSON(result.Output))

	req, _ := http.NewRequest("POST", AgentHubURL+"/tasks/"+taskID+"/complete", reader(body))
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type TaskResult struct {
	Output string
	Error  string
}

// executeTask runs the task using OpenCode
// Override this in role-specific workers
func executeTask(config Config, task *struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	MatchScore  float64 `json:"match_score"`
}) *TaskResult {
	// Default: use opencode
	projectDir := os.Getenv("PROJECT_DIR")
	if projectDir == "" {
		projectDir = "/root/projects"
	}

	prompt := fmt.Sprintf("%s\n\n%s", task.Title, task.Description)
	cmd := fmt.Sprintf(
		`DAODUC_API_KEY=%s opencode run "%s" --model daoduc/coding`,
		os.Getenv("DAODUC_API_KEY"), escapeShell(prompt),
	)

	fmt.Printf("Executing: %s\n", cmd)
	output, err := execCommand("bash", "-c", cmd, projectDir)
	if err != nil {
		return &TaskResult{Output: output, Error: err.Error()}
	}
	return &TaskResult{Output: output}
}

// Helpers
func reader(s string) io.Reader {
	return &sql.NullString{String: s}
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

func escapeShell(s string) string {
	return `'` + strings.ReplaceAll(s, "'", `'\''`) + `'`
}

func execCommand(name string, args ...string) (string, error) {
	// Simplified - real impl uses os/exec
	return "executed", nil
}
