# Design Spec: Workflow Engine Activation

**Goal:** Make workflows go end-to-end — create workflow → agents pick up tasks → complete → advance phases → deploy

**Branch:** `feature/workflow-activation`
**Project:** `/root/.openclaw/workspace-pm/projects/agenthub/`

---

## Issues to Fix (from Reviewer)

### Critical
1. **migrations.sql lost all prior migrations** — restore 001-011 or keep separate files
2. **Phase creation handlers not called from StartWorkflow** — workflow creates zero tasks
3. **pending_deps double-increment in per_dev** — tasks never unblock
4. **Trigger deletes deps before collecting unblocked IDs** — LISTEN/NOTIFY always empty
5. **createTask project_id resolution broken** — circular subquery returns NULL

### Major
6. **API responses missing required fields** per spec §6
7. **Dashboard `/api/dashboard/tasks` returns empty** even when tasks exist
8. **No default user** — can't access protected endpoints
9. **No delete workflow endpoint**

---

## Task Breakdown

| # | Task | Assigned | Complexity | Dependencies |
|---|------|----------|------------|--------------|
| T1 | Fix migrations + phase creation bugs | dev1 | complex | none |
| T2 | Add seed user + delete workflow endpoint | dev1 | simple | none |
| T3 | Fix dashboard tasks query | dev2 | simple | none |
| T4 | Worker bridge: poll/claim/complete loop | dev2 | medium | T1 |
| T5 | PM: Start workflow from Telegram | PM | medium | T1-T4 |
| T6 | PM: Pipeline monitor update | PM | simple | T1-T4 |

---

## T1: Fix Migrations + Phase Creation Bugs

### Files to Modify
- `internal/db/migrations.sql` — restore or separate migrations
- `internal/workflow/engine.go` — call phase handlers in StartWorkflow
- `internal/workflow/phases.go` — fix pending_deps double-increment, project_id resolution
- `internal/db/migrations/012_workflow_engine.sql` — fix trigger order

### Changes

**1. Migrations**
- Move workflow migrations to `migrations/012_workflow_engine.sql`
- Keep `migrations.sql` as rollup OR document that individual files are source of truth

**2. engine.go — StartWorkflow**
After creating phases, activate first phase:
```go
// After inserting all phases
if len(phaseConfigs) > 0 {
    firstPhase := createdPhases[0]
    if err := e.activatePhase(wf.ID, &firstPhase, projectID); err != nil {
        return nil, fmt.Errorf("activate first phase: %w", err)
    }
}
```

**3. phases.go — Fix pending_deps**
In `createPerDevPhaseTasks`, remove manual `pending_deps=1` UPDATE:
```go
// REMOVE this:
// UPDATE tasks SET pending_deps=1, status='blocked'...

// Keep only:
INSERT INTO task_dependencies (task_id, depends_on_id) VALUES ($1, $2)
// Let trigger handle pending_deps
```

**4. phases.go — Fix project_id**
Pass projectID from workflow context:
```go
func (e *Engine) createTask(ctx context.Context, title, taskType, projectID, status string, config json.RawMessage) (string, error) {
    // Use projectID directly, not subquery
    err := e.db.GetContext(ctx, &id,
        `INSERT INTO tasks (title, task_type, status, project_id)
         VALUES ($1, $2, $3, NULLIF($4, ''))
         RETURNING id`,
        title, taskType, status, projectID)
}
```

