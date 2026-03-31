# Dashboard — AgentHub Monitoring

## Task: Monitoring Dashboard + Worker Health Cron

### API Contract

#### GET /api/dashboard/summary
Returns combined overview for single page load.
- **Auth:** None (internal tool)
- **Response 200:**
```json
{
  "agents": [
    {
      "name": "dev1",
      "role": "developer",
      "status": "idle",
      "current_tasks": 1,
      "max_tasks": 3,
      "total_completed": 42,
      "total_failed": 3,
      "last_heartbeat": "2026-03-31T15:00:00Z",
      "online": true
    }
  ],
  "tasks": {
    "available": 5,
    "claimed": 1,
    "in_progress": 2,
    "done": 120,
    "failed": 8,
    "needs_fix": 3,
    "escalated": 1,
    "total": 140
  },
  "queue": {
    "dev": 3,
    "review": 0,
    "test": 1,
    "general": 2
  },
  "recent_tasks": [
    {
      "id": "uuid",
      "title": "string",
      "status": "string",
      "task_type": "string",
      "assignee": "string|null",
      "priority": "string",
      "progress": 50,
      "created_at": "ISO8601",
      "claimed_at": "ISO8601|null",
      "completed_at": "ISO8601|null"
    }
  ]
}
```

#### GET /api/dashboard/agents
Returns agent status summary only.
- **Response 200:** `{ "agents": [...] }` (same shape as above)

#### GET /api/dashboard/tasks
Returns task stats + recent tasks.
- **Query params:** `limit` (default 20, max 100)
- **Response 200:** `{ "tasks": {...stats}, "recent_tasks": [...] }`

#### GET /dashboard
Serves the HTML dashboard page.
- **Content-Type:** text/html
- Auto-refreshes every 10 seconds via JavaScript fetch to /api/dashboard/summary

### Behavior

**Agent online detection:**
- Agent is "online" if `last_heartbeat` is within 2 minutes of now
- Agent is "idle" if `status = 'idle'`
- Agent is "busy" if `current_tasks > 0`
- Agent is "offline" if `last_heartbeat` is older than 2 minutes

**Task counts:**
- Group by status from DB: `SELECT status, COUNT(*) FROM tasks GROUP BY status`
- Recent tasks: `ORDER BY updated_at DESC LIMIT ?`

**Queue depth:**
- `SELECT task_type, COUNT(*) FROM tasks WHERE status = 'available' GROUP BY task_type`

**Dashboard HTML:**
- Single embedded HTML file (no templates, no framework)
- CSS inline or in `<style>` block
- JavaScript fetches /api/dashboard/summary every 10s
- Color-coded agent cards: green=online idle, blue=online busy, red=offline
- Color-coded task status badges
- Responsive (works on mobile)

### Edge Cases

- No agents registered → agents array empty, no crash
- No tasks in any status → all counts 0
- Agent has NULL last_heartbeat → treated as offline
- Dashboard requested during DB migration → 500 with error message
- Empty queue → all queue depths 0

### Consistency Notes

- Follow existing handler pattern in `internal/task/handler.go`
- Use `sqlx.DB` for queries
- HTML embedded via `embed.FS` or inline string (no template files)
- Dashboard routes registered in main.go alongside existing routes
- No auth middleware on dashboard routes (internal tool)

### Worker Health Cron

**Cron job:** Every 20 minutes, check if:
1. AgentHub API is reachable (`GET /api/health`)
2. At least one agent has recent heartbeat (< 5 min ago)
3. No tasks stuck in `in_progress` for > 60 minutes without progress update

**If any check fails:** Alert PM via Telegram with details.

**Implementation:** Isolated cron session with `sessionTarget: "isolated"` and `payload.kind: "agentTurn"`. The cron calls the health check and reports only on problems.
