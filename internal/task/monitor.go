package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/tuyen/agenthub/internal/websocket"
)

// StartStaleTaskMonitor runs a background goroutine that periodically checks for
// stale (orphaned) tasks and auto-releases or escalates them.
func StartStaleTaskMonitor(ctx context.Context, db *sqlx.DB, hub *websocket.Hub) {
	interval := time.Duration(getEnvInt("STALE_CHECK_INTERVAL_MINUTES", 5)) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Printf("[Monitor] Stale task monitor started (interval: %v)", interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("[Monitor] Stale task monitor stopped")
			return
		case <-ticker.C:
			checkStaleTasks(db, hub)
		}
	}
}

func checkStaleTasks(db *sqlx.DB, hub *websocket.Hub) {
	if db == nil {
		return
	}
	log.Println("[StaleMonitor] Checking for stale tasks...")

	claimTimeout := getEnvInt("CLAIM_TIMEOUT_MINUTES", 30)
	progressTimeout := getEnvInt("PROGRESS_TIMEOUT_MINUTES", 15)

	type releasedTask struct {
		ID       string  `db:"id"`
		Assignee *string `db:"assignee"`
	}

	// Auto-release pass: tasks under max_retries → available
	rows, err := db.Queryx(
		`UPDATE tasks SET
			status = 'available',
			assignee = NULL,
			retry_count = retry_count + 1,
			release_count = release_count + 1,
			claimed_at = NULL,
			updated_at = NOW()
		 WHERE id IN (
			SELECT id FROM tasks
			WHERE status IN ('claimed', 'in_progress')
			AND claimed_at IS NOT NULL
			AND claimed_at < NOW() - ($1 || ' minutes')::interval
			AND updated_at < NOW() - ($2 || ' minutes')::interval
			AND retry_count < max_retries
		 )
		 RETURNING id, assignee`,
		claimTimeout, progressTimeout)
	if err == nil {
		for rows.Next() {
			var t releasedTask
			rows.StructScan(&t)
			if t.Assignee != nil {
				db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *t.Assignee)
			}
			db.Exec(
				`INSERT INTO task_events (task_id, agent, event, from_status, to_status, note)
				 VALUES ($1, $2, 'auto_released', $3, 'available', $4)`,
				t.ID, "system", "claimed", fmt.Sprintf("Task auto-released: no progress for %d minutes", progressTimeout))
			if hub != nil {
				event := map[string]interface{}{
					"task_id":     t.ID,
					"from_status": "claimed",
					"to_status":   "available",
					"agent":       nil,
					"event":       "task_released",
					"timestamp":   time.Now().UTC(),
				}
				data, _ := json.Marshal(event)
				hub.Broadcast(data)
			}
			log.Printf("[StaleMonitor] Auto-released stale task: %s", t.ID)
		}
		rows.Close()
	}

	// Escalation pass: tasks at max_retries → escalated
	rows, err = db.Queryx(
		`UPDATE tasks SET
			status = 'escalated',
			escalated = true,
			updated_at = NOW()
		 WHERE id IN (
			SELECT id FROM tasks
			WHERE status IN ('claimed', 'in_progress')
			AND claimed_at IS NOT NULL
			AND claimed_at < NOW() - ($1 || ' minutes')::interval
			AND updated_at < NOW() - ($2 || ' minutes')::interval
			AND retry_count >= max_retries
		 )
		 RETURNING id, assignee`,
		claimTimeout, progressTimeout)
	if err == nil {
		for rows.Next() {
			var t releasedTask
			rows.StructScan(&t)
			if t.Assignee != nil {
				db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *t.Assignee)
			}
			db.Exec(
				`INSERT INTO task_events (task_id, agent, event, from_status, to_status, note)
				 VALUES ($1, $2, 'escalated', $3, 'escalated', $4)`,
				t.ID, "system", "claimed", "Max retries exceeded, task escalated")
			if hub != nil {
				event := map[string]interface{}{
					"task_id":     t.ID,
					"from_status": "claimed",
					"to_status":   "escalated",
					"agent":       nil,
					"event":       "task_updated",
					"timestamp":   time.Now().UTC(),
				}
				data, _ := json.Marshal(event)
				hub.Broadcast(data)
			}
			log.Printf("[StaleMonitor] Escalated stale task (max retries): %s", t.ID)
		}
		rows.Close()
	}

	if err != nil {
		log.Printf("[StaleMonitor] Error: %v", err)
	}
}
