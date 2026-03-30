package dashboard

import (
	"log"

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
	var totalTasks, available, inProgress, completed int
	err := h.db.Get(&totalTasks, "SELECT COUNT(*) FROM tasks")
	if err != nil {
		log.Printf("[Dashboard] Error getting totalTasks: %v", err)
	}
	h.db.Get(&available, "SELECT COUNT(*) FROM tasks WHERE status = 'available'")
	h.db.Get(&inProgress, "SELECT COUNT(*) FROM tasks WHERE status IN ('claimed', 'in_progress', 'fix_in_progress')")
	h.db.Get(&completed, "SELECT COUNT(*) FROM tasks WHERE status IN ('done', 'deployed')")

	var totalAgents, activeAgents, idleAgents int
	h.db.Get(&totalAgents, "SELECT COUNT(*) FROM agents")
	h.db.Get(&activeAgents, "SELECT COUNT(*) FROM agents WHERE status = 'working'")
	h.db.Get(&idleAgents, "SELECT COUNT(*) FROM agents WHERE status = 'idle'")

	var recentEvents []struct {
		TaskID    string `db:"task_id" json:"task_id"`
		Agent     string `db:"agent" json:"agent"`
		Event     string `db:"event" json:"event"`
		ToStatus  string `db:"to_status" json:"to_status"`
		Note      string `db:"note" json:"note"`
		CreatedAt string `db:"created_at" json:"created_at"`
	}
	err = h.db.Select(&recentEvents,
		`SELECT task_id, agent, event, to_status, note, created_at
		 FROM task_events ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		log.Printf("[Dashboard] Error getting recentEvents: %v", err)
	}

	if totalTasks == 0 && err != nil {
		c.JSON(500, gin.H{"error": "Failed to get dashboard data"})
		return
	}

	if recentEvents == nil {
		recentEvents = []struct {
			TaskID    string `db:"task_id" json:"task_id"`
			Agent     string `db:"agent" json:"agent"`
			Event     string `db:"event" json:"event"`
			ToStatus  string `db:"to_status" json:"to_status"`
			Note      string `db:"note" json:"note"`
			CreatedAt string `db:"created_at" json:"created_at"`
		}{}
	}

	c.JSON(200, gin.H{
		"tasks": gin.H{
			"total":       totalTasks,
			"available":   available,
			"in_progress": inProgress,
			"completed":   completed,
		},
		"agents": gin.H{
			"total":  totalAgents,
			"active": activeAgents,
			"idle":   idleAgents,
		},
		"recent_events": recentEvents,
	})
}

func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/dashboard", h.Get)
}
