# AgentHub — Tasks

## In Progress

### BUG-1: ph() function breaks for idx >= 10 (CRITICAL)
- **Files:** `internal/project/handler.go`, `internal/feature/handler.go`
- **Bug:** `ph()` uses `string(rune('0'+idx))` — overflows at idx=10+ (produces `$:` not `$10`)
- **Fix:** Use `fmt.Sprintf("$%d", idx)` instead
- **Assignee:** dev1

### BUG-2: No admin user seeded (CRITICAL)
- **Files:** `internal/auth/handler.go`, `cmd/server/main.go`
- **Bug:** Only `tuyen` (role=user) exists. `DeleteAgent` requires admin role — impossible to delete agents. API.md says admin/admin123 but user doesn't exist.
- **Fix:** Add seed logic in main.go to create admin user on startup if none exists
- **Assignee:** dev2

### BUG-3: CompleteTask doesn't decrement agent current_tasks counter (CRITICAL)
- **File:** `internal/task/handler.go`
- **Bug:** When task fails, agent's `current_tasks` counter never decrements. Only `TestTask` pass decrements it. Counter leaks over time.
- **Fix:** Decrement `current_tasks` in `CompleteTask` when status becomes `failed` or `escalated`
- **Assignee:** dev1

### BUG-4: ReviewTask/TestTask have no assignee authorization (MAJOR)
- **File:** `internal/task/handler.go`
- **Bug:** Any agent can review/test any task. `CompleteTask` checks `assignee = $3` but `ReviewTask` and `TestTask` don't.
- **Fix:** Add `AND assignee = $agentName` check to review/test status transitions
- **Assignee:** dev2

### BUG-5: ocTeam driver can't access AGENTHUB_ADMIN_PASS (MAJOR)
- **File:** `scripts/octeam-driver.sh`
- **Bug:** Cron sandbox doesn't inherit `/etc/environment`. Script auth fails silently.
- **Fix:** Add `[ -f /etc/agenthub/agenthub.env ] && source /etc/agenthub/agenthub.env` at script top
- **Assignee:** dev1

## Backlog

### IMPROVE-1: Remove worker binaries from git
- **Files:** `.gitignore`, repo cleanup
- Binary files `agenthub-worker`, `ateam-worker`, `worker` committed. Add to .gitignore, git rm.

### IMPROVE-2: Worker queue filtering by role
- **Files:** `internal/agent/handler.go`, `workers/*.go`
- All 3 workers poll same `/api/agent/tasks/queue` — reviewer picks up dev tasks. The endpoint already filters by role via `taskTypeFilter`, but workers all register with the same role-based filter. Verify and fix worker task type matching.

### IMPROVE-3: docker-compose.yml cleanup
- Remove obsolete `version` key, remove redundant `migrations.sql` copy from Dockerfile

### IMPROVE-4: Rate limiting
- No rate limiting on any endpoint. Add middleware for auth endpoints.

### IMPROVE-5: CORS for web dashboard
- React frontend needs CORS headers to call API from browser

## Completed

### Initial v1.0 (2026-03-29)
- API server with auth, tasks, agents, projects, features, dashboard, review
- Worker framework (dev, review, test)
- Web dashboard (React/Vite)
- Docker Compose deployment
- ocTeam driver script
