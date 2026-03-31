# ocTeam Worker Bridge — Design Specification

> **Status:** Draft  
> **Author:** PM  
> **Date:** 2026-03-31  
> **Replaces:** `internal/worker/` (core.go, api.go, types.go, prompts.go) and `workers/` package  
> **Design basis:** `openclaw agent --json` CLI + AgentHub queue API

---

## 1. Overview

### What This Is

A **bridge worker** that replaces the OpenCode dependency in the existing Go worker with `openclaw agent` CLI calls. The worker polls AgentHub for tasks, delegates work to ocTeam agents (dev1, dev2, reviewer, tester) via `openclaw agent`, monitors execution, verifies results, and reports back to AgentHub.

### What This Is NOT

- A new AgentHub server component
- A rewrite of the AgentHub API
- A new task queue or scheduler
- A replacement for `openclaw agent` itself

### Key Design Principle

The bridge is **stateless at the task level** — it receives a task, delegates it, waits, verifies, reports. Session state is tracked only for monitoring/recovery (session IDs stored in memory, not persisted across restarts).

---

## 2. Architecture

### Language & Packaging

- **Language:** Go (keeps existing AgentHub dependencies, avoids a new runtime)
- **Package:** `internal/workerbridge/`
- **Entry point:** `cmd/workerbridge/main.go`
- **Config:** environment variables (no config file required)

### Component Layout

```
agenthub/
├── cmd/
│   └── workerbridge/
│       └── main.go              # entry point, signal handling, main loop
├── internal/
│   ├── workerbridge/
│   │   ├── bridge.go            # core state machine: poll → claim → delegate → monitor → verify → report
│   │   ├── agent.go             # openclaw agent invocation wrapper
│   │   ├── monitor.go           # session monitoring (idle, aborted, timeout detection)
│   │   ├── recovery.go          # orphaned task detection and session transcript recovery
│   │   ├── verifier.go          # result parsing (dev/review/test-specific)
│   │   ├── router.go            # task_type → agent routing
│   │   └── config.go           # config structs + env var loading
│   └── worker/
│       └── api.go               # [REUSE] AgentHub API client (already exists)
└── docker-compose.yml           # add workerbridge service
```

### Process Model

```
┌──────────────────────────────────────────────────────────────────┐
│  workerbridge process (one per role: dev, review, test)          │
│                                                                  │
│  main loop (sequential, non-blocking per task)                    │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  POLL  →  CLAIM  →  DELEGATE  →  MONITOR  →  VERIFY  →  REPORT  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  Background goroutines:                                          │
│  - Heartbeat pinger (every 5 min to AgentHub)                    │
│  - Stale session monitor (every 2 min)                           │
│  - Task timeout watcher (per-task, kills session on exceed)        │
└──────────────────────────────────────────────────────────────────┘
```

### Concurrency

- **One task per agent at a time.** The worker holds only one task's context at once; the `openclaw agent` process itself is the work unit.
- **Sequential polling.** The main loop polls, claims one task, processes it to completion, then polls again. This avoids lock contention and session conflicts.
- **Up to 3 concurrent worker processes** (dev1, dev2, reviewer) running simultaneously as separate OS processes or goroutine pool workers within one process. Each operates on a different AgentHub queue type.

### Deployment

Three instances of the same binary, distinguished by `--role`:

```bash
# Terminal 1
./workerbridge --role=dev --agent-id=dev1 --api=http://localhost:8081

# Terminal 2
./workerbridge --role=dev --agent-id=dev2 --api=http://localhost:8081

# Terminal 3
./workerbridge --role=review --agent-id=reviewer --api=http://localhost:8081
```

Or as systemd services / Docker containers. The `--role` flag selects:
- Which AgentHub queue to poll (`?task_type=dev`, `?task_type=review`, `?task_type=test`)
- Which `openclaw agent --agent <id>` to invoke
- Which result verifier to use

---

## 3. Task Lifecycle

Each task goes through the following state machine:

```
POLL → CLAIM → DELEGATE → MONITOR → VERIFY → REPORT
  │                    │           │         │
  └─ no tasks         timeout      fix       fail
  └─ claim fail       abort        loop      done
```

### 3.1 POLL

```
GET /api/agent/tasks/queue?task_type={role}
```

- Uses the existing `internal/worker/api.go` API client (reuse unchanged).
- Returns `nil` if queue is empty (HTTP 204) — wait `poll_interval`, retry.
- Returns first task if any available.

