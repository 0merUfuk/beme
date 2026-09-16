# Be Me — Integrations

## MCP server

`beme serve --projection <personal|work-safe> --capability <name> --transport stdio`

Stdio only in v1 (ADR-014). Each serving process binds one immutable
capability and one projection store; requests may narrow, never widen
(FR-010/011). `--capability` is a free-form label you choose (for example
`cap_daily`); it names the capability bound to that process and appears in
traces, so pick something you will recognize.

Smoke-check the server without installing a harness — it completes the stdio
handshake and exits, with no model call:

```sh
{ printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'
  sleep 2; } \
  | beme serve --projection personal --capability cap_smoke --transport stdio
```

A JSON-RPC result naming `beme` and its version means the handshake
completed, and the command exits 0. The `sleep` is required: the server shuts
down as soon as its input closes, so piping `printf` alone closes stdin before
the reply is written and the command fails with `mcp server exited: server is
closing: EOF` (exit 1). An interactive session likewise needs stdin held open.

Expansion is pack-bound (ADR-029): pass the `pack_id` of a pack returned by
`beme.resolve_context` in the same server session and the `record_id` of an
item selected into it. Packs expire after 30 minutes and on server restart or
projection rebuild; unknown, expired, unselected, denied, revoked, and
nonexistent items all return "context item not available". Every tool reads
through the durable tombstone ledger and returns `policy_blocked` when it
cannot be read.

Tool surface (ADR-009, contract-tested):

| Tool | Purpose |
|---|---|
| `beme.resolve_context` | scoped ContextPack for the current task |
| `beme.get_context_item` | expand a record selected into a ContextPack this server issued in the same session; requires `pack_id` + `record_id` (ADR-029) |
| `beme.report_feedback` | quarantined observation/correction candidate |
| `beme.status` | safe health/capability metadata (private sources hidden in work-safe) |

## Harness adapters

| Harness | Tier | Mechanism | Assurance (v1) |
|---|---|---|---|
| Codex | 1 | user-level `AGENTS.md` managed block + MCP + skill | advisory |
| Claude Code | 1 | user-level `CLAUDE.md` managed block + MCP + hooks where supported | advisory |
| Hermes | 2 | `AGENTS.md` block + `.agents/skills/beme` + MCP | advisory |
| Cursor | 2 | `AGENTS.md` block + `.cursor/rules/beme.mdc` + MCP | advisory |

The `adapter` subcommand manages the bootstrap block for the harnesses whose
instruction files it knows: `codex` or `claude-code`. The other rows describe the
documented contract; register those manually.

`adapter install|remove|verify <harness>` manages the bootstrap block
(marker-delimited; unrelated config preserved; idempotent). MCP registration
is printed for the operator to add — harness config files are user-owned and
never silently edited outside the managed block.

`assured` labeling requires a verified harness boundary resolving context
before every material decision with measured 100% pre-decision use
(FR-045/046). No surface is labeled assured in v1.

### Verification levels (installed harnesses, re-run 2026-09-16)

Four levels are tracked separately; a higher level is never inferred from a
lower one. The table below was re-run at the final head against installed
Claude Code 2.1.271 and Codex 0.154.0 in isolated config homes, with no model
calls: `TestAdapterInstallRemoveByteExact`, `TestClaudeCodeHarnessIntegration`,
`TestCodexHarnessIntegration` and `TestMCPClientEndToEnd` all pass.

| Harness (installed) | MCP protocol | Config lifecycle | Harness connection | Pre-decision use |
|---|---|---|---|---|
| Claude Code 2.1.271 | verified | verified | verified | not verified |
| Codex 0.154.0 | verified | verified | not verified | not verified |
| Hermes | verified | documented contract only | not run | not verified |
| Cursor (not installed) | verified | documented contract only | not run | not verified |

- **MCP protocol** — a real MCP client (official Go SDK) drives the real
  stdio server (`TestMCPClientEndToEnd`). Harness-independent, so it holds
  for every row.
- **Config lifecycle** — the installed harness CLI registers, parses, lists,
  and removes the beme server in an isolated config home
  (`TestClaudeCodeHarnessIntegration`, `TestCodexHarnessIntegration`), and
  `beme adapter install|remove` is byte-exact on the harness instruction file
  (`TestAdapterInstallRemoveByteExact`).
- **Harness connection** — the harness itself spawns beme and completes the
  MCP handshake. Claude Code: `claude mcp get beme` reports Connected. Codex
  has no MCP health check without a model session (`codex doctor` validates
  config only; `codex exec` would spend model usage), so it stays
  unverified.
- **Pre-decision use** — whether the agent resolves context before each
  material decision. Measuring it needs live model sessions (owner-run), so
  it is unverified for every harness and no surface is `assured`.

Run the installed-harness tests locally (they skip in CI; no model calls,
real harness configs untouched):

```sh
BEME_HARNESS_INTEGRATION=1 go test ./cmd/beme -run HarnessIntegration -v
```

