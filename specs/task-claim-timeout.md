## Task: Task Claim Timeout & Orphan Recovery

> **Status:** Draft  
> **Author:** Reviewer  
> **Date:** 2026-03-31  
> **Files touched:** `internal/task/handler.go`, `internal/agent/handler.go`, `internal/db/migrations.sql`, `cmd/server/main.go`

---

### Context

**Current state of the codebase:**

- `ClaimTask` (`handler.go:455`) sets `status = 'claimed'`, `claimed_at = NOW()`, increments `agents.current_tasks`
- `UpdateProgress` (`handler.go:496`) sets `updated_at = NOW()` on each progress ping, auto-transitions `claimed → in_progress`
- Workers send a 5-minute heartbeat to `POST /api/agent/heartbeat` (`worker/core.go:98-106`, `worker/api.go:154`), which updates `agents.last_heartbeat` and `agents.status`
- `StartHealthMonitor` (`agent/handler.go:290`) runs every 5 min, marks agents `warning` (>15 min) or `dead` (>30 min), reassigns their tasks to backup agents or marks `orphaned`
- `validTransitions` already includes `"orphaned": {"claimed"}` — so orphaned tasks can be reclaimed
- `tasks.updated_at` is set on every write, `tasks.claimed_at` is set on claim only
- There is **no** mechanism to release a task that's stuck in `claimed`/`in_progress` with no progress updates
- There is **no** mechanism to detect tasks whose assignee agent is `idle`/`warning` but the task itself is stale
- There is **no** user-facing API to list or manually release stale tasks
- `DeleteTask` already has the pattern for decrementing `agents.current_tasks` on release (`handler.go:391-397`)

**What's missing:**

1. Automatic detection and release of stale claimed/in_progress tasks
2. A heartbeat-based staleness check at the **task** level (agent heartbeats only track agent health, not per-task liveness)
3. User-facing APIs to inspect and release stale tasks
4. Capacity tracking fix when tasks are released

---

### API Contract

#### 1. `GET /api/tasks?stale=true` — List Stale Tasks

Adds `stale` as an optional boolean query parameter to the existing `ListTasks` handler.

| Parameter | Type   | Required | Default | Description |
|-----------|--------|----------|---------|-------------|
| `stale`   | bool   | No       | false   | When true, only return tasks that are stale (see definition below) |
| `status`  | string | No       | (none)  | Existing filter |
| `type`    | string | No       | (none)  | Existing filter |
| `project_id` | string | No   | (none)  | Existing filter |
| `page`    | int    | No       | 1       | ≥ 1 |
| `limit`   | int    | No       | 100     | 1–100 |

**Stale definition:** A task is stale when ALL of the following are true:
- `status IN ('claimed', 'in_progress')`
- `claimed_at` is not NULL
- `claimed_at < NOW() - INTERVAL '30 minutes'` (claim timeout)
- `updated_at < NOW() - INTERVAL '15 minutes'` (progress heartbeat timeout)

