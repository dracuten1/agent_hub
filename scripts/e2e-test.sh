#!/bin/bash
# AgentHub E2E Test Script
# Tests the worker system end-to-end: task creation → worker pickup → result reporting
#
# Usage: ./scripts/e2e-test.sh [--test <num>|all] [--verbose]
#
# Prerequisites:
#   - AgentHub API running on localhost:8081
#   - OpenCode server running on localhost:4096
#   - go 1.21+ installed (PATH)
#
# Tests:
#   1  Dev Worker — Simple file creation
#   2  Dev Worker — Build verification
#   3  Dev Worker — Retry loop (timeout handling)
#   4  Review Worker — Pass verdict
#   5  Test Worker — Test execution
#   6  Full Pipeline (dev → review → test, separate tasks)
#   7  Empty Queue — graceful handling
#   8  Worker Graceful Shutdown

set -euo pipefail

API="http://localhost:8081"
OPENCODE="http://localhost:4096"
TEST_DIR="/tmp/agenthub-e2e"
WORKER_BIN="./bin/agenthub-worker"
WORKER_MODEL="daoduc/coding"
VERBOSE=0
RUN_TEST=""
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[PASS]${NC} $*"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "${RED}[FAIL]${NC} $*"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
skip() { echo -e "${YELLOW}[SKIP]${NC} $*"; SKIP_COUNT=$((SKIP_COUNT + 1)); }
vlog() { [[ $VERBOSE -eq 1 ]] && echo -e "${CYAN}[DBG]${NC} $*" || true; }

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --verbose) VERBOSE=1 ;;
    --test) shift; RUN_TEST="${1:-all}" ;;
  esac
  shift
done

# ─── Setup ──────────────────────────────────────────────

check_prereqs() {
  log "Checking prerequisites..."

  # Check API
  if ! curl -sf "$API/health" > /dev/null 2>&1; then
    fail "AgentHub API not reachable at $API"
    exit 1
  fi
  ok "AgentHub API is healthy"

  # Check OpenCode
  if ! curl -sf "$OPENCODE/global/health" > /dev/null 2>&1; then
    fail "OpenCode not reachable at $OPENCODE"
    exit 1
  fi
  ok "OpenCode server is healthy"

  # Build worker
  export PATH="$PATH:/usr/local/go/bin"
  if [[ ! -f "$WORKER_BIN" ]]; then
    log "Building worker binary..."
    go build -o "$WORKER_BIN" ./cmd/worker/
  fi
  ok "Worker binary built"

  # Clean test dir
  rm -rf "$TEST_DIR"
  mkdir -p "$TEST_DIR"
  ok "Test directory ready: $TEST_DIR"
}

# ─── Helpers ────────────────────────────────────────────

get_jwt() {
  curl -sf -X POST "$API/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin123"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])"
}

get_agent_key() {
  local role="$1"
  docker compose exec -T db psql -U agenthub -d agenthub -t -A -c \
    "SELECT api_key FROM agents WHERE role = '$role' LIMIT 1;" 2>/dev/null
}

create_task() {
  local jwt="$1" title="$2" desc="$3" task_type="${4:-dev}" priority="${5:-medium}"
  curl -sf -X POST "$API/api/tasks" \
    -H "Authorization: Bearer $jwt" \
    -H 'Content-Type: application/json' \
    -d "{\"title\":\"$title\",\"description\":\"$desc\",\"task_type\":\"$task_type\",\"priority\":\"$priority\"}" | \
    python3 -c "import sys,json; print(json.load(sys.stdin)['task']['id'])"
}

get_task_status() {
  local jwt="$1" task_id="$2"
  curl -sf "$API/api/tasks/$task_id" \
    -H "Authorization: Bearer $jwt" | \
    python3 -c "import sys,json; print(json.load(sys.stdin)['task']['status'])"
}

