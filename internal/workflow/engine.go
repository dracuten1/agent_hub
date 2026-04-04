package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Phase status constants
const (
	PhasePending         = "pending"
	PhaseActive          = "active"
	PhaseComplete       = "complete"
	PhaseWaitingApproval = "waiting_approval"
	PhaseRunning        = "running"
	PhaseCompleted      = "completed"
)

// Phase types
const (
	PhaseTypeNormal   = "normal"
	PhaseTypeGate    = "gate"
	PhaseTypeSingle  = "single"
	PhaseTypeMulti   = "multi"
	PhaseTypePerDev  = "per_dev"
	PhaseTypeDecision = "decision"
)

// Workflow status constants
const (
	StatusActive    = "active"
	StatusPaused   = "paused"
	StatusComplete = "complete"
	StatusCancelled = "cancelled"
)

// Phase represents a workflow phase row from DB
type Phase struct {
	ID             string    `db:"id"`
	WorkflowID     string    `db:"workflow_id"`
	PhaseName      string    `db:"phase_name"`
	PhaseIndex     int       `db:"phase_index"`
	PhaseType      string    `db:"phase_type"`
	Status         string    `db:"status"`
	TotalTasks     int       `db:"total_tasks"`
	CompletedTasks int       `db:"completed_tasks"`
	FailedTasks   int       `db:"failed_tasks"`
	Config         string    `db:"config"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

// WorkflowPhase represents a phase used by the Engine (not a DB row)
type WorkflowPhase struct {
	ID             string          `db:"id" json:"id"`
	WorkflowID     string          `db:"workflow_id" json:"workflow_id"`
	PhaseName      string          `db:"phase_name" json:"name"`
	PhaseIndex     int             `db:"phase_index" json:"index"`
	PhaseType      string          `db:"phase_type" json:"phase_type"`
	TaskType       string          `db:"task_type" json:"task_type"`
	Status         string          `db:"status" json:"status"`
	Config         json.RawMessage `db:"config" json:"config,omitempty"`
	TotalTasks     int             `db:"total_tasks" json:"total_tasks"`
	CompletedTasks int             `db:"completed_tasks" json:"completed_tasks"`
	FailedTasks    int             `db:"failed_tasks" json:"failed_tasks"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
}

// Workflow represents a workflow instance
type Workflow struct {
	ID          string `db:"id"`
	ProjectID   string `db:"project_id"`
	Name        string `db:"name"`
	TotalPhases int    `db:"total_phases"`
	Status      string `db:"status"`
}

// PhaseConfig represents configuration for a workflow phase
type PhaseConfig struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// Engine manages workflow operations
type Engine struct {
	db *sqlx.DB
}

// NewEngine creates a new workflow engine
func NewEngine(db *sqlx.DB) *Engine {
	return &Engine{db: db}
}

// activatePhase activates a workflow phase and creates its tasks based on phase type.
func (e *Engine) activatePhase(workflowID string, phase *WorkflowPhase, projectID string) error {
	ctx := context.Background()

	switch phase.PhaseType {
	case PhaseTypeSingle:
		return e.createSinglePhaseTasks(ctx, workflowID, projectID, phase)
	case PhaseTypeMulti:
		return e.createMultiPhaseTasks(ctx, workflowID, projectID, phase)
	case PhaseTypePerDev:
		return e.createPerDevPhaseTasks(ctx, workflowID, projectID, phase)
	case PhaseTypeGate:
		return e.enterGatePhase(ctx, workflowID, phase)
	case PhaseTypeDecision:
		return e.evaluateDecisionPhase(ctx, workflowID, phase)
	case PhaseTypeNormal, "":
		_, err := e.db.ExecContext(ctx,
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseRunning, phase.ID)
		return err
	default:
		log.Printf("[workflow] unknown phase type %q for phase %s", phase.PhaseType, phase.ID)
		return nil
	}
}

