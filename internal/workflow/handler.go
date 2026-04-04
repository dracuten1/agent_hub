package workflow

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// ─── Handler setup ──────────────────────────────────────────────────────────

// Handler handles workflow HTTP requests.
type Handler struct {
	db     *sqlx.DB
	engine *Engine
}

// NewHandler creates a workflow handler.
func NewHandler(db *sqlx.DB, engine *Engine) *Handler {
	return &Handler{db: db, engine: engine}
}

// RegisterRoutes registers all workflow routes on the given router group.
func RegisterRoutes(r gin.IRouter, db *sqlx.DB, engine *Engine) {
	h := NewHandler(db, engine)
	g := r.Group("/api/workflows")
	g.POST("/start", h.StartWorkflow)
	g.POST("/:id/approve", h.Approve)
	g.GET("/:id", h.GetWorkflow)
	g.GET("", h.ListWorkflows)
	g.GET("/templates", h.ListTemplates)
	g.POST("/templates", h.CreateTemplate)
	g.DELETE("/:id", h.DeleteWorkflow)
}

// ─── Request types ─────────────────────────────────────────────────────────

// StartWorkflowRequest matches spec §6.1.
type StartWorkflowRequest struct {
	TemplateID   string `json:"template_id"`
	TemplateName string `json:"template_name"`
	Name         string `json:"name" binding:"required"`
	ProjectID    string `json:"project_id"`
	Description  string `json:"description"`
	Variables    map[string]interface{} `json:"variables"`
}

// ApproveRequest matches spec §6.2.
type ApproveRequest struct {
	Note string `json:"note"`
}

// ─── POST /api/workflows/start ─────────────────────────────────────────────

func (h *Handler) StartWorkflow(c *gin.Context) {
	var req StartWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Resolve template: prefer template_id, fall back to template_name
	templateID := req.TemplateID
	if templateID == "" && req.TemplateName != "" {
		var id string
		if err := h.db.Get(&id,
			`SELECT id FROM workflow_templates WHERE name = $1`, req.TemplateName); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("template %q not found", req.TemplateName)})
			return
		}
		templateID = id
	}
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template_id or template_name is required"})
		return
	}

	var variables string
	if len(req.Variables) > 0 {
		b, _ := json.Marshal(req.Variables)
		variables = string(b)
	} else {
		variables = "{}"
	}
	wf, err := h.engine.StartWorkflow(templateID, req.Name, req.ProjectID, req.Description, variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Load phases to return
	var phases []WorkflowPhase
	h.db.Select(&phases,
		`SELECT id,workflow_id,phase_index,phase_type,phase_name,status,total_tasks,completed_tasks
		 FROM workflow_phases WHERE workflow_id=$1 ORDER BY phase_index`,
		wf.ID)

	// Count tasks created in the first phase
	var tasksCreated int
	h.db.Get(&tasksCreated,
		`SELECT COUNT(*) FROM workflow_task_map WHERE workflow_id=$1 AND phase_id=$2`,
		wf.ID, phases[0].ID)

	c.JSON(http.StatusCreated, gin.H{
		"workflow":      wf,
		"phases":        phases,
		"tasks_created": tasksCreated,
	})
}

// ─── POST /api/workflows/:id/approve ───────────────────────────────────────

func (h *Handler) Approve(c *gin.Context) {
	workflowID := c.Param("id")
	var req ApproveRequest
	c.ShouldBindJSON(&req) // note is optional

	if err := h.engine.ApproveGate(workflowID, req.Note); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Load updated workflow and next phase for response
	var wf Workflow
	if err := h.db.Get(&wf,
		`SELECT id,status,current_phase FROM workflows WHERE id=$1`, workflowID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "workflow not found after approval"})
		return
	}

	var nextPhase WorkflowPhase
	h.db.Get(&nextPhase,
		`SELECT id,phase_type,phase_name,status,total_tasks
		 FROM workflow_phases
		 WHERE workflow_id=$1 AND phase_index=$2`,
		workflowID, wf.CurrentPhase)

	c.JSON(http.StatusOK, gin.H{
		"message":   "Gate approved",
		"workflow":  wf,
		"next_phase": nextPhase,
	})
}

// ─── GET /api/workflows/:id ─────────────────────────────────────────────────

