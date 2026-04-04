# Feature Plan: Workflow Self-Driving Engine

## Vision
PM and Team Leader become workflow workers — polling the queue like dev/reviewer/tester.
The workflow drives itself. Only 2 human gates pause everything.
One lightweight watcher cron handles all external communication.

## Current Problems
1. PM operates outside the workflow — manually creates tasks, updates DB, nudges agents
2. No auto-advancement — dev finishes but review/test never gets queued automatically
3. Engine gaps — `SELECT *`, no error handling, verdicts not parsed, phases auto-advance on 0 tasks
4. Reviewer verdicts lost — written in text but not stored in DB field
5. Human gates have no notification — Tuyên doesn't know when approval is needed

## Architecture

### Workflow Template: 11wf v2

```
Phase 0: PM Plan (gate)         → PM bridge auto-approves
Phase 1: Design (single)        → Team Leader bridge writes spec
Phase 2: PM Spec Review (gate)  → PM bridge reviews spec, approve/reject
Phase 3: Owner Approval (gate)  → HUMAN GATE — pauses, watcher notifies Tuyên
Phase 4: Development (multi)    → dev bridges pick up tasks
Phase 5: Code Review (per_dev)  → reviewer bridge picks up (auto-queued after dev done)
Phase 6: Testing (per_dev)      → tester bridge picks up (auto-queued after review pass)
Phase 7: Quality Gate (decision)→ engine evaluates conditions
Phase 8: PM Review (gate)       → PM bridge reviews combined output
Phase 9: Owner Deploy (gate)    → HUMAN GATE — pauses, watcher notifies Tuyên
Phase 10: Deploy (single)       → dev bridge picks up
```

### Phase Types
- `gate` — needs someone to approve. Subtypes: `agent_gate` (bridge picks up) vs `human_gate` (pause + notify)
- `single` — one task, one agent
- `multi` — multiple tasks, multiple agents can work in parallel
- `per_dev` — one review/test task per dev task (auto-generated)
- `decision` — engine evaluates conditions (pass/fail/retry)

### Auto-Queue Rules (Engine)
When a task completes in a phase:
1. **Dev task done** → auto-queue review task (type=review) in next phase, link to same workflow_task_map
2. **Review pass** → auto-queue test task (type=test) in next phase
3. **Review fail** → requeue dev task (increment retry_count), if max retries → escalate
4. **Test pass** → mark review task as test_passed
5. **Test fail** → requeue dev task, if max retries → escalate
6. **All tasks in phase done** → advance to next phase
7. **Phase is human_gate** → pause, set status=waiting_approval, watcher handles notification
8. **Phase is agent_gate** → create a gate decision task, bridge picks up

### Worker Bridges
Each bridge polls the queue and picks up tasks matching its role:

| Agent | Role | Bridge | What it does |
|-------|------|--------|-------------|
| PM | pm | pm bridge | Reads context, approves/rejects gates, reviews specs, breaks down into tasks |
| Team Leader | designer | tl bridge | Reads plan, writes detailed design spec |
| Dev1 | dev | dev1 bridge | Implements code changes |
| Dev2 | dev | dev2 bridge | Implements code changes |
| Reviewer | review | reviewer bridge | Reviews code quality |
| Tester | test | tester bridge | Validates against spec |

**PM bridge task types:**
- `pm_plan` — confirm plan is ready (auto-pass)
- `pm_spec_review` — read spec, approve or reject with feedback
- `pm_review` — review combined code output, approve or send back

**Team Leader bridge task types:**
- `plan` — read plan description, write detailed design spec

### Watcher Cron (single, lightweight)
**Interval:** every 2 minutes
**Scope:**
1. Check for human gates with `status=waiting_approval`
2. If found → send Tuyên a message with:
   - What gate (spec approval or deploy approval)
   - The spec/summary content
   - Link to dashboard
3. Check for failed/escalated tasks
4. If found → send alert to PM session
5. If nothing → NO_REPLY

**PM bridge is NOT nudged by cron.** PM bridge polls the queue like every other worker.

## Implementation Tasks

### Batch 1: Engine Foundation
1. **Fix SELECT * everywhere** — Replace all ~15 `SELECT *` queries with explicit column lists across task/, project/, feature/, workflow/engine.go
2. **Add error handling to all db calls** — Log errors instead of silently swallowing
3. **Fix nullable column types** — Audit all structs, use pointer types for nullable columns
4. **Fix StartWorkflow first phase bug** — Line 649 sets PhaseRunning instead of PhaseActive
5. **Fix advanceWorkflow auto-advance on 0 tasks** — Phases with 0 total_tasks should not auto-complete

### Batch 2: Auto-Queue Engine
6. **Auto-queue review after dev** — When dev task completes, auto-create review task in review phase
7. **Auto-queue test after review pass** — When review passes, auto-create test task
8. **Review fail → requeue dev** — With retry tracking, max 2 retries then escalate
9. **Test fail → requeue dev** — Same retry logic
10. **Auto-advance phases** — When all tasks in a phase complete, advance to next phase
11. **Phase subtype: agent_gate vs human_gate** — Add config to phase template, engine behavior differs

### Batch 3: PM & Team Leader Bridges
12. **PM bridge** — New bridge process that uses `openclaw agent --agent pm` to poll and make decisions
13. **Team Leader bridge** — New bridge for design spec writing
14. **PM decision task format** — Structured task description with context (spec content, previous phase results, decision options)
15. **Bridge protocol: structured verdict** — Bridge output parsed for PASS/FAIL/REJECT + reason, stored in DB

### Batch 4: Watcher Cron
16. **Watcher cron** — Isolated session, checks human gates and failed tasks only
17. **Human gate notification format** — Rich message with spec/summary, approve link, reject option
18. **Dashboard gate approval** — Verify approve/reject buttons work on human_gate phases

### Batch 5: Template & UI
19. **Update 11wf template** — v2 with agent_gate/human_gate distinction
20. **Workflow detail page** — Show phase subtype (agent vs human gate), auto-queued task count
21. **Task result display** — Show task results in phase detail (already done, verify works end-to-end)

## Key Design Decisions
- **PM is a worker, not external** — no special treatment, polls queue like everyone else
- **No nudging** — cron only talks to Tuyên (human gates) or alerts on failures
- **Verdicts are structured** — bridge output must include `VERDICT: PASS|FAIL|REJECT` line, parsed by bridge into DB
- **Auto-queue is engine-level** — not a cron, built into `advanceWorkflow` and task completion handlers
- **Human gates are the only pause point** — everything else flows automatically

## Risks
- PM bridge could get stuck in a loop (approve → advance → approve) — mitigated by sequential phase processing
- Auto-queue could create infinite review cycles — mitigated by max retry count
- PM bridge making bad decisions without human oversight — mitigated by human gates at Phase 3 and 9
- Bridge token/auth issues — already encountered reviewer token typo, need robust error handling

## Effort Estimate
- Batch 1 (Engine fixes): 3-4 dev tasks
- Batch 2 (Auto-queue): 2-3 dev tasks
- Batch 3 (Bridges): 2 tasks (bridge config + PM/TL task handling)
- Batch 4 (Watcher): 1-2 tasks
- Batch 5 (Template + UI): 2 tasks
- **Total: ~12-14 tasks across 5 batches**
