# Be Me — Claude Code Adapter (Tier 1)

Managed bootstrap for Claude Code: a tiny `CLAUDE.md` pointer block (never a
copy of canonical content) plus a printed manual MCP registration example.
Hook configuration is separate from this V1 installer.

## Install

```sh
beme adapter install claude-code
```

Effects (idempotent, marker-delimited):

1. Installs the managed bootstrap block into the user-level `~/.claude/CLAUDE.md`.
2. Prints the MCP registration fragment for `~/.claude.json` / project
   `.mcp.json` (user-owned; the adapter does not write harness config
   outside its managed block).

The V1 installer **does not print prompt hooks** or configure
`UserPromptSubmit`. Any such hook is an operator-managed, version-sensitive
integration, not an installer effect. See
[INTEGRATIONS.md](../../docs/INTEGRATIONS.md#harness-adapters) and the separate
brain-hook commands below.

## Optional brain hooks

`beme brain hooks` prints a fragment for SessionStart, PreCompact and
SessionEnd. `beme brain install-hooks --settings FILE --dry-run` previews its
separate settings merge; only an explicit invocation without `--dry-run`
changes settings. V1 adapter installation does not install these hooks.
See [`docs/BRAIN.md`](../../docs/BRAIN.md). No installed-harness or model-use
assurance is claimed by this source integration.

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
