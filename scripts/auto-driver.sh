#!/usr/bin/env bash
# Auto-driver: sequential LLM-agent execution of llama-runner's -auto feature.
#
# Each phase runs in a fresh `claude -p` session with its own system prompt.
# A reviewer session between phases checks invariants P1-P7 and can reject.
# Every accepted phase is git-committed (rollback points).
#
# Usage:
#   ./scripts/auto-driver.sh                    # run all phases
#   ./scripts/auto-driver.sh --from B.2         # resume from phase B.2
#   ./scripts/auto-driver.sh --to D.1           # stop after phase D.1
#   ./scripts/auto-driver.sh --dry-run          # print plan, don't execute
#   ./scripts/auto-driver.sh --skip-review      # no reviewer sessions (dangerous)
#
# Env overrides:
#   AUTO_TIMEOUT_CODING=240m      # phases A, B.1, B.2, B.3, D.1, D.2
#   AUTO_TIMEOUT_LIGHT=90m        # phases 0, C, E
#   AUTO_TIMEOUT_REVIEW=30m       # reviewer after each phase
#   AUTO_MODEL_PROFILE=agent      # llama-server profile
#   AUTO_MODEL_PARALLEL=1         # forced parallel; do not raise unless intended
#   AUTO_BRANCH=auto/impl         # working branch
#   AUTO_SKIP_SERVER=0            # if 1, assume llama-server already running

set -euo pipefail

# ---------- config ----------
TIMEOUT_CODING="${AUTO_TIMEOUT_CODING:-240m}"
TIMEOUT_LIGHT="${AUTO_TIMEOUT_LIGHT:-90m}"
TIMEOUT_REVIEW="${AUTO_TIMEOUT_REVIEW:-30m}"
PROFILE="${AUTO_MODEL_PROFILE:-agent}"
PARALLEL="${AUTO_MODEL_PARALLEL:-1}"
BRANCH="${AUTO_BRANCH:-auto/impl}"
SKIP_SERVER="${AUTO_SKIP_SERVER:-0}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

LOGS="logs"
PROMPTS="prompts"
DOCS="docs/auto"
STATUS="$LOGS/status.txt"
DRIVER_LOG="$LOGS/driver.log"
HEARTBEAT_PID=""
SERVER_PID=""

# ---------- phases ----------
# format: id|type|coding|light|prompt|description
PHASES=(
  "0|light|prompts/phase-0-baseline.md|Planner: baseline snapshot"
  "A|coding|prompts/phase-A-foundation.md|Foundation: catalog, env, flags"
  "B.1|coding|prompts/phase-B1-servermgr.md|ServerManager.Stop/SwapTo"
  "B.2|coding|prompts/phase-B2-swapper.md|Swapper interface + profileSwapper"
  "B.3|coding|prompts/phase-B3-proxy.md|Proxy: in-flight drain, queue"
  "C|light|prompts/phase-C-menu.md|Menu + --append-system-prompt"
  "D.1|coding|prompts/phase-D1-runauto.md|runAuto skeleton + planner"
  "D.2|coding|prompts/phase-D2-orchestration.md|Orchestration e2e"
  "E|light|prompts/phase-E-docs.md|Documentation"
)

# ---------- cli ----------
FROM=""
TO=""
DRY_RUN=0
SKIP_REVIEW=0

while [ $# -gt 0 ]; do
  case "$1" in
    --from) FROM="$2"; shift 2 ;;
    --to) TO="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --skip-review) SKIP_REVIEW=1; shift ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

# ---------- helpers ----------
log()  { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$DRIVER_LOG"; }
warn() { printf '[%s] WARN: %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$DRIVER_LOG" >&2; }
die()  { printf '[%s] FATAL: %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$DRIVER_LOG" >&2; exit 1; }

phase_timeout() {
  case "$1" in
    coding) echo "$TIMEOUT_CODING" ;;
    light)  echo "$TIMEOUT_LIGHT" ;;
    review) echo "$TIMEOUT_REVIEW" ;;
    *)      die "unknown timeout kind: $1" ;;
  esac
}

phase_file_id() {
  # "B.1" -> "B1" for filenames
  echo "$1" | tr -d '.'
}

# Heartbeat: every 5 min write status + log size to status file.
start_heartbeat() {
  (
    while true; do
      sleep 300
      {
        echo "time: $(date -Iseconds)"
        echo "phase: $1"
        echo "log: $(wc -c < "$2" 2>/dev/null || echo 0) bytes"
        echo "log_lines: $(wc -l < "$2" 2>/dev/null || echo 0)"
      } > "$STATUS.tmp" && mv "$STATUS.tmp" "$STATUS"
    done
  ) &
  HEARTBEAT_PID=$!
}

