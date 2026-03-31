## Task: Task Type Field + Filtered Polling

> **Status:** Draft  
> **Author:** Reviewer  
> **Date:** 2026-03-31  
> **Files touched:** `internal/task/handler.go`, `internal/agent/handler.go`, `internal/db/migrations.sql`, `internal/worker/types.go`

---

### Context

AgentHub already has a `task_type` column (`VARCHAR(20) DEFAULT 'general'`) added in migration `009_add_task_type.sql`. The `Task` struct in `handler.go` already has `TaskType string`, and `CreateTaskRequest` validates against `oneof=general dev review test`. The agent queue endpoint (`GET /api/agent/tasks/queue`) already filters by task type based on agent role or `?task_type=` query param.

**What's missing:**

1. The `type` enum requested is `coding/review/testing/planning`, but the codebase uses `general/dev/review/test`. The spec below uses the **existing values** (`general`, `dev`, `review`, `test`) to avoid a breaking rename. If the PM wants the naming changed to `coding/review/testing/planning`, that's a separate migration — flag it in Consistency Notes.
2. The **user-facing** `GET /api/tasks` list endpoint does NOT support `?type=` filtering yet.
3. There is no validation for the `type` query param on the user-facing list — needs input validation.
4. No `planning` type exists — only `general` (the catch-all). We treat `general` as the "planning" equivalent, or we add `planning` as a new enum value.

This spec covers the minimal changes to close those gaps.

---

### API Contract

#### 1. `GET /api/tasks?type=<task_type>` — Filtered Task Listing

Adds `type` as an optional query parameter to the existing `ListTasks` handler.

| Parameter | Type   | Required | Default | Allowed Values               |
|-----------|--------|----------|---------|------------------------------|
| `type`    | string | No       | (none)  | `general`, `dev`, `review`, `test` |
| `status`  | string | No       | (none)  | any valid status             |
| `project_id` | string | No   | (none)  | any project UUID             |
| `assignee`   | string | No   | (none)  | any agent name               |
| `page`    | int    | No       | 1       | ≥ 1                          |
| `limit`   | int    | No       | 100     | 1–100                        |

**Response** (unchanged shape):

```json
{
  "tasks": [...],
  "total": 42,
  "page": 1,
  "limit": 100
}
```

When `type` is provided, only tasks with `task_type = <value>` are returned. When omitted, all types are returned (no filter applied).

#### 2. `POST /api/tasks` — Create with Type (already implemented)

The `CreateTaskRequest.task_type` field already exists with validation `oneof=general dev review test` and defaults to `"general"` when empty. No changes needed.

#### 3. `GET /api/agent/tasks/queue?type=<task_type>` — Agent Queue (already implemented)

The agent queue already supports `?task_type=` and falls back to agent role. No changes needed.

---

### Behavior

#### A. `ListTasks` — Add `type` Filter

**File:** `internal/task/handler.go`, method `ListTasks`

Current code builds a dynamic WHERE clause from `status`, `project_id`, `assignee`. Add `taskType` to the same pattern:

```go
// After existing filters (around line that checks assignee):
taskType := c.Query("type")
if taskType != "" {
    whereClause += argPlaceholder(argIdx, "task_type")
    args = append(args, taskType)
    argIdx++
}
```

**Validation:** If `taskType` is provided but not one of the allowed values, return `400`:

```go
validTypes := map[string]bool{"general": true, "dev": true, "review": true, "test": true}
if taskType != "" && !validTypes[taskType] {
    c.JSON(400, gin.H{"error": "Invalid type filter", "allowed": []string{"general", "dev", "review", "test"}})
    return
}
```

This validation must happen **before** the query is built.

#### B. Database Index (already exists)

Migration `009_add_task_type.sql` already creates `idx_tasks_task_type` on `tasks(task_type)`. The query will use this index automatically.

#### C. Backward Compatibility

