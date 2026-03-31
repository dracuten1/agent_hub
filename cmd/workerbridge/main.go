package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tuyen/agenthub/internal/workerbridge"
)

func main() {
	cfg := workerbridge.FromEnv()

	flag.StringVar(&cfg.APIURL, "api", cfg.APIURL, "AgentHub API URL (env: AGENTHUB_URL)")
	flag.StringVar(&cfg.AgentToken, "token", cfg.AgentToken, "Agent auth token (env: AGENT_TOKEN)")
	flag.StringVar(&cfg.TaskType, "role", cfg.TaskType, "Worker role: dev, review, test (env: WORKER_ROLE)")
	flag.StringVar(&cfg.AgentID, "agent-id", cfg.AgentID, "OpenClaw agent ID: dev1, dev2, reviewer, tester (env: AGENT_ID)")
	flag.StringVar(&cfg.SessionID, "session-id", cfg.SessionID, "Resume existing session (env: SESSION_ID)")
	flag.IntVar(&cfg.PollInterval, "poll", cfg.PollInterval, "Seconds between polls (env: POLL_INTERVAL_SECONDS)")
	flag.IntVar(&cfg.TaskTimeout, "timeout", cfg.TaskTimeout, "Task timeout in seconds (env: TASK_TIMEOUT_MINUTES)")
	flag.IntVar(&cfg.IdleThreshold, "idle-threshold", cfg.IdleThreshold, "Idle threshold in minutes (env: IDLE_THRESHOLD_MINUTES)")
	flag.Parse()

	if cfg.AgentToken == "" {
		log.Fatal("AGENT_TOKEN required (env or --token)")
	}
	if cfg.AgentID == "" {
		log.Fatal("AGENT_ID required (env or --agent-id)")
	}

	log.Printf("Worker bridge: api=%s role=%s agent=%s poll=%ds timeout=%ds",
		cfg.APIURL, cfg.TaskType, cfg.AgentID, cfg.PollInterval, cfg.TaskTimeout)

	bridge := workerbridge.NewBridge(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	if err := bridge.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Bridge error: %v", err)
	}
}
