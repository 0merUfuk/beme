# Be Me — Cursor Adapter (Tier 2, documented)

Contract (blueprint §13.6), fixture-tested before being called supported:

- `AGENTS.md` for portable repository bootstrap (same managed block).
- `.cursor/rules/beme.mdc` only for Cursor-specific Always Apply behavior —
  installed by `beme adapter install cursor` behind markers.
- MCP registration at user or project scope; project scope may not widen
  projection/capability (FR-013).
- Fire-and-forget/session hooks and cloud-agent behavior are NOT treated as
  guaranteed prompt injection; they never justify an `assured` label.
- Standard mode is `advisory`; an explicit assured workflow/wrapper is the
  only path to an `assured` label (FR-045/046).
