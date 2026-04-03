package workflow

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// SendGateNotification sends a gate approval notification via webhook to OpenClaw.
func SendGateNotification(workflowID, phaseID, phaseName string) {
	payload := map[string]interface{}{
		"text": "Gate approval needed: " + phaseName + " (workflow " + workflowID + ")",
		"mode": "now",
	}
	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post("http://127.0.0.1:9000/api/cron/wake", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[workflow] gate notification failed: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("[workflow] gate notification sent: workflow=%s phase=%s", workflowID, phaseName)
}

// SendEscalationNotification sends an escalation alert (stub for future notification channels).
func SendEscalationNotification(workflowID, taskID string) {
	log.Printf("[workflow] escalation notification: workflow=%s task=%s — task has stalled or failed", workflowID, taskID)
}
