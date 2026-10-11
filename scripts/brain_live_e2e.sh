#!/usr/bin/env bash
# Live verification of Be Me V2 session continuity against a REAL Claude Code.
#
# Owner-run, NOT part of CI: stages 2-5 invoke real model calls (paid/quota) and
# need a logged-in `claude`. Same honesty contract as ADR-023: when the model
# stages cannot run the script says `not_run` (exit 3) — it never reports a
# pass it did not observe.
#
#   Stage 1  isolated deployment (own config root; never touches real dirs)
#   Stage 2  harness hook contract (also invokes a model): `claude` executes the
#            SessionStart hook and accepts an output that carries seeded memory
#   Stage 3  SESSION 1  — agent follows the injected protocol and captures
#   Stage 4  SESSION 2  — clean session, NO tools: must recall from injection
#   Stage 5  CONTROL    — same question from an unregistered directory: must not
#
# Usage:  scripts/brain_live_e2e.sh            (evidence: $BRAIN_LIVE_DIR or a temp dir)
# Needs:  go, python3, git, claude (logged in for stages 3-5)
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=${BRAIN_LIVE_DIR:-$(mktemp -d)}
CLAUDE=${CLAUDE_BIN:-claude}
MODEL_ARGS=()
[ -n "${BRAIN_LIVE_MODEL:-}" ] && MODEL_ARGS=(--model "$BRAIN_LIVE_MODEL")
EVID=$WORK/evidence
mkdir -p "$WORK/bin" "$WORK/cfg" "$WORK/proj/probe" "$WORK/proj/alpha" "$WORK/proj/elsewhere" "$EVID"
fail=0
say() { printf '\n== %s\n' "$*"; }
ok()  { printf 'PASS  %s\n' "$*"; }
bad() { printf 'FAIL  %s\n' "$*"; fail=1; }

command -v "$CLAUDE" >/dev/null || { echo "claude not found"; exit 1; }

say "Stage 1 — isolated deployment ($WORK)"
(cd "$ROOT" && go build -o "$WORK/bin/beme" ./cmd/beme) || { echo "build failed"; exit 1; }
export PATH="$WORK/bin:$PATH"
B="beme"
CFG="--config $WORK/cfg"
git -C "$WORK/proj/alpha" init -q 2>/dev/null
$B brain init $CFG >/dev/null
$B brain project add probe --root "$WORK/proj/probe" --name "Probe Service" $CFG >/dev/null
$B brain project add alpha --root "$WORK/proj/alpha" --name "Alpha Service" $CFG >/dev/null
cat > "$WORK/cfg/data/brain/identity.md" <<'EOF'
# Identity

Role: execution engineer working for the operator.
Decision rule: make the call with one recommendation; do not send option menus.
Evidence rule: "it works" is a claim until verified.
EOF
# the real installer (not a hand-written file): its output is what stage 2 exercises
$B brain install-hooks --settings "$WORK/settings.json" --binary "$WORK/bin/beme" $CFG >/dev/null
# Seeded memory for the owner-run hook-contract stage (probe project).
$B brain capture $CFG --project probe --type decision --title "Use SQLite for local state" --topic storage \
  --body "Local state lives in a single SQLite file, not a server database." --why "Zero-ops and offline-first." >/dev/null
$B brain capture $CFG --project probe --type open --title "Wire retry policy" \
  --body "Retry policy for the sync client is not wired yet." --next "Add exponential backoff" >/dev/null
$B brain doctor --quiet $CFG
ok "deployment ready; hooks installed into $WORK/settings.json by install-hooks"

# run_claude <cwd> <evidence-name> <prompt> [extra claude args…]
run_claude() {
  local cwd=$1 name=$2 prompt=$3; shift 3
  (cd "$cwd" && "$CLAUDE" -p "$prompt" --setting-sources project --settings "$WORK/settings.json" \
      --output-format stream-json --verbose --include-hook-events --no-session-persistence \
      ${MODEL_ARGS[@]+"${MODEL_ARGS[@]}"} "$@" < /dev/null > "$EVID/$name.jsonl" 2> "$EVID/$name.err")
}
# extract <name> <what>: hook_ctx | result | events
extract() {
python3 - "$EVID/$1.jsonl" "$2" <<'PY'
import json, sys
path, what = sys.argv[1], sys.argv[2]
hook, result, outcome = "", "", ""
for line in open(path):
    try: e = json.loads(line)
    except Exception: continue
    if e.get("type") == "system" and e.get("subtype") == "hook_response" and e.get("hook_event") == "SessionStart":
        outcome = e.get("outcome", "")
        try: hook = json.loads(e.get("stdout") or "{}")["hookSpecificOutput"]["additionalContext"]
        except Exception: hook = ""
    if e.get("type") == "result": result = e.get("result") or ""
print({"hook_ctx": hook, "result": result, "outcome": outcome}[what])
PY
}

