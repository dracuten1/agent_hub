package dashboard

import (
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	db *sqlx.DB
}

func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Get(c *gin.Context) {
	// Task stats
	var stats struct {
		Total      int `db:"total"`
		Available  int `db:"available"`
		InProgress int `db:"in_progress"`
		Review     int `db:"review"`
		Test       int `db:"test"`
		Done       int `db:"done"`
		Deployed   int `db:"deployed"`
		Escalated  int `db:"escalated"`
		Failed     int `db:"failed"`
		NeedsFix   int `db:"needs_fix"`
		Orphaned   int `db:"orphaned"`
	}

	h.db.Get(&stats.Total, "SELECT COUNT(*) as total FROM tasks")
	h.db.Get(&stats.Available, "SELECT COUNT(*) as available FROM tasks WHERE status = 'available'")
	h.db.Get(&stats.InProgress, "SELECT COUNT(*) as in_progress FROM tasks WHERE status IN ('claimed', 'in_progress', 'fix_in_progress')")
	h.db.Get(&stats.Review, "SELECT COUNT(*) as review FROM tasks WHERE status = 'review'")
	h.db.Get(&stats.Test, "SELECT COUNT(*) as test FROM tasks WHERE status = 'test'")
	h.db.Get(&stats.Done, "SELECT COUNT(*) as done FROM tasks WHERE status = 'done'")
	h.db.Get(&stats.Deployed, "SELECT COUNT(*) as deployed FROM tasks WHERE status = 'deployed'")
	h.db.Get(&stats.Escalated, "SELECT COUNT(*) as escalated FROM tasks WHERE status = 'escalated'")
	h.db.Get(&stats.Failed, "SELECT COUNT(*) as failed FROM tasks WHERE status = 'failed'")
	h.db.Get(&stats.NeedsFix, "SELECT COUNT(*) as needs_fix FROM tasks WHERE status IN ('needs_fix')")
	h.db.Get(&stats.Orphaned, "SELECT COUNT(*) as orphaned FROM tasks WHERE status = 'orphaned'")

	// Agent stats
	var agentStats []struct {
		Name           string `db:"name" json:"name"`
		Role           string `db:"role" json:"role"`
		Status         string `db:"status" json:"status"`
		CurrentTasks   int    `db:"current_tasks" json:"current_tasks"`
		MaxTasks       int    `db:"max_tasks" json:"max_tasks"`
		TotalCompleted int    `db:"total_completed" json:"total_completed"`
		TotalFailed    int    `db:"total_failed" json:"total_failed"`
		LastHeartbeat  *string `db:"last_heartbeat" json:"last_heartbeat"`
	}
	h.db.Select(&agentStats,
		`SELECT name, role, status, current_tasks, max_tasks, total_completed, total_failed, last_heartbeat
		 FROM agents ORDER BY name`)

	// Recent events
	var recentEvents []struct {
		TaskID    string `db:"task_id" json:"task_id"`
		Agent     string `db:"agent" json:"agent"`
		Event     string `db:"event" json:"event"`
		ToStatus  string `db:"to_status" json:"to_status"`
		Note      string `db:"note" json:"note"`
		CreatedAt string `db:"created_at" json:"created_at"`
	}
	h.db.Select(&recentEvents,
		`SELECT task_id, agent, event, to_status, note, created_at
		 FROM task_events ORDER BY created_at DESC LIMIT 20`)

	// Escalated tasks
	var escalated []struct {
		ID        string `db:"id" json:"id"`
		Title     string `db:"title" json:"title"`
		Assignee  string `db:"assignee" json:"assignee"`
		RetryCount int   `db:"retry_count" json:"retry_count"`
	}
	h.db.Select(&escalated,
		`SELECT id, title, COALESCE(assignee, '') as assignee, retry_count
		 FROM tasks WHERE status = 'escalated' ORDER BY updated_at DESC`)

	c.JSON(200, gin.H{
		"tasks":    stats,
		"agents":   agentStats,
		"events":   recentEvents,
		"escalated": escalated,
	})
}

func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/dashboard", h.Get)
}
