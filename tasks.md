# AgentHub — Tasks

## In Progress

### IMPROVE-1: Remove worker binaries from git (Dev1)
- **Files:** `.gitignore`, repo cleanup
- 3 binary files committed: `agenthub-worker`, `ateam-worker`, `worker`
- Add to .gitignore, `git rm --cached`, verify clean

### IMPROVE-2: CORS middleware for web dashboard (Dev2)
- **Files:** `cmd/server/main.go` + new `middleware/cors.go`
- React frontend needs CORS to call API from browser
- Add configurable CORS middleware (allow origins from env `CORS_ALLOWED_ORIGINS`, default `*`)
- Apply to all `/api/` routes

### IMPROVE-3: Rate limiting middleware (Dev1)
- **Files:** new `middleware/ratelimit.go`, `cmd/server/main.go`
- No rate limiting on any endpoint — add token-bucket rate limiter
- Configurable via env: `RATE_LIMIT_RPM` (requests per minute, default 60)
- Stricter limits on auth endpoints: 5 req/min

### IMPROVE-4: Docker Compose cleanup (Dev2)
- **Files:** `docker-compose.yml`, `Dockerfile`
- Remove obsolete `version: '3.8'` key
- Remove redundant `migrations.sql` copy from Dockerfile (embed handles it)
- Add `ADMIN_PASSWORD` env var to api service environment

### IMPROVE-5: Env var name alignment (Dev2)
- **Files:** `cmd/server/main.go`, `scripts/octeam-driver.sh`, `.env`, `docker-compose.yml`
- Code reads `ADMIN_PASSWORD` but docker/env uses `AGENTHUB_ADMIN_PASS`
- Standardize on `AGENTHUB_ADMIN_PASS` everywhere (it's already in .env and driver script)

### IMPROVE-6: Worker queue role-based filtering (Dev1)
- **Files:** `internal/agent/handler.go`, `workers/dev.go`, `workers/review.go`, `workers/test.go`
- All 3 workers poll same `/api/agent/tasks/queue` — reviewer picks up dev tasks
- Add `task_type` filter param to queue endpoint (already partially done)
- Workers should send their role when polling

### IMPROVE-7: Redundant COUNT query in admin seed (Dev2)
- **File:** `cmd/server/main.go`
- Currently does `SELECT COUNT(*) FROM users WHERE role='admin'` then `INSERT`
- Replace with single `INSERT ... ON CONFLICT DO NOTHING` (race-safe, one query)

## Backlog

### IMPROVE-8: Pagination limit cap
- Reset pagination should cap at 100

### IMPROVE-9: Dashboard route rename
- Minor naming inconsistency

### IMPROVE-10: Dashboard error handling
- Add proper error responses

### IMPROVE-11: Duplicate anonymous struct
- DRY refactor in agent handler

### IMPROVE-12: TestTask fail path — task slot leak
- Same as BUG-3 pattern but for test fail

### IMPROVE-13: WebSocket notifications
- Replace polling with real-time updates

## Completed

### Bug Fix Sprint (2026-03-30)
- BUG-1: ph() overflow ✅
- BUG-2: Admin user seed ✅
- BUG-3: current_tasks counter leak ✅
- BUG-4: Review/Test auth checks ✅
- BUG-5: Driver env loading ✅

### Test Pipeline (2026-03-30)
- GET /api/hello endpoint ✅ (full cycle: dev → review fail → fix → review pass → test pass → commit)

### Initial v1.0 (2026-03-29)
- API server with auth, tasks, agents, projects, features, dashboard, review
- Worker framework (dev, review, test)
- Web dashboard (React/Vite)
- Docker Compose deployment
- ocTeam driver script
