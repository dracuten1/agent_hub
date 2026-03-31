package dashboard

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// DashboardEvent represents a task event for the dashboard.
type DashboardEvent struct {
	TaskID    string `db:"task_id" json:"task_id"`
	Agent     string `db:"agent" json:"agent"`
	Event     string `db:"event" json:"event"`
	ToStatus  string `db:"to_status" json:"to_status"`
	Note      string `db:"note" json:"note"`
	CreatedAt string `db:"created_at" json:"created_at"`
}

type Handler struct {
	db *sqlx.DB
}

func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}

// dashboardStats holds all stats for the dashboard response.
type dashboardStats struct {
	Tasks  gin.H            `json:"tasks"`
	Agents gin.H            `json:"agents"`
	Events []DashboardEvent `json:"recent_events"`
}

func (h *Handler) Get(c *gin.Context) {
	stats, err := h.getStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get dashboard data: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) getStats() (*dashboardStats, error) {
	// Task counts — single query instead of 4 separate ones
	var totalTasks, available, inProgress, completed int
	err := h.db.QueryRow(
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'available'),
			COUNT(*) FILTER (WHERE status IN ('claimed', 'in_progress', 'fix_in_progress')),
			COUNT(*) FILTER (WHERE status IN ('done', 'deployed'))
		 FROM tasks`).Scan(&totalTasks, &available, &inProgress, &completed)
	if err != nil {
		return nil, err
	}

	// Agent counts — single query instead of 3 separate ones
	var totalAgents, activeAgents, idleAgents int
	err = h.db.QueryRow(
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'working'),
			COUNT(*) FILTER (WHERE status = 'idle')
		 FROM agents`).Scan(&totalAgents, &activeAgents, &idleAgents)
	if err != nil {
		// Non-fatal: agent counts wrong but tasks are fine
		log.Printf("[Dashboard] Error getting agent counts: %v", err)
	}

	// Recent events
	var recentEvents []DashboardEvent
	err = h.db.Select(&recentEvents,
		`SELECT task_id, agent, event, to_status, note, created_at
		 FROM task_events ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		log.Printf("[Dashboard] Error getting recent events: %v", err)
	}
	if recentEvents == nil {
		recentEvents = []DashboardEvent{}
	}

	return &dashboardStats{
		Tasks: gin.H{
			"total":       totalTasks,
			"available":   available,
			"in_progress": inProgress,
			"completed":   completed,
		},
		Agents: gin.H{
			"total":  totalAgents,
			"active": activeAgents,
			"idle":   idleAgents,
		},
		Events: recentEvents,
	}, nil
}

func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/dashboard/stats", h.Get)
}