// createSinglePhaseTasks creates one task for a "single" phase type.
func (e *Engine) createSinglePhaseTasks(ctx context.Context, wfID, projectID string, phase *WorkflowPhase) error {
	title := e.renderTemplate(phase.Config, "task_title_template", wfID)
	if title == "" {
		title = fmt.Sprintf("%s: %s", phase.PhaseName, wfID)
	}
	taskType := phase.TaskType
	if taskType == "" {
		taskType = "dev"
	}
	taskID, err := e.createTask(ctx, title, taskType, projectID, "available", phase.Config)
	if err != nil {
		return fmt.Errorf("createSinglePhaseTasks: create task: %w", err)
	}
	if err := e.addWorkflowMapping(ctx, taskID, wfID, phase.ID); err != nil {
		return fmt.Errorf("createSinglePhaseTasks: add mapping: %w", err)
	}
	_, err = e.db.ExecContext(ctx,
		`UPDATE workflow_phases SET status=$1,total_tasks=1,updated_at=NOW() WHERE id=$2`,
		PhaseRunning, phase.ID)
	return err
}

// createMultiPhaseTasks creates N parallel tasks for a "multi" phase type.
func (e *Engine) createMultiPhaseTasks(ctx context.Context, wfID, projectID string, phase *WorkflowPhase) error {
	cfg := parsePhaseConfig(phase.Config)
	count := cfg.GetInt("count")
	if count <= 0 {
		count = 1
	}
	titles := e.renderMultiTitles(phase.Config, "task_title_template", wfID, count)
	for _, title := range titles {
		taskType := phase.TaskType
		if taskType == "" {
			taskType = "dev"
		}
		taskID, err := e.createTask(ctx, title, taskType, projectID, "available", phase.Config)
		if err != nil {
			return fmt.Errorf("createMultiPhaseTasks: create task: %w", err)
		}
		if err := e.addWorkflowMapping(ctx, taskID, wfID, phase.ID); err != nil {
			return fmt.Errorf("createMultiPhaseTasks: add mapping: %w", err)
		}
	}
	_, err := e.db.ExecContext(ctx,
		`UPDATE workflow_phases SET status=$1,total_tasks=$2,updated_at=NOW() WHERE id=$3`,
		PhaseRunning, count, phase.ID)
	return err
}

// createPerDevPhaseTasks creates one follow-up task per completed task from previous phase.
// Dependencies are set via task_dependencies table; fn_dep_added trigger handles pending_deps.
func (e *Engine) createPerDevPhaseTasks(ctx context.Context, wfID, projectID string, phase *WorkflowPhase) error {
	prevPhaseIndex := phase.PhaseIndex - 1

	var prevTasks []struct {
		ID    string `db:"id"`
		Title string `db:"title"`
	}
	err := e.db.SelectContext(ctx, &prevTasks,
		`SELECT t.id, t.title
		 FROM   tasks t
		 JOIN   workflow_task_map m ON m.task_id = t.id
		 JOIN   workflow_phases p  ON m.phase_id = p.id
		 WHERE  p.workflow_id = $1 AND p.phase_index = $2
		   AND  t.status IN ('done', 'deployed')`,
		wfID, prevPhaseIndex)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("createPerDevPhaseTasks: query prev tasks: %w", err)
	}

	if len(prevTasks) == 0 {
		_, err = e.db.ExecContext(ctx,
			`UPDATE workflow_phases SET status=$1,total_tasks=0,updated_at=NOW() WHERE id=$2`,
			PhaseCompleted, phase.ID)
		return err
	}

	cfg := parsePhaseConfig(phase.Config)
	taskTemplate := cfg.GetString("task_title_template")
	taskType := phase.TaskType
	if taskType == "" {
		taskType = "dev"
	}

	for _, prev := range prevTasks {
		title := strings.Replace(taskTemplate, "{parent_title}", prev.Title, -1)
		if title == "" {
			title = fmt.Sprintf("%s: %s", phase.PhaseName, prev.Title)
		}
		taskID, err := e.createTask(ctx, title, taskType, projectID, "blocked", phase.Config)
		if err != nil {
			return fmt.Errorf("createPerDevPhaseTasks: create task: %w", err)
		}
		// fn_dep_added trigger handles pending_deps++
		_, err = e.db.ExecContext(ctx,
			`INSERT INTO task_dependencies (task_id, depends_on_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			taskID, prev.ID)
		if err != nil {
			return fmt.Errorf("createPerDevPhaseTasks: add dep: %w", err)
		}
		if err := e.addWorkflowMapping(ctx, taskID, wfID, phase.ID); err != nil {
			return fmt.Errorf("createPerDevPhaseTasks: add mapping: %w", err)
		}
	}

	_, err = e.db.ExecContext(ctx,
		`UPDATE workflow_phases SET status=$1,total_tasks=$2,updated_at=NOW() WHERE id=$3`,
		PhaseRunning, len(prevTasks), phase.ID)
	return err
}

// enterGatePhase transitions a gate phase to waiting_approval and sends notification.
func (e *Engine) enterGatePhase(ctx context.Context, wfID string, phase *WorkflowPhase) error {
	_, err := e.db.ExecContext(ctx,
		`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
		PhaseWaitingApproval, phase.ID)
	if err != nil {
		return fmt.Errorf("enterGatePhase: update phase: %w", err)
	}
	_, err = e.db.ExecContext(ctx,
		`UPDATE workflows SET status=$1,updated_at=NOW() WHERE id=$2`,
		StatusPaused, wfID)
	if err != nil {
		return fmt.Errorf("enterGatePhase: pause workflow: %w", err)
	}
	cfg := parsePhaseConfig(phase.Config)
	e.sendGateNotification(wfID, phase, cfg.GetString("approver"), cfg.GetBool("require_owner"))
	return nil
}

