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
	Description    string          `db:"description" json:"description,omitempty"`
	TotalTasks     int             `db:"total_tasks" json:"total_tasks"`
	CompletedTasks int             `db:"completed_tasks" json:"completed_tasks"`
	FailedTasks    int             `db:"failed_tasks" json:"failed_tasks"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
}

// Workflow represents a workflow instance
type Workflow struct {
	ID           string `db:"id" json:"id"`
	ProjectID    string `db:"project_id" json:"project_id"`
	Name         string `db:"name" json:"name"`
	TotalPhases  int    `db:"total_phases" json:"total_phases"`
	Status       string `db:"status" json:"status"`
	CurrentPhase int    `db:"current_phase" json:"current_phase"`
	Description  string `db:"description" json:"description"`
	CreatedAt    string `db:"created_at" json:"created_at"`
	UpdatedAt    string `db:"updated_at" json:"updated_at"`
}

// Template represents a workflow template
type Template struct {
	ID          string       `db:"id" json:"id"`
	Name        string       `db:"name" json:"name"`
	Description  string       `db:"description" json:"description"`
	Phases      []PhaseConfig `db:"-" json:"phases,omitempty"`
	CreatedAt   string       `db:"created_at" json:"created_at"`
	UpdatedAt   string       `db:"updated_at" json:"updated_at"`
}

// PhaseConfig represents configuration for a workflow phase
type PhaseConfig struct {
	Type         string          `json:"phase_type"`
	Name         string          `json:"phase_name"`
	TaskType     string          `json:"task_type"`
	Count        int             `json:"count"`
	PassCondition string         `json:"pass_condition"`
	MaxRetries   int             `json:"max_retries"`
	Auto         bool            `json:"auto"`
	RequireOwner bool            `json:"require_owner"`
	Approver     string          `json:"approver"`
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
	taskID, err := e.createTask(ctx, title, taskType, projectID, "available", phase.Description)
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
		taskID, err := e.createTask(ctx, title, taskType, projectID, "available", phase.Description)
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

	// Skip if tasks already exist for this phase (idempotent — avoids duplicating
	// review tasks already auto-queued by CheckAndAdvancePhase)
	var existing int
	e.db.GetContext(ctx, &existing,
		`SELECT COUNT(*) FROM workflow_task_map WHERE phase_id = $1`, phase.ID)
	if existing > 0 {
		log.Printf("[workflow] per_dev phase %s already has tasks, skipping", phase.ID)
		_, _ = e.db.ExecContext(ctx,
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseRunning, phase.ID)
		return nil
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
		desc := fmt.Sprintf("Implement follow-up work for task %s (%s).\nPhase: %s\nTask type: %s",
			prev.ID, prev.Title, phase.PhaseName, taskType)
		taskID, err := e.createTask(ctx, title, taskType, projectID, "blocked", desc)
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
	if err := e.db.GetContext(ctx, &prevTotal,
		`SELECT COALESCE(total_tasks,0) FROM workflow_phases WHERE workflow_id=$1 AND phase_index=$2`,
		wfID, prevPhaseIndex); err != nil {
		log.Printf("[workflow] evaluateDecision: get prevTotal: %v", err)
	}
	if err := e.db.GetContext(ctx, &prevCompleted,
		`SELECT COALESCE(completed_tasks,0) FROM workflow_phases WHERE workflow_id=$1 AND phase_index=$2`,
		wfID, prevPhaseIndex); err != nil {
		log.Printf("[workflow] evaluateDecision: get prevCompleted: %v", err)
	}
	if err := e.db.GetContext(ctx, &prevFailed,
		`SELECT COALESCE(failed_tasks,0) FROM workflow_phases WHERE workflow_id=$1 AND phase_index=$2`,
		wfID, prevPhaseIndex); err != nil {
		log.Printf("[workflow] evaluateDecision: get prevFailed: %v", err)
	}

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
		        total_tasks, completed_tasks, failed_tasks, config, description, created_at, updated_at
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

	// A phase is complete when all tasks are done.
	// Empty non-gate phases auto-complete so the workflow can advance.
	// Gate phases with 0 tasks still wait for approval.
	isDone := phase.CompletedTasks == phase.TotalTasks && phase.TotalTasks > 0
	isEmpty := phase.TotalTasks == 0 && phase.FailedTasks == 0 && phase.PhaseType != PhaseTypeGate && phase.PhaseType != PhaseTypeDecision

	if isDone || isEmpty {
		_, _ = e.db.Exec(
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseCompleted, phase.ID)

		// Get next pending phase
		var next Phase
		err := e.db.Get(&next,
			`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
			        total_tasks, completed_tasks, failed_tasks, config, description, created_at, updated_at
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
// description is the task description string.
func (e *Engine) createTask(ctx context.Context, title, taskType, projectID, status, description string) (string, error) {
	var id string
	err := e.db.GetContext(ctx, &id,
		`INSERT INTO tasks (title, description, task_type, status, project_id)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		 RETURNING id`,
		title, description, taskType, status, projectID)
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

// CreateWorkflowTestTask creates a test task in the testing phase when a review passes.
// Called from task handler after review verdict=pass.
func (e *Engine) CreateWorkflowTestTask(reviewTaskID string) error {
	ctx := context.Background()

	// 1. Look up the review task info and its workflow/phase
	var info struct {
		Title      string `db:"title"`
		WorkflowID string `db:"workflow_id"`
		PhaseID    string `db:"phase_id"`
		PhaseIndex int    `db:"phase_index"`
	}
	err := e.db.GetContext(ctx, &info,
		`SELECT t.title, m.workflow_id, m.phase_id, p.phase_index
		 FROM tasks t
		 JOIN workflow_task_map m ON m.task_id = t.id
		 JOIN workflow_phases p ON p.id = m.phase_id
		 WHERE t.id = $1`,
		reviewTaskID)
	if err != nil {
		return fmt.Errorf("CreateWorkflowTestTask: lookup review task: %w", err)
	}

	// 2. Find the next phase (testing phase = current phase index + 1)
	var testPhase struct {
		ID         string `db:"id"`
		TotalTasks int    `db:"total_tasks"`
	}
	err = e.db.GetContext(ctx, &testPhase,
		`SELECT id, total_tasks FROM workflow_phases
		 WHERE workflow_id = $1 AND phase_index = $2`,
		info.WorkflowID, info.PhaseIndex+1)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("[workflow] no testing phase found for review task %s (workflow %s)", reviewTaskID, info.WorkflowID)
			return nil // no testing phase = nothing to do
		}
		return fmt.Errorf("CreateWorkflowTestTask: lookup testing phase: %w", err)
	}

	// 3. Get workflow description for task context
	var wfDesc string
	e.db.GetContext(ctx, &wfDesc,
		`SELECT COALESCE(description,'') FROM workflows WHERE id=$1`, info.WorkflowID)

	// 4. Build test task description
	desc := fmt.Sprintf("## Feature / Requirement\n%s\n\n## Testing Instructions\nValidate implementation for task %s (%s).\n- Run build and verify it passes\n- Check for regressions\n- Verify spec compliance", wfDesc, reviewTaskID, info.Title)

	// 5. Create test task via createTask (projectID from workflow)
	var projectID string
	e.db.GetContext(ctx, &projectID,
		`SELECT project_id FROM workflows WHERE id=$1`, info.WorkflowID)
	testTaskID, err := e.createTask(ctx, "Test: "+info.Title, "test", projectID, "available", desc)
	if err != nil {
		return fmt.Errorf("CreateWorkflowTestTask: create test task: %w", err)
	}

	// 6. Map to workflow and testing phase
	if err := e.addWorkflowMapping(ctx, testTaskID, info.WorkflowID, testPhase.ID); err != nil {
		return fmt.Errorf("CreateWorkflowTestTask: add mapping: %w", err)
	}

	// 7. Increment total_tasks on testing phase
	_, err = e.db.ExecContext(ctx,
		`UPDATE workflow_phases SET total_tasks = total_tasks + 1, updated_at = NOW() WHERE id = $1`,
		testPhase.ID)
	if err != nil {
		return fmt.Errorf("CreateWorkflowTestTask: update phase total_tasks: %w", err)
	}

	log.Printf("[workflow] test task %s created for review task %s in workflow %s", testTaskID, reviewTaskID, info.WorkflowID)
	return nil
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
			`SELECT phases FROM workflow_templates WHERE id=$1`, templateID)
		if err == nil && len(cfgBytes) > 0 {
			json.Unmarshal(cfgBytes, &phaseConfigs)
		}
	}
	if len(phaseConfigs) == 0 {
		// Default: single dev phase
		phaseConfigs = []PhaseConfig{{Type: PhaseTypeSingle, Name: "Development"}}
	}

	_, err = tx.Exec(
		`INSERT INTO workflows (id, template_id, name, project_id, total_phases, status, description)
		 VALUES ($1, $2, $3, $4, $5, 'active', $6)`,
		wfID, templateID, name, projectID, len(phaseConfigs), description)
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
			`INSERT INTO workflow_phases (id, workflow_id, phase_name, phase_index, phase_type, task_type, status, config)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			phaseID, wfID, pc.Name, i, pc.Type, pc.TaskType, status, []byte("{}"))
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

// CheckAndAdvancePhase checks if all tasks in the current phase are done,
// marks the phase completed, and advances to the next phase via advanceWorkflow.
func (e *Engine) CheckAndAdvancePhase(taskID string) error {
	ctx := context.Background()

	var phaseID, wfID string
	err := e.db.GetContext(ctx, &phaseID,
		`SELECT phase_id FROM workflow_task_map WHERE task_id = $1`, taskID)
	if err != nil {
		return nil
	}
	err = e.db.GetContext(ctx, &wfID,
		`SELECT workflow_id FROM workflow_phases WHERE id = $1`, phaseID)
	if err != nil {
		return nil
	}

	var totalTasks, completedTasks int
	e.db.GetContext(ctx, &totalTasks,
		`SELECT COALESCE(total_tasks,0) FROM workflow_phases WHERE id=$1`, phaseID)
	e.db.GetContext(ctx, &completedTasks,
		`SELECT COALESCE(completed_tasks,0) FROM workflow_phases WHERE id=$1`, phaseID)

	if completedTasks >= totalTasks && totalTasks > 0 {
		e.db.ExecContext(ctx,
			`UPDATE workflow_phases SET status=$1,updated_at=NOW() WHERE id=$2`,
			PhaseCompleted, phaseID)

		// Auto-queue follow-up tasks for per_dev / review phases in the next phase
		e.autoQueueNextPhaseTasks(ctx, wfID, phaseID)

		return e.advanceWorkflow(wfID)
	}
	return nil
}

// autoQueueNextPhaseTasks creates follow-up (e.g. review) tasks for the next phase
// when the current phase completes. It is called from CheckAndAdvancePhase so that
// review tasks are queued as dev tasks finish (rather than all at once when the
// phase advances). It avoids duplicating work if createPerDevPhaseTasks also runs.
func (e *Engine) autoQueueNextPhaseTasks(ctx context.Context, wfID, phaseID string) {
	var next Phase
	err := e.db.GetContext(ctx, &next,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
		        total_tasks, completed_tasks, failed_tasks, config, description, created_at, updated_at
		 FROM workflow_phases
		 WHERE workflow_id=$1 AND status=$2
		 ORDER BY phase_index ASC LIMIT 1`,
		wfID, PhasePending)
	if err != nil {
		return // no next phase
	}

	// Only queue for per_dev and review-type phases
	if next.PhaseType != PhaseTypePerDev && next.PhaseType != "review" {
		return
	}

	// Get current (just-completed) phase index and type
	var current Phase
	if err := e.db.GetContext(ctx, &current,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
		        total_tasks, completed_tasks, failed_tasks, config, description, created_at, updated_at
		 FROM workflow_phases WHERE id=$1`, phaseID); err != nil {
		return
	}

	// Get all tasks from the current (just-completed) phase
	var completedDevTasks []struct {
		ID    string `db:"id"`
		Title string `db:"title"`
	}
	err = e.db.SelectContext(ctx, &completedDevTasks,
		`SELECT t.id, t.title
		 FROM tasks t
		 JOIN workflow_task_map m ON m.task_id = t.id
		 WHERE m.phase_id = $1 AND t.status IN ('done','deployed')`,
		phaseID)
	if err != nil || len(completedDevTasks) == 0 {
		return
	}

	projectID := ""
	e.db.GetContext(ctx, &projectID,
		`SELECT project_id FROM workflows WHERE id=$1`, wfID)

	for _, dev := range completedDevTasks {
		title := fmt.Sprintf("Code Review: %s", dev.Title)
		desc := fmt.Sprintf(
			"Review code changes from task %s: %s. Check build, code quality, no regressions.",
			dev.ID, dev.Title)
		reviewTaskID, err := e.createTask(ctx, title, "review", projectID, "available", desc)
		if err != nil {
			log.Printf("[workflow] autoQueueNextPhaseTasks: create review task error: %v", err)
			continue
		}
		if err := e.addWorkflowMapping(ctx, reviewTaskID, wfID, next.ID); err != nil {
			log.Printf("[workflow] autoQueueNextPhaseTasks: add mapping error: %v", err)
			continue
		}
		log.Printf("[workflow] auto-queued review task %s for dev task %s (%s)", reviewTaskID, dev.ID, wfID)
	}

	// Update next phase total_tasks count to reflect auto-queued tasks
	e.db.ExecContext(ctx,
		`UPDATE workflow_phases SET total_tasks=total_tasks+$1,updated_at=NOW() WHERE id=$2`,
		len(completedDevTasks), next.ID)
}

// ApproveGate advances a gate phase that is waiting for approval.
func (e *Engine) ApproveGate(workflowID, note string) error {
	ctx := context.Background()

	var gate Phase
	err := e.db.GetContext(ctx, &gate,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status,
		        total_tasks, completed_tasks, failed_tasks, config, description, created_at, updated_at
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
		        total_tasks, completed_tasks, failed_tasks, config, description, created_at, updated_at
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

// CreateTemplate inserts a new workflow template.
func CreateTemplate(db *sqlx.DB, name, description string, phases []PhaseConfig) (*Template, error) {
	phasesJSON, err := json.Marshal(phases)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	_, err = db.Exec(
		`INSERT INTO workflow_templates (id, name, description, phases, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, name, description, phasesJSON, now, now)
	if err != nil {
		return nil, err
	}
	return &Template{
		ID: id, Name: name, Description: description,
		Phases: phases, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// ListTemplates returns all workflow templates.
func ListTemplates(db *sqlx.DB) ([]Template, error) {
	var templates []Template
	err := db.Select(&templates,
		`SELECT id, name, description, created_at, updated_at FROM workflow_templates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	if templates == nil {
		templates = []Template{}
	}
	return templates, nil
}
