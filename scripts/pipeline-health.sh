#!/bin/bash
# pipeline-health.sh — deterministic health checks, no AI needed
# Only outputs when PM needs to act. Empty output = healthy.

cd /root/.openclaw/workspace-pm/projects/agenthub
NUDGE=""

# 1. AgentHub API health
if ! curl -sf http://localhost:8081/api/health > /dev/null 2>&1; then
  docker compose up -d > /dev/null 2>&1
  sleep 2
  if curl -sf http://localhost:8081/api/health > /dev/null 2>&1; then
    NUDGE="$NUDGE\n🔧 API was down, restarted docker compose."
  else
    NUDGE="$NUDGE\n❌ API down and restart failed."
  fi
fi

# Agent tokens
declare -A TOKENS
TOKENS[dev1]="ah_7b5371093f8c8427f78a8bc60b22f6e17af2513cc65f57bf"
TOKENS[dev2]="ah_9102812974795ee1754c084b7912d9b4e312e1e7d0a31541"
TOKENS[reviewer]="ah_9c5d299bf515068c058cf6330ce390835bbd0e6c3c7dc1b7"
TOKENS[tester]="ah_031b448f9a7f4729b2f06bb97c02ed4ef33752f31c085e44"
declare -A ROLES
ROLES[dev1]="dev"
ROLES[dev2]="dev"
ROLES[reviewer]="review"
ROLES[tester]="test"

# 2. Bridge health — check log freshness
FIXED=""

for AGENT in dev1 dev2 reviewer tester; do
  LOG="/tmp/bridge-${AGENT}.log"
  [ -f "$LOG" ] || continue
  
  AGE=$(( $(date +%s) - $(stat -c %Y "$LOG") ))
  
  if [ $AGE -gt 180 ]; then
    # Bridge is zombie — kill, release task, restart
    pkill -f "workerbridge.*--agent-id ${AGENT}" 2>/dev/null
    sleep 1
    
    # Release any in_progress task for this agent
    STUCK=$(docker exec agenthub-db-1 psql -U agenthub -d agenthub -t -A -c \
      "SELECT id FROM tasks WHERE status='in_progress' AND assignee='${AGENT}' LIMIT 1;" 2>/dev/null)
    
    if [ -n "$STUCK" ]; then
      docker exec agenthub-db-1 psql -U agenthub -d agenthub -c \
        "UPDATE tasks SET status='available', assignee=NULL, progress=0, claimed_at=NULL WHERE id='${STUCK}';" > /dev/null 2>&1
    fi
    
    # Restart bridge
    nohup ./workerbridge \
      --agent-id "$AGENT" \
      --role "${ROLES[$AGENT]}" \
      --token "${TOKENS[$AGENT]}" \
      --poll 10 \
      --timeout 600 \
      > "$LOG" 2>&1 &
    
    FIXED="$FIXED $AGENT"
  fi
done

if [ -n "$FIXED" ]; then
  NUDGE="$NUDGE\n🔧 Restarted zombie bridges:$FIXED"
fi

# 3. Stuck tasks (in_progress >20 min, no recent progress)
STUCK_TASKS=$(docker exec agenthub-db-1 psql -U agenthub -d agenthub -t -A -c \
  "SELECT id::varchar(8), assignee, EXTRACT(EPOCH FROM (NOW()-claimed_at))/60 as mins \
   FROM tasks WHERE status='in_progress' AND claimed_at < NOW() - INTERVAL '20 minutes';" 2>/dev/null)

if [ -n "$STUCK_TASKS" ]; then
  NUDGE="$NUDGE\n⏰ Stuck tasks (>20 min):\n$STUCK_TASKS"
fi

# 4. Failed or escalated tasks — needs PM decision
FAILED=$(docker exec agenthub-db-1 psql -U agenthub -d agenthub -t -A -c \
  "SELECT id::varchar(8), status, LEFT(title,40) FROM tasks WHERE status IN ('failed','escalated') ORDER BY updated_at DESC LIMIT 5;" 2>/dev/null)

if [ -n "$FAILED" ]; then
  NUDGE="$NUDGE\n❌ Failed/escalated tasks:\n$FAILED"
fi

# 5. Gates waiting approval — needs PM decision
GATES=$(docker exec agenthub-db-1 psql -U agenthub -d agenthub -t -A -c \
  "SELECT w.name || ' → ' || wp.phase_name || ' (phase ' || wp.phase_index || ')' \
   FROM workflows w JOIN workflow_phases wp ON wp.workflow_id=w.id \
   WHERE wp.status='waiting_approval' AND w.status IN ('active','paused');" 2>/dev/null)

if [ -n "$GATES" ]; then
  NUDGE="$NUDGE\n🚦 Gates waiting approval:\n$GATES"
fi

# 6. No bridges running but tasks available — start dev bridges
BRIDGE_COUNT=$(ps aux | grep workerbridge | grep -v grep | grep -v bash | wc -l)
AVAILABLE=$(docker exec agenthub-db-1 psql -U agenthub -d agenthub -t -A -c \
  "SELECT COUNT(*) FROM tasks WHERE status='available';" 2>/dev/null)

if [ "$BRIDGE_COUNT" -eq 0 ] && [ "$AVAILABLE" -gt 0 ]; then
  for AGENT in dev1 dev2; do
    nohup ./workerbridge \
      --agent-id "$AGENT" \
      --role "dev" \
      --token "${TOKENS[$AGENT]}" \
      --poll 10 \
      --timeout 600 \
      > "/tmp/bridge-${AGENT}.log" 2>&1 &
  done
  NUDGE="$NUDGE\n🔧 No bridges running, started dev1+dev2 ($AVAILABLE tasks in queue)"
fi

# Output: empty = healthy (cron agent replies NO_REPLY), non-empty = nudge PM
if [ -n "$NUDGE" ]; then
  echo -e "$NUDGE"
fi
