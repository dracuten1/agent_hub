# AgentHub — Tasks

## In Progress

### AgentHub Fix Sprint (2026-03-31)

#### BUG-6: TestTask fail path slot leak (Dev1) — P0
- In `internal/task/handler.go`, `TestTask` fail path (line ~666) decrements the **tester's** `current_tasks`, but the tester never claimed the task — the **assignee (dev)** did
- Fix: use the task's `assignee` field to decrement the correct agent's counter, same as `CompleteTask` does on line 648
- Verify: `ReviewTask` fail path doesn't have this issue (it correctly doesn't decrement anyone)
- Test: add unit test for TestTask fail path verifying assignee's current_tasks decremented

#### IMPROVE-14: Dashboard route rename + error handling (Dev2) — P1
- Rename route: `/api/dashboard` → `/api/dashboard/stats` in `internal/dashboard/handler.go` RegisterRoutes
- Fix error handling: the dashboard does 3 separate queries without checking errors on most of them (lines 14-21). If the DB query fails, the count stays 0 — silently wrong
- Fix: check each query error, return 500 if critical queries fail
- Extract the duplicate anonymous struct (used for `recentEvents` var + nil check) into a named type `DashboardEvent`
- Update `octeam-driver.sh` if it references the old route

#### IMPROVE-15: GetQueue consolidation (Dev1) — P1
- In `internal/agent/handler.go`, `GetQueue` does 3 separate queries to get `skills`, `current_tasks`, `max_tasks` from the agents table (lines 133-136)
- Consolidate into a single query: `SELECT skills, current_tasks, max_tasks FROM agents WHERE name = $1`
- Same query for all 3 fields, no reason for 3 roundtrips

#### IMPROVE-16: CORS multi-origin support (Dev2) — P2
- `middleware/cors.go` reads `CORS_ALLOWED_ORIGINS` env but sets the entire string as a single `Access-Control-Allow-Origin` header
- Fix: parse comma-separated origins, match against `Origin` request header dynamically
- If origin doesn't match, don't set the header (browser will block)
- Keep `*` as default when env is empty

#### IMPROVE-17: Agent registration rate limiting (Dev1) — P2
- `internal/agent/handler.go` `RegisterAgent` has no rate limiting
- Add simple in-memory rate limiter (e.g. 5 registrations per minute per IP)
- Use `sync.Map` or a simple map with mutex + cleanup goroutine
- Return 429 when rate exceeded

#### IMPROVE-18: Admin seeding skip bcrypt on no-op (Dev2) — P2
- Check if there's a seed/migration that hashes the admin password every time
- If INSERT uses ON CONFLICT DO NOTHING, the bcrypt hash is wasted — skip it when no insert happens
- Likely in `internal/db/` seed or migration

#### IMPROVE-19: gofmt cmd/server/main.go (Dev1) — P3
- Run `gofmt -w cmd/server/main.go` — only import ordering issues
- Trivial but backlog item

## Next Up (P3)
- WH-4: Role prompt templates
- WH-5: Parallel executor with worktree isolation

## Backlog
- WH-6: Fix iteration loop (reuse session, max 5 rounds)
- WH-7: Question handler (auto-answer vs escalate)
- WH-8: Structured output parsing

## Completed

### Bug Fix Sprint (2026-03-30)
- BUG-1 through BUG-5: All fixed ✅

### Improvement Sprint (2026-03-30)
- IMPROVE-1 through IMPROVE-13: All done ✅

### Worker Rewrite (2026-03-30)
- WH-1: OpenCode HTTP Client ✅
- WH-2: Context Builder ✅
- WH-3: Generic Worker Core ✅
- WH-4: CLI Entrypoint ✅
- 127/128 tests pass ✅

### E2E Testing (2026-03-30)
- 10 E2E test cases designed ✅
- E2E tests executed, 5 tasks created ✅
- Stale tasks cleaned (2026-03-31) ✅