// evaluateDecisionPhase evaluates a "decision" phase based on pass condition.
func (e *Engine) evaluateDecisionPhase(ctx context.Context, wfID string, phase *WorkflowPhase) error {
	cfg := parsePhaseConfig(phase.Config)
	passCondition := cfg.GetString("pass_condition")
	maxRetries    := cfg.GetInt("max_retries")
	retryCount    := cfg.GetInt("retry_count")
	prevPhaseIndex := phase.PhaseIndex - 1

	var prevTotal, prevCompleted, prevFailed int
	e.db.GetContext(ctx, &prevTotal,
		`SELECT COALESCE(total_tasks,0) FROM workflow_phases WHERE workflow_id=$1 AND phase_index=$2`,
		wfID, prevPhaseIndex)
	e.db.GetContext(ctx, &prevCompleted,
		`SELECT COALESCE(completed_tasks,0) FROM workflow_phases WHERE workflow_id=$1 AND phase_index=$2`,
		wfID, prevPhaseIndex)
	e.db.GetContext(ctx, &prevFailed,
		`SELECT COALESCE(failed_tasks,0) FROM workflow_phases WHERE workflow_id=$1 AND phase_index=$2`,
		wfID, prevPhaseIndex)

	switch passCondition {
	case "all":
		if prevFailed == 0 {
			_, _ = e.db.ExecContext(ctx,
				`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
				PhaseCompleted, phase.ID)
			return e.advanceWorkflow(wfID)
		}
		if retryCount < maxRetries {
			prevPhase := &WorkflowPhase{ID: fmt.Sprintf("phase-%s-%d", wfID, prevPhaseIndex), WorkflowID: wfID}
			return e.retryPreviousPhase(wfID, prevPhase)
		}
		return e.escalateWorkflow(wfID, phase, prevFailed, prevTotal)

	case "any":
		if prevCompleted > 0 {
			_, _ = e.db.ExecContext(ctx,
				`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
				PhaseCompleted, phase.ID)
			return e.advanceWorkflow(wfID)
		}
		return e.escalateWorkflow(wfID, phase, prevFailed, prevTotal)

	case "threshold":
		if prevTotal == 0 {
			_, _ = e.db.ExecContext(ctx,
				`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
				PhaseCompleted, phase.ID)
			return e.advanceWorkflow(wfID)
		}
		passRate := float64(prevCompleted) / float64(prevTotal)
		threshold := cfg.GetInt("threshold")
		if passRate >= float64(threshold)/100.0 {
			_, _ = e.db.ExecContext(ctx,
				`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
				PhaseCompleted, phase.ID)
			return e.advanceWorkflow(wfID)
		}
		if retryCount < maxRetries {
			prevPhase := &WorkflowPhase{ID: fmt.Sprintf("phase-%s-%d", wfID, prevPhaseIndex), WorkflowID: wfID}
			return e.retryPreviousPhase(wfID, prevPhase)
		}
		return e.escalateWorkflow(wfID, phase, prevFailed, prevTotal)

	default:
		_, _ = e.db.ExecContext(ctx,
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseCompleted, phase.ID)
		return e.advanceWorkflow(wfID)
	}
}

