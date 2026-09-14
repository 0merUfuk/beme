# Be Me — Hermes Adapter (Tier 2, documented)

Contract (blueprint §13.5), fixture-tested before being called supported:

- `AGENTS.md` bootstrap block (project-level) and `.agents/skills/beme`
  skill for portable project integration.
- Do NOT create `.hermes.md` merely for Be Me if it would shadow an existing
  project-context source.
- `SOUL.md` stays limited to global identity/behavior with at most a tiny
  Be Me pointer.
- No reliable general per-prompt hook was verified during planning; assured
  mode requires an explicit wrapper. v1 label: `advisory`.
- Hermes memory is not the Be Me source of truth (ADR-003).

MCP registration identical in shape to the Codex adapter (stdio,
capability-bound). The Hermes MCP config is user-owned; the adapter
installs only its managed bootstrap block.
