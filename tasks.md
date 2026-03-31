# AgentHub — Tasks

## Completed

### AgentHub Fix Sprint (2026-03-31) ✅

#### BUG-6: TestTask fail path slot leak — P0 ✅
- Fixed: `TestTask` fail path now decrements the **assignee's** `current_tasks`, not the tester's
- Added: `ClaimTask` now increments `current_tasks` on claim
- Test: E2E verified — slot released on test fail

#### IMPROVE-14: Dashboard route + error handling + dedup — P1 ✅
- Route renamed: `/api/dashboard` → `/api/dashboard/stats`
- Queries consolidated: 4 task queries → 1 with FILTER, 3 agent queries → 1 with FILTER
- Named struct `DashboardEvent` replaces anonymous struct
- Proper error handling added

#### IMPROVE-15: GetQueue consolidation — P1 ✅
- 3 separate agent queries → 1 query
- Already done in earlier session

#### IMPROVE-16: CORS multi-origin — P2 ✅
- Parses comma-separated `CORS_ALLOWED_ORIGINS`
- Matches request `Origin` header dynamically
- Falls back to `*` when env empty
- Tests added

#### IMPROVE-17: Rate limiting — P2 ✅
- Middleware-based rate limiting via `golang.org/x/time/rate`
- Auth paths: 5/min per IP
- General paths: 60/min per IP
- X-Forwarded-For support for reverse proxies
- Tests added

#### IMPROVE-18: Admin seeding optimization — P2 ✅
- Skips bcrypt hash when admin already exists
- Single query check before expensive hash

#### IMPROVE-19: gofmt — P3 ✅
- Import ordering fixed

### E2E Tests Verified ✅

**Happy Path:**
1. Create → available
2. Claim → claimed
3. Progress → in_progress
4. Complete → done
5. Review pass → test
6. Test pass → deployed
7. Slot freed, total_completed++

**Review Fail:**
1-4 same
5. Review fail → needs_fix
6. Slot still held (dev needs to fix)

**Test Fail:**
1-5 same (review pass)
6. Test fail → needs_fix
7. Slot freed (BUG-6 fix verified)

## Next Up

### Go Worker Integration
- Wire up systemd services for `agenthub-worker dev|review|test`
- Test with OpenCode server running
- Deploy to production

### Frontend
- React dashboard for task management
- Real-time updates via WebSocket

## Backlog
- WH-6: Fix iteration loop (reuse session, max 5 rounds)
- WH-7: Question handler (auto-answer vs escalate)
- WH-8: Structured output parsing