### 3.2 CLAIM

```
POST /api/agent/tasks/:id/claim
```

- Claim the task to prevent other workers from picking it up.
- If claim fails (409 Conflict), skip to POLL immediately.
- Update task progress to 10% on successful claim.

### 3.3 DELEGATE

Construct a prompt from the task and invoke `openclaw agent`:

```bash
openclaw agent \
  --agent {agentId} \
  --session-id {sessionKey} \
  --message "{builtPrompt}" \
  --timeout {taskTimeout} \
  --json
```

**Session key strategy:**
- If resuming: reuse the session key stored from a previous attempt (from `recovery.go`).
- If new task: omit `--session-id` to let `openclaw agent` create a fresh session.

**Built prompt** is assembled from:
- Task title + description
- Project directory (from payload)
- Context block (git diff, affected files — see §3 Context Injection)

**Timeout:** Default 30 minutes. Configurable via `TASK_TIMEOUT_MINUTES` env var (default: 30).

### 3.4 MONITOR

While the `openclaw agent` process is running, the bridge runs three concurrent monitors:

#### Monitor 1: Session Idle (>5 min no activity)

Poll `openclaw sessions --agent {agentId} --active N --json` every 2 minutes while a task is in progress.

- If `updatedAt` of the task's session is stale (>5 min ago), the agent may be stuck in a long thought or waiting on external input.
- **Action:** Send a nudge message to the same session: `"Are you still working? If blocked, summarize your current status and what you need to continue."`
- If the agent was already nudged and still idle after another 5 min → abort the session and fail the task with `"agent_idle"` reason.

#### Monitor 2: Session Aborted

Check `abortedLastRun` field in the session list response.

- If `true`, the agent's last run was interrupted (SIGTERM, crash, OOM).
- **Action:** Attempt recovery via session transcript (see §5 Recovery).

#### Monitor 3: Task Timeout (>30 min)

The `openclaw agent --timeout` flag is the hard ceiling. If that timeout fires:

- The process exits with a non-zero code or the bridge detects the timeout via process state.
- **Action:** Mark task as failed with `"timeout"` reason.

### 3.5 VERIFY

Parse the agent's JSON response to extract the result. Three role-specific verifiers:

#### Dev Verifier
1. Parse `result.payloads[].text` — concatenate all text payloads.
2. Run `go build ./...` in the project directory.
   - If build fails: mark task as failed with build output.
   - If build succeeds: extract `files_changed` and `commit_hash` from the agent's text output (regex patterns below).
3. If no files changed detected and no error: warn but allow (agent may have decided task was no-op).

#### Review Verifier
1. Parse `result.payloads[].text` for structured output.
2. Extract issues from text using ISSUES section (see `internal/worker/core.go` existing patterns — reuse).
3. Parse `VERDICT: PASS` / `VERDICT: FAIL` line.
4. Classify severity from issue content.
5. If no structured output found: use heuristics (presence of "lgtm", "approved", emoji patterns).

#### Test Verifier
1. Parse `result.payloads[].text` for test counts.
2. Use existing `parseTestCounts()` from `internal/worker/core.go` (reuse).
3. If exit code is non-zero from a test runner invocation (agent ran tests internally): extract from output.
4. Report `passed`, `failed`, `skipped` counts.

### 3.6 REPORT

```
# Dev
POST /api/agent/tasks/:id/complete
{
  "status": "done",
  "notes": "<summary output>",
  "data": { "files_changed": [...], "commit_hash": "..." }
}

# Review
POST /api/agent/tasks/:id/review
{
  "verdict": "pass|fail",
  "severity": "critical|major|minor",
  "issues": [...]
}

# Test
POST /api/agent/tasks/:id/test
{
  "verdict": "pass|fail",
  "passed": N,
  "failed": N,
  "skipped": N,
  "failures": [...]
}
```

On failure (build error, agent error, timeout):
```
POST /api/agent/tasks/:id/complete
{
  "status": "failed",
  "notes": "<error description>"
}
```

---

## 4. Agent Routing

### Route Table

| `task_type` | Queue filter (`?task_type=`) | `--agent <id>` | Role |
|------------|-------------------------------|----------------|------|
| `dev` | `dev` | `dev1` or `dev2` (round-robin or least-busy) | dev |
| `review` | `review` | `reviewer` | review |
| `test` | `test` | `tester` | test |