// advanceWorkflow advances a completed phase to the next phase.
// A phase is considered complete when all its tasks are done OR it has zero tasks.
func (e *Engine) advanceWorkflow(workflowID string) error {
	// Get the current active phase
	var phase Phase
	err := e.db.Get(&phase,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
		        total_tasks, completed_tasks, failed_tasks, config, created_at, updated_at
		 FROM workflow_phases
		 WHERE workflow_id = $1 AND status = $2
		 ORDER BY phase_index ASC LIMIT 1`,
		workflowID, PhaseActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[workflow] no active phase for workflow %s", workflowID)
			return nil
		}
		return err
	}

	// A phase is complete when all tasks are done, or when it has zero tasks.
	// (Empty phases auto-complete so the workflow can advance.)
	isDone := phase.CompletedTasks == phase.TotalTasks
	isEmpty := phase.TotalTasks == 0 && phase.FailedTasks == 0

	if isDone || isEmpty {
		_, _ = e.db.Exec(
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseCompleted, phase.ID)

		// Get next pending phase
		var next Phase
		err := e.db.Get(&next,
			`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
			        total_tasks, completed_tasks, failed_tasks, config, created_at, updated_at
			 FROM workflow_phases
			 WHERE workflow_id = $1 AND status = $2
			 ORDER BY phase_index ASC LIMIT 1`,
			workflowID, PhasePending)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Printf("[workflow] workflow %s complete - no more phases", workflowID)
				e.db.Exec(`UPDATE workflows SET status=$1,updated_at=NOW() WHERE id=$2`, StatusComplete, workflowID)
				return nil
			}
			return err
		}

		// Mark current phase as complete
		_, err = e.db.Exec(
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseComplete, phase.ID)
		if err != nil {
			return err
		}

		// Check if next phase is a gate
		if IsGatePhase(next.PhaseType) {
			_, err = e.db.Exec(
				`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
				PhaseWaitingApproval, next.ID)
			if err != nil {
				return err
			}
			SendGateNotification(workflowID, next.ID, next.PhaseName)
			log.Printf("[workflow] workflow %s waiting at gate: %s", workflowID, next.PhaseName)
			return nil
		}

		// Activate next phase
		_, err = e.db.Exec(
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseActive, next.ID)
		if err != nil {
			return err
		}
		// Start tasks for the next phase
		projectID := ""
		e.db.Get(&projectID, `SELECT project_id FROM workflows WHERE id=$1`, workflowID)
		nextPhase := &WorkflowPhase{
			ID: next.ID, WorkflowID: next.WorkflowID,
			PhaseName: next.PhaseName, PhaseIndex: next.PhaseIndex,
			PhaseType: next.PhaseType, TotalTasks: next.TotalTasks,
		}
		if err := e.activatePhase(workflowID, nextPhase, projectID); err != nil {
			log.Printf("[workflow] activatePhase error for next phase %s: %v", next.ID, err)
		}

		log.Printf("[workflow] workflow %s advanced to phase: %s", workflowID, next.PhaseName)
	}

	return nil
}

// retryPreviousPhase recreates failed tasks from the previous phase.
func (e *Engine) retryPreviousPhase(wfID string, prevPhase *WorkflowPhase) error {
	ctx := context.Background()
	_, err := e.db.ExecContext(ctx,
		`UPDATE tasks t
		 SET    status='available', updated_at=NOW()
		 FROM   workflow_task_map m
		 WHERE  m.task_id = t.id
		   AND  m.workflow_id = $1
		   AND  m.phase_id = $2
		   AND  t.status IN ('failed','skipped')`,
		wfID, prevPhase.ID)
	if err != nil {
		return fmt.Errorf("retryPreviousPhase: %w", err)
	}
	_, err = e.db.ExecContext(ctx,
		`UPDATE workflow_phases
		 SET    status=$1, failed_tasks=0, updated_at=NOW()
		 WHERE  id = $2`,
		PhaseRunning, prevPhase.ID)
	return err
}

