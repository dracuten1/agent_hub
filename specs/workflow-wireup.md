# AgentHub Workflow Engine Wire-Up — Design Specification

> **Status:** Draft  
> **Author:** Team Leader  
> **Date:** 2026-04-03  
> **Project:** `/root/.openclaw/workspace-pm/projects/agenthub/`  
> **Branch:** `feature/workflow-activation`  
> **Build:** `export PATH=$PATH:/usr/local/go/bin && go build ./...`

---

## 1. Current State Analysis

### What Exists
- **Database:** Migrations 001-010 exist (users, agents, projects, features, tasks, task_events, comments, indexes, task_type, stale check). **No workflow tables yet.**
- **Task handler:** Full CRUD + transitions + claim/complete/review/test + agent routes
- **Agent handler:** Registration, queue (`GetQueue`), heartbeat, health
- **Dashboard handler:** Summary, agents list, tasks list, HTML page (all public routes)
- **Worker bridge:** `cmd/workerbridge/main.go` + `internal/workerbridge/{bridge,agent,config}.go` — polls queue, claims, delegates to `openclaw agent`, reports
- **Auth:** JWT middleware, admin user seeding on startup

### What's Missing (Blocking Issues)

| # | Issue | Root Cause | Impact |
|---|-------|------------|--------|
| 1 | Dashboard `/api/dashboard/tasks` returns empty | Dashboard queries are correct — but **no tasks exist** because workflow engine not wired | Can't verify system works |
| 2 | No default user credentials | Admin user seeded but password from env `AGENTHUB_ADMIN_PASS` — may not be set | Can't login to test protected routes |
| 3 | No delete workflow endpoint | Workflow package doesn't exist yet | Can't clean up test workflows |
| 4 | Workflow engine not implemented | No `internal/workflow/` package, no migrations 011-012 | Tasks never created, phases never advance |
| 5 | Task type filter rejects workflow types | `GetQueue` only maps `dev/review/test` → falls to `general` for `plan/deploy/task-breakdown/verify` | Workers can't pick up workflow tasks |
| 6 | Gate notifications missing | No Telegram integration | PM never notified to approve gates |

### Previous Reviewer Findings (MUST ADDRESS)
1. ~~migrations.sql lost all prior migrations~~ — **FIXED**, 001-010 exist
2. Phase creation handlers not called from StartWorkflow — **workflow package doesn't exist yet**
3. pending_deps double-increment — **workflow tables don't exist yet**
4. Trigger deletes deps before collecting unblocked IDs — **triggers don't exist yet**
5. createTask project_id circular query — **workflow package doesn't exist yet**
6. API responses missing fields — **workflow package doesn't exist yet**

---

## 2. Scope Definition

**T1-T4 only.** T5 (Worker Bridge) and T6 (Gate Notifications) are PM-only.

| Task | Description | Priority |
|------|-------------|----------|
| **T1** | Fix dashboard + add seed task + verify tasks visible | Critical |
| **T2** | Add workflow engine core (types, migrations, basic API) | Critical |
| **T3** | Wire phase creation + auto-advance on task completion | Critical |
| **T4** | Add DELETE workflow + fix task type filter for workflow types | Major |

---

## 3. Task Breakdown

### Task T1: Fix Dashboard + Verify Tasks Visible

**Assignee:** dev1  
**Priority:** Critical  
**Estimated time:** 30 min

**Problem:** Dashboard shows empty because no tasks exist. Need to verify the full stack works.

**Subtasks:**

1. **Add a seed task on startup** (if no tasks exist):
   - File: `cmd/server/main.go`
   - After admin user seeding, check `SELECT COUNT(*) FROM tasks`
   - If 0, insert a test task: `{title: "Test task", status: "available", task_type: "dev", priority: "medium"}`
   - This proves the dashboard can display tasks

2. **Ensure admin password is set:**
   - File: `cmd/server/main.go`
   - If `AGENTHUB_ADMIN_PASS` env is empty, log a warning with the default: `"Admin password not set — using default 'admin123'. Set AGENTHUB_ADMIN_PASS for production."`
   - This makes it clear how to login