stop_heartbeat() {
  if [ -n "$HEARTBEAT_PID" ]; then
    kill "$HEARTBEAT_PID" 2>/dev/null || true
    wait "$HEARTBEAT_PID" 2>/dev/null || true
    HEARTBEAT_PID=""
  fi
}

server_is_up() {
  curl -sf --max-time 3 "http://127.0.0.1:${AUTO_SERVER_PORT:-8080}/health" >/dev/null
}

cleanup_all() {
    stop_heartbeat
    stop_server
    pkill -TERM -f 'claude -p Phase' 2>/dev/null || true
    pkill -TERM -f 'timeout .* claude' 2>/dev/null || true
    sleep 2
    pkill -KILL -f 'claude -p Phase' 2>/dev/null || true
    pkill -KILL -f 'timeout .* claude' 2>/dev/null || true
}

start_server() {
  if [ "$SKIP_SERVER" = "1" ]; then
    log "SKIP_SERVER=1, assuming llama-server is up"
    server_is_up || die "llama-server is NOT up on :8080"
    return
  fi
  if ss -ltn "sport = :8080" 2>/dev/null | grep -q LISTEN; then
    die "port 8080 is already in use — run: pkill -f llama-server"
  fi
  log "starting llama-server (profile=$PROFILE, parallel=$PARALLEL)"
  local reasoning_flag=""
  if [ "${AUTO_REASONING:-on}" = "off" ]; then
    reasoning_flag="--reasoning off"
    log "reasoning: OFF (AUTO_REASONING=off)"
  fi
  llama-runner -server -p "$PROFILE" -parallel "$PARALLEL" $reasoning_flag \
    > "$LOGS/llama-server.out" 2>&1 &
  SERVER_PID=$!
  local n=0
  while ! server_is_up; do
    sleep 2; n=$((n+2))
    [ "$n" -gt 900 ] && die "llama-server didn't become ready in 15min"
  done
  log "llama-server ready (pid=$SERVER_PID)"
}

stop_server() {
  if [ -n "$SERVER_PID" ]; then
    log "stopping llama-server (pid=$SERVER_PID)"
    kill -TERM "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    SERVER_PID=""
  fi
}

claude_env() {
  export ANTHROPIC_BASE_URL="http://127.0.0.1:${AUTO_SERVER_PORT:-8080}"
  export ANTHROPIC_AUTH_TOKEN="local-llama"
  export ANTHROPIC_API_KEY="local-llama"
  export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
  export CLAUDE_CODE_MAX_CONTEXT_TOKENS="${AUTO_CONTEXT_WINDOW:-147456}"
  export CLAUDE_CODE_AUTO_COMPACT_WINDOW="${AUTO_CONTEXT_WINDOW:-147456}"

  local alias="${AUTO_MODEL_ALIAS:-glm-4.7-flash}"
  export ANTHROPIC_MODEL="$alias"
  export ANTHROPIC_DEFAULT_OPUS_MODEL="$alias"
  export ANTHROPIC_DEFAULT_SONNET_MODEL="$alias"
  export ANTHROPIC_DEFAULT_HAIKU_MODEL="$alias"
  export ANTHROPIC_DEFAULT_FABLE_MODEL="$alias"
}

# ---------- phase runner ----------
run_phase() {
  local id="$1" kind="$2" prompt="$3" desc="$4"
  local fid; fid="$(phase_file_id "$id")"
  local out="$DOCS/phase-${fid}-output.md"
  local phase_log="$LOGS/phase-${fid}.log"
  local timeout_s; timeout_s="$(phase_timeout "$kind")"

  log "=== phase $id: $desc (timeout=$timeout_s) ==="
  start_heartbeat "$id" "$phase_log"

  if ! timeout "$timeout_s" claude -p "$(cat "$prompt")" \
      --append-system-prompt "$(cat prompts/system-agent.md)" \
      --dangerously-skip-permissions \
      2>&1 | tee "$phase_log"
  then
    local rc=${PIPESTATUS[0]}
    stop_heartbeat
    if [ "$rc" -eq 124 ]; then
      die "phase $id TIMED OUT after $timeout_s — see $phase_log"
    fi
    die "phase $id FAILED (rc=$rc) — see $phase_log"
  fi
  stop_heartbeat

  if ! go build ./... 2>&1 | tee -a "$phase_log"; then
    die "phase $id: go build FAILED — see $phase_log"
  fi
  if ! go test -short -count=1 ./internal/... 2>&1 | tail -30 | tee -a "$phase_log"; then
    die "phase $id: go test FAILED — see $phase_log"
  fi
  if [ ! -f "$out" ]; then
    die "phase $id: output file missing: $out"
  fi
  log "phase $id OK"
}

