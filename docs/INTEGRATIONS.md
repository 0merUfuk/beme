# Be Me — Integrations

## MCP server

`beme serve --projection <personal|work-safe> --capability <name> --transport stdio`

Stdio only in v1 (ADR-014). Each serving process binds one immutable
capability and one projection store; requests may narrow, never widen
(FR-010/011).

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

`adapter install|remove|verify <harness>` manages the bootstrap block
(marker-delimited; unrelated config preserved; idempotent). MCP registration
is printed for the operator to add — harness config files are user-owned and
never silently edited outside the managed block.

`assured` labeling requires a verified harness boundary resolving context
before every material decision with measured 100% pre-decision use
(FR-045/046). No surface is labeled assured in v1.

### Verification levels (installed harnesses, 2026-09-15)

Four levels are tracked separately; a higher level is never inferred from a
lower one.

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
