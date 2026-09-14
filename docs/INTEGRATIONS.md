# Be Me — Integrations

## MCP server

`beme serve --projection <personal|work-safe> --capability <name> --transport stdio`

Stdio only in v1 (ADR-014). Each serving process binds one immutable
capability and one projection store; requests may narrow, never widen
(FR-010/011).

Tool surface (ADR-009, contract-tested):

| Tool | Purpose |
|---|---|
| `beme.resolve_context` | scoped ContextPack for the current task |
| `beme.get_context_item` | expand a record already authorized in a current pack |
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
(FR-045/046). No surface is labeled assured in v1; hook mechanics are
version-sensitive and were not verified against installed harnesses during
this build session. Before calling any surface supported, verify against the
currently installed harness version.

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