### Dev Agent Selection (dev1 vs dev2)

The existing AgentHub queue already returns tasks with a `match_score` and `assignee` hint. For tasks that aren't pre-assigned:

1. Poll `GET /api/agent/tasks/queue?task_type=dev` — gets all available dev tasks.
2. For each worker instance (dev1, dev2), maintain a `lastAssigned map[string]int64` to track round-robin position.
3. Alternate between `dev1` and `dev2` on each poll cycle.
4. If a task has `assignee` set (pre-assigned by PM), use that agent regardless.

### Agent Capacity

Agents have a `current_tasks < max_tasks` constraint enforced by AgentHub. The bridge does NOT enforce this — it relies on the queue endpoint to not return tasks when the agent is at capacity. If a claim fails (409), the bridge skips and polls again.

---

## 5. Monitoring

### Three Signals

| Signal | Detection | Threshold | Action |
|--------|-----------|-----------|--------|
| **Session idle** | `updatedAt` from `openclaw sessions --active N --json` | >5 min since last session update | Send nudge; if still idle after 2nd check → abort |
| **Session aborted** | `abortedLastRun == true` in session list | Any | Attempt recovery via transcript |
| **Task timeout** | Process exit or hard timeout | >30 min (`--timeout` flag) | Fail with `"timeout"` reason |

### Monitoring Goroutines

All monitors run as background goroutines within the bridge process:

```
main goroutine:         poll → claim → delegate → (wait) → verify → report
                        ↑__________________________________|

monitor goroutine:      idle monitor (polls sessions every 2 min while task active)
abort goroutine:        watches for SIGCHLD / aborted signal
heartbeat goroutine:    sends PATCH /progress every 5 min while task active
```

### Structured Logging

Every lifecycle event is logged with a structured JSON line:

```json
{"level":"INFO","ts":"2026-03-31T16:00:00Z","event":"task_claimed","task_id":"abc123","agent":"dev1","role":"dev"}
{"level":"INFO","ts":"2026-03-31T16:05:00Z","event":"agent_delegated","task_id":"abc123","session_id":"sess-xyz","agent":"dev1"}
{"level":"INFO","ts":"2026-03-31T16:10:00Z","event":"session_idle","task_id":"abc123","session_id":"sess-xyz","idle_minutes":5}
{"level":"INFO","ts":"2026-03-31T16:15:00Z","event":"session_nudged","task_id":"abc123","session_id":"sess-xyz"}
{"level":"WARN","ts":"2026-03-31T16:20:00Z","event":"session_aborted","task_id":"abc123","session_id":"sess-xyz"}
{"level":"INFO","ts":"2026-03-31T16:25:00Z","event":"task_completed","task_id":"abc123","verdict":"pass","duration_seconds":1500}
```

---

## 6. Recovery

### 6.1 Orphaned Tasks (Worker Crash)

When the bridge process crashes or is killed while a task is in progress:

1. The task remains in `claimed`/`in_progress` status on AgentHub.
2. The stale task monitor (existing `StartStaleTaskMonitor` in `internal/task/monitor.go`) will auto-release the task after 30 min of no progress.
3. When a new bridge instance starts, it polls the queue. The released task becomes available.
4. The new instance claims it and processes it as a new task (no session to resume — the old session on the agent side is orphaned).

**Alternative (if session persistence is needed):**
- On each delegation, store `{taskID → sessionID}` in a local file (`/tmp/workerbridge/sessions.json`).
- On startup, scan for any `{taskID → sessionID}` where the task is still `claimed`/`in_progress`.
- Attempt to resume: `openclaw agent --agent {agent} --session-id {sessionId} --message "{status check prompt}"`.
- If session is recoverable (not aborted, not too stale), continue from transcript. Otherwise, fail and let PM handle.

### 6.2 Session Aborted Mid-Task

If `abortedLastRun == true` is detected during monitoring:

1. Attempt to get the session transcript: `openclaw sessions --agent {agent} --active 60 --json` to find the session.
2. If session still exists and is recent (<60 min old):
   - Extract the last user message from the transcript.
   - Send a recovery prompt: `"Your previous attempt was interrupted. Here is what you were doing: <last_message>. Here is what happened: interrupted. Please continue from where you left off. If the task is already complete, report that. If you cannot continue, explain why."`
