# DELETE /api/workflows/:id — Design Specification

> **Status:** Draft  
> **Date:** 2026-04-03  
> **Project:** `/root/.openclaw/workspace-pm/projects/agenthub/`  
> **Build:** `export PATH=$PATH:/usr/local/go/bin && go build ./...`

---

## 1. Overview

Add `DELETE /api/workflows/:id` endpoint with conditional soft/hard delete based on workflow state and admin-only access.

---

## 2. API Endpoint

```
DELETE /api/workflows/:id
Authorization: Bearer {jwt} (admin role required)
```

### Response: 200

```json
{
  "message": "Workflow deleted",
  "workflow_id": "uuid",
  "deleted_type": "hard",
  "summary": {
    "phases_removed": 8,
    "task_mappings_removed": 12,
    "tasks_released": 3,
    "dependencies_removed": 2
  }
}
```

### Response: 404

```json
{
  "error": "Workflow not found"
}
```

### Response: 403

```json
{
  "error": "Admin access required"
}
```

### Response: 409

```json
{
  "error": "Cannot delete workflow with active tasks. Cancel the workflow first.",
  "active_tasks": 3,
  "workflow_status": "active"
}
```

---

## 3. Design Decisions

### 3.1 Soft Delete vs Hard Delete

**Decision:** Use the workflow's existing `status` column for soft delete. No new `deleted_at` column needed.