When `stale=true`, the `status` filter is overridden to only match `claimed`/`in_progress` (other statuses can't be stale).

**Response** (same shape as ListTasks):

```json
{
  "tasks": [
    {
      "id": "...",
      "title": "...",
      "status": "claimed",
      "assignee": "dev-1",
      "claimed_at": "2026-03-31T09:00:00Z",
      "updated_at": "2026-03-31T09:10:00Z",
      "stale_reason": "no_progress",
      "stale_duration_minutes": 47
    }
  ],
  "total": 3,
  "page": 1,
  "limit": 100
}
```

`stale_reason` and `stale_duration_minutes` are computed fields (not in DB), added in the response when `stale=true`.

#### 2. `POST /api/tasks/:id/release` — Manually Release a Stale Task

**Auth:** JWT (user route) — PM/owner action.

**Request body:** (optional)

```json
{
  "reason": "Agent dev-1 is unresponsive"
}
```

**Response:**

```json
{
  "message": "Task released",
  "task": { ... },
  "previous_assignee": "dev-1"
}
```

**Behavior:**
- Validate task is in a releasable status: `claimed`, `in_progress`, `orphaned`
- Set `status = 'available'`, `assignee = NULL`, increment `retry_count`
- Decrement previous assignee's `agents.current_tasks` (using `GREATEST(current_tasks - 1, 0)`)
- Log event: `released` with `from_status`, note
- Broadcast WS event
- If task exceeds `max_retries`, escalate instead of releasing

**Error responses:**
- `404` — task not found
- `400` — task not in a releasable status (e.g., already `available`, `done`, `deployed`)
- `409` — task was concurrently released by another process (optimistic locking via `updated_at` check)

---

### Behavior

#### A. Claim Timeout — Automatic Staleness Detection

Add a background goroutine `StartStaleTaskMonitor` (similar to existing `StartHealthMonitor` in `agent/handler.go:290`).

**Configurable thresholds** (env vars with defaults):

| Env Var | Default | Description |
|---------|---------|-------------|
| `CLAIM_TIMEOUT_MINUTES` | 30 | Time since `claimed_at` before a task is considered stale |
| `PROGRESS_TIMEOUT_MINUTES` | 15 | Time since last `updated_at` with no progress |
| `STALE_CHECK_INTERVAL_MINUTES` | 5 | How often to run the check |

**Staleness query** (single SQL statement):

```sql
UPDATE tasks SET
  status = 'available',
  assignee = NULL,
  retry_count = retry_count + 1,
  claimed_at = NULL,
  updated_at = NOW()
WHERE status IN ('claimed', 'in_progress')
  AND claimed_at IS NOT NULL
  AND claimed_at < NOW() - ($1 || ' minutes')::interval
  AND updated_at < NOW() - ($2 || ' minutes')::interval
  AND retry_count < max_retries
RETURNING id, assignee
```

For each returned row:
1. `UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1` (previous assignee)
2. Log event: `auto_released` with from_status, note "Task auto-released: no progress for N minutes"
3. Broadcast WS event: `task_released`

**Escalation path:** Tasks where `retry_count >= max_retries` are set to `escalated` instead of `available`:

```sql
UPDATE tasks SET
  status = 'escalated',
  escalated = true,
  updated_at = NOW()
WHERE status IN ('claimed', 'in_progress')
  AND claimed_at IS NOT NULL
  AND claimed_at < NOW() - ($1 || ' minutes')::interval
  AND updated_at < NOW() - ($2 || ' minutes')::interval
  AND retry_count >= max_retries
RETURNING id, assignee
```

Same capacity fix + event logging for these.

#### B. Progress Heartbeat Linkage

**Current behavior:** `UpdateProgress` (`handler.go:496`) updates `tasks.updated_at` on every call. Workers call this at key milestones (30%, 80%, 100% in `core.go:182-189`).

**No changes needed to the progress endpoint.** The `updated_at` column already serves as the heartbeat timestamp. The stale monitor checks `updated_at < NOW() - 15 min` to detect dead tasks.

**Worker heartbeat** (`POST /api/agent/heartbeat` every 5 min) is independent — it tracks **agent** health, not per-task progress. A healthy agent can still have a stuck task. The stale monitor catches the task-level case.

**Optional enhancement** (not in v1): Workers could send `PATCH /api/agent/tasks/:id/progress` more frequently (e.g., every 2 min with same progress value) to act as a task-level heartbeat. This would keep `updated_at` fresh even during long-running operations. Not required for the initial implementation — the 15-minute threshold is generous enough.

#### C. Manual Release Endpoint — `ReleaseTask`

**File:** `internal/task/handler.go`, new method `ReleaseTask`.

```go
func (h *Handler) ReleaseTask(c *gin.Context) {
    taskID := c.Param("id")

    // Get current state
    var status string
    var assignee *string
    var retryCount, maxRetries int
    err := h.db.QueryRow(
        "SELECT status, assignee, retry_count, max_retries FROM tasks WHERE id = $1",
        taskID,
    ).Scan(&status, &assignee, &retryCount, &maxRetries)
    if err != nil {
        c.JSON(404, gin.H{"error": "Task not found"})
        return
    }

    releasable := map[string]bool{"claimed": true, "in_progress": true, "orphaned": true}
    if !releasable[status] {
        c.JSON(400, gin.H{
            "error": "Task cannot be released",
            "current_status": status,
            "releasable_statuses": []string{"claimed", "in_progress", "orphaned"},
        })
        return
    }

    // Check max retries — escalate instead
    if retryCount >= maxRetries {
        h.db.Exec("UPDATE tasks SET status = 'escalated', escalated = true, updated_at = NOW() WHERE id = $1", taskID)
        if assignee != nil {
            h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *assignee)
        }
        h.logEvent(taskID, "pm", "escalated", status, "escalated", "Max retries exceeded, task escalated")
        h.broadcastTaskEvent(taskID, "task_updated", status, "escalated")
        c.JSON(200, gin.H{"message": "Task escalated (max retries exceeded)", "new_status": "escalated"})
        return
    }

    // Release
    var req struct {
        Reason string `json:"reason"`
    }
    c.ShouldBindJSON(&req)

    note := "Task released by PM"
    if req.Reason != "" {
        note += ": " + req.Reason
    }

    _, err = h.db.Exec(
        "UPDATE tasks SET status = 'available', assignee = NULL, retry_count = retry_count + 1, claimed_at = NULL, updated_at = NOW() WHERE id = $1",
        taskID,
    )
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to release task"})
        return
    }

    // Decrement previous assignee's capacity
    if assignee != nil {
        h.db.Exec("UPDATE agents SET current_tasks = GREATEST(current_tasks - 1, 0) WHERE name = $1", *assignee)
    }

    h.logEvent(taskID, "pm", "released", status, "available", note)
    h.broadcastTaskEvent(taskID, "task_updated", status, "available")

    c.JSON(200, gin.H{"message": "Task released", "previous_assignee": assignee})
}
```

**Route registration:** Add to `RegisterUserRoutes`:

```go
g.POST("/tasks/:id/release", h.ReleaseTask)
```

#### D. Stale Filter in ListTasks

**File:** `internal/task/handler.go`, modify `ListTasks`.

Add after the existing `type` filter block:

```go
if stale := c.Query("stale"); stale == "true" {
    claimTimeout := getEnvInt("CLAIM_TIMEOUT_MINUTES", 30)
    progressTimeout := getEnvInt("PROGRESS_TIMEOUT_MINUTES", 15)

    whereClause += " AND status IN ('claimed', 'in_progress')"
    whereClause += " AND claimed_at IS NOT NULL"
    whereClause += " AND claimed_at < NOW() - ($" + placeholder(argIdx) + " || ' minutes')::interval"
    args = append(args, claimTimeout)
    argIdx++
    whereClause += " AND updated_at < NOW() - ($" + placeholder(argIdx) + " || ' minutes')::interval"
    args = append(args, progressTimeout)
    argIdx++
}
```

**Computed fields** (`stale_reason`, `stale_duration_minutes`): These are added in a post-processing step after the query, not in SQL:

```go
if stale == "true" {
    for i := range tasks {
        claimedAt, _ := time.Parse(time.RFC3339, *tasks[i].ClaimedAt)
        tasks[i].StaleDurationMinutes = int(time.Since(claimedAt).Minutes())
        // stale_reason is always "no_progress" for now (only one staleness criteria)
    }
}
```

This requires adding `StaleDurationMinutes` and `StaleReason` as non-DB fields to the `Task` struct (or using a response wrapper).

#### E. DB Migration

**File:** `internal/db/migrations.sql`, add migration `010`.

```sql
-- 010_stale_task_indexes.sql
-- Index for stale task detection queries
CREATE INDEX IF NOT EXISTS idx_tasks_stale_check
  ON tasks (status, claimed_at, updated_at)
  WHERE status IN ('claimed', 'in_progress');

-- Add column to track release count (useful for analytics)
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS release_count INT DEFAULT 0;
```

The partial index is critical — it keeps the index small (only covers active tasks) and makes the stale monitor query fast.

Update the `ReleaseTask` and auto-release queries to also increment `release_count`:

```sql
... retry_count = retry_count + 1, release_count = release_count + 1 ...
```

#### F. Startup — Register Stale Monitor

**File:** `cmd/server/main.go`.

```go
// Start stale task monitor (alongside existing health monitor)
go task.StartStaleTaskMonitor(database, wsHub, staleCheckInterval)
```

Place alongside the existing `go agent.StartHealthMonitor(database, 5*time.Minute)` line.

---

### Edge Cases

| Case | Behavior |
|------|----------|
| Task just claimed (<1 min) | Not stale — `claimed_at` is recent, `updated_at` just set. Short-circuited by `claimed_at < NOW() - 30 min` check |
| Task at 99% progress, no update for 20 min | Stale — progress value doesn't matter, only `updated_at` recency. Will be auto-released. If the worker finishes right after release, `CompleteTask` will fail with "invalid transition from available to done" — worker should handle gracefully (already does: logs error and moves on) |
| Multiple workers try to claim same released task | Already handled — `ClaimTask` does `UPDATE ... WHERE id = $2` without optimistic locking, but the `status = 'available'` check via `isValidTransition` ensures only one claim succeeds. First worker to call `ClaimTask` wins; others get "invalid transition" |
| Task released but no other agent available | Task stays `available` in queue. `GetQueue` already returns it. Will be picked up when an agent has capacity |
| Task released, retry_count exceeds max_retries | Auto-escalated to PM instead of released back to available |
| Manual release of a task already being completed | Race condition: if worker calls `CompleteTask` concurrently, one of the two writes wins. If release wins, worker's complete fails with "invalid transition". If complete wins, release gets "Task cannot be released" (status is now `done`/`review`). Both outcomes are safe |
| Agent marked `dead` by health monitor AND task auto-released | Both paths decrement `current_tasks`. Double-decrement is safe because of `GREATEST(current_tasks - 1, 0)` — can't go below 0. Task won't be double-released because the stale monitor checks current `status` in the UPDATE WHERE clause |
| Task in `needs_fix` or `fix_in_progress` status | Not stale — stale monitor only targets `claimed`/`in_progress`. These statuses have their own retry/escalation paths via review/test flows |
| `stale=true` combined with `status=done` | Override: `stale=true` forces `status IN ('claimed', 'in_progress')`, ignoring the user's `status=done` filter. Alternatively, return 400 if both are specified. **Recommendation:** ignore `status` when `stale=true` and document this |
| Worker sends progress update right as stale monitor runs | `updated_at` gets set to NOW(), so the task no longer matches the stale query. No release. This is the correct race outcome |
| `claimed_at` is NULL | Not stale — query requires `claimed_at IS NOT NULL`. This handles legacy tasks that were manually set to `claimed` without `claimed_at` |

---

### Consistency Notes

1. **`orphaned` status already exists** in `validTransitions` with transition to `claimed`. The `StartHealthMonitor` already sets tasks to `orphaned` when an agent dies and no backup is available. The new `ReleaseTask` endpoint also handles `orphaned` tasks (releasable). The stale monitor should NOT target `orphaned` tasks — they're already flagged for manual intervention.

2. **`DeleteTask` has a capacity fix pattern** (`handler.go:391-397`) that checks if the task is in a non-terminal state before decrementing. The release flow should follow the same pattern. However, for stale release, we know the status is always `claimed`/`in_progress` (non-terminal), so the decrement is always needed.

3. **`ReassignTask` increments `retry_count`** (`handler.go:410`) but does NOT decrement the old assignee's `current_tasks`. This is a pre-existing bug. The new `ReleaseTask` should correctly decrement, but `ReassignTask` should also be fixed. **Not in scope for this spec** but flagging for PM.

4. **Worker `SendHeartbeat`** (`worker/api.go:154`) only sends `status` as a string. It doesn't send `active_tasks` list. The agent heartbeat handler (`agent/handler.go:112`) accepts `ActiveTasks []string` but the worker doesn't populate it. This means the server can't cross-reference which specific tasks a worker is actively processing. The stale monitor must rely on `updated_at` timestamps instead. **Enhancement for future:** have workers send their active task IDs in heartbeats.

5. **`StartHealthMonitor` runs every 5 min.** The new `StartStaleTaskMonitor` should use a different default interval (also 5 min is fine) but should be independently configurable. Both monitors should log clearly so their output can be distinguished in logs.

6. **`validTransitions` update:** `available` already transitions to `claimed`. The reverse (`claimed → available`) is already in the map. No changes needed to the transition table.

---

### Implementation Checklist

- [ ] Add `POST /api/tasks/:id/release` endpoint (`ReleaseTask` method in `handler.go`)
- [ ] Add `?stale=true` filter to `ListTasks` with computed response fields
- [ ] Add `StartStaleTaskMonitor` goroutine (in `task/` or new `internal/stale/` package)
- [ ] Add migration `010_stale_task_indexes.sql` with partial index and `release_count` column
- [ ] Register stale monitor in `cmd/server/main.go` alongside health monitor
- [ ] Add env vars: `CLAIM_TIMEOUT_MINUTES` (30), `PROGRESS_TIMEOUT_MINUTES` (15), `STALE_CHECK_INTERVAL_MINUTES` (5)
- [ ] Add `ReleaseTask` route to `RegisterUserRoutes`
- [ ] Tests: release endpoint (releasable, non-releasable, not found, max retries escalation)
- [ ] Tests: stale filter (stale tasks returned, non-stale excluded)
- [ ] Tests: auto-release monitor logic (mock time or use very short thresholds)
- [ ] Flag `ReassignTask` capacity bug to PM (separate fix)