3. If session is gone or too old: fail the task with `"agent_aborted"` reason.

### 6.3 Git Conflict

If the agent encounters a git conflict:
- The agent's output will contain conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`).
- The verifier detects this and marks the task as failed with `"git_conflict"` reason.
- The task is released back to the queue for manual resolution by PM.

### 6.4 Container/Process Restart

Each bridge instance writes a PID file to `/tmp/workerbridge/{role}.pid` on startup.
- If the PID file exists and the process is still alive, refuse to start (prevent double-instance).
- On clean shutdown (SIGTERM), delete the PID file.

---

## 7. Heartbeat

### AgentHub Heartbeat

While a task is in progress, the bridge sends a heartbeat to AgentHub every **5 minutes**:

```
PATCH /api/agent/tasks/:id/progress
{ "progress": 30 }  # at delegation
{ "progress": 60 }  # after monitor confirms active
{ "progress": 90 }  # after verify, before report
```

This keeps `tasks.updated_at` fresh, preventing the stale task monitor from releasing an in-progress task.

### OpenClaw Session Heartbeat

The bridge does NOT send heartbeats to OpenClaw directly — `openclaw agent` manages its own session lifecycle. The bridge only monitors via `openclaw sessions --active N`.

---

## 8. Context Injection

Before delegating, the bridge assembles a rich context block. This replaces the `internal/context/builder.go` logic but calls the same underlying git/shell commands.

### Context Block Structure

```
## Task: {title}
## Project: {project_name} (branch: {branch})
## Affected Files: {files}

## Recent Commits:
{last 5 commits in --oneline format}

## Git Diff:
{full diff of last commit vs HEAD~1, limited to affected files}

## Constraints:
- Do NOT modify: {constraint_files}
- Build: {build_cmd}
- Test: {test_cmd}

## Description:
{title}\n{description}
```

### Implementation

- Reuse `internal/context/builder.go` functions (`GetGitDiff`, `GetRecentCommits`, `LoadConventions`) directly — import the package.
- Call from `agent.go` before constructing the prompt.

---

## 9. Verification

### Dev Verification Flow

```
1. Parse agent JSON response → extract result.payloads[].text
2. Run: go build ./...  (or project-specific build from payload)
   ├─ Success → extract files from output
   │   ├─ files_changed (regex: "changed/modified/created: <list>")
   │   └─ commit_hash   (regex: "commit: <hash>")
   └─ Failure → fail task with build error output
3. Check for git conflict markers in result text
4. If no files changed and build passes → warn, report anyway
```

### Review Verification Flow

Reuse `extractReviewIssues`, `isApproved`, `hasCriticalIssues` from `internal/worker/core.go`.

### Test Verification Flow

Reuse `parseTestCounts`, `extractFailedTestNames` from `internal/worker/core.go`.

### Edge: Agent Returns "Already Done"

If the agent's output contains patterns like:
- `"already done"`, `"nothing to do"`, `"task complete"`, `"no changes needed"`

The bridge checks:
1. Was `go build` clean? If yes, report as `done` with no files changed.
2. Was the build not run? Run it to verify. If build passes, report `done`. If build fails, treat as a regular failure.

---

## 10. Edge Cases

| Edge Case | Handling |
|-----------|----------|
| **Agent stuck in infinite loop** | Monitor detects idle >5 min → nudge → if still idle 5 min later → SIGTERM the process → fail with `"agent_stuck"` |
| **Git conflict during work** | Verifier detects `<<<<<<<` markers → fail with `"git_conflict"` reason → task released to queue for PM |
| **Build fails** | Build command exits non-zero → fail task with stdout/stderr → task released to queue |
| **Agent returns "already done"** | Run `go build` to verify → if clean, report as `done` with no files changed |
| **Container restart mid-task** | PID file deleted on clean shutdown only → stale task auto-releases after 30 min → new instance picks it up as new |
| **openclaw agent process hangs** | Hard timeout via `--timeout` flag (30 min default) → process killed → fail with `"timeout"` |
| **Two workers claim same task** | AgentHub enforces one claim; second worker gets 409 → skips to next poll |
| **Session aborted (SIGKILL on agent)** | Monitor detects `abortedLastRun == true` → attempt session transcript recovery → retry or fail |
| **Network error calling openclaw** | Retry once with 10s backoff → if still fails, fail task with `"agent_unavailable"` |
| **Malformed JSON from openclaw --json** | Log raw output, fail task with `"agent_malformed_response"` |
| **Agent exits 0 but output is empty** | Treat as success but warn → report with empty output |
| **openclaw sessions list returns empty** | If task is supposedly active but no session found → assume aborted → attempt recovery |
| **Task timeout < monitor interval** | The `--timeout` flag on `openclaw agent` is the governing timeout; monitors are informational |

