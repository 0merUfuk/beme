# Be Me — Hermes Integration (Tier 2, manual contract)

There is **no managed installer** for Hermes in this version. The managed CLI
targets only Codex and Claude Code; see
[the current integration boundary](../../docs/INTEGRATIONS.md#harness-adapters).
The following describes proposed manual integration surfaces, not automatic
installation or verified harness/model behavior (blueprint §13.5):

- An `AGENTS.md` bootstrap pointer at project level and a portable skill
  template. Skill discovery/location must be verified against the installed
  Hermes version; Be Me neither installs nor guarantees discovery of a skill.
- Do NOT create `.hermes.md` merely for Be Me if it would shadow an existing
  project-context source.
- `SOUL.md` stays limited to global identity/behavior with at most a tiny
  Be Me pointer.
- No reliable general per-prompt hook was verified during planning; assured
  mode requires an explicit wrapper. v1 label: `advisory`.
- Hermes memory is not the Be Me source of truth (ADR-003).

MCP registration identical in shape to the Codex adapter (stdio,
capability-bound). Hermes configuration and any manual bootstrap/skill edits
are user-owned. This CLI does not edit them or verify a Hermes installation.
