package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/tuyen/agenthub/internal/db"
	"github.com/tuyen/agenthub/internal/websocket"
)

var validTransitions = map[string][]string{
	"available":       {"claimed"},
	"assigned":        {"claimed"},
	"orphaned":        {"claimed"},
	"claimed":         {"in_progress", "done", "review", "available"},
	"in_progress":     {"done", "review", "needs_fix", "failed"},
	"done":            {"review", "test", "needs_fix", "escalated"},
	"review":          {"done", "test", "needs_fix"},
	"needs_fix":       {"in_progress", "claimed", "failed"},
	"fix_in_progress": {"done", "needs_fix"},
	"test":            {"deployed", "needs_fix"},
	"failed":          {"escalated"},
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

type Handler struct {
	db               *sqlx.DB
	hub              *websocket.Hub
	WorkflowAdvancer interface {
		CheckAndAdvancePhase(taskID string) error
	}
}

type Task struct {
	ID             string         `json:"id" db:"id"`
	ProjectID      *string        `json:"project_id" db:"project_id"`
	FeatureID      *string        `json:"feature_id" db:"feature_id"`
	Title          string         `json:"title" db:"title"`
	Description    string         `json:"description" db:"description"`
	Priority       string         `json:"priority" db:"priority"`
	Status         string         `json:"status" db:"status"`
	TaskType       string         `json:"task_type" db:"task_type"`
	Assignee       *string        `json:"assignee" db:"assignee"`
	RequiredSkills db.StringArray `json:"required_skills" db:"required_skills"`
	RetryCount        int            `json:"retry_count" db:"retry_count"`
	MaxRetries        int            `json:"max_retries" db:"max_retries"`
	ReleaseCount      int            `json:"release_count" db:"release_count"`
	Progress          int            `json:"progress" db:"progress"`
	ReviewVerdict     *string        `json:"review_verdict" db:"review_verdict"`
	ReviewSeverity    *string        `json:"review_severity" db:"review_severity"`
	ReviewIssues      db.StringArray `json:"review_issues" db:"review_issues"`
	TestVerdict       *string        `json:"test_verdict" db:"test_verdict"`
	TestIssues        db.StringArray `json:"test_issues" db:"test_issues"`
	Escalated         bool           `json:"escalated" db:"escalated"`
	ClaimedAt         *string        `json:"claimed_at" db:"claimed_at"`
	CompletedAt       *string        `json:"completed_at" db:"completed_at"`
	Deadline          *string        `json:"deadline" db:"deadline"`
	CreatedBy         *string        `json:"created_by" db:"created_by"`
	UserID            *string        `json:"user_id" db:"user_id"`
	CreatedAt         string         `json:"created_at" db:"created_at"`
	UpdatedAt         string         `json:"updated_at" db:"updated_at"`
	StaleReason       string         `json:"stale_reason,omitempty" db:"-"`
	StaleDurationMinutes int         `json:"stale_duration_minutes,omitempty" db:"-"`
}

type CreateTaskRequest struct {
	ProjectID      string   `json:"project_id"`
	FeatureID      string   `json:"feature_id"`
	Title          string   `json:"title" binding:"required,max=500"`
	Description    string   `json:"description"`
	Priority       string   `json:"priority" binding:"omitempty,oneof=low medium high critical"`
	RequiredSkills []string `json:"required_skills"`
	MaxRetries     int      `json:"max_retries"`
	Deadline       string   `json:"deadline"`
	Assignee       string   `json:"assignee"`
	TaskType       string   `json:"task_type" binding:"omitempty,oneof=general dev review test"`
}

type UpdateTaskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Priority    *string `json:"priority"`
	Status      *string `json:"status"`
	Assignee    *string `json:"assignee"`
	Progress    *int    `json:"progress"`
	Deadline    *string `json:"deadline"`
}

type ClaimRequest struct {
	Note string `json:"note"`
}