3. **Add debug logging to dashboard handlers:**
   - File: `internal/dashboard/handler.go`
   - In `Summary()`, `Agents()`, `Tasks()`: log query results count
   - Helps diagnose empty responses

**Files to modify:**
- `cmd/server/main.go` (seed task + password warning)
- `internal/dashboard/handler.go` (debug logging)

**Acceptance criteria:**
- `GET /api/dashboard/summary` returns `recent_tasks: [{id, title, ...}]` with at least one task
- `GET /api/dashboard/tasks` returns non-empty `recent_tasks`
- Server logs show "Admin password not set — using default 'admin123'" if env missing
- `go build ./...` passes

---

### Task T2: Add Workflow Engine Core

**Assignee:** dev2  
**Priority:** Critical  
**Estimated time:** 90 min

**Goal:** Create the minimal workflow engine: migrations, types, basic CRUD API.

**Subtasks:**

1. **Add migrations 011-012:**
   - File: `internal/db/migrations.sql` — append after line 124
   - Migration 011: Add `pending_deps` column to tasks, `task_dependencies` table, triggers
   - Migration 012: Add `workflow_templates`, `workflows`, `workflow_phases`, `workflow_task_map` tables + indexes
   - **CRITICAL:** Use `IF NOT EXISTS` for all CREATEs, `ADD COLUMN IF NOT EXISTS` for ALTERs
   - **Address reviewer finding #4:** In `fn_task_completed` trigger, collect unblocked IDs **before** deleting dependency rows:
     ```sql
     -- 1. Update tasks and collect unblocked IDs in one pass
     UPDATE tasks t SET pending_deps = GREATEST(pending_deps - 1, 0), ...
     WHERE t.id IN (SELECT task_id FROM task_dependencies WHERE depends_on_id = NEW.id)
     RETURNING t.id INTO unblocked_ids;
     
     -- 2. Delete resolved deps
     DELETE FROM task_dependencies WHERE depends_on_id = NEW.id;
     
     -- 3. Notify with correct IDs
     PERFORM pg_notify('task_unblocked', array_to_string(unblocked_ids, ','));
     ```

2. **Create workflow package:**
   - Files: `internal/workflow/types.go`, `engine.go`, `handler.go`
   
   **types.go:**
   ```go
   type Workflow struct {
       ID           string `json:"id" db:"id"`
       TemplateID   string `json:"template_id" db:"template_id"`
       ProjectID    string `json:"project_id" db:"project_id"`
       Name         string `json:"name" db:"name"`
       Status       string `json:"status" db:"status"` // active, paused, completed, failed
       CurrentPhase int    `json:"current_phase" db:"current_phase"`
       TotalPhases  int    `json:"total_phases" db:"total_phases"`
       CreatedAt    string `json:"created_at" db:"created_at"`
   }
   
   type WorkflowPhase struct {
       ID              string          `json:"id" db:"id"`
       WorkflowID      string          `json:"workflow_id" db:"workflow_id"`
       PhaseIndex      int             `json:"phase_index" db:"phase_index"`
       PhaseType       string          `json:"phase_type" db:"phase_type"` // single, multi, per_dev, gate, decision
       PhaseName       string          `json:"phase_name" db:"phase_name"`
       TaskType        string          `json:"task_type" db:"task_type"`
       Status          string          `json:"status" db:"status"` // pending, active, completed
       TotalTasks      int             `json:"total_tasks" db:"total_tasks"`
       CompletedTasks  int             `json:"completed_tasks" db:"completed_tasks"`
       Config          json.RawMessage `json:"config" db:"config"`
   }
   ```

   **engine.go:**
   ```go
   func NewEngine(db *sqlx.DB) *Engine
   func (e *Engine) StartWorkflow(templateName, name, projectID string) (*Workflow, error)
   func (e *Engine) GetWorkflow(id string) (*Workflow, []WorkflowPhase, error)
   func (e *Engine) ListWorkflows(status string, limit, offset int) ([]Workflow, int, error)
   func (e *Engine) DeleteWorkflow(id string) error
   ```

   **handler.go:**
   - `POST /api/workflows/start` — start from template
   - `GET /api/workflows/:id` — get workflow + phases
   - `GET /api/workflows` — list workflows
   - `DELETE /api/workflows/:id` — delete workflow (cascades to phases and task_map)