run_review() {
  local id="$1"
  local fid; fid="$(phase_file_id "$id")"
  local timeout_s; timeout_s="$(phase_timeout review)"
  local review_log="$LOGS/review-${fid}.log"

  log "=== reviewer for phase $id (timeout=$timeout_s) ==="
  local verdict
  verdict=$(timeout "$timeout_s" claude -p "$(cat prompts/reviewer.md)" \
      --append-system-prompt "$(cat prompts/system-agent.md)" \
      --dangerously-skip-permissions 2>&1 | tee "$review_log")

  if ! printf '%s' "$verdict" | grep -q '^ACCEPT'; then
    log "reviewer REJECTED phase $id"
    log "verdict saved: $review_log"
    die "phase $id rejected by reviewer — stopping for manual inspection"
  fi
  log "phase $id accepted"
}

commit_phase() {
  local id="$1"
  local fid; fid="$(phase_file_id "$id")"
  git add -A ':!logs'
  if git diff --cached --quiet; then
    warn "phase $id produced no changes — nothing to commit"
    return
  fi
  local out="$DOCS/phase-${fid}-output.md"
  local subject="feat(auto): phase $id"
  if [ -f "$out" ]; then
    local first_line; first_line="$(head -1 "$out" | sed 's/^#\s*//')"
    [ -n "$first_line" ] && subject="$subject — $first_line"
  fi
  git commit -m "$subject" -m "Auto-driver commit. See $out for details."
  git tag -f "phase-${fid}" >/dev/null
  log "committed: $subject (tag phase-${fid})"
}

# ---------- orchestration ----------
main() {
  mkdir -p "$LOGS" "$DOCS"
  : > "$DRIVER_LOG"
  cleanup_all() {
      stop_heartbeat
      stop_server
      pkill -TERM -f 'claude -p Phase' 2>/dev/null || true
      pkill -TERM -f 'timeout.*claude' 2>/dev/null || true
      sleep 2
      pkill -KILL -f 'claude -p Phase' 2>/dev/null || true
      pkill -KILL -f 'timeout.*claude' 2>/dev/null || true
  }
  trap cleanup_all EXIT INT TERM

  # branch
  if [ "$(git branch --show-current)" != "$BRANCH" ]; then
    if git show-ref --quiet "refs/heads/$BRANCH"; then
      log "switching to existing branch $BRANCH"
      git checkout "$BRANCH"
    else
      log "creating branch $BRANCH from current HEAD"
      git checkout -b "$BRANCH"
    fi
  fi

  # prereq
  command -v claude >/dev/null || die "claude not found in PATH"
  command -v timeout >/dev/null || die "timeout (coreutils) not found"
  command -v llama-runner >/dev/null || die "llama-runner not found in PATH"
  [ -f prompts/system-agent.md ] || die "prompts/system-agent.md missing"
  [ -f prompts/reviewer.md ] || die "prompts/reviewer.md missing"

  if [ "$DRY_RUN" = "1" ]; then
    log "DRY RUN — plan only"
    for entry in "${PHASES[@]}"; do
      IFS='|' read -r id kind prompt desc <<< "$entry"
      echo "  phase $id ($kind) — $desc — $prompt"
    done
    return 0
  fi

  claude_env
  start_server

  local started=0
  for entry in "${PHASES[@]}"; do
    IFS='|' read -r id kind prompt desc <<< "$entry"
    if [ -n "$FROM" ] && [ "$started" = "0" ] && [ "$id" != "$FROM" ]; then
      log "skipping phase $id (before --from $FROM)"
      continue
    fi
    started=1

    run_phase "$id" "$kind" "$prompt" "$desc"

    if [ "$SKIP_REVIEW" = "0" ]; then
      run_review "$id"
    fi

    commit_phase "$id"

    if [ -n "$TO" ] && [ "$id" = "$TO" ]; then
      log "reached --to $TO, stopping"
      break
    fi
  done

  log "=== final verification ==="
  gofmt -l . | tee -a "$DRIVER_LOG"
  go build ./...  2>&1 | tee -a "$DRIVER_LOG"
  go vet ./...    2>&1 | tee -a "$DRIVER_LOG"
  go test -race -cover ./... 2>&1 | tail -40 | tee -a "$DRIVER_LOG"

  log "DONE. Branch: $BRANCH. Tags: $(git tag -l 'phase-*' | tr '\n' ' ')"
  log "Inspect: git log --oneline, docs/auto/phase-*-output.md"
  log "Next: run 'llama-runner -auto -v' manually to verify end-to-end"
}

main "$@"