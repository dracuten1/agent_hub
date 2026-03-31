# AgentHub — AI Worker Task Orchestration Platform

## Overview
AgentHub is a task orchestration platform where **AI workers plug in as virtual agents**. Instead of humans managing tasks, AI workers (coding, review, testing, planning) poll the API, claim tasks matching their role, execute work on feature branches, and report results — all coordinated through AgentHub.

The platform dogfoods itself: the ocTeam uses AgentHub to manage their own improvement work.

## Architecture

```
                    ┌─────────────────────┐
                    │   Team Leader Worker│  Scans codebase, creates improvement plans
                    │   (planning agent)  │  Breaks down into tasks, assigns priority
                    └──────────┬──────────┘
                               │ POST /api/tasks
                               ▼
                    ┌─────────────────────┐
                    │    AgentHub API     │  Task queue, status, assignment, branches
                    │   (Go + PostgreSQL)  │  Auth, WebSocket events, comments
                    └──┬─────┬─────┬─────┘
                       │     │     │
              ┌────────┘     │     └────────┐
              ▼              ▼              ▼
     ┌────────────┐  ┌────────────┐  ┌────────────┐
     │  Coding    │  │  Review    │  │   Testing  │
     │  Worker    │  │  Worker    │  │  Worker    │
     │ (OpenCode) │  │ (diff/AST) │  │ (go test)  │
     └─────┬──────┘  └─────┬──────┘  └─────┬──────┘
           │               │               │
           ▼               ▼               ▼
     Feature branch    Review comments   Test results
     → moves to        → PASS/needs_fix  → PASS/needs_fix
       review                              → merge
```

## Worker Lifecycle

1. **Register** — Worker connects with role type (coding/review/testing/planning)
2. **Poll** — GET /api/tasks?status=available&type=coding
3. **Claim** — PUT /api/tasks/:id/claim (moves to `in_progress`)
4. **Execute** — Create feature branch, implement, commit
5. **Report** — Update task status: `done` (for review) or `needs_fix` (blocker)
6. **Next** — Poll again

## Branch Strategy
- Every task gets its own branch: `feature/<task-id>-<slug>`
- Workers NEVER commit to `main`
- Only PM-approved merges reach `main`
- If a task breaks, branch is discarded — main stays clean

## Tech Stack
- **Backend:** Go 1.21, Gin, PostgreSQL 15, sqlx, JWT auth
- **Frontend:** React (Vite) dashboard
- **Workers:** Standalone binaries using OpenCode for coding tasks
- **Infra:** Docker Compose

## Components
- **API Server** (port 8081) — REST API + WebSocket events
- **Workers** — Role-specific binaries (systemd or standalone)
- **Web Dashboard** — React frontend for monitoring
- **Task Types:** `coding`, `review`, `testing`, `planning`

## Current Status

### Completed ✅
- Core API: tasks CRUD, auth (JWT + API key), queue management
- Task Comments (full CRUD with ownership + auth)
- WebSocket events (task status changes)
- Bug fixes: slot leaks, status guards, query optimization
- 16 files gofmt'd

### In Progress 🔧
- WebSocket Events — Backend (review fixes)
- Worker framework design (this doc = the plan)

### Backlog 📋
- Worker registration & auth
- Task type field + filtered polling
- Auto-assignment (claim by role)
- Team Leader worker (codebase scanning → task creation)
- Coding worker (branch + OpenCode integration)
- Review worker (diff analysis → pass/needs_fix)
- Test worker (go test → pass/needs_fix)
- Enhanced Health Check

## Repo
- **Local:** `/root/.openclaw/workspace-pm/projects/agenthub/`
- **API:** http://localhost:8081
- **Branch:** main (workers always work on feature branches)
