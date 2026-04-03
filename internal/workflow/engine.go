package workflow

import (
	"database/sql"
	"log"

	"github.com/jmoiron/sqlx"
)

// Phase status constants
const (
	PhasePending    = "pending"
	PhaseActive     = "active"
	PhaseComplete   = "complete"
	PhaseWaitingApproval = "waiting_approval"
)

// Phase types
const (
	PhaseTypeNormal = "normal"
	PhaseTypeGate   = "gate"
)

// Phase represents a workflow phase
type Phase struct {
	ID         string `db:"id"`
	WorkflowID string `db:"workflow_id"`
	PhaseName  string `db:"phase_name"`
	PhaseIndex int    `db:"phase_index"`
	PhaseType  string `db:"phase_type"`
	Status     string `db:"status"`
}

// IsGatePhase returns true if the phase type is a gate (requires approval)
func IsGatePhase(phaseType string) bool {
	return phaseType == PhaseTypeGate
}

// AdvanceWorkflow moves the workflow to the next phase if the current phase is complete.
// If the next phase is a gate, it sets status to waiting_approval and sends a notification.
func AdvanceWorkflow(db *sqlx.DB, workflowID string) error {
	// Get current active phase
	var current Phase
	err := db.Get(&current,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status
		 FROM workflow_phases
		 WHERE workflow_id = $1 AND status = $2
		 ORDER BY phase_index ASC LIMIT 1`,
		workflowID, PhaseActive)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[workflow] no active phase for workflow %s", workflowID)
			return nil
		}
		return err
	}

	// Get next phase
	var next Phase
	err = db.Get(&next,
		`SELECT id, workflow_id, phase_name, phase_index, phase_type, status
		 FROM workflow_phases
		 WHERE workflow_id = $1 AND phase_index > $2
		 ORDER BY phase_index ASC LIMIT 1`,
		workflowID, current.PhaseIndex)
	if err != nil {
		if err == sql.ErrNoRows {
			// No more phases - workflow complete
			log.Printf("[workflow] workflow %s complete - no more phases", workflowID)
			db.Exec("UPDATE workflows SET status = 'complete', updated_at = NOW() WHERE id = $1", workflowID)
			return nil
		}
		return err
	}

	// Mark current phase as complete
	_, err = db.Exec(
		`UPDATE workflow_phases SET status = $1, updated_at = NOW() WHERE id = $2`,
		PhaseComplete, current.ID)
	if err != nil {
		return err
	}

	// Check if next phase is a gate
	if IsGatePhase(next.PhaseType) {
		// Set next phase to waiting_approval
		_, err = db.Exec(
			`UPDATE workflow_phases SET status = $1, updated_at = NOW() WHERE id = $2`,
			PhaseWaitingApproval, next.ID)
		if err != nil {
			return err
		}

		// Send gate notification to OpenClaw
		SendGateNotification(workflowID, next.ID, next.PhaseName)

		log.Printf("[workflow] workflow %s waiting at gate: %s", workflowID, next.PhaseName)
		return nil
	}

	// Activate next phase
	_, err = db.Exec(
		`UPDATE workflow_phases SET status = $1, updated_at = NOW() WHERE id = $2`,
		PhaseActive, next.ID)
	if err != nil {
		return err
	}

	log.Printf("[workflow] workflow %s advanced to phase: %s", workflowID, next.PhaseName)
	return nil
}

// ApproveGate advances a gate phase that is waiting for approval.
func ApproveGate(db *sqlx.DB, workflowID, phaseID string) error {
	// Update phase status to active (briefly) then complete
	_, err := db.Exec(
		`UPDATE workflow_phases SET status = $1, updated_at = NOW() WHERE id = $2 AND workflow_id = $3`,
		PhaseActive, phaseID, workflowID)
	if err != nil {
		return err
	}

	// Advance to next phase
	return AdvanceWorkflow(db, workflowID)
}
