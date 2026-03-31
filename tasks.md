# AgentHub — Task Board

## Project
- **Project:** agenthub
- **Repo:** `/root/.openclaw/workspace-pm/projects/agenthub/`
- **Tech:** Go (Gin) + PostgreSQL + Docker
- **API:** http://localhost:8081
- **Branch:** main

---

## Phase 1: Worker Framework (Current)

### Available (Backlog)
1. **Task type field** — Add `type` enum to tasks: `coding`, `review`, `testing`, `planning` (high)
2. **Worker registration** — POST /api/workers/register with role, auth token (high)
3. **Filtered task polling** — GET /api/tasks?status=available&type=coding (high)
4. **Task claiming by worker** — PUT /api/tasks/:id/claim with worker_id (high)
5. **Auto-branch on claim** — Worker creates `feature/<id>-<slug>` branch (medium)

### Design Needed
1. **Worker binary structure** — How workers poll, execute, report back
2. **OpenCode integration** — How coding worker invokes OpenCode for implementation
3. **Review worker logic** — Diff analysis, comment posting, pass/fail criteria
4. **Team Leader worker** — Codebase scanning → improvement plan → task creation
5. **Merge gate** — PM approves before feature branch merges to main

### In Progress
1. **WebSocket Events — Backend** (dev2) — 6 review issues, needs_fix

### Completed ✅
1. Core API: tasks CRUD, auth (JWT + API key), queue management
2. Task Comments — full CRUD with ownership + auth (reviewed + tested)
3. Bug fixes: slot leaks, status guards, GetQueue 3→1 query, rate limiting
4. 16 files gofmt'd
5. Live smoke tests passing

---

## Phase 2: Self-Improvement (After Phase 1)
- Team Leader worker scans AgentHub codebase every 2h
- Creates 1-2 improvement tasks automatically
- Workers pick up tasks, work on branches, never touch main
- PM reviews plans and approves merges

## Phase 3: Multi-Project Support
- Workers can target any project repo
- Project config in AgentHub (repo path, branch, tech stack)
- Dashboard shows cross-project task status

---

## Monitoring
- Build guard: isolated cron, every 10min, announce only on failure
- Daily sprint digest: 8 AM
- Session health: every 30min, reset >80% token usage

## Commits
- `fdc01e1` — fix: done→needs_fix/escalated transitions + delete slot release