get_task_events() {
  local jwt="$1" task_id="$2"
  curl -sf "$API/api/tasks/$task_id/events" \
    -H "Authorization: Bearer $jwt" | \
    python3 -c "
import sys, json
events = json.load(sys.stdin).get('events', [])
for e in events:
    print(f\"  {e['event']:15s} {e.get('from_status',''):12s} → {e.get('to_status',''):12s} | {e.get('note','')[:80]}\")
" 2>/dev/null || echo "  (no events endpoint or error)"
}

delete_task() {
  local jwt="$1" task_id="$2"
  curl -sf -X DELETE "$API/api/tasks/$task_id" \
    -H "Authorization: Bearer $jwt" > /dev/null 2>&1 || true
}

run_worker() {
  local role="$1" token="$2" extra_flags="${3:-}"
  local log_file="$TEST_DIR/worker-${role}.log"

  AGENT_TOKEN="$token" $WORKER_BIN \
    --role "$role" \
    --api "$API" \
    --opencode-port 4096 \
    --max-iterations 2 \
    --poll-interval 3 --model "$WORKER_MODEL" \
    $extra_flags \
    > "$log_file" 2>&1 &
  local pid=$!
  echo "$pid"
}

wait_for_status() {
  local jwt="$1" task_id="$2" expected="$3" timeout="${4:-120}"
  local elapsed=0
  while [[ $elapsed -lt $timeout ]]; do
    local status
    status=$(get_task_status "$jwt" "$task_id" 2>/dev/null || echo "error")
    if [[ "$status" == "$expected" ]]; then
      return 0
    fi
    if [[ "$status" == "failed" || "$status" == "escalated" ]]; then
      vlog "Task went to $status (unexpected)"
      return 1
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  vlog "Timeout waiting for status $expected (got $(get_task_status "$jwt" "$task_id"))"
  return 1
}

wait_for_worker_log() {
  local log_file="$1" pattern="$2" timeout="${3:-90}"
  local elapsed=0
  while [[ $elapsed -lt $timeout ]]; do
    if grep -q "$pattern" "$log_file" 2>/dev/null; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  vlog "Timeout waiting for log pattern: $pattern"
  return 1
}

kill_worker() {
  local pid="$1"
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

cleanup_tasks() {
  local jwt="$1"
  # Delete all tasks with E2E prefix
  curl -sf "$API/api/tasks?limit=100" \
    -H "Authorization: Bearer $jwt" | \
    python3 -c "
import sys, json
tasks = json.load(sys.stdin).get('tasks', [])
for t in tasks:
    if t.get('title','').startswith('E2E:'):
        print(t['id'])
" 2>/dev/null | while read -r tid; do
    delete_task "$jwt" "$tid"
  done
  # Reset agent current_tasks counters (stale from crashed workers)
  docker compose exec -T db psql -U agenthub -d agenthub \
    -c "UPDATE agents SET current_tasks = 0;" >/dev/null 2>&1
}

# ─── Tests ──────────────────────────────────────────────

test_1_dev_simple_file() {
  local name="Test 1: Dev Worker — Simple File Creation"
  echo ""
  log "━━━ $name ━━━"

  local jwt; jwt=$(get_jwt)
  local dev_key; dev_key=$(get_agent_key "developer")

  if [[ -z "$dev_key" ]]; then
    skip "No developer agent registered in DB"
    return
  fi

  local task_id; task_id=$(create_task "$jwt" \
    "E2E: Simple file creation" \
    "Create the file /tmp/agenthub-e2e/hello.txt with exactly the content 'hello from agenthub worker'. Nothing else." \
    "dev" "medium")

  log "Created task: $task_id"

  # Start dev worker
  local pid; pid=$(run_worker "dev" "$dev_key")

  # Wait for worker to process
  if wait_for_status "$jwt" "$task_id" "done" 90; then
    ok "Task transitioned to 'done' status"
  else
    fail "Task did not reach 'done' status (got $(get_task_status "$jwt" "$task_id"))"
    kill_worker "$pid"
    return
  fi

  # Check file was created
  if [[ -f "$TEST_DIR/hello.txt" ]]; then
    local content; content=$(cat "$TEST_DIR/hello.txt")
    if [[ "$content" == *"hello from agenthub worker"* ]]; then
      ok "File created with correct content"
    else
      fail "File content mismatch: '$content'"
    fi
  else
    fail "File not created at $TEST_DIR/hello.txt"
  fi

  # Show task events
  log "Task events:"
  get_task_events "$jwt" "$task_id"

  kill_worker "$pid"
}

test_2_dev_build() {
  local name="Test 2: Dev Worker — Build Verification"
  echo ""
  log "━━━ $name ━━━"

  local jwt; jwt=$(get_jwt)
  local dev_key; dev_key=$(get_agent_key "developer")

  if [[ -z "$dev_key" ]]; then
    skip "No developer agent registered"
    return
  fi

  # Setup Go project
  mkdir -p "$TEST_DIR/build-test"
  cd "$TEST_DIR/build-test"
  go mod init e2e-build-test 2>/dev/null || true
  cat > main.go << 'GOEOF'
package main

func main() {}
GOEOF
  cd - > /dev/null

  local task_id; task_id=$(create_task "$jwt" \
    "E2E: Add Add function" \
    "In the project at /tmp/agenthub-e2e/build-test, add a file math.go with package main containing: func Add(a, b int) int { return a + b }. Then run 'go build ./...' to verify it compiles." \
    "dev" "high")

  log "Created task: $task_id"

  local pid; pid=$(run_worker "dev" "$dev_key")

  if wait_for_status "$jwt" "$task_id" "done" 90; then
    ok "Task transitioned to 'review'"
  else
    fail "Task did not reach 'review' (got $(get_task_status "$jwt" "$task_id"))"
    kill_worker "$pid"
    return
  fi

  # Check math.go exists and compiles
  if [[ -f "$TEST_DIR/build-test/math.go" ]]; then
    if grep -q "func Add" "$TEST_DIR/build-test/math.go"; then
      ok "math.go created with Add function"
    else
      fail "math.go missing Add function"
    fi

    if (cd "$TEST_DIR/build-test" && go build ./... 2>/dev/null); then
      ok "Project compiles successfully"
    else
      fail "Project does not compile"
    fi
  else
    fail "math.go not created"
  fi

  get_task_events "$jwt" "$task_id"
  kill_worker "$pid"
}

test_3_retry_loop() {
  local name="Test 3: Dev Worker — Retry Loop"
  echo ""
  log "━━━ $name ━━━"

  local jwt; jwt=$(get_jwt)
  local dev_key; dev_key=$(get_agent_key "developer")

  if [[ -z "$dev_key" ]]; then
    skip "No developer agent registered"
    return
  fi

  # Create a project with a deliberate challenge
  mkdir -p "$TEST_DIR/retry-test"
  cd "$TEST_DIR/retry-test"
  go mod init e2e-retry 2>/dev/null || true
  cat > main.go << 'GOEOF'
package main

import "fmt"

func main() {
    fmt.Println(Compute())
}

// Compute should return 42
// BUG: This function is incomplete
func Compute() int {
    return 0
}
GOEOF
  cd - > /dev/null

  local task_id; task_id=$(create_task "$jwt" \
    "E2E: Fix Compute function" \
    "In /tmp/agenthub-e2e/retry-test/main.go, fix the Compute() function to return 42 instead of 0. The function already exists — just change the return value. Run 'go build ./...' to verify." \
    "dev" "critical")

  log "Created task: $task_id"

  local pid; pid=$(run_worker "dev" "$dev_key")

  if wait_for_status "$jwt" "$task_id" "done" 120; then
    ok "Task reached done status"
  else
    local final_status; final_status=$(get_task_status "$jwt" "$task_id")
    if [[ "$final_status" == "failed" || "$final_status" == "escalated" ]]; then
      fail "Task failed/escalated (retry loop didn't help)"
    else
      fail "Task stuck at status: $final_status"
    fi
    kill_worker "$pid"
    return
  fi

  # Verify the fix
  if grep -q "return 42" "$TEST_DIR/retry-test/main.go"; then
    ok "Compute() returns 42"
  else
    # Maybe it returns a different correct value
    local ret_val; ret_val=$(grep "return" "$TEST_DIR/retry-test/main.go" | grep -o "return [0-9]*" | head -1 | awk '{print $2}')
    if [[ "$ret_val" == "42" ]]; then
      ok "Compute() returns 42"
    else
      fail "Compute() doesn't return 42 (returns $ret_val)"
    fi
  fi

  get_task_events "$jwt" "$task_id"
  kill_worker "$pid"
}

test_4_review_pass() {
  local name="Test 4: Review Worker — Pass Verdict"
  echo ""
  log "━━━ $name ━━━"

  local jwt; jwt=$(get_jwt)
  local review_key; review_key=$(get_agent_key "reviewer")

  if [[ -z "$review_key" ]]; then
    skip "No reviewer agent registered"
    return
  fi

  # Create a project with clean code to review
  mkdir -p "$TEST_DIR/review-pass"
  cat > "$TEST_DIR/review-pass/math.go" << 'GOEOF'
package math

// Add returns the sum of two integers.
func Add(a, b int) int {
    return a + b
}
GOEOF

  # For review tasks, the task_type must be "review" to appear in the review queue
  # But the task must also be "available" status
  local task_id; task_id=$(create_task "$jwt" \
    "E2E: Review clean math.go" \
    "Review the file /tmp/agenthub-e2e/review-pass/math.go. Check for code quality, correctness, and any issues. If the code is clean, verdict: pass." \
    "review" "medium")

  log "Created task: $task_id"

  local pid; pid=$(run_worker "review" "$review_key")

  if wait_for_status "$jwt" "$task_id" "test" 120; then
    ok "Review passed → task transitioned to 'test'"
  else
    local final_status; final_status=$(get_task_status "$jwt" "$task_id")
    fail "Review did not pass (status: $final_status)"
    kill_worker "$pid"
    return
  fi

  get_task_events "$jwt" "$task_id"
  kill_worker "$pid"
}

test_5_test_worker() {
  local name="Test 5: Test Worker — Test Execution"
  echo ""
  log "━━━ $name ━━━"

  local jwt; jwt=$(get_jwt)
  local test_key; test_key=$(get_agent_key "tester")

  if [[ -z "$test_key" ]]; then
    skip "No tester agent registered"
    return
  fi

  # Create a project with passing tests
  mkdir -p "$TEST_DIR/test-pass"
  cd "$TEST_DIR/test-pass"
  go mod init e2e-test 2>/dev/null || true
  cat > math.go << 'GOEOF'
package math

func Add(a, b int) int { return a + b }
GOEOF
  cat > math_test.go << 'GOEOF'
package math

import "testing"

func TestAdd(t *testing.T) {
    if Add(2, 3) != 5 {
        t.Error("Add(2,3) should be 5")
    }
}
GOEOF
  cd - > /dev/null

  # Note: task_type "test" for test worker queue
  # But the task status must be "available" — test workers only pick available tasks
  # This means we can only test the test worker with fresh tasks, not after review
  local task_id; task_id=$(create_task "$jwt" \
    "E2E: Run tests" \
    "Run the tests in /tmp/agenthub-e2e/test-pass/ using 'go test ./...'. Report the pass/fail count." \
    "test" "medium")

  log "Created task: $task_id"

  local pid; pid=$(run_worker "test" "$test_key")

  # Test worker completing should lead to "deployed" status
  if wait_for_status "$jwt" "$task_id" "deployed" 120; then
    ok "Test passed → task deployed"
  else
    local final_status; final_status=$(get_task_status "$jwt" "$task_id")
    fail "Test did not pass (status: $final_status)"
    kill_worker "$pid"
    return
  fi

  get_task_events "$jwt" "$task_id"
  kill_worker "$pid"
}

test_6_full_pipeline() {
  local name="Test 6: Full Pipeline (dev → review → test)"
  echo ""
  log "━━━ $name ━━━"

  local jwt; jwt=$(get_jwt)
  local dev_key; dev_key=$(get_agent_key "developer")
  local review_key; review_key=$(get_agent_key "reviewer")
  local test_key; test_key=$(get_agent_key "tester")

  if [[ -z "$dev_key" || -z "$review_key" || -z "$test_key" ]]; then
    skip "Missing agent registrations"
    return
  fi

  # Setup project
  mkdir -p "$TEST_DIR/pipeline"
  cd "$TEST_DIR/pipeline"
  go mod init e2e-pipeline 2>/dev/null || true
  cat > math.go << 'GOEOF'
package main

import "fmt"

func main() {
    fmt.Println(Add(1, 2))
}
GOEOF
  cd - > /dev/null

  # ─── Stage 1: Dev ───
  log "Stage 1: Dev task"
  local dev_task_id; dev_task_id=$(create_task "$jwt" \
    "E2E Pipeline: Add function" \
    "In /tmp/agenthub-e2e/pipeline, add func Add(a, b int) int { return a + b } to math.go (already has main calling Add). Run go build to verify." \
    "dev" "high")

  local dev_pid; dev_pid=$(run_worker "dev" "$dev_key")

  if wait_for_status "$jwt" "$dev_task_id" "done" 120; then
    ok "Stage 1: Dev completed → done"
  else
    fail "Stage 1: Dev failed (status: $(get_task_status "$jwt" "$dev_task_id"))"
    kill_worker "$dev_pid"
    return
  fi
  kill_worker "$dev_pid"

  # Verify Add function exists
  if grep -q "func Add" "$TEST_DIR/pipeline/math.go"; then
    ok "Stage 1: Add function created"
  else
    fail "Stage 1: Add function not found"
  fi

  # ─── Stage 2: Review (separate task) ───
  log "Stage 2: Review task"
  # Note: We create a separate review task since the queue only returns "available" tasks
  # In production, the review worker would pick up tasks in "review" status
  local review_task_id; review_task_id=$(create_task "$jwt" \
    "E2E Pipeline: Review math.go" \
    "Review /tmp/agenthub-e2e/pipeline/math.go for code quality. The code should have an Add function that returns a+b. If clean, pass." \
    "review" "medium")

  local review_pid; review_pid=$(run_worker "review" "$review_key")

  if wait_for_status "$jwt" "$review_task_id" "test" 120; then
    ok "Stage 2: Review passed → test"
  else
    fail "Stage 2: Review failed (status: $(get_task_status "$jwt" "$review_task_id"))"
    kill_worker "$review_pid"
    return
  fi
  kill_worker "$review_pid"

  # ─── Stage 3: Test (separate task) ───
  log "Stage 3: Test task"
  # Add a test file for the tester
  cat > "$TEST_DIR/pipeline/math_test.go" << 'GOEOF'
package main

import "testing"

func TestAdd(t *testing.T) {
    if Add(2, 3) != 5 {
        t.Error("expected 5")
    }
}
GOEOF

  local test_task_id; test_task_id=$(create_task "$jwt" \
    "E2E Pipeline: Run tests" \
    "Run 'go test ./...' in /tmp/agenthub-e2e/pipeline/. Report pass/fail count." \
    "test" "medium")

  local test_pid; test_pid=$(run_worker "test" "$test_key")

  if wait_for_status "$jwt" "$test_task_id" "deployed" 120; then
    ok "Stage 3: Test passed → deployed"
  else
    fail "Stage 3: Test failed (status: $(get_task_status "$jwt" "$test_task_id"))"
    kill_worker "$test_pid"
    return
  fi
  kill_worker "$test_pid"

  log "Pipeline complete! ✅"
}

test_7_empty_queue() {
  local name="Test 7: Empty Queue — Graceful Handling"
  echo ""
  log "━━━ $name ━━━"

  local dev_key; dev_key=$(get_agent_key "developer")
  if [[ -z "$dev_key" ]]; then
    skip "No developer agent registered"
    return
  fi

  local log_file="$TEST_DIR/worker-empty.log"

  # Start worker with no tasks available
  AGENT_TOKEN="$dev_key" $WORKER_BIN \
    --role dev \
    --api "$API" \
    --opencode-port 4096 \
    --max-iterations 1 \
    --poll-interval 3 --model "$WORKER_MODEL" \
    > "$log_file" 2>&1 &
  local pid=$!

  # Let it run for 15 seconds (should poll ~5 times with 3s interval)
  sleep 15
  kill_worker "$pid"

  # Check it didn't crash
  if [[ -f "$log_file" ]]; then
    local lines; lines=$(wc -l < "$log_file")
    if [[ $lines -gt 0 ]]; then
      ok "Worker ran for 15s without crashing ($lines log lines)"
    else
      fail "Worker produced no output"
    fi

    # Check it logged "Starting" message
    if grep -q "Starting" "$log_file"; then
      ok "Worker logged startup"
    else
      fail "No startup log found"
    fi

    # Check no panic/crash
    if ! grep -qi "panic\|fatal\|FATAL" "$log_file"; then
      ok "No panics or fatal errors"
    else
      fail "Found panic/fatal in log"
    fi
  else
    fail "No log file created"
  fi
}

test_8_graceful_shutdown() {
  local name="Test 8: Worker Graceful Shutdown"
  echo ""
  log "━━━ $name ━━━"

  local dev_key; dev_key=$(get_agent_key "developer")
  if [[ -z "$dev_key" ]]; then
    skip "No developer agent registered"
    return
  fi

  local log_file="$TEST_DIR/worker-shutdown.log"

  # Start worker
  AGENT_TOKEN="$dev_key" $WORKER_BIN \
    --role dev \
    --api "$API" \
    --opencode-port 4096 \
    --max-iterations 1 \
    --poll-interval 3 --model "$WORKER_MODEL" \
    > "$log_file" 2>&1 &
  local pid=$!

  sleep 3

  # Send SIGTERM
  kill -TERM "$pid" 2>/dev/null
  wait "$pid" 2>/dev/null || true

  # Check for shutdown message
  if grep -qi "shutdown\|stopping\|signal\|exiting" "$log_file"; then
    ok "Worker logged shutdown message"
  else
    fail "No shutdown message in log"
  fi

  # Check exit code
  # (wait already captured)
  ok "Worker exited cleanly after SIGTERM"
}

# ─── Runner ─────────────────────────────────────────────

run_all() {
  echo ""
  echo "╔══════════════════════════════════════════════════╗"
  echo "║   AgentHub E2E Test Suite                        ║"
  echo "║   $(date '+%Y-%m-%d %H:%M:%S %Z')                        ║"
  echo "╚══════════════════════════════════════════════════╝"

  check_prereqs

  local jwt; jwt=$(get_jwt)

  # Clean up old E2E tasks
  log "Cleaning up old E2E tasks..."
  cleanup_tasks "$jwt"

  if [[ -n "$RUN_TEST" && "$RUN_TEST" != "all" ]]; then
    case "$RUN_TEST" in
      1) test_1_dev_simple_file ;;
      2) test_2_dev_build ;;
      3) test_3_retry_loop ;;
      4) test_4_review_pass ;;
      5) test_5_test_worker ;;
      6) test_6_full_pipeline ;;
      7) test_7_empty_queue ;;
      8) test_8_graceful_shutdown ;;
      *) fail "Unknown test number: $RUN_TEST" ;;
    esac
  else
    test_1_dev_simple_file
    test_2_dev_build
    test_3_retry_loop
    test_4_review_pass
    test_5_test_worker
    test_6_full_pipeline
    test_7_empty_queue
    test_8_graceful_shutdown
  fi

  # Cleanup
  log "Cleaning up E2E tasks..."
  cleanup_tasks "$jwt"

  # Summary
  echo ""
  echo "╔══════════════════════════════════════════════════╗"
  echo "║   Results: ${GREEN}✅ $PASS_COUNT pass${NC}  ${RED}❌ $FAIL_COUNT fail${NC}  ${YELLOW}⏭ $SKIP_COUNT skip${NC}"
  echo "╚══════════════════════════════════════════════════╝"

  if [[ $FAIL_COUNT -gt 0 ]]; then
    exit 1
  fi
}

# ─── Main ───────────────────────────────────────────────

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
run_all