3. **Seed default template on startup:**
   - File: `cmd/server/main.go`
   - After migrations, check if `workflow_templates` has a row with `name = 'default'`
   - If not, insert the default 8-phase template (see §5)

4. **Register workflow routes:**
   - File: `cmd/server/main.go`
   - Add `workflowHandler.RegisterUserRoutes(user)` after task handler registration

**Files to create:**
- `internal/workflow/types.go`
- `internal/workflow/engine.go`
- `internal/workflow/handler.go`

**Files to modify:**
- `internal/db/migrations.sql` (append migrations)
- `cmd/server/main.go` (seed template, register routes)

**Acceptance criteria:**
- `go build ./...` passes
- Server starts without errors
- `SELECT * FROM workflow_templates WHERE name = 'default'` returns 1 row
- `POST /api/workflows/start` with `{template_name: "default", name: "Test"}` creates a workflow with 8 phases
- `GET /api/workflows` returns the created workflow
- `DELETE /api/workflows/:id` removes the workflow and its phases

---

### Task T3: Wire Phase Creation + Auto-Advance

**Assignee:** dev2 (continues from T2)  
**Priority:** Critical  
**Estimated time:** 60 min

**Goal:** When a workflow starts, create tasks for phase 0. When a task completes, advance the workflow.

**Subtasks:**

1. **Implement phase creation handlers:**
   - File: `internal/workflow/phases.go` (new)
   - `createSinglePhaseTasks(ctx, workflowID, phase, projectID)` — creates 1 task
   - `createMultiPhaseTasks(ctx, workflowID, phase, projectID)` — creates N tasks from config
   - `enterGatePhase(ctx, workflowID, phase)` — sets workflow status to `paused`, logs "waiting for approval"
   - For now, skip `per_dev` and `decision` — return error "not implemented"

2. **Wire phase creation into StartWorkflow:**
   - File: `internal/workflow/engine.go`
   - After creating workflow + phase rows, call `activatePhase(workflowID, phase0)`
   - **Address reviewer finding #2:** Phase creation handlers MUST be called from StartWorkflow
   ```go
   func (e *Engine) StartWorkflow(...) (*Workflow, error) {
       // ... create workflow and phases ...
       
       // Activate phase 0
       phase0, _ := e.getPhase(wf.ID, 0)
       if err := e.activatePhase(wf.ID, &phase0, projectID); err != nil {
           return nil, err
       }
       return wf, nil
   }
   ```

3. **Add auto-advance on task completion:**
   - File: `internal/task/handler.go`
   - In `CompleteTask()`, after updating task status, check if task belongs to a workflow:
     ```go
     var workflowMapping struct {
         WorkflowID string `db:"workflow_id"`
         PhaseID    string `db:"phase_id"`
     }
     h.db.Get(&workflowMapping,
         `SELECT workflow_id, phase_id FROM workflow_task_map WHERE task_id = $1`,
         taskID)
     
     if workflowMapping.WorkflowID != "" {
         go h.advanceWorkflowIfComplete(workflowMapping.WorkflowID, workflowMapping.PhaseID)
     }
     ```
   - Add `advanceWorkflowIfComplete()` method that:
     1. Increments `completed_tasks` on the phase
     2. If `completed_tasks == total_tasks`, mark phase as `completed`
     3. Advance to next phase (increment `current_phase`)
     4. Call `activatePhase()` for the new phase

