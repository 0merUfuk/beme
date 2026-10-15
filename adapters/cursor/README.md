# Be Me — Cursor Integration (Tier 2, manual contract)

There is **no managed installer** for Cursor in this version. The managed CLI
targets only Codex and Claude Code; see
[the current integration boundary](../../docs/INTEGRATIONS.md#harness-adapters).
The following is a manual integration contract, not installed behavior or
verified Cursor/model support (blueprint §13.6):

- A repository bootstrap pointer in `AGENTS.md`, preserving existing content.
- A Cursor-specific rule may use `.cursor/rules/beme.mdc` for Always Apply
  behavior after the operator verifies that mechanism in the installed version.
  Be Me does not create or verify that file.
- MCP registration at user or project scope; project scope may not widen
  projection/capability (FR-013).
- Fire-and-forget/session hooks and cloud-agent behavior are NOT treated as
  guaranteed prompt injection; they never justify an `assured` label.
- Standard mode is `advisory`; an explicit assured workflow/wrapper is the
  only possible path to an `assured` label, requiring separate measured
  pre-decision-use evidence (FR-045/046).
