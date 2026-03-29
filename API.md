# AgentHub API Reference

## Base URL
http://localhost:8081

## Admin Account
- Username: admin
- Password: admin123

## Get Token
```bash
TOKEN=$(curl -s -X POST http://localhost:8081/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")
```

## Agent API Keys
Each agent registered via `/api/agent/register` and received an API key.
Check `/etc/agenthub/` for saved configs.

## Key Endpoints

### Agent Endpoints (API Key auth)
```
POST /api/agent/register     - Register new agent
POST /api/agent/heartbeat    - Send heartbeat
GET  /api/agent/tasks/queue  - Poll available tasks
POST /api/agent/tasks/:id/claim    - Claim a task
PATCH /api/agent/tasks/:id/progress - Update progress
POST /api/agent/tasks/:id/complete  - Complete task
POST /api/agent/tasks/:id/review    - Review task
POST /api/agent/tasks/:id/test      - Test task
```

### User Endpoints (JWT auth)
```
GET/POST/PATCH/DELETE /api/tasks
GET/POST/PATCH/DELETE /api/projects
GET/POST/PATCH/DELETE /api/features
GET  /api/dashboard
GET  /api/agents
GET  /api/agents/health
GET  /api/review/queue
POST /api/tasks/:id/reassign
POST /api/tasks/:id/escalate
```

## Task Status Flow
available → claimed → in_progress → done → review → test → deployed
                                                      ↘ needs_fix → fix_in_progress → review (loop)
                                                      ↘ escalated (PM decides)
