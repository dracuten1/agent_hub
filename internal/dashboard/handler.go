package dashboard

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	db *sqlx.DB
}

func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}

// --- Types ---

type AgentInfo struct {
	Name           string `json:"name" db:"name"`
	Role           string `json:"role" db:"role"`
	Status         string `json:"status" db:"status"`
	CurrentTasks   int    `json:"current_tasks" db:"current_tasks"`
	MaxTasks       int    `json:"max_tasks" db:"max_tasks"`
	TotalCompleted int    `json:"total_completed" db:"total_completed"`
	TotalFailed    int    `json:"total_failed" db:"total_failed"`
	LastHeartbeat  string `json:"last_heartbeat" db:"last_heartbeat"`
	Online         bool   `json:"online"`
}

type TaskCount struct {
	Status string `json:"status" db:"status"`
	Count  int    `json:"count" db:"count"`
}

type QueueDepth struct {
	TaskType string `json:"task_type" db:"task_type"`
	Count    int    `json:"count" db:"count"`
}

type RecentTask struct {
	ID          string  `json:"id" db:"id"`
	Title       string  `json:"title" db:"title"`
	Status      string  `json:"status" db:"status"`
	TaskType    string  `json:"task_type" db:"task_type"`
	Assignee    *string `json:"assignee" db:"assignee"`
	Priority    string  `json:"priority" db:"priority"`
	Progress    int     `json:"progress" db:"progress"`
	CreatedAt   string  `json:"created_at" db:"created_at"`
	ClaimedAt   *string `json:"claimed_at" db:"claimed_at"`
	CompletedAt *string `json:"completed_at" db:"completed_at"`
}

type SummaryResponse struct {
	Agents     []AgentInfo  `json:"agents"`
	TaskCounts []TaskCount  `json:"task_counts"`
	Queue      []QueueDepth `json:"queue"`
	Recent     []RecentTask `json:"recent_tasks"`
}

// --- Endpoint ---

func (h *Handler) Summary(c *gin.Context) {
	resp := SummaryResponse{}

	agents, err := h.getAgents()
	if err != nil {
		log.Printf("[Dashboard] agents error: %v", err)
	}
	resp.Agents = agents

	h.db.Select(&resp.TaskCounts, `SELECT status, COUNT(*) as count FROM tasks GROUP BY status ORDER BY count DESC`)
	h.db.Select(&resp.Queue, `SELECT task_type, COUNT(*) as count FROM tasks WHERE status = 'available' GROUP BY task_type`)

	recent, err := h.getRecentTasks(20)
	if err != nil {
		log.Printf("[Dashboard] recent tasks error: %v", err)
	}
	resp.Recent = recent

	c.JSON(http.StatusOK, resp)
}

// --- Helpers ---

func (h *Handler) getAgents() ([]AgentInfo, error) {
	var agents []AgentInfo
	rows, err := h.db.Queryx(`
		SELECT name, role, status, current_tasks, max_tasks,
		       total_completed, total_failed, last_heartbeat
		FROM agents
		ORDER BY role, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var a AgentInfo
		var lastHb *time.Time
		if err := rows.Scan(&a.Name, &a.Role, &a.Status, &a.CurrentTasks, &a.MaxTasks,
			&a.TotalCompleted, &a.TotalFailed, &lastHb); err != nil {
			log.Printf("[Dashboard] scan agent error: %v", err)
			continue
		}
		if lastHb != nil {
			a.Online = time.Since(*lastHb) < 2*time.Minute
			a.LastHeartbeat = lastHb.Format(time.RFC3339)
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func (h *Handler) getRecentTasks(limit int) ([]RecentTask, error) {
	var tasks []RecentTask
	err := h.db.Select(&tasks, `
		SELECT id, title, status, task_type, assignee, priority, progress,
		       created_at, claimed_at, completed_at
		FROM tasks
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	return tasks, err
}