| Scenario                     | Behavior                                                        |
|------------------------------|-----------------------------------------------------------------|
| `GET /api/tasks` (no `type`) | Returns all tasks — **no change** from current behavior         |
| `GET /api/tasks?type=dev`    | Returns only `dev` tasks                                        |
| Existing tasks without type  | Column default is `'general'`, so they match `?type=general`   |
| `POST /api/tasks` (no type)  | Defaults to `"general"` — **already implemented**               |

#### D. Default Value for Backward Compat

The column default `'general'` is already set in migration `009`. All existing rows got `task_type = 'general'` via the ALTER. No additional migration needed for existing data.

If the PM wants `type=coding` (not `dev`) as the default name, that requires a data migration:

```sql
-- Only if renaming dev → coding
UPDATE tasks SET task_type = 'coding' WHERE task_type = 'dev';
-- Plus validation enum change in handler.go
```

---

### Edge Cases

| Case                                    | Expected Behavior                                                                    |
|-----------------------------------------|--------------------------------------------------------------------------------------|
| `?type=invalid_value`                   | `400 Bad Request` with `{"error": "Invalid type filter", "allowed": [...]}`          |
| `?type=dev` with no matching tasks      | `200 OK` with `{"tasks": [], "total": 0, "page": 1, "limit": 100}`                  |
| `?type=dev&type=review` (duplicate key) | Go's `c.Query("type")` returns the **first** value. Only `dev` filter is applied.    |
| `?type=` (empty string)                 | Treated as no filter — return all tasks                                              |
| Combined `?status=available&type=dev`   | Both filters applied with AND — returns available dev tasks only                     |
| `POST /api/tasks` with `task_type=""`   | Defaults to `"general"` — already handled in `CreateTask`                            |
| `POST /api/tasks` with `task_type="coding"` | `400` — not in allowed values (`oneof=general dev review test`)                 |
| Agent queue with unknown role           | Falls back to `task_type IN ('general')` — conservative, only picks untyped tasks    |

---

### Consistency Notes

1. **Naming mismatch:** The PM asked for `coding/review/testing/planning`, but the codebase uses `general/dev/review/test`. The `"general"` type serves as the planning/catch-all category. If the PM wants the exact naming, the following files need updating:
   - `internal/task/handler.go` — `CreateTaskRequest.TaskType` validation tag
   - `internal/agent/handler.go` — `GetQueue` role-to-type mapping switch
   - Migration to `UPDATE` existing rows and change the column default
   - **Recommendation:** Keep current names (`dev`, `review`, `test`, `general`) to avoid a breaking change across all workers.

2. **Worker polling:** Workers already pass `?task_type=dev` etc. when calling the queue endpoint (`workers/dev.go`, `workers/review.go`, `workers/test.go`). No worker changes needed.

3. **`worker/types.go` `Task` struct** doesn't have a `TaskType` field — workers receive it from the queue JSON but don't deserialize it. If workers ever need to branch behavior by type, add `TaskType string \`json:"task_type"\`` to the worker `Task` struct. Not needed for this spec.

4. **The `broadcastTaskEvent` method** in `handler.go` has a signature mismatch — it's defined with 4 params (`taskID, fromStatus, toStatus, agent`) but called with 2 params (`taskID, "task_updated"`). This is an existing bug, not introduced by this change. Flagging for PM.

5. **`ClaimTask` has a duplicate `c.JSON` call** (lines sending 400 twice with different variables `oldStatus` vs undefined `status`). Existing bug — flagging for PM.

---

### Implementation Checklist

- [ ] Add `type` query param validation + filter in `ListTasks` (`internal/task/handler.go`)
- [ ] Add validation before query construction (reject invalid types with 400)
- [ ] Verify existing index `idx_tasks_task_type` covers the new filter
- [ ] Manual test: `GET /api/tasks`, `GET /api/tasks?type=dev`, `GET /api/tasks?type=invalid`
- [ ] No migration needed — column and index already exist
- [ ] Flag naming mismatch to PM if `coding/review/testing/planning` naming is required
