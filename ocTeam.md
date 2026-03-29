# ocTeam — OpenClaw Development Team

ocTeam is the AI development team managed by PM (me). We build software via the **AgentHub** platform.

## Members

| Role | Agent | Model | Purpose |
|------|-------|-------|---------|
| Developer 1 | `dev1` | daoduc/coding | Frontend, React, UI |
| Developer 2 | `dev2` | daoduc/coding | Backend, Go, workers |
| Code Reviewer | `reviewer` | daoduc/agentic | Security, correctness, quality |
| Tester | `tester` | daoduc/agentic | Validation, regression, integration |

## Trigger Command

**`/ocTeam [task description]`** — Tuyên asks ocTeam to implement something.

Example: `/ocTeam build a user profile page with avatar upload`

## Workflow (auto-driven)

```
1. PM receives /ocTeam [task]
2. PM breaks task into subtasks
3. PM creates tasks in AgentHub API
4. Devs poll queue, claim, implement
5. PM monitors via ocTeam Driver cron
6. Reviewer reviews code
7. Tester validates
8. PM reports DONE to Tuyên
```

**Full cycle (never skip):**
```
Implement → Review → Fix → Re-review → Test → Fix → Re-test → DONE
```

## ocTeam Driver Cron

An isolated agent runs every 10 minutes to:
- Check agent activity (last message timestamp)
- Check AgentHub API health
- Check task progress
- Nudge stalled agents (>15 min no response)
- Send to Reviewer when dev done
- Send to Tester when reviewer passes
- Report blockers to Tuyên
- Self-heal: if agent session dead, restart assignment

## Communication

All agent-to-agent communication via **AgentHub API** (not sessions_send which is unreliable).

Devs report completion via sessions_send to PM.
PM drives via sessions_send to agents.

## Quality Rules

1. NO code ships without Reviewer approval
2. NO feature ships without Tester validation
3. NO skipping steps — even "small" fixes go full cycle
4. PM always reports to Tuyên after each stage