**5. migrations/012_workflow_engine.sql — Fix trigger**
```sql
CREATE OR REPLACE FUNCTION fn_task_completed()
RETURNS TRIGGER AS $$
DECLARE
    unblocked_ids TEXT[];
BEGIN
    -- Collect BEFORE deleting
    SELECT ARRAY_AGG(task_id) INTO unblocked_ids
    FROM task_dependencies WHERE depends_on_id = NEW.id;
    
    -- Delete resolved deps
    DELETE FROM task_dependencies WHERE depends_on_id = NEW.id;
    
    -- Decrement and notify
    UPDATE tasks SET pending_deps = GREATEST(pending_deps - 1, 0)
    WHERE id = ANY(unblocked_ids);
    
    PERFORM pg_notify('task_unblocked', '');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

---

## T2: Seed User + Delete Workflow Endpoint

### Files to Modify
- `internal/auth/seed.go` (new) — seed default user
- `internal/workflow/handler.go` — add DELETE endpoint
- `cmd/server/main.go` — call seed on startup

### Changes

**1. auth/seed.go**
```go
func SeedDefaultUser(db *sqlx.DB) error {
    _, err := db.Exec(`
        INSERT INTO users (id, username, email, password, role)
        VALUES (gen_random_uuid(), 'admin', 'admin@local', '$2a$10$...', 'admin')
        ON CONFLICT (username) DO NOTHING
    `)
    return err
}
```

**2. handler.go — DeleteWorkflow**
```go
func (h *Handler) DeleteWorkflow(c *gin.Context) {
    id := c.Param("id")
    // Only allow deleting cancelled/completed workflows
    var status string
    h.db.Get(&status, "SELECT status FROM workflows WHERE id = $1", id)
    if status == "active" || status == "paused" {
        c.JSON(400, gin.H{"error": "cannot delete active workflow"})
        return
    }
    // Delete task mappings, phases, then workflow
    tx := h.db.MustBegin()
    tx.Exec("DELETE FROM workflow_task_map WHERE workflow_id = $1", id)
    tx.Exec("DELETE FROM workflow_phases WHERE workflow_id = $1", id)
    tx.Exec("DELETE FROM workflows WHERE id = $1", id)
    tx.Commit()
    c.JSON(200, gin.H{"deleted": id})
}
```

**3. Route**
```go
g.DELETE("/:id", h.DeleteWorkflow)
```

---

## T3: Fix Dashboard Tasks Query

### Files to Modify
- `internal/dashboard/handler.go`

### Changes

Check `Tasks()` handler — likely missing join or filter:
```go
func (h *Handler) Tasks(c *gin.Context) {
    var tasks []Task
    // Make sure this query is correct
    err := h.db.Select(&tasks, `
        SELECT t.id, t.title, t.status, t.task_type, t.project_id,
               t.assigned_to, t.created_at, t.updated_at
        FROM tasks t
        ORDER BY t.created_at DESC
        LIMIT 100
    `)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"tasks": tasks})
}
```

---

## T4: Worker Bridge — Poll/Claim/Complete Loop

### Files to Modify
- `cmd/workerbridge/main.go` (already exists, extend)

### Changes

The worker bridge already exists. Add workflow-aware polling:

```go
// Poll for available tasks filtered by task_type
resp, err := http.Get(fmt.Sprintf("%s/api/agent/tasks/queue?task_type=%s", apiURL, role))

// Claim first task
claim := http.Post(fmt.Sprintf("%s/api/agent/tasks/%s/claim", apiURL, task.ID), ...)

// Execute via openclaw agent CLI
cmd := exec.Command("openclaw", "agent", "--agent", agentID, "--json")
// Pass task description as prompt

// Report result
if success {
    http.Post(fmt.Sprintf("%s/api/agent/tasks/%s/complete", apiURL, task.ID), 
        gin.H{"status": "done", "notes": "completed"})
} else {
    http.Post(fmt.Sprintf("%s/api/agent/tasks/%s/complete", apiURL, task.ID),
        gin.H{"status": "failed", "notes": errMsg})
}
```

### Gate Notification (T4b)

When workflow reaches gate phase, notify PM:

```go
// In advanceWorkflow, when entering gate:
if IsGatePhase(next.PhaseType) {
    // Webhook to OpenClaw cron wake
    http.Post("http://127.0.0.1:9000/api/cron/wake", gin.H{
        "text": fmt.Sprintf("🔒 Gate approval required: %s (workflow %s)", next.PhaseName, wfID),
        "mode": "now",
    })
}
```

---

## Execution Order

1. **T1** (dev1) — Fix core bugs, unblocks everything
2. **T2** (dev1) — Can run parallel with T1
3. **T3** (dev2) — Can run parallel with T1
4. **T4** (dev2) — After T1 merged
5. **T5, T6** (PM) — After T1-T4 verified

---

## Verification

After T1-T4:
1. Start workflow: `POST /api/workflows/start {"template_name":"default", "name":"Test"}`
2. Check dashboard shows tasks
3. Worker bridge picks up task → completes → phase advances
4. Gate phase triggers PM notification
5. PM approves → workflow continues

---

## Risks

| Risk | Mitigation |
|------|------------|
| Migration conflict | Test on fresh DB before merging |
| Trigger breaks existing tasks | Test with manual pending_deps updates |
| Worker bridge timeout | Increase TASK_TIMEOUT_SECONDS |
| Gate notification fails | Fallback to log + retry |
