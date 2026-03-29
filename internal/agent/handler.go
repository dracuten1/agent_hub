package agent

import (
	"database/sql"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/tuyen/agenthub/internal/auth"
	"github.com/tuyen/agenthub/internal/db"
)

type Handler struct {
	db *sqlx.DB
}

type Agent struct {
	ID             string          `json:"id" db:"id"`
	Name           string          `json:"name" db:"name"`
	Role           string          `json:"role" db:"role"`
	Skills         db.StringArray  `json:"skills" db:"skills"`
	Status         string          `json:"status" db:"status"`
	LastHeartbeat  *string         `json:"last_heartbeat" db:"last_heartbeat"`
	CurrentTasks   int             `json:"current_tasks" db:"current_tasks"`
	MaxTasks       int             `json:"max_tasks" db:"max_tasks"`
	TotalCompleted int             `json:"total_completed" db:"total_completed"`
	TotalFailed    int             `json:"total_failed" db:"total_failed"`
	Model          *string         `json:"model" db:"model"`
	Tool           *string         `json:"tool" db:"tool"`
	APIKey         string          `json:"-" db:"api_key"`
}

type RegisterRequest struct {
	Name   string   `json:"name" binding:"required"`
	Role   string   `json:"role" binding:"required,oneof=developer reviewer tester"`
	Skills []string `json:"skills"`
	MaxTasks int    `json:"max_tasks"`
	Model  string   `json:"model"`
	Tool   string   `json:"tool"`
}

type HeartbeatRequest struct {
	Status      string `json:"status" binding:"required,oneof=idle working error"`
	ActiveTasks []string `json:"active_tasks"`
	Model       string `json:"model"`
	Tool        string `json:"tool"`
}

func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterAgent(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// Check if already exists
	var count int
	h.db.Get(&count, "SELECT COUNT(*) FROM agents WHERE name = $1", req.Name)
	if count > 0 {
		c.JSON(409, gin.H{"error": "Agent already registered"})
		return
	}

	apiKey := auth.GenerateAPIKey()
	maxTasks := 3
	if req.MaxTasks > 0 {
		maxTasks = req.MaxTasks
	}

	var agent Agent
	err := h.db.QueryRowx(
		`INSERT INTO agents (name, role, skills, api_key, max_tasks, model, tool)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, name, role, skills, status, current_tasks, max_tasks, total_completed, total_failed, model, tool`,
		req.Name, req.Role, db.StringArray(req.Skills), apiKey, maxTasks,
		nilIfEmpty(req.Model), nilIfEmpty(req.Tool),
	).StructScan(&agent)

	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to register agent"})
		return
	}

	c.JSON(201, gin.H{
		"agent":   agent,
		"api_key": apiKey,
	})
}

func (h *Handler) Heartbeat(c *gin.Context) {
	agentName, _ := c.Get("agentName")

	var req HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	result, err := h.db.Exec(
		`UPDATE agents SET status = $1, last_heartbeat = NOW(), current_tasks = $2,
		 model = COALESCE(NULLIF($3, ''), model),
		 tool = COALESCE(NULLIF($4, ''), tool)
		 WHERE name = $5`,
		req.Status, len(req.ActiveTasks), req.Model, req.Tool, agentName,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to update heartbeat"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(404, gin.H{"error": "Agent not found"})
		return
	}

	c.JSON(200, gin.H{"status": "ok", "agent": agentName})
}

func (h *Handler) GetQueue(c *gin.Context) {
	agentName, _ := c.Get("agentName")

	// Get agent info
	var agentSkills db.StringArray
	h.db.Get(&agentSkills, "SELECT skills FROM agents WHERE name = $1", agentName)
	var currentTasks int
	h.db.Get(&currentTasks, "SELECT current_tasks FROM agents WHERE name = $1", agentName)
	var maxTasks int
	h.db.Get(&maxTasks, "SELECT max_tasks FROM agents WHERE name = $1", agentName)

	if currentTasks >= maxTasks {
		c.JSON(200, gin.H{
			"tasks":    []interface{}{},
			"message":  "At max capacity",
			"capacity": gin.H{"current": currentTasks, "max": maxTasks},
		})
		return
	}

	// Get available tasks with skill matching
	rows, err := h.db.Queryx(
		`SELECT id, title, description, priority, required_skills, created_at
		 FROM tasks
		 WHERE status = 'available'
		 ORDER BY
		   CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 END,
		   created_at ASC
		 LIMIT 10`)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get task queue"})
		return
	}
	defer rows.Close()

	type QueueTask struct {
		ID             string         `json:"id" db:"id"`
		Title          string         `json:"title" db:"title"`
		Description    string         `json:"description" db:"description"`
		Priority       string         `json:"priority" db:"priority"`
		RequiredSkills db.StringArray `json:"required_skills" db:"required_skills"`
		MatchScore     float64        `json:"match_score"`
	}

	var tasks []QueueTask
	for rows.Next() {
		var t QueueTask
		rows.StructScan(&t)
		t.MatchScore = calculateMatch(agentSkills, t.RequiredSkills)
		tasks = append(tasks, t)
	}

	c.JSON(200, gin.H{
		"tasks":    tasks,
		"capacity": gin.H{"current": currentTasks, "max": maxTasks},
	})
}

func (h *Handler) ListAgents(c *gin.Context) {
	var agents []Agent
	err := h.db.Select(&agents,
		`SELECT id, name, role, skills, status, last_heartbeat, current_tasks, max_tasks,
		        total_completed, total_failed, model, tool
		 FROM agents ORDER BY name`)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to list agents"})
		return
	}

	c.JSON(200, gin.H{"agents": agents})
}

