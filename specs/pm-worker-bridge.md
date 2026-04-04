# PM Worker Bridge — Adaptive Review & Decision Engine

## Problem
PM operates outside the workflow. When gates need PM decisions, the workflow blocks.
PM can't review work, reject bad output, or send tasks back for redo.

## Solution
One cron job acts as the PM worker bridge. It polls for PM tasks, reads context,
makes informed decisions, and submits verdicts via the AgentHub API.

## PM Worker Responsibilities

### 1. Spec Review (Phase 2)
When a `pm_spec_review` task is available:
- Read the design spec from the workflow
- Check: does it match the plan? Is it detailed enough? Are acceptance criteria clear?
- Decision:
  - PASS → workflow advances to human gate (Phase 3)
  - FAIL → create feedback task for Team Leader, set workflow phase back to Design

### 2. Quality Review (Phase 8)
When a `pm_review` task is available:
- Read all completed tasks from the workflow (dev, review, test results)
- Read the spec (what was requested)
- Check: does the output match the spec? Are all tasks passing?
- Decision:
  - PASS → workflow advances to human gate (Phase 9) with PM summary
  - FAIL → identify which tasks need redo, submit verdict with target phase

### 3. Escalation Handling
When a task is escalated to PM:
- Read the task, its history, and retry count
- Decision:
  - Retry with different approach → requeue with updated description
  - Accept partial → mark as done, note the gap
  - Unblock by updating spec → send back to design phase
  - Give up → mark as failed, notify Tuyên

## API Changes Needed

### 1. CheckGateDecision: add `reason` and `target_phase` params
```go
func (e *Engine) CheckGateDecision(taskID string, verdict string, reason string, targetPhaseIndex *int) error
```
- `verdict`: "pass" or "fail"
- `reason`: why PM made this decision (stored in task review_issues)
- `targetPhaseIndex`: which phase to requeue (nil = previous phase, current behavior)

### 2. Task status update: accept `reason` field
```go
type CompleteRequest struct {
    Status  string `json:"status"`
    Verdict string `json:"verdict"`
    Reason  string `json:"reason"`          // NEW
    TargetPhase *int `json:"target_phase"`  // NEW: which phase to requeue on fail
}
```

### 3. Gate decision task creation: include context
When engine creates a gate_decision task for PM, include:
- Spec content (or link to spec)
- Summary of previous phase results
- What decision is needed
- Acceptance criteria

## Watcher Cron Upgrade

The single watcher cron handles ALL PM work:

```
Every 5 minutes:
1. Run watcher.sh script (checks DB)
2. If PM_TASK found:
   a. Read full task context from API
   b. Read related files (spec, code changes)
   c. Evaluate against criteria
   d. Submit verdict via API with reason
3. If HUMAN_GATE found:
   a. Read workflow context
   b. Send Tuyên a message with PM summary + dashboard link
4. If FAILED_TASK found:
   a. Alert PM
5. If nothing → NO_REPLY
```

## Verdict Protocol
PM bridge output format for gate_decision tasks:
```
VERDICT: PASS|FAIL
REASON: <why>
TARGET_PHASE: <optional phase index to requeue>
```

## Flow Example

```
Phase 4: Dev (3 tasks)
  → dev1 completes task 1
  → dev2 completes task 2, 3
  → all done → advance to Phase 5

Phase 5: Code Review (3 review tasks auto-queued)
  → reviewer reviews each → 2 pass, 1 fail
  → failed task → back to dev with retry
  → dev fixes → reviewer passes
  → all done → advance to Phase 6

Phase 6: Testing (3 test tasks auto-queued)
  → tester validates each → all pass
  → advance to Phase 7

Phase 7: Quality Gate (decision)
  → engine checks: all reviews pass, all tests pass → PASS
  → advance to Phase 8

Phase 8: PM Review (gate_decision task created for PM)
  → watcher cron finds PM task
  → reads spec, all task results, review verdicts, test verdicts
  → PM evaluates: output matches spec
  → PM submits: VERDICT: PASS, REASON: "All 3 tasks match spec, reviews and tests pass"
  → workflow advances to Phase 9

Phase 9: Owner Deploy (human_gate)
  → watcher sends Tuyên: "PM approved deploy. Summary: ..."
  → Tuyên approves on dashboard
  → advance to Phase 10
```

## Implementation Tasks

### Task 1: Engine API upgrades
- CheckGateDecision: add reason + targetPhaseIndex params
- CompleteTask: accept reason + target_phase fields
- Gate decision task creation: include context (spec content, prev phase summary)
- Store PM decision reason in task review_issues field

### Task 2: PM watcher cron prompt
- Upgrade watcher.sh to detect PM tasks with full context
- Write prompt that instructs the cron agent to:
  - Read task context from API
  - Read spec and related files
  - Make a decision
  - Submit verdict via curl to API
- Include PM decision criteria in prompt

### Task 3: Test full adaptive loop
- Create a test workflow
- Dev task with intentional bug → PM rejects → dev fixes → PM approves
- Verify target_phase routing works
- Verify PM summary reaches Tuyên at human gate