say "Stage 2 — harness hook contract (real claude executes the SessionStart hook)"
run_claude "$WORK/proj/probe" stage2 "Reply with the single word: pong" --tools ""
oc=$(extract stage2 outcome); ctx=$(extract stage2 hook_ctx)
[ "$oc" = "success" ] && ok "SessionStart hook ran under the real harness (outcome=success)" || bad "hook outcome: '${oc:-none}' (see $EVID/stage2.jsonl)"
echo "$ctx" | grep -q "Use SQLite for local state" && echo "$ctx" | grep -q "Wire retry policy" \
  && ok "hook output carries the seeded decision and open item (${#ctx} chars)" || bad "hook output is missing seeded memory"
[ "${#ctx}" -gt 0 ] && [ "${#ctx}" -le 10000 ] && ok "hook output present and within the harness cap" || bad "hook output empty or over 10000 chars"

say "Stages 3-5 — model-level recall"
if ! "$CLAUDE" auth status 2>/dev/null | grep -q '"loggedIn": true'; then
  echo "NOT_RUN  claude is not logged in: session 1/2/control need a real model call."
  echo "         Log in (\`claude auth login\`) and re-run: BRAIN_LIVE_DIR=$WORK $0"
  echo "         Stage 2 (hook contract) result above is real; stages 3-5 are unverified."
  [ $fail -eq 0 ] && exit 3 || exit 1
fi

say "Stage 3 — SESSION 1: capture by protocol"
run_claude "$WORK/proj/alpha" session1 \
"You are wrapping up a work session on this project. Facts from the session: (1) We decided to keep local state in a single SQLite file instead of a server database, because it needs zero ops and must work offline. (2) The sync client's retry policy is still NOT wired; the next step is to add exponential backoff. (3) The sync client itself landed behind a feature flag. (4) Along the way we tried three backoff libraries and two were flaky — scratch noise, not worth keeping. Follow your memory protocol to record what should persist, then answer with one line listing the note IDs you captured." \
  --allowedTools "Bash(beme brain:*)"
$B brain doctor $CFG | tee "$EVID/session1.doctor.txt"
$B brain list --project alpha $CFG | tee "$EVID/session1.list.txt"
n=$($B brain list --project alpha --json $CFG | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')
[ "$n" -ge 2 ] && ok "session 1 captured $n durable notes" || bad "session 1 captured only $n notes"
grep -qi "flaky\|three backoff libraries" "$EVID/session1.list.txt" && bad "scratch noise was persisted" || ok "trial-and-error noise was not persisted"

say "Stage 4 — SESSION 2: clean session, tools disabled (recall must come from injection)"
run_claude "$WORK/proj/alpha" session2 \
"Without using any tools: what architectural decisions have already been made on this project, and what work is still open? If you do not know, say you do not know." --tools ""
ans=$(extract session2 result); echo "$ans" | tee "$EVID/session2.answer.txt"
echo "$ans" | grep -qi "sqlite" && ok "recalled the storage decision" || bad "did not recall the SQLite decision"
echo "$ans" | grep -qi "backoff\|retry" && ok "recalled the open work" || bad "did not recall the open retry work"

say "Stage 5 — CONTROL: same question from an unregistered directory"
run_claude "$WORK/proj/elsewhere" control \
"Without using any tools: what architectural decisions have already been made on this project, and what work is still open? If you do not know, say you do not know." --tools ""
cans=$(extract control result); echo "$cans" | tee "$EVID/control.answer.txt"
echo "$cans" | grep -qi "sqlite" && bad "control leaked project memory" || ok "control has no access to the project's memory"

say "Result"
echo "evidence: $EVID"
[ $fail -eq 0 ] && { echo "LIVE E2E: PASSED"; exit 0; } || { echo "LIVE E2E: FAILED"; exit 1; }