| Workflow Status | Delete Behavior |
|-----------------|-----------------|
| `complete` | **Hard delete** — remove workflow, phases, task_map rows. Tasks stay (they're done). |
| `cancelled` | **Hard delete** — same as complete. |
| `paused` (gate waiting) | **Hard delete** — gate will never be approved. Release any claimed tasks. |
| `active` with active tasks | **Reject 409** — must cancel workflow first. |
| `active` with no active tasks | **Hard delete** — safe, nothing in-flight. |

### 3.2 Why Not True Soft Delete

- Workflows are transient (design → dev → test → done). No audit trail needed.
- `status = 'cancelled'` already serves as "this workflow didn't complete."
- Hard delete keeps the DB clean. Task rows are preserved (they may have value independently).

### 3.3 Task Handling on Delete

- **Tasks with status `done`/`deployed`/`failed`/`escalated`:** Leave untouched. Remove `workflow_task_map` entries only.
- **Tasks with status `available`:** Leave untouched (someone might manually claim them later).
- **Tasks with status `claimed`/`in_progress`:** Release them (set `assignee = NULL`, `status = 'available'`, increment `release_count`). Then remove mappings.

### 3.4 Dependency Cleanup

When tasks are released, their dependencies in `task_dependencies` are preserved (the tasks still exist and may be useful). Only `workflow_task_map` rows are deleted.

---

## 4. Implementation

### 4.1 Engine Method

File: `internal/workflow/engine.go`

```go
// DeleteWorkflow deletes a workflow. Hard-deletes completed/cancelled/paused workflows.
// Rejects active workflows that have in-flight tasks.
func (e *Engine) DeleteWorkflow(workflowID string) (*DeleteSummary, error) {
    // 1. Check workflow exists
    var wf Workflow
    if err := e.db.Get(&wf, `SELECT * FROM workflows WHERE id=$1`, workflowID); err != nil {
        if err == sql.ErrNoRows {
            return nil, &NotFoundError{Resource: "workflow", ID: workflowID}
        }
        return nil, err
    }

    // 2. Check for active tasks
    var activeTaskCount int
    e.db.Get(&activeTaskCount,
        `SELECT COUNT(*) FROM workflow_task_map m
         JOIN tasks t ON t.id = m.task_id
         WHERE m.workflow_id = $1 AND t.status IN ('claimed','in_progress')`,
        workflowID)

    if activeTaskCount > 0 {
        return nil, &ActiveTasksError{Count: activeTaskCount, Status: wf.Status}
    }

    // 3. Release any available/claimed tasks (edge case: claimed but not in_progress)
    var releasedCount int64
    result, err := e.db.Exec(
        `UPDATE tasks t
         SET assignee = NULL, status = 'available', release_count = release_count + 1, updated_at = NOW()
         FROM workflow_task_map m
         WHERE m.task_id = t.id AND m.workflow_id = $1
           AND t.status IN ('claimed','in_progress')`,
        workflowID)
    if err == nil {
        releasedCount, _ = result.RowsAffected()
    }

    // 4. Delete task mappings
    var mappingsRemoved int64
    result, err = e.db.Exec(
        `DELETE FROM workflow_task_map WHERE workflow_id = $1`, workflowID)
    if err == nil {
        mappingsRemoved, _ = result.RowsAffected()
    }

    // 5. Delete phases
    var phasesRemoved int64
    result, err = e.db.Exec(
        `DELETE FROM workflow_phases WHERE workflow_id = $1`, workflowID)
    if err == nil {
        phasesRemoved, _ = result.RowsAffected()
    }

    // 6. Delete workflow
    e.db.Exec(`DELETE FROM workflows WHERE id = $1`, workflowID)

    return &DeleteSummary{
        WorkflowID:           workflowID,
        DeletedType:          "hard",
        PhasesRemoved:        phasesRemoved,
        TaskMappingsRemoved:  mappingsRemoved,
        TasksReleased:        releasedCount,
    }, nil
}
```

### 4.2 Error Types

```go
type NotFoundError struct {
    Resource string
    ID       string
}
func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s %s not found", e.Resource, e.ID)
}

type ActiveTasksError struct {
    Count  int
    Status string
}
func (e *ActiveTasksError) Error() string {
    return fmt.Sprintf("cannot delete: %d active tasks (workflow status: %s)", e.Count, e.Status)
}
```

### 4.3 HTTP Handler

File: `internal/workflow/handler.go`

```go
func (h *Handler) DeleteWorkflow(c *gin.Context) {
    // Admin check (role from JWT middleware)
    role, _ := c.Get("userRole")
    if role != "admin" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
        return
    }

    id := c.Param("id")
    summary, err := h.engine.DeleteWorkflow(id)

    if err != nil {
        if errors.Is(err, sql.ErrNoRows) || isNotFound(err) {
            c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
            return
        }
        var activeErr *ActiveTasksError
        if errors.As(err, &activeErr) {
            c.JSON(http.StatusConflict, gin.H{
                "error":        "Cannot delete workflow with active tasks. Cancel the workflow first.",
                "active_tasks": activeErr.Count,
                "workflow_status": activeErr.Status,
            })
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":      "Workflow deleted",
        "workflow_id":  summary.WorkflowID,
        "deleted_type": summary.DeletedType,
        "summary": map[string]interface{}{
            "phases_removed":        summary.PhasesRemoved,
            "task_mappings_removed": summary.TaskMappingsRemoved,
            "tasks_released":        summary.TasksReleased,
        },
    })
}
```

### 4.4 Route Registration

File: `internal/workflow/handler.go` — add to `RegisterRoutes`:

```go
g.DELETE("/:id", h.DeleteWorkflow)
```

Note: This route is under `/api/workflows` which should be behind user JWT middleware (not agent middleware). The admin role check is inside the handler.

### 4.5 Middleware Setup

File: `cmd/server/main.go` — ensure the workflow group uses user JWT middleware:

```go
// Workflow routes (user auth required, admin-only for delete)
workflowGroup := r.Group("/api/workflows")
workflowGroup.Use(authMiddleware)  // JWT middleware
workflow.RegisterRoutes(workflowGroup, database, workflowEngine)
```

Currently `RegisterRoutes` creates its own group internally. Need to change the function signature to accept a group instead of a router, or add the middleware inside `RegisterRoutes`. Simplest fix:

```go
func RegisterRoutes(r gin.IRouter, db *sqlx.DB, engine *Engine, authMiddleware gin.HandlerFunc) {
    h := NewHandler(db, engine)
    g := r.Group("")
    // DELETE requires admin
    g.DELETE("/:id", authMiddleware, adminOnly(h.DeleteWorkflow))
    // Other routes (public or user-level)
    g.POST("/start", h.StartWorkflow)
    // ... etc
}
```

Or simpler — keep `RegisterRoutes` as-is but register DELETE separately in `main.go`:

```go
// In cmd/server/main.go:
workflowGroup := r.Group("/api/workflows", authMiddleware)
workflowGroup.DELETE("/:id", workflowHandler.DeleteWorkflow)
```

---

## 5. Data Model Changes

**None.** No schema changes needed. Uses existing tables:
- `workflows` — delete row
- `workflow_phases` — delete rows (CASCADE would handle this but explicit is clearer)
- `workflow_task_map` — delete rows (CASCADE would handle this too)
- `tasks` — only UPDATE status, never DELETE

---

## 6. Files Summary

| File | Change |
|------|--------|
| `internal/workflow/engine.go` | Add `DeleteWorkflow()`, `DeleteSummary` struct, `NotFoundError`, `ActiveTasksError` |
| `internal/workflow/handler.go` | Add `DeleteWorkflow` handler, register `DELETE /:id` route |
| `cmd/server/main.go` | Ensure DELETE route has JWT middleware (admin check is in handler) |

---

## 7. Acceptance Criteria

- [ ] `DELETE /api/workflows/:id` with admin JWT on completed workflow returns 200 with summary
- [ ] Workflow row, all phases, all task_map entries are deleted
- [ ] Done/deployed tasks remain in `tasks` table (not deleted)
- [ ] Active workflow with in-flight tasks returns 409 with `active_tasks` count
- [ ] Non-existent workflow returns 404
- [ ] Non-admin user gets 403
- [ ] Unauthenticated request gets 401 (from JWT middleware)
- [ ] Paused workflow (at gate) can be hard-deleted
- [ ] Claimed/in_progress tasks are released (status → available, assignee → NULL)
- [ ] `go build ./...` passes

---

## 8. Edge Cases

| Case | Behavior |
|------|----------|
| Workflow already deleted (double delete) | 404 (idempotent) |
| Workflow has 0 phases (corrupted data) | Hard delete succeeds, summary shows 0 phases |
| Task belongs to multiple workflows | Impossible — `workflow_task_map.task_id` has UNIQUE constraint |
| Agent tries to delete | 403 (agent middleware vs user middleware — agent token won't pass JWT check) |
| Transaction safety | Wrap delete operations in a DB transaction |

### Transaction Wrapping

```go
func (e *Engine) DeleteWorkflow(workflowID string) (*DeleteSummary, error) {
    tx, err := e.db.Beginx()
    if err != nil {
        return nil, err
    }
    defer tx.Rollback()

    // ... all delete operations use tx.ExecContext instead of e.db.Exec ...

    if err := tx.Commit(); err != nil {
        return nil, err
    }
    return summary, nil
}
```

---

*Last updated: 2026-04-03*
