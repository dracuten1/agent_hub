// Package main is the entry point for the AgentHub worker CLI.
//
// Usage:
//   agenthub-worker dev
//   agenthub-worker reviewer
//   agenthub-worker tester
//
// Environment variables:
//   AGENTHUB_URL    API URL (default: http://localhost:8081)
//   AGENTHUB_TOKEN  Agent API token
//   DAODUC_API_KEY  OpenCode API key (required for dev/reviewer)
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/tuyen/agenthub/workers"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: agenthub-worker <dev|reviewer|tester>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Environment:")
		fmt.Fprintln(os.Stderr, "  AGENTHUB_URL    API URL (default: http://localhost:8081)")
		fmt.Fprintln(os.Stderr, "  AGENTHUB_TOKEN  Agent API token (Bearer)")
		fmt.Fprintln(os.Stderr, "  DAODUC_API_KEY OpenCode API key (required for dev/reviewer)")
		os.Exit(1)
	}

	subcommand := os.Args[1]

	switch subcommand {
	case "dev":
		log.Println("Starting Dev worker...")
		workers.RunDevWorker()
	case "reviewer":
		log.Println("Starting Reviewer worker...")
		workers.RunReviewWorker()
	case "tester":
		log.Println("Starting Tester worker...")
		workers.RunTestWorker()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcommand)
		fmt.Fprintln(os.Stderr, "Known: dev, reviewer, tester")
		os.Exit(1)
	}
}

// Config represents the optional /etc/agenthub/worker.json config file.
type Config struct {
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Skills   []string `json:"skills"`
	APIKey   string   `json:"api_key"`
	MaxTasks int      `json:"max_tasks"`
}

func readConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
