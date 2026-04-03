package workflow

import (
	"log"
)

// SendGateNotification logs gate approval requirement.
// Gate notifications are handled by the PM Pipeline Monitor cron
// which polls for waiting_approval phases periodically.
func SendGateNotification(workflowID, phaseID, phaseName string) {
	log.Printf("[workflow] gate approval required: workflow=%s phase=%q", workflowID, phaseName)
}

func SendEscalationNotification(workflowID, taskID string) {
	log.Printf("[workflow] escalation notification: workflow=%s task=%s -- task has stalled or failed", workflowID, taskID)
}