func (h *Handler) GetAgentDetail(c *gin.Context) {
	name := c.Param("name")

	var agent Agent
	err := h.db.Get(&agent,
		`SELECT id, name, role, skills, status, last_heartbeat, current_tasks, max_tasks,
		        total_completed, total_failed, model, tool
		 FROM agents WHERE name = $1`, name)
	if err == sql.ErrNoRows {
		c.JSON(404, gin.H{"error": "Agent not found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get agent"})
		return
	}

	// Get agent's current tasks
	var tasks []struct {
		ID       string `json:"id" db:"id"`
		Title    string `json:"title" db:"title"`
		Status   string `json:"status" db:"status"`
		Progress int    `json:"progress" db:"progress"`
	}
	h.db.Select(&tasks,
		"SELECT id, title, status, progress FROM tasks WHERE assignee = $1 AND status NOT IN ('done', 'deployed', 'cancelled')",
		name)

	c.JSON(200, gin.H{"agent": agent, "current_tasks": tasks})
}

func (h *Handler) HealthOverview(c *gin.Context) {
	var healthy, warning, dead int
	h.db.Get(&healthy, "SELECT COUNT(*) FROM agents WHERE status = 'idle' OR status = 'working'")
	h.db.Get(&warning, "SELECT COUNT(*) FROM agents WHERE status = 'warning'")
	h.db.Get(&dead, "SELECT COUNT(*) FROM agents WHERE status = 'dead'")

	c.JSON(200, gin.H{
		"healthy": healthy,
		"warning": warning,
		"dead":    dead,
	})
}

func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/heartbeat", h.Heartbeat)
}

func (h *Handler) RegisterAgentRoutes(g *gin.RouterGroup) {
	g.GET("/tasks/queue", h.GetQueue)
}

func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	g.GET("/agents", h.ListAgents)
	g.GET("/agents/health", h.HealthOverview)
	g.GET("/agents/:name", h.GetAgentDetail)
}

// Health Monitor - runs as goroutine
func StartHealthMonitor(db *sqlx.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("[HealthMonitor] Checking agent health...")

		// Check all agents
		rows, err := db.Queryx(
			`SELECT id, name, status, last_heartbeat, current_tasks
			 FROM agents WHERE status != 'dead'`)
		if err != nil {
			log.Printf("[HealthMonitor] Error querying agents: %v", err)
			continue
		}

		type AgentStatus struct {
			ID            string  `db:"id"`
			Name          string  `db:"name"`
			Status        string  `db:"status"`
			LastHeartbeat *string `db:"last_heartbeat"`
			CurrentTasks  int     `db:"current_tasks"`
		}

		for rows.Next() {
			var a AgentStatus
			rows.StructScan(&a)

			if a.LastHeartbeat == nil {
				continue
			}

			lastSeen, err := time.Parse(time.RFC3339, *a.LastHeartbeat)
			if err != nil {
				continue
			}
			since := time.Since(lastSeen)

			switch {
			case since > 30*time.Minute:
				// DEAD - reassign tasks
				log.Printf("[HealthMonitor] 💀 Agent %s DEAD (no heartbeat for %v)", a.Name, since)
				db.Exec("UPDATE agents SET status = 'dead' WHERE id = $1", a.ID)

				// Reassign tasks
				var taskIDs []string
				db.Select(&taskIDs, "SELECT id FROM tasks WHERE assignee = $1 AND status NOT IN ('done', 'deployed', 'cancelled', 'available')", a.Name)

				for _, taskID := range taskIDs {
					// Find backup agent
					var backupName *string
					db.QueryRow(
						`SELECT name FROM agents
						 WHERE role = (SELECT role FROM agents WHERE name = $1)
						 AND name != $1 AND status IN ('idle', 'working')
						 AND current_tasks < max_tasks
						 ORDER BY current_tasks ASC LIMIT 1`,
						a.Name).Scan(&backupName)

					if backupName != nil {
						db.Exec("UPDATE tasks SET assignee = $1, status = 'available', retry_count = retry_count + 1 WHERE id = $2",
							*backupName, taskID)
						log.Printf("[HealthMonitor] Reassigned task %s from %s to %s", taskID, a.Name, *backupName)

						// Log event
						db.Exec(`INSERT INTO task_events (task_id, agent, event, from_status, to_status, note)
							VALUES ($1, $2, 'reassigned', $3, 'available', 'Agent dead, reassigned')`,
							taskID, a.Name, a.Status)
					} else {
						db.Exec("UPDATE tasks SET status = 'orphaned' WHERE id = $1", taskID)
						log.Printf("[HealthMonitor] Task %s orphaned (no backup agent)", taskID)
					}
				}

			case since > 15*time.Minute:
				// WARNING
				log.Printf("[HealthMonitor] 🚨 Agent %s WARNING (no heartbeat for %v)", a.Name, since)
				db.Exec("UPDATE agents SET status = 'warning' WHERE id = $1", a.ID)

			case since > 10*time.Minute:
				log.Printf("[HealthMonitor] ⚠️ Agent %s suspicious (no heartbeat for %v)", a.Name, since)
			}
		}
		rows.Close()
	}
}

func calculateMatch(agentSkills, taskSkills db.StringArray) float64 {
	if len(taskSkills) == 0 {
		return 1.0 // No skill requirement = anyone can do it
	}
	matches := 0
	for _, s := range taskSkills {
		for _, a := range agentSkills {
			if s == a {
				matches++
				break
			}
		}
	}
	return float64(matches) / float64(len(taskSkills))
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