### Owner-run procedure for the unverified levels (H3–H5)

Pre-decision use cannot be measured without live model sessions, which spend
the owner's model usage. The procedure below is prepared and runnable; it has
NOT been run. It uses a synthetic deployment only — no private corpus, no
canonical personal knowledge.

```sh
# 1. Isolated deployment with synthetic sources (nothing of yours is read)
export BEME_HOME="$(mktemp -d)"
export BEME_CONFIG_HOME="$BEME_HOME/cfg" BEME_DATA_HOME="$BEME_HOME/data" BEME_CACHE_HOME="$BEME_HOME/cache"
# (descriptor and entry shapes: docs/OPERATIONS.md "Registering a source")
mkdir -p "$BEME_CONFIG_HOME/sources" "$BEME_HOME/src/entries"
cat > "$BEME_HOME/src/entries/SYN-001.md" <<'ENTRY'
---
id: SYN-001
title: "Synthetic preference"
type: preference
status: active
---

Prefer small, reviewable changes over large rewrites.
ENTRY
cat > "$BEME_CONFIG_HOME/sources/syn.yaml" <<DESC
schema_version: "1"
source_id: synthetic-predecision
type: directory
root: $BEME_HOME/src
purpose: [reusable_knowledge]
trust: canonical
instruction_semantics: registered_files_only
authority_ceiling: default
sensitivity: personal_private
profiles_allowed: [personal]
ingestion_mode: index_content
include: ["entries/**/*.md"]
DESC
go install ./cmd/beme
export PATH="$(go env GOPATH)/bin:$PATH"
beme build --profile personal

# 2. Register the server in an ISOLATED harness config home
export CLAUDE_CONFIG_DIR="$BEME_HOME/claude"          # Codex: CODEX_HOME
claude mcp add beme -- "$(go env GOPATH)/bin/beme" serve   --projection personal --capability cap_predecision
claude mcp get beme                                    # expect: Connected

# 3. One live session per task, with the managed bootstrap block installed.
#    `adapter install` resolves ~/.claude/CLAUDE.md through the OS home
#    directory, so run it with HOME pointed at the isolated deployment —
#    otherwise it edits your real harness instruction file.
HOME="$BEME_HOME" beme adapter install claude-code
#    run three tasks that each require a material decision the synthetic
#    corpus has an opinion about, plus one negative-control task the corpus
#    says nothing about

# 4. Evidence to keep (outside the repository)
#    - the session transcript showing beme.resolve_context calls
#    - for each material decision: whether the call precedes it (H4)
#    - whether the decision uses the retrieved item, and whether the
#      negative control produced an invented preference (H5)
```

Acceptance: H3 needs one transcript with a real `beme.resolve_context` call;
H4 needs the call to precede each material decision; H5 needs the retrieved
item used and zero invented preferences on the negative control. Report the
counts, not the transcript text, in `docs/ACCEPTANCE.md`.

### Owner-run procedure for the evaluation gates (E5, E6)

```sh
# Retrieval measurement on the private corpus (ADR-023; owner-gated, local).
# No model is called: retrieval metrics come from the resolver itself.
export BEME_PRIVATE_EVAL_DIR=/path/to/private/corpus     # never in the repo
export BEME_EVAL_GIT_COMMIT="$(git rev-parse HEAD)"      # worktrees stamp the main checkout
make validate-private                                     # schema check first
go run ./cmd/beme-eval retrieval \
  --corpus "$BEME_PRIVATE_EVAL_DIR" --config "$BEME_CONFIG_HOME" \
  --out "$BEME_HOME/evidence" --json

# Blind paired behavioral run (E6) — SPENDS MODEL USAGE, owner decision.
# --dry-run first reports how many generations would run, executing nothing.
go run ./cmd/beme-eval behavioral \
  --corpus "$BEME_PRIVATE_EVAL_DIR" --config "$BEME_CONFIG_HOME" \
  --provider command --command '<your harness CLI> --stdin' \
  --arms B0,B4 --repeats 3 --out "$BEME_HOME/evidence" --dry-run
```

Exit codes follow `Summary.ExitCode`: 0 passed, 1 failed, 3 not_run (for
example no corpus given), 2 usage.

Evidence bundles stay outside the repository (ADR-023). Only `blinded/` is
grader-visible; manifests record the observed model settings, prompt hashes
and corpus revisions.

## Rifja (ADR-019)

Episodic evidence enters through Rifja's existing bounded surfaces (MCP
stdio tool server or `--json` CLI). Be Me normalizes authorized results as
precedent/evidence references. No second transcript importer exists in Be
Me. If deeper factoring becomes desirable, an ADR precedes any ownership
move. Rifja availability is a degradation, not a failure.

## Version sensitivity

Codex/Claude Code/Cursor/Hermes hook and skill behavior and the MCP spec
change over time. Pin minimum tested versions and record installed,
documented, ported, and unverified combinations separately (revalidation
list: blueprint §13.8).