// escalateWorkflow marks a workflow as cancelled and logs the escalation.
func (e *Engine) escalateWorkflow(wfID string, phase *WorkflowPhase, failedTasks, totalTasks int) error {
	ctx := context.Background()
	_, err := e.db.ExecContext(ctx,
		`UPDATE workflows SET status=$1,updated_at=NOW() WHERE id=$2`,
		StatusCancelled, wfID)
	if err != nil {
		return fmt.Errorf("escalateWorkflow: %w", err)
	}
	log.Printf("[workflow] workflow %s escalated at phase %s (failed: %d/%d)",
		wfID, phase.PhaseName, failedTasks, totalTasks)
	return nil
}

// sendGateNotification logs gate approval requirement.
func (e *Engine) sendGateNotification(wfID string, phase *WorkflowPhase, approver string, requireOwner bool) {
	log.Printf("[workflow] gate approval required: workflow=%s phase=%q approver=%q require_owner=%v",
		wfID, phase.PhaseName, approver, requireOwner)
	SendGateNotification(wfID, phase.ID, phase.PhaseName)
}

// createTask inserts a new task row and returns its ID.
// projectID is passed explicitly to avoid circular subquery.
func (e *Engine) createTask(ctx context.Context, title, taskType, projectID, status string, config json.RawMessage) (string, error) {
	var id string
	err := e.db.GetContext(ctx, &id,
		`INSERT INTO tasks (title, task_type, status, project_id)
		 VALUES ($1, $2, $3, NULLIF($4, ''))
		 RETURNING id`,
		title, taskType, status, projectID)
	if err != nil {
		return "", err
	}
	return id, nil
}

// addWorkflowMapping records the task→workflow→phase association.
func (e *Engine) addWorkflowMapping(ctx context.Context, taskID, workflowID, phaseID string) error {
	_, err := e.db.ExecContext(ctx,
		`INSERT INTO workflow_task_map (task_id, workflow_id, phase_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (task_id) DO UPDATE SET phase_id = EXCLUDED.phase_id`,
		taskID, workflowID, phaseID)
	return err
}

// renderTemplate substitutes placeholders in a string template.
func (e *Engine) renderTemplate(config json.RawMessage, key, workflowID string) string {
	var m map[string]interface{}
	if err := json.Unmarshal(config, &m); err != nil {
		return ""
	}
	tpl, _ := m[key].(string)
	tpl = strings.ReplaceAll(tpl, "{workflow_id}", workflowID)
	return tpl
}

// renderMultiTitles generates N task titles from a template string.
func (e *Engine) renderMultiTitles(config json.RawMessage, key, workflowID string, count int) []string {
	var m map[string]interface{}
	_ = json.Unmarshal(config, &m)
	tpl, _ := m[key].(string)
	if tpl == "" {
		tpl = "{workflow_id} task"
	}
	titles := make([]string, count)
	for i := 0; i < count; i++ {
		title := tpl
		title = strings.ReplaceAll(title, "{workflow_id}", workflowID)
		title = strings.ReplaceAll(title, "{n}", fmt.Sprintf("%d", i+1))
		titles[i] = title
	}
	return titles
}

// parsePhaseConfig parses a phase's JSONB config into a map.
func parsePhaseConfig(config json.RawMessage) phaseConfigMap {
	var m map[string]interface{}
	if err := json.Unmarshal(config, &m); err != nil {
		return phaseConfigMap{}
	}
	return phaseConfigMap(m)
}

// phaseConfigMap is a map-based config accessor.
type phaseConfigMap map[string]interface{}

