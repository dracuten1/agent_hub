# AgentHub Team — Setup Guide

## Overview

Two ways agents collaborate:

| Team | Communication | Best For |
|------|--------------|----------|
| **ocTeam** | OpenClaw sessions_send | Quick tasks, interactive dev |
| **AgentHub Team** | Poll API (systemd workers) | Long-running, autonomous, 24/7 |

## AgentHub Team (New)

### How It Works
Workers are **standalone Go binaries** running as systemd services.
They poll AgentHub API for tasks, execute, and report back.
No OpenClaw needed.

```
Tuyên → PM → AgentHub API → Worker polls → Works → Reports
                              ↓
                         Health Monitor
                         (auto-reassign dead workers)
```

### Workers

| Worker | Binary | What It Does |
|--------|--------|-------------|
| Dev Worker | `agenthub-worker dev` | Polls queue → claims → runs OpenCode → reports |
| Review Worker | `agenthub-worker review` | Polls review queue → reviews code → reports pass/fail |
| Test Worker | `agenthub-worker test` | Polls test queue → runs tests → reports pass/fail |

### Setup

```bash
# 1. Register workers
for name in hub-dev1 hub-dev2 hub-reviewer hub-tester; do
  curl -X POST http://localhost:8081/api/agent/register \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$name\",\"role\":\"...\",\"skills\":[\"...\"]}"
done

# 2. Configure env
cat > /etc/agenthub/worker.env << 'EOF'
AGENTHUB_URL=http://localhost:8081
AGENTHUB_TOKEN=<api_key_from_step1>
DAODUC_API_KEY=<daoduc_key>
POLL_INTERVAL=30s
PROJECT_DIR=/root/projects
EOF

# 3. Install systemd services
cd /root/.openclaw/workspace-pm/projects/agenthub
./install.sh

# 4. Start
systemctl start agenthub-worker-dev
systemctl start agenthub-worker-reviewer
systemctl start agenthub-worker-tester
```

### Current Status

- ⚠️ Workers need API path updates (old paths vs actual API)
- ⚠️ OpenCode integration not tested with real tasks
- ✅ Worker binaries compile
- ✅ Systemd service files ready
- ✅ Graceful shutdown implemented
- ✅ Exponential backoff on errors

### TODO to Go Live

1. Fix worker API paths to match actual AgentHub endpoints
2. Test OpenCode execution in worker context
3. Add per-project workspace config
4. Add WebSocket notifications (instead of polling)
5. Test end-to-end with a real task