4. **Fix createTask to accept status parameter:**
   - File: `internal/workflow/phases.go`
   - **Address reviewer finding #3:** Don't double-increment `pending_deps`
   - `createTask(ctx, title, taskType, projectID, status string)` — status is `"available"` or `"blocked"`
   - For `single` and `multi` phases: create with `status = "available"`
   - For `per_dev` phases (when implemented): create with `status = "blocked"`, insert dependency, let trigger set `pending_deps = 1`

**Files to create:**
- `internal/workflow/phases.go`

**Files to modify:**
- `internal/workflow/engine.go` (wire phase creation)
- `internal/task/handler.go` (auto-advance hook)

**Acceptance criteria:**
- `POST /api/workflows/start` creates workflow + phase 0 has `status = "active"`, `total_tasks = 1`
- One task is created with `status = "available"`, `task_type = "plan"`
- When that task is completed via `POST /api/agent/tasks/:id/complete`, the workflow advances to phase 1
- Phase 0 has `status = "completed"`, `completed_tasks = 1`
- `go build ./...` passes

---

### Task T4: Add DELETE Workflow + Fix Task Type Filter

**Assignee:** dev1  
**Priority:** Major  
**Estimated time:** 30 min

**Subtasks:**

1. **Add DELETE /api/workflows/:id:**
   - File: `internal/workflow/handler.go`
   - Handler checks workflow exists, then:
     ```go
     // Delete task mappings first
     h.db.Exec(`DELETE FROM workflow_task_map WHERE workflow_id = $1`, id)
     // Delete phases
     h.db.Exec(`DELETE FROM workflow_phases WHERE workflow_id = $1`, id)
     // Delete workflow
     h.db.Exec(`DELETE FROM workflows WHERE id = $1`, id)
     ```
   - Return 200 with `{message: "Workflow deleted"}` or 404 if not found

2. **Fix task type filter in GetQueue:**
   - File: `internal/agent/handler.go`
   - Extend the switch to support workflow task types:
     ```go
     switch taskTypeParam {
     case "dev":
         taskTypeFilter = "'dev', 'general'"
     case "review":
         taskTypeFilter = "'review', 'general'"
     case "test":
         taskTypeFilter = "'test', 'general'"
     case "plan":
         taskTypeFilter = "'plan'"
     case "deploy":
         taskTypeFilter = "'deploy'"
     case "task-breakdown":
         taskTypeFilter = "'task-breakdown'"
     case "verify":
         taskTypeFilter = "'verify'"
     default:
         taskTypeFilter = "'general'"
     }
     ```

3. **Fix task type filter in ListTasks:**
   - File: `internal/task/handler.go`
   - Extend the allowed map:
     ```go
     allowed := map[string]bool{
         "general": true, "dev": true, "review": true, "test": true,
         "plan": true, "deploy": true, "task-breakdown": true, "verify": true,
     }
     ```

**Files to modify:**
- `internal/workflow/handler.go` (DELETE endpoint)
- `internal/agent/handler.go` (GetQueue filter)
- `internal/task/handler.go` (ListTasks filter)

**Acceptance criteria:**
- `DELETE /api/workflows/:id` returns 200 and removes workflow + phases + mappings
- `GET /api/agent/tasks/queue?task_type=plan` returns tasks with `task_type = "plan"`
- `GET /api/tasks?type=plan` returns tasks with `task_type = "plan"`
- `go build ./...` passes

---

## 4. Dependency Graph

```
T1 (Dashboard fix + seed task)
  └── No dependencies — can start immediately

T2 (Workflow engine core)
  └── No dependencies — can start in parallel with T1

T3 (Phase creation + auto-advance)
  └── Depends on T2 (needs workflow package + migrations)

T4 (DELETE + filter fix)
  └── Depends on T2 (needs workflow handler for DELETE)
  └── Independent of T3
```

**Parallelization:**
- **Dev1:** T1 → T4 (sequential)
- **Dev2:** T2 → T3 (sequential)

**Timeline:**
- T1: 30 min (Dev1)
- T2: 90 min (Dev2)
- T3: 60 min (Dev2) — starts after T2
- T4: 30 min (Dev1) — starts after T1, can overlap with T2/T3

