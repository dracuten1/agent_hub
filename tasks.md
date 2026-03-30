# AgentHub — Tasks

## In Progress

### Round 2 — Testing
- Tester validating all 7 improvements (CORS, rate limit, worker filter, cleanup, env, seed, git cleanup)
- Awaiting results

## Next Up (Round 3)

### IMPROVE-8: TestTask fail path — task slot leak (Dev1)
- `internal/task/handler.go` TestTask method
- When test fails and task goes back to `needs_fix`, agent's `current_tasks` isn't decremented
- Same pattern as BUG-3 fix in CompleteTask

### IMPROVE-9: Task status machine validation (Dev2)
- `internal/task/handler.go` — all status transition methods
- Currently any status can transition to any status via direct API calls
- Add status machine: only allow valid transitions (available→claimed→in_progress→done→review→test→deployed, or →needs_fix→in_progress, or →failed/escalated)

### IMPROVE-10: API response standardization (Dev1)
- All handlers return slightly different error formats
- Standardize: `{error: string}`, `{task: object}`, `{tasks: array}`, `{agents: array}` etc
- Add proper HTTP status codes (404 for not found, 403 for forbidden, 400 for bad request)

### IMPROVE-11: Request logging middleware (Dev2)
- New `middleware/logging.go`
- Log all API requests with: method, path, status, duration, IP
- Structured JSON logging format
- Skip health check endpoint to reduce noise

### IMPROVE-12: Feature GET by ID endpoint (Dev1)
- `internal/feature/handler.go` — missing `GET /features/:id`
- Projects have Get by ID but features don't

### IMPROVE-13: Pagination limit cap (Dev2)
- Dev2 added pagination but no max cap
- Cap at 100 items per page regardless of `?limit=` param

## Completed

### Bug Fix Sprint (2026-03-30)
- BUG-1: ph() overflow ✅
- BUG-2: Admin user seed ✅
- BUG-3: current_tasks counter leak ✅
- BUG-4: Review/Test auth checks ✅
- BUG-5: Driver env loading ✅

### Improvement Sprint (2026-03-30)
- IMPROVE-1: Git binary cleanup ✅
- IMPROVE-2: CORS middleware ✅
- IMPROVE-3: Rate limiting ✅
- IMPROVE-4: Docker Compose cleanup ✅
- IMPROVE-5: Env var alignment ✅
- IMPROVE-6: Worker queue filtering ✅
- IMPROVE-7: Seed query simplification ✅

### Test Pipeline (2026-03-30)
- GET /api/hello endpoint ✅

### Initial v1.0 (2026-03-29)
- API server, workers, web dashboard, Docker, driver script
