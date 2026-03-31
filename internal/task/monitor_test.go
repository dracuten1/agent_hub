package task

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestStartStaleTaskMonitorCancellation(t *testing.T) {
	// With nil DB and nil hub, monitor should not panic on tick (db nil check)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately — monitor should stop right away

	done := make(chan struct{})
	go func() {
		StartStaleTaskMonitor(ctx, nil, nil)
		close(done)
	}()

	select {
	case <-done:
		// Expected: monitor stopped after cancel
	case <-time.After(2 * time.Second):
		t.Fatal("Monitor did not stop after context cancel")
	}
}

// Ensure StartStaleTaskMonitor accepts the correct signature
var _ = func(db *sqlx.DB) {
	ctx := context.Background()
	StartStaleTaskMonitor(ctx, db, nil)
}
