# Be Me — Claude Code Adapter (Tier 1)

Managed bootstrap for Claude Code: a tiny `CLAUDE.md` pointer block (never a
copy of canonical content), MCP registration, and optional hooks where the
installed version supports them.

## Install

```sh
beme adapter install claude-code
```

Effects (idempotent, marker-delimited):

1. Installs the managed bootstrap block into the user-level `~/.claude/CLAUDE.md`.
2. Prints the MCP registration fragment for `~/.claude.json` / project
   `.mcp.json` (user-owned; the adapter does not write harness config
   outside its managed block).
3. Where the installed Claude Code version supports hooks, prints a
   `UserPromptSubmit` hook snippet (verified against the installed version —
   hook availability is version-sensitive; see `docs/INTEGRATIONS.md`).

## Uninstall

```sh
beme adapter remove claude-code
```

Only the managed block goes; unrelated content is preserved (FR-043).

## Why a pointer, not a copy

Per the canonical governance model: adapter files contain pointers to
canonical content, never copies, because copies diverge. The Be Me bootstrap
points at the runtime; the runtime reads canonical sources through adapters.

## MCP registration (reference)

```json
{
  "mcpServers": {
    "beme": {
      "command": "beme",
      "args": ["serve", "--projection", "work-safe", "--capability", "work-safe-default", "--transport", "stdio"]
    }
  }
}
```

Project `.mcp.json` may register the same server but must not widen
projection or capability (FR-013). Project MCP configuration is untrusted
until the harness trust flow has approved it (blueprint §13.4).

## Assurance

`advisory` in v1: `SessionStart`/`UserPromptSubmit` hooks can encourage
resolution but measured 100% pre-decision use is required before any surface
is labeled `assured` (FR-045/046). Hook mechanics verified against installed
versions at WP8 time.