func (h *Handler) GetWorkflow(c *gin.Context) {
	id := c.Param("id")

	var wf Workflow
	if err := h.db.Get(&wf, `SELECT * FROM workflows WHERE id=$1`, id); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Load all phases
	var phases []WorkflowPhase
	h.db.Select(&phases,
		`SELECT * FROM workflow_phases WHERE workflow_id=$1 ORDER BY phase_index`,
		id)

	// Per-phase task summary: only for current and past phases
	type TaskSummary struct {
		ID       string `json:"id"       db:"id"`
		Title    string `json:"title"    db:"title"`
		Status   string `json:"status"   db:"status"`
		Assignee string `json:"assignee" db:"assignee"`
	}

	type PhaseWithTasks struct {
		WorkflowPhase
		Tasks []TaskSummary `json:"tasks,omitempty"`
	}

	enriched := make([]PhaseWithTasks, 0, len(phases))
	for _, p := range phases {
		pt := PhaseWithTasks{WorkflowPhase: p}
		// Only load tasks for completed or active phases (not future ones)
		if p.Status == PhaseCompleted || p.Status == PhaseRunning || p.PhaseIndex <= wf.CurrentPhase {
			h.db.Select(&pt.Tasks,
				`SELECT t.id, t.title, t.status, t.assignee
				 FROM tasks t
				 JOIN workflow_task_map m ON m.task_id = t.id
				 WHERE m.phase_id=$1`,
				p.ID)
		}
		enriched = append(enriched, pt)
	}

	// Progress summary
	var total, done int
	h.db.Get(&total, `SELECT COUNT(*) FROM workflow_task_map WHERE workflow_id=$1`, id)
	h.db.Get(&done,
		`SELECT COUNT(*) FROM workflow_task_map m JOIN tasks t ON t.id=m.task_id
		 WHERE m.workflow_id=$1 AND t.status IN ('done','deployed')`, id)

	var pct int
	if total > 0 {
		pct = (done * 100) / total
	}

	c.JSON(http.StatusOK, gin.H{
		"workflow":  wf,
		"phases":    enriched,
		"progress": map[string]int{
			"total_tasks":      total,
			"completed_tasks":   done,
			"percentage":        pct,
		},
	})
}

// ─── GET /api/workflows ─────────────────────────────────────────────────────

func (h *Handler) ListWorkflows(c *gin.Context) {
	status := c.Query("status")
	projectID := c.Query("project_id")
	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	// Build filtered query
	query := `SELECT id, name, status, current_phase, total_phases, project_id, description, variables
	          FROM workflows WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		query += fmt.Sprintf(" AND status=$%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if projectID != "" {
		query += fmt.Sprintf(" AND project_id=$%d", argIdx)
		args = append(args, projectID)
		argIdx++
	}

	// Total count
	countQuery := `SELECT COUNT(*) FROM workflows WHERE 1=1`
	if status != "" {
		countQuery += fmt.Sprintf(" AND status=$1")
	}
	if projectID != "" {
		if status != "" {
			countQuery += fmt.Sprintf(" AND project_id=$2")
		} else {
			countQuery += fmt.Sprintf(" AND project_id=$1")
		}
	}
	var total int
	h.db.Get(&total, countQuery, args...)

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	var workflows []Workflow
	h.db.Select(&workflows, query, args...)

	// Enrich each with progress
	type WorkflowWithProgress struct {
		Workflow
		Progress *int `json:"progress,omitempty"`
	}

	result := make([]WorkflowWithProgress, 0, len(workflows))
	for _, wf := range workflows {
		var totalTasks, doneTasks int
		h.db.Get(&totalTasks,
			`SELECT COUNT(*) FROM workflow_task_map WHERE workflow_id=$1`, wf.ID)
		h.db.Get(&doneTasks,
			`SELECT COUNT(*) FROM workflow_task_map m JOIN tasks t ON t.id=m.task_id
			 WHERE m.workflow_id=$1 AND t.status IN ('done','deployed')`, wf.ID)

		var pct *int
		if totalTasks > 0 {
			v := (doneTasks * 100) / totalTasks
			pct = &v
		}
		result = append(result, WorkflowWithProgress{Workflow: wf, Progress: pct})
	}

	c.JSON(http.StatusOK, gin.H{
		"workflows": result,
		"total":     total,
	})
}

// ─── Template handlers ────────────────────────────────────────────────────

// CreateTemplateRequest is the body for POST /api/workflows/templates.
type CreateTemplateRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	Phases      json.RawMessage `json:"phases" binding:"required"`
}

func (h *Handler) CreateTemplate(c *gin.Context) {
	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var phases []PhaseConfig
	if err := json.Unmarshal(req.Phases, &phases); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid phases JSON"})
		return
	}
	if len(phases) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one phase required"})
		return
	}

	tmpl, err := CreateTemplate(h.db, req.Name, req.Description, phases)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"template": tmpl})
}

func (h *Handler) ListTemplates(c *gin.Context) {
	templates, err := ListTemplates(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

// ─── DELETE /api/workflows/:id ──────────────────────────────────────────────

func (h *Handler) DeleteWorkflow(c *gin.Context) {
	workflowID := c.Param("id")

	// Check existence
	var wf Workflow
	if err := h.db.Get(&wf, `SELECT id FROM workflows WHERE id=$1`, workflowID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// Delete workflow — cascade deletes phases and task_map rows
	if _, err := h.db.Exec(`DELETE FROM workflows WHERE id=$1`, workflowID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workflow deleted"})
}