**Total wall time:** ~3 hours with 2 devs in parallel

---

## 5. Default Workflow Template

Seed this on startup if `workflow_templates` is empty for `name = 'default'`:

```json
{
  "name": "default",
  "description": "Standard 8-phase development workflow",
  "phases": [
    {"name": "Design", "type": "single", "task_type": "plan", "config": {}},
    {"name": "Design Gate", "type": "gate", "task_type": "", "config": {"approver": "admin"}},
    {"name": "Development", "type": "multi", "task_type": "dev", "config": {"count": 3}},
    {"name": "Code Review", "type": "per_dev", "task_type": "review", "config": {}},
    {"name": "Testing", "type": "per_dev", "task_type": "test", "config": {}},
    {"name": "Quality Gate", "type": "decision", "task_type": "", "config": {"pass_condition": "all"}},
    {"name": "PM Review", "type": "gate", "task_type": "", "config": {"approver": "admin", "require_owner": true}},
    {"name": "Deploy", "type": "single", "task_type": "deploy", "config": {}}
  ]
}
```

---

## 6. Risks & Blockers

| Risk | Mitigation |
|------|------------|
| Migration 011-012 fail on existing DB | Use `IF NOT EXISTS` everywhere; test on fresh DB first |
| Trigger `fn_task_completed` breaks existing task completion | Add guard: `IF NEW.status NOT IN ('done', 'deployed', 'skipped') THEN RETURN NEW; END IF;` |
| Workflow start creates 0 tasks (reviewer finding #2) | T3 explicitly wires `activatePhase` call in `StartWorkflow` |
| Double-increment of `pending_deps` (reviewer finding #3) | T3 creates tasks with correct status; trigger handles counter |
| LISTEN/NOTIFY always empty (reviewer finding #4) | T2 trigger collects IDs before DELETE |
| API responses missing fields (reviewer finding #6) | T2 handler returns full workflow + phases with all fields |

---

## 7. Files Summary

### Files to Create
| File | Task | Purpose |
|------|------|---------|
| `internal/workflow/types.go` | T2 | Workflow, WorkflowPhase, Template structs |
| `internal/workflow/engine.go` | T2 | StartWorkflow, GetWorkflow, ListWorkflows, DeleteWorkflow |
| `internal/workflow/handler.go` | T2, T4 | HTTP handlers for workflow API |
| `internal/workflow/phases.go` | T3 | createSinglePhaseTasks, createMultiPhaseTasks, enterGatePhase |

### Files to Modify
| File | Task | Changes |
|------|------|---------|
| `cmd/server/main.go` | T1, T2 | Seed task, password warning, seed template, register routes |
| `internal/dashboard/handler.go` | T1 | Debug logging |
| `internal/db/migrations.sql` | T2 | Append migrations 011-012 |
| `internal/task/handler.go` | T3, T4 | Auto-advance hook, extend task type filter |
| `internal/agent/handler.go` | T4 | Extend GetQueue task type filter |

---

## 8. Verification Checklist

After all tasks complete:

- [ ] `go build ./...` passes with zero errors
- [ ] Server starts without panic
- [ ] `GET /api/dashboard/summary` returns at least one task in `recent_tasks`
- [ ] `POST /api/auth/login` with `{username: "admin", password: "admin123"}` returns JWT
- [ ] `POST /api/workflows/start` with `{template_name: "default", name: "Test WF"}` creates workflow + 8 phases
- [ ] `GET /api/workflows` lists the created workflow
- [ ] `GET /api/workflows/:id` returns workflow with all phases
- [ ] Phase 0 has `status: "active"`, `total_tasks: 1`
- [ ] One task with `task_type: "plan"` exists in `tasks` table
- [ ] `DELETE /api/workflows/:id` removes workflow + phases
- [ ] `GET /api/agent/tasks/queue?task_type=plan` returns the plan task
- [ ] Completing the plan task advances workflow to phase 1

---

*Last updated: 2026-04-03*