func (c phaseConfigMap) GetString(key string) string {
	if v, ok := c[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c phaseConfigMap) GetInt(key string) int {
	if v, ok := c[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 0
}

func (c phaseConfigMap) GetBool(key string) bool {
	if v, ok := c[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// ─── Package-level functions (used by cmd/server) ─────────────────────────────

// StartWorkflow creates and starts a new workflow from a template.
func (e *Engine) StartWorkflow(templateID, name, projectID, description, variables string) (*Workflow, error) {
	tx, err := e.db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	wfID := uuid.New().String()

	// Parse template phases if provided
	var phaseConfigs []PhaseConfig
	if templateID != "" {
		var cfgBytes []byte
		err := tx.Get(&cfgBytes,
			`SELECT phase_configs FROM workflow_templates WHERE id=$1`, templateID)
		if err == nil && len(cfgBytes) > 0 {
			json.Unmarshal(cfgBytes, &phaseConfigs)
		}
	}
	if len(phaseConfigs) == 0 {
		// Default: single dev phase
		phaseConfigs = []PhaseConfig{{Type: PhaseTypeSingle, Name: "Development"}}
	}

	_, err = tx.Exec(
		`INSERT INTO workflows (id, name, project_id, total_phases, status)
		 VALUES ($1, $2, $3, $4, 'active')`,
		wfID, name, projectID, len(phaseConfigs))
	if err != nil {
		return nil, fmt.Errorf("insert workflow: %w", err)
	}

	firstPhaseID := uuid.New().String()
	for i, pc := range phaseConfigs {
		phaseID := firstPhaseID
		if i > 0 {
			phaseID = uuid.New().String()
		}
		status := PhasePending
		if i == 0 {
			status = PhaseActive // Bug 1 fix: was PhaseRunning, should be PhaseActive
		}
		_, err = tx.Exec(
			`INSERT INTO workflow_phases (id, workflow_id, phase_name, phase_index, phase_type, status)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			phaseID, wfID, pc.Name, i, pc.Type, status)
		if err != nil {
			return nil, fmt.Errorf("insert phase: %w", err)
		}
		if i == 0 {
			firstPhaseID = phaseID
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Activate first phase
	firstPhase := &WorkflowPhase{ID: firstPhaseID, WorkflowID: wfID, PhaseType: phaseConfigs[0].Type}
	if err := e.activatePhase(wfID, firstPhase, projectID); err != nil {
		log.Printf("[workflow] activatePhase error: %v", err)
	}

	log.Printf("[workflow] started workflow %s (%s), %d phases", wfID, name, len(phaseConfigs))
	return &Workflow{ID: wfID, ProjectID: projectID, Name: name, TotalPhases: len(phaseConfigs), Status: StatusActive}, nil
}

// AdvanceWorkflow moves the workflow to the next phase if the current phase is complete.
// (Standalone function for use by task completion hooks.)
func AdvanceWorkflow(db *sqlx.DB, workflowID string) error {
	e := &Engine{db: db}
	return e.advanceWorkflow(workflowID)
}

// ApproveGate advances a gate phase that is waiting for approval.
func (e *Engine) ApproveGate(workflowID, note string) error {
	ctx := context.Background()

	var gate Phase
	err := e.db.GetContext(ctx, &gate,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
		        total_tasks, completed_tasks, failed_tasks, config, created_at, updated_at
		 FROM workflow_phases
		 WHERE workflow_id=$1 AND status=$2`,
		workflowID, PhaseWaitingApproval)
	if err != nil {
		return fmt.Errorf("approveGate: %w", err)
	}

	_, err = e.db.ExecContext(ctx,
		`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
		PhaseActive, gate.ID)
	if err != nil {
		return fmt.Errorf("approveGate: activate phase: %w", err)
	}

	_, err = e.db.ExecContext(ctx,
		`UPDATE workflows SET status=$1,updated_at=NOW() WHERE id=$2`,
		StatusActive, workflowID)
	if err != nil {
		return fmt.Errorf("approveGate: resume workflow: %w", err)
	}

	return e.advanceWorkflow(workflowID)
}

// GetGatePhase returns the current waiting_approval phase for a workflow.
func (e *Engine) GetGatePhase(workflowID string) (*Phase, error) {
	var phase Phase
	err := e.db.Get(&phase,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
		        total_tasks, completed_tasks, failed_tasks, config, created_at, updated_at
		 FROM workflow_phases WHERE workflow_id=$1 AND status=$2`,
		workflowID, PhaseWaitingApproval)
	if err != nil {
		return nil, err
	}
	return &phase, nil
}

// IsGatePhase returns true if the phase type is a gate
func IsGatePhase(phaseType string) bool {
	return phaseType == PhaseTypeGate
}