---

## 11. Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENTHUB_URL` | `http://localhost:8081` | AgentHub API base URL |
| `AGENT_TOKEN` | *(required)* | Agent auth token |
| `WORKER_ROLE` | `dev` | Worker role: `dev`, `review`, `test` |
| `AGENT_ID` | *(required)* | OpenClaw agent ID to invoke: `dev1`, `dev2`, `reviewer`, `tester` |
| `POLL_INTERVAL_SECONDS` | `10` | Seconds between queue polls |
| `TASK_TIMEOUT_MINUTES` | `30` | Hard timeout for agent execution |
| `IDLE_THRESHOLD_MINUTES` | `5` | Minutes without session activity before nudge |
| `NUDGE_LIMIT` | `2` | Max nudges before aborting a stuck session |
| `LOG_FORMAT` | `json` | Log format: `json` or `text` |
| `WORK_DIR` | `/tmp/workerbridge` | Working directory for PID files and session tracking |

### CLI Flags (override env vars)

```
--role        dev|review|test     (default: dev)
--agent-id    string              (required)
--api         string              (default: http://localhost:8081)
--timeout     int (minutes)       (default: 30)
--poll        int (seconds)       (default: 10)
--log         json|text            (default: json)
```

---

## 12. Comparison: Old vs New

| Aspect | Old (opencode serve) | New (openclaw agent) |
|--------|---------------------|----------------------|
| Session management | HTTP API, custom client | Built into CLI, session keys |
| Context injection | Builder package + session creation | Pass context in prompt on each call |
| Diff retrieval | `session summary` endpoint | Parse from agent output text |
| Monitoring | `WaitForResult` polling | `openclaw sessions --active N --json` |
| Recovery | Session ID stored, reused | Same session key + nudge pattern |
| Result format | `MessageResponse.parts[].text` | `result.payloads[].text` |
| Concurrency | Goroutine pool in Go | Separate OS processes or goroutine pool |
| Dependencies | `opencode` binary + HTTP API | `openclaw` binary only |

---

## 13. Implementation Phases

### Phase 1: Core Bridge (P0)
- `config.go` — env vars + struct
- `api.go` — reuse `internal/worker/api.go`
- `agent.go` — `openclaw agent` invocation + JSON parsing
- `bridge.go` — main loop: poll → claim → delegate → wait → verify → report
- `cmd/workerbridge/main.go` — entry point, signals

### Phase 2: Monitoring & Recovery (P1)
- `monitor.go` — idle detection, aborted detection, nudging
- `recovery.go` — session transcript recovery
- Heartbeat goroutine

### Phase 3: Verification (P1)
- `verifier.go` — dev/review/test verifiers (import existing funcs from `internal/worker/core.go`)
- Build verification for dev role
- Structured output parsing for review/test

### Phase 4: Routing & Polish (P2)
- `router.go` — dev1/dev2 round-robin selection
- PID file management
- Structured logging
- Docker compose integration

---

## 14. Reuse Checklist

The following are **already implemented** and must be reused (not rewritten):

- [x] `internal/worker/api.go` — AgentHub API client (HTTP, JSON, auth)
- [x] `internal/worker/types.go` — `Task`, `TaskResult`, `ReviewResult`, `TestResult` structs
- [x] `internal/worker/core.go` — `extractReviewIssues`, `isApproved`, `hasCriticalIssues`, `parseTestCounts`, `extractFailedTestNames`, `stripThinking`
- [x] `internal/worker/prompts.go` — `DevPrompt`, `ReviewPrompt`, `TestPrompt` templates
- [x] `internal/context/builder.go` — `BuildContext`, `GetGitDiff`, `GetRecentCommits`, `LoadConventions`
- [x] `internal/task/monitor.go` — `StartStaleTaskMonitor` (server-side, not bridge-owned)
- [x] `docker-compose.yml` — add workerbridge service definitions

---

*Last updated: 2026-03-31*