type CompleteRequest struct {
	Status string   `json:"status" binding:"required,oneof=done failed blocked needs_fix"`
	Files  []string `json:"files_changed"`
	Branch string   `json:"branch"`
	Notes  string   `json:"notes"`
}

type ReviewRequest struct {
	Verdict  string   `json:"verdict" binding:"required,oneof=pass fail"`
	Severity string   `json:"severity" binding:"omitempty,oneof=minor major critical"`
	Issues   []string `json:"issues"`
	Notes    string   `json:"notes"`
}

type TestRequest struct {
	Verdict string   `json:"verdict" binding:"required,oneof=pass fail"`
	Issues  []string `json:"issues"`
	Notes   string   `json:"notes"`
}

type ProgressRequest struct {
	Progress int    `json:"progress" binding:"min=0,max=100"`
	Note     string `json:"note"`
}

type ReassignRequest struct {
	Agent  string `json:"agent" binding:"required"`
	Reason string `json:"reason"`
}

func NewHandler(db *sqlx.DB, hub *websocket.Hub) *Handler {
	return &Handler{db: db, hub: hub}
}

// --- USER ROUTES (PM/Owner) ---

func (h *Handler) CreateTask(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	priority := "medium"
	if req.Priority != "" {
		priority = req.Priority
	}

	maxRetries := 2
	if req.MaxRetries > 0 {
		maxRetries = req.MaxRetries
	}

	taskID := uuid.New().String()
	status := "available"
	var assignee *string
	if req.Assignee != "" {
		assignee = &req.Assignee
		status = "assigned"
	}

	taskType := "general"
	if req.TaskType != "" {
		taskType = req.TaskType
	}

	_, err := h.db.Exec(
		`INSERT INTO tasks (id, project_id, feature_id, title, description, priority, status,
		 assignee, required_skills, max_retries, deadline, created_by, task_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		taskID, nilIfEmpty(req.ProjectID), nilIfEmpty(req.FeatureID),
		req.Title, req.Description, priority, status,
		assignee, db.StringArray(req.RequiredSkills), maxRetries, nilIfEmpty(req.Deadline), userID, taskType,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create task"})
		return
	}

	// Log event
	h.logEvent(taskID, "pm", "created", "", status, "Task created by PM")

	var task Task
	if err := h.db.Get(&task, "SELECT * FROM tasks WHERE id = $1", taskID); err != nil {
		c.JSON(500, gin.H{"error": "Failed to get created task"})
		return
	}

	c.JSON(201, gin.H{"task": task})
}

func (h *Handler) ListTasks(c *gin.Context) {
	status := c.Query("status")
	projectID := c.Query("project_id")
	assignee := c.Query("assignee")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit < 1 {
		limit = 100
	}
	offset := (page - 1) * limit

	whereClause := "1=1"
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		whereClause += argPlaceholder(argIdx, "status")
		args = append(args, status)
		argIdx++
	}

	// Filter by task_type
	if taskType := c.Query("type"); taskType != "" {
		allowed := map[string]bool{"general": true, "dev": true, "review": true, "test": true}
		if !allowed[taskType] {
			c.JSON(400, gin.H{
				"error":    "Invalid type filter",
				"allowed":  []string{"general", "dev", "review", "test"},
			})
			return
		}
		whereClause += " AND task_type = " + placeholder(argIdx)
		args = append(args, taskType)
		argIdx++
	}

	if projectID != "" {
		whereClause += " AND project_id = " + placeholder(argIdx)
		args = append(args, projectID)
		argIdx++
	}
	if assignee != "" {
		whereClause += " AND assignee = " + placeholder(argIdx)
		args = append(args, assignee)
		argIdx++
	}

	stale := c.DefaultQuery("stale", "") == "true"
	if stale {
		claimTimeout := getEnvInt("CLAIM_TIMEOUT_MINUTES", 30)
		progressTimeout := getEnvInt("PROGRESS_TIMEOUT_MINUTES", 15)
		whereClause += " AND status IN ('claimed', 'in_progress') AND claimed_at IS NOT NULL"
		whereClause += " AND claimed_at < NOW() - ($" + placeholder(argIdx) + " || ' minutes')::interval"
		args = append(args, claimTimeout)
		argIdx++
		whereClause += " AND updated_at < NOW() - ($" + placeholder(argIdx) + " || ' minutes')::interval"
		args = append(args, progressTimeout)
		argIdx++
	}

	countQuery := "SELECT COUNT(*) FROM tasks WHERE " + whereClause
	var total int
	h.db.Get(&total, countQuery, args...)

	var orderBy string
	if stale {
		orderBy = " ORDER BY claimed_at ASC NULLS LAST"
	} else {
		orderBy = " ORDER BY CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 END, created_at DESC"
	}

	query := "SELECT * FROM tasks WHERE " + whereClause + orderBy
	query += " LIMIT " + placeholder(argIdx) + " OFFSET " + placeholder(argIdx+1)
	args = append(args, limit, offset)

	var tasks []Task
	err := h.db.Select(&tasks, query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to list tasks"})
		return
	}

	if tasks == nil {
		tasks = []Task{}
	}

	if stale {
		for i := range tasks {
			tasks[i].StaleReason = "no_progress"
			if tasks[i].ClaimedAt != nil {
				claimedAt, err := time.Parse(time.RFC3339, *tasks[i].ClaimedAt)
				if err == nil {
					tasks[i].StaleDurationMinutes = int(time.Since(claimedAt).Minutes())
				}
			}
		}
	}

	c.JSON(200, gin.H{"tasks": tasks, "total": total, "page": page, "limit": limit})
}

func (h *Handler) GetTask(c *gin.Context) {
	id := c.Param("id")

	var task Task
	err := h.db.Get(&task, "SELECT * FROM tasks WHERE id = $1", id)
	if err == sql.ErrNoRows {
		c.JSON(404, gin.H{"error": "Task not found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get task"})
		return
	}

	// Get events
	var events []struct {
		ID         int64   `json:"id" db:"id"`
		Agent      string  `json:"agent" db:"agent"`
		Event      string  `json:"event" db:"event"`
		FromStatus *string `json:"from_status" db:"from_status"`
		ToStatus   *string `json:"to_status" db:"to_status"`
		Note       *string `json:"note" db:"note"`
		CreatedAt  string  `json:"created_at" db:"created_at"`
	}
	h.db.Select(&events, "SELECT * FROM task_events WHERE task_id = $1 ORDER BY created_at DESC", id)

	c.JSON(200, gin.H{"task": task, "events": events})
}

func (h *Handler) UpdateTask(c *gin.Context) {
	id := c.Param("id")

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	var oldStatus string
	var err error
	h.db.Get(&oldStatus, "SELECT status FROM tasks WHERE id = $1", id)

	updates := "updated_at = NOW()"
	args := []interface{}{id}
	argIdx := 2

	if req.Title != nil {
		updates += ", title = " + placeholder(argIdx)
		args = append(args, *req.Title)
		argIdx++
	}
	if req.Description != nil {
		updates += ", description = " + placeholder(argIdx)
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Priority != nil {
		updates += ", priority = " + placeholder(argIdx)
		args = append(args, *req.Priority)
		argIdx++
	}
	if req.Status != nil {
		updates += ", status = " + placeholder(argIdx)
		args = append(args, *req.Status)
		argIdx++
	}
	if req.Assignee != nil {
		updates += ", assignee = " + placeholder(argIdx)
		args = append(args, *req.Assignee)
		argIdx++
	}
	if req.Progress != nil {
		updates += ", progress = " + placeholder(argIdx)
		args = append(args, *req.Progress)
		argIdx++
	}

	query := "UPDATE tasks SET " + updates + " WHERE id = $1 RETURNING *"
	var task Task
	err = h.db.QueryRowx(query, args...).StructScan(&task)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to update task"})
		return
	}

	if req.Status != nil {
		h.logEvent(id, "pm", "updated", oldStatus, *req.Status, "Updated by PM")
	}

	c.JSON(200, gin.H{"task": task})
}

func (h *Handler) DeleteTask(c *gin.Context) {
	id := c.Param("id")

	// Decrement assignee's current_tasks if task is in an active (non-terminal) state.
	var assignee *string
	var status string
	h.db.Get(&assignee, "SELECT assignee FROM tasks WHERE id = $1", id)
	h.db.Get(&status, "SELECT status FROM tasks WHERE id = $1", id)
	if assignee != nil && status != "" {
		terminal := map[string]bool{"deployed": true, "failed": true, "escalated": true}
		if !terminal[status] {
			h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *assignee)
		}
	}

	result, err := h.db.Exec("DELETE FROM tasks WHERE id = $1", id)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete task"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(404, gin.H{"error": "Task not found"})
		return
	}

	c.JSON(200, gin.H{"message": "Task deleted"})
}

func (h *Handler) ReassignTask(c *gin.Context) {
	id := c.Param("id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	var req ReassignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	var oldAssignee *string
	h.db.Get(&oldAssignee, "SELECT assignee FROM tasks WHERE id = $1", id)

	_, err := h.db.Exec(
		"UPDATE tasks SET assignee = $1, status = 'available', retry_count = retry_count + 1, updated_at = NOW() WHERE id = $2",
		req.Agent, id)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to reassign"})
		return
	}

	if oldAssignee != nil {
		h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *oldAssignee)
	}

	h.logEvent(id, agentNameStr, "reassigned", "", "available", req.Reason)

	c.JSON(200, gin.H{"message": "Task reassigned to " + req.Agent})
}

func (h *Handler) EscalateTask(c *gin.Context) {
	id := c.Param("id")

	_, err := h.db.Exec(
		"UPDATE tasks SET status = 'escalated', escalated = true, updated_at = NOW() WHERE id = $1", id)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to escalate"})
		return
	}

	h.logEvent(id, "system", "escalated", "", "escalated", "Task escalated to PM")
	h.broadcastTaskEvent(id, "task_updated", "", "escalated")

	c.JSON(200, gin.H{"message": "Task escalated to PM"})
}

func (h *Handler) ReleaseTask(c *gin.Context) {
	taskID := c.Param("id")

	var req struct {
		Reason string `json:"reason"`
	}
	c.ShouldBindJSON(&req)

	var status string
	var assignee *string
	var retryCount, maxRetries int
	err := h.db.QueryRow(
		"SELECT status, assignee, retry_count, max_retries FROM tasks WHERE id = $1",
		taskID,
	).Scan(&status, &assignee, &retryCount, &maxRetries)
	if err != nil {
		c.JSON(404, gin.H{"error": "Task not found"})
		return
	}

	releasable := map[string]bool{"claimed": true, "in_progress": true, "orphaned": true}
	if !releasable[status] {
		c.JSON(400, gin.H{
			"error":                "Task cannot be released",
			"current_status":       status,
			"releasable_statuses":  []string{"claimed", "in_progress", "orphaned"},
		})
		return
	}

	if retryCount >= maxRetries {
		h.db.Exec("UPDATE tasks SET status = 'escalated', escalated = true, updated_at = NOW() WHERE id = $1", taskID)
		if assignee != nil {
			h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *assignee)
		}
		h.logEvent(taskID, "pm", "escalated", status, "escalated", "Max retries exceeded, task escalated")
		h.broadcastTaskEvent(taskID, "task_updated", status, "escalated")
		c.JSON(200, gin.H{"message": "Task escalated (max retries exceeded)", "new_status": "escalated"})
		return
	}

	note := "Task released by PM"
	if req.Reason != "" {
		note += ": " + req.Reason
	}

	_, err = h.db.Exec(
		"UPDATE tasks SET status = 'available', assignee = NULL, retry_count = retry_count + 1, release_count = release_count + 1, claimed_at = NULL, updated_at = NOW() WHERE id = $1",
		taskID,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to release task"})
		return
	}

	if assignee != nil {
		h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *assignee)
	}

	h.logEvent(taskID, "pm", "released", status, "available", note)
	h.broadcastTaskEvent(taskID, "task_updated", status, "available")

	var task Task
	h.db.Get(&task, "SELECT * FROM tasks WHERE id = $1", taskID)

	c.JSON(200, gin.H{"message": "Task released", "task": task, "previous_assignee": assignee})
}

// --- AGENT ROUTES ---

func (h *Handler) ClaimTask(c *gin.Context) {
	taskID := c.Param("id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	// Check task is available and transition is valid
	var oldStatus string
	var err error
	err = h.db.Get(&oldStatus, "SELECT status FROM tasks WHERE id = $1", taskID)
	if err != nil {
		c.JSON(404, gin.H{"error": "Task not found"})
		return
	}
	if !isValidTransition(oldStatus, "claimed") {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid transition from %s to claimed", oldStatus)})
		return
	}

	// Claim
	_, err = h.db.Exec(
		"UPDATE tasks SET assignee = $1, status = 'claimed', claimed_at = NOW(), updated_at = NOW() WHERE id = $2",
		agentNameStr, taskID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to claim task"})
		return
	}

	// Increment agent's current_tasks counter
	h.db.Exec("UPDATE agents SET current_tasks = current_tasks + 1 WHERE name = $1", agentNameStr)

	h.logEvent(taskID, agentNameStr, "claimed", oldStatus, "claimed", "Task claimed by agent")

	var task Task
	h.db.Get(&task, "SELECT * FROM tasks WHERE id = $1", taskID)

	h.broadcastTaskEvent(taskID, "task_updated", oldStatus, "claimed")

	c.JSON(200, gin.H{"task": task})
}

func (h *Handler) UpdateProgress(c *gin.Context) {
	taskID := c.Param("id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	var req ProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// Only update progress for active statuses (don't overwrite done/review/test/failed)
	_, err := h.db.Exec(
		`UPDATE tasks SET progress = $1, updated_at = NOW()
		 WHERE id = $2 AND assignee = $3 AND status IN ('claimed', 'in_progress', 'needs_fix', 'fix_in_progress')`,
		req.Progress, taskID, agentNameStr)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to update progress"})
		return
	}

	// Auto-transition claimed → in_progress on first progress update
	h.db.Exec(
		`UPDATE tasks SET status = 'in_progress', updated_at = NOW()
		 WHERE id = $1 AND assignee = $2 AND status = 'claimed'`,
		taskID, agentNameStr)

	h.logEvent(taskID, agentNameStr, "progress", "", "in_progress", req.Note)

	h.broadcastTaskEvent(taskID, "task_updated", "", "in_progress")

	c.JSON(200, gin.H{"message": "Progress updated", "progress": req.Progress, "new_status": "in_progress"})
}

func (h *Handler) CompleteTask(c *gin.Context) {
	taskID := c.Param("id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	var req CompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	var oldStatus string
	var err error
	err = h.db.Get(&oldStatus, "SELECT status FROM tasks WHERE id = $1", taskID)
	if err != nil {
		c.JSON(404, gin.H{"error": "Task not found"})
		return
	}

	newStatus := "review" // auto-transition to review for review/test workers
	if req.Status == "done" {
		// Dev worker: task is done, PM reviews manually
		newStatus = "done"
	} else if req.Status == "failed" {
		newStatus = "failed"
	} else if req.Status == "blocked" {
		newStatus = "escalated"
	}

	if !isValidTransition(oldStatus, newStatus) {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid transition from %s to %s", oldStatus, newStatus)})
		return
	}

	_, err = h.db.Exec(
		`UPDATE tasks SET status = $1, progress = 100, completed_at = NOW(), updated_at = NOW(),
		 assignee = NULL
		 WHERE id = $2 AND assignee = $3`,
		newStatus, taskID, agentNameStr)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to complete task"})
		return
	}
	// Decrement agent counter regardless of newStatus (task is leaving the agent's queue)
	h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", agentNameStr)

	note := req.Notes
	if req.Branch != "" {
		note += " (branch: " + req.Branch + ")"
	}
	h.logEvent(taskID, agentNameStr, "completed", oldStatus, newStatus, note)

	// If critical severity or max retries, escalate
	if newStatus == "failed" {
		var retryCount, maxRetries int
		h.db.QueryRow("SELECT retry_count, max_retries FROM tasks WHERE id = $1", taskID).Scan(&retryCount, &maxRetries)
		if retryCount >= maxRetries {
			h.db.Exec("UPDATE tasks SET status = 'escalated', escalated = true WHERE id = $1", taskID)
			h.logEvent(taskID, "system", "escalated", "failed", "escalated", "Max retries exceeded")
			h.broadcastTaskEvent(taskID, "task_updated", "failed", "escalated")
		}
	}

	h.broadcastTaskEvent(taskID, "task_updated", oldStatus, newStatus)

	// Check if this task completion should advance the workflow phase
	if newStatus == "done" || newStatus == "deployed" {
		if h.WorkflowAdvancer != nil {
			if err := h.WorkflowAdvancer.CheckAndAdvancePhase(taskID); err != nil {
				log.Printf("[task] workflow advance error for %s: %v", taskID, err)
			}
		}
	}

	c.JSON(200, gin.H{"message": "Task completed", "new_status": newStatus})
}

func (h *Handler) ReviewTask(c *gin.Context) {
	taskID := c.Param("id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	var req ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	var oldStatus string
	err := h.db.Get(&oldStatus, "SELECT status FROM tasks WHERE id = $1", taskID)
	if err != nil {
		c.JSON(404, gin.H{"error": "Task not found"})
		return
	}

	if req.Verdict == "pass" {
		if !isValidTransition(oldStatus, "test") {
			c.JSON(400, gin.H{"error": fmt.Sprintf("invalid transition from %s to test", oldStatus)})
			return
		}
		// Pass → auto to test
		_, err = h.db.Exec(
			`UPDATE tasks SET review_verdict = 'pass', review_severity = NULL, review_issues = '{}',
			 status = 'test', updated_at = NOW()
			 WHERE id = $1 AND status IN ('review', 'done')`,
			taskID)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to update review"})
			return
		}
		h.logEvent(taskID, agentNameStr, "reviewed", oldStatus, "test", "Review passed")
		h.broadcastTaskEvent(taskID, "task_updated", oldStatus, "test")

		c.JSON(200, gin.H{"message": "Review passed", "new_status": "test"})

	} else {
		// Fail
		severity := "major"
		_ = severity // test severity noted
		if req.Severity != "" {
			severity = req.Severity
		}

		if severity == "critical" {
			if !isValidTransition(oldStatus, "escalated") {
				c.JSON(400, gin.H{"error": fmt.Sprintf("invalid transition from %s for review (escalated)", oldStatus)})
				return
			}
			// Critical → immediate escalation
			h.db.Exec(
				`UPDATE tasks SET review_verdict = 'fail', review_severity = $1, review_issues = $2,
				 status = 'escalated', escalated = true, updated_at = NOW()
				 WHERE id = $3 AND status IN ('review', 'done')`,
				severity, db.StringArray(req.Issues), taskID)
			h.logEvent(taskID, agentNameStr, "reviewed", oldStatus, "escalated", "Critical issues found: "+joinIssues(req.Issues))
			h.broadcastTaskEvent(taskID, "task_updated", oldStatus, "escalated")
			c.JSON(200, gin.H{"message": "Critical issues found, escalated to PM", "new_status": "escalated"})

		} else {
			if !isValidTransition(oldStatus, "needs_fix") && !isValidTransition(oldStatus, "fix_in_progress") {
				c.JSON(400, gin.H{"error": fmt.Sprintf("invalid transition from %s for review (back to dev)", oldStatus)})
				return
			}
			// Major/Minor → back to dev
			newStatus := "needs_fix"
			if severity == "minor" {
				newStatus = "fix_in_progress"
			}

			h.db.Exec(
				`UPDATE tasks SET review_verdict = 'fail', review_severity = $1, review_issues = $2,
				 retry_count = CASE WHEN $3 = 'minor' THEN retry_count ELSE retry_count + 1 END,
				 status = $4, progress = 0, updated_at = NOW()
				 WHERE id = $5 AND status IN ('review', 'done')`,
				severity, db.StringArray(req.Issues), severity, newStatus, taskID)

			// Check max retries
			var retryCount, maxRetries int
			h.db.QueryRow("SELECT retry_count, max_retries FROM tasks WHERE id = $1", taskID).Scan(&retryCount, &maxRetries)
			if retryCount >= maxRetries {
				h.db.Exec("UPDATE tasks SET status = 'escalated', escalated = true WHERE id = $1", taskID)
				h.logEvent(taskID, "system", "escalated", newStatus, "escalated", "Max retries exceeded after review")
				h.broadcastTaskEvent(taskID, "task_updated", newStatus, "escalated")
				c.JSON(200, gin.H{"message": "Max retries exceeded, escalated to PM", "new_status": "escalated"})
				return
			}

			h.logEvent(taskID, agentNameStr, "reviewed", oldStatus, newStatus, "Review failed ("+severity+"): "+joinIssues(req.Issues))
			h.broadcastTaskEvent(taskID, "task_updated", oldStatus, newStatus)
			c.JSON(200, gin.H{"message": "Review failed", "new_status": newStatus, "severity": severity})
		}
	}
}

func (h *Handler) TestTask(c *gin.Context) {
	taskID := c.Param("id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	var req TestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	var oldStatus string
	err := h.db.Get(&oldStatus, "SELECT status FROM tasks WHERE id = $1", taskID)
	if err != nil {
		c.JSON(404, gin.H{"error": "Task not found"})
		return
	}

	if req.Verdict == "pass" {
		if !isValidTransition(oldStatus, "deployed") {
			c.JSON(400, gin.H{"error": fmt.Sprintf("invalid transition from %s to deployed", oldStatus)})
			return
		}
		h.db.Exec(
			`UPDATE tasks SET test_verdict = 'pass', test_issues = '{}',
			 status = 'deployed', updated_at = NOW()
			 WHERE id = $1 AND status = 'test'`,
			taskID)

		// Update agent stats
		var assignee string
		h.db.Get(&assignee, "SELECT COALESCE(assignee, '') FROM tasks WHERE id = $1", taskID)
		if assignee != "" {
			h.db.Exec("UPDATE agents SET total_completed = total_completed + 1, current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", assignee)
		}

		h.logEvent(taskID, agentNameStr, "tested", oldStatus, "deployed", "All tests passed")
		h.broadcastTaskEvent(taskID, "task_updated", oldStatus, "deployed")
		c.JSON(200, gin.H{"message": "Tests passed, task deployed", "new_status": "deployed"})

	} else {
		// Fail
		if !isValidTransition(oldStatus, "needs_fix") && !isValidTransition(oldStatus, "fix_in_progress") {
			c.JSON(400, gin.H{"error": fmt.Sprintf("invalid transition from %s for test fail", oldStatus)})
			return
		}
		newStatus := "needs_fix"

		h.db.Exec(
			`UPDATE tasks SET test_verdict = 'fail', test_issues = $1,
			 retry_count = retry_count + 1, status = $2, progress = 0, updated_at = NOW()
			 WHERE id = $3 AND status = 'test'`,
			db.StringArray(req.Issues), newStatus, taskID)

		// Decrement the original assignee's current_tasks counter (not the tester's).
		// The tester never claimed the task — the assignee (dev) did via ClaimTask.
		var assignee string
		h.db.Get(&assignee, "SELECT COALESCE(assignee, '') FROM tasks WHERE id = $1", taskID)
		if assignee != "" {
			h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", assignee)
		}

		// Check max retries
		var retryCount, maxRetries int
		h.db.QueryRow("SELECT retry_count, max_retries FROM tasks WHERE id = $1", taskID).Scan(&retryCount, &maxRetries)
		if retryCount >= maxRetries {
			h.db.Exec("UPDATE tasks SET status = 'escalated', escalated = true WHERE id = $1", taskID)
			h.logEvent(taskID, "system", "escalated", newStatus, "escalated", "Max retries exceeded after test")
			h.broadcastTaskEvent(taskID, "task_updated", newStatus, "escalated")
			c.JSON(200, gin.H{"message": "Max retries exceeded, escalated to PM", "new_status": "escalated"})
			return
		}

		h.logEvent(taskID, agentNameStr, "tested", oldStatus, newStatus, "Tests failed: "+joinIssues(req.Issues))
		h.broadcastTaskEvent(taskID, "task_updated", oldStatus, newStatus)
		c.JSON(200, gin.H{"message": "Tests failed", "new_status": newStatus})
	}
}

func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	g.GET("/tasks", h.ListTasks)
	g.GET("/tasks/:id", h.GetTask)
	g.POST("/tasks", h.CreateTask)
	g.PATCH("/tasks/:id", h.UpdateTask)
	g.DELETE("/tasks/:id", h.DeleteTask)
	g.POST("/tasks/:id/reassign", h.ReassignTask)
	g.POST("/tasks/:id/escalate", h.EscalateTask)
	g.POST("/tasks/:id/release", h.ReleaseTask)
}

func (h *Handler) RegisterAgentRoutes(g *gin.RouterGroup) {
	// Note: /tasks/queue is registered by agent handler
	g.POST("/tasks/:id/claim", h.ClaimTask)
	g.PATCH("/tasks/:id/progress", h.UpdateProgress)
	g.POST("/tasks/:id/complete", h.CompleteTask)
	g.POST("/tasks/:id/review", h.ReviewTask)
	g.POST("/tasks/:id/test", h.TestTask)
}

func (h *Handler) logEvent(taskID, agent, event, fromStatus, toStatus, note string) {
	h.db.Exec(
		`INSERT INTO task_events (task_id, agent, event, from_status, to_status, note)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		taskID, agent, event, nilIfEmpty(fromStatus), nilIfEmpty(toStatus), note)
}

func (h *Handler) broadcastTaskEvent(taskID, eventType, fromStatus, toStatus string) {
	var task Task
	if err := h.db.Get(&task, "SELECT id, assignee FROM tasks WHERE id = $1", taskID); err != nil {
		return
	}
	if h.hub == nil {
		return
	}
	event := map[string]interface{}{
		"task_id":    task.ID,
		"from_status": fromStatus,
		"to_status":   toStatus,
		"agent":       task.Assignee,
		"event":       eventType,
		"timestamp":   time.Now().UTC(),
	}
	data, _ := json.Marshal(event)
	h.hub.Broadcast(data)
}

func nilOrEmpty(s []string) interface{} {
	if s == nil || len(s) == 0 {
		return []string{}
	}
	return s
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func placeholder(idx int) string {
	return fmt.Sprintf("$%d", idx)
}

func argPlaceholder(idx int, col string) string {
	return fmt.Sprintf(" AND %s = $%d", col, idx)
}

func joinIssues(issues []string) string {
	if len(issues) == 0 {
		return ""
	}
	result := issues[0]
	for _, i := range issues[1:] {
		result += "; " + i
	}
	return result
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
