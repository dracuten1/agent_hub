#!/bin/bash
# ocTeam Driver — health + task + agent status
set -euo pipefail

# Load AgentHub secrets
[ -f /etc/agenthub/agenthub.env ] && set -a && source /etc/agenthub/agenthub.env && set +a

# Load secrets from env
AGENTHUB_ADMIN_PASS="${AGENTHUB_ADMIN_PASS:-}"

API="http://localhost:8081"

# 1. Health
HEALTH=$(curl -s --max-time 5 "$API/health" 2>/dev/null || echo "DOWN")
if [ "$HEALTH" = '{"status":"ok"}' ]; then
  echo "✅ API: OK"
else
  echo "❌ API: DOWN"
  cd ~/workspace-pm/projects/agenthub 2>/dev/null && docker compose restart api 2>/dev/null && echo "🔄 Restarted"
  exit 1
fi

# 2. Docker
docker ps --filter name=agenthub --format '📦 {{.Names}}: {{.Status}}' 2>/dev/null

# 3. Login + get data
TOKEN=$(curl -s --max-time 5 -X POST "$API/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"tuyen","password":"'$AGENTHUB_ADMIN_PASS'"}' 2>/dev/null \
  | python3 -c "import sys,json;print(json.load(sys.stdin).get('token',''))" 2>/dev/null)

if [ -z "$TOKEN" ]; then
  echo "❌ Auth failed"
  exit 1
fi

# 4. Task summary
TASKS=$(curl -s --max-time 5 "$API/api/tasks" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
echo "$TASKS" | python3 -c "
import sys,json
d=json.load(sys.stdin)
tasks=d.get('tasks',[])
total=d.get('total',0)
by_status={}
for t in tasks:
  s=t.get('status','unknown')
  by_status[s]=by_status.get(s,0)+1
if total==0:
  print('📋 Tasks: none')
else:
  parts=[f'{v} {k}' for k,v in sorted(by_status.items())]
  print(f'📋 Tasks: {total} total ({', '.join(parts)})')
  # Show in-progress tasks
  for t in tasks:
    if t.get('status') in ('available','in_progress','claimed','review','testing'):
      print(f'  🔄 {t[\"title\"]} [{t[\"status\"]}] assignee={t.get(\"assignee\",\"-\")}')
"

# 5. Review queue
REVIEW=$(curl -s --max-time 5 "$API/api/review/queue" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
echo "$REVIEW" | python3 -c "
import sys,json
d=json.load(sys.stdin)
q=d.get('review_queue',[])
if q:
  print(f'🔍 Review queue: {len(q)} tasks')
  for t in q[:5]:
    print(f'  📝 {t.get(\"title\",\"?\")} [{t.get(\"priority\",\"?\")}]')
else:
  print('🔍 Review queue: empty')
"

# 6. Agent status
AGENTS=$(curl -s --max-time 5 "$API/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null)
echo "$AGENTS" | python3 -c "
import sys,json
d=json.load(sys.stdin)
agents=d.get('agents',[])
active=[a for a in agents if a.get('status')=='active']
idle=[a for a in agents if a.get('status')!='active']
print(f'👥 Agents: {len(active)} active, {len(idle)} idle')
for a in agents:
  name=a.get('name','?')
  st='🟢' if a.get('status')=='active' else '⚪'
  hb=a.get('last_heartbeat')
  if hb and 'T' in hb:
    from datetime import datetime,timezone
    try:
      t=datetime.fromisoformat(hb.replace('Z','+00:00'))
      ago=int((datetime.now(timezone.utc)-t).total_seconds()/60)
      hb_str=f'{ago}m ago'
    except: hb_str=hb
  else:
    hb_str='never'
  print(f'  {st} {name}: {hb_str}')
"
