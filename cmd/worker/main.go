package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tuyen/agenthub/internal/worker"
)

func main() {
	role := flag.String("role", "dev", "Worker role: dev, review, test")
	apiURL := flag.String("api", "http://localhost:8081", "AgentHub API URL")
	opencodePort := flag.Int("opencode-port", 4096, "OpenCode server port")
	model := flag.String("model", "daoduc/agentic-turbo", "OpenCode model ID")
	maxIter := flag.Int("max-iterations", 3, "Max fix iterations")
	pollInterval := flag.Int("poll-interval", 10, "Seconds between queue polls")
	token := flag.String("token", os.Getenv("AGENT_TOKEN"), "Agent auth token (env: AGENT_TOKEN)")
	flag.Parse()

	if *token == "" {
		log.Fatal("AGENT_TOKEN required (env or --token)")
	}

	cfg := worker.Config{
		Role:          *role,
		TaskType:      *role,
		APIBaseURL:    *apiURL,
		OpenCodePort:  *opencodePort,
		Model:         *model,
		MaxIterations: *maxIter,
		PollInterval:  time.Duration(*pollInterval) * time.Second,
		AgentToken:    *token,
	}
	w := worker.NewWorkerWithConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down worker...")
		cancel()
	}()

	log.Printf("Worker starting: role=%s api=%s opencode=%d", *role, *apiURL, *opencodePort)
	w.Run(ctx)
}
