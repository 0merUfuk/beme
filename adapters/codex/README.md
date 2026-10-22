# Be Me — Codex Adapter (Tier 1)

Managed bootstrap for Codex: a small `AGENTS.md` block plus a printed MCP
registration example. Registration is manual; availability is not enforcement.

## Install

```sh
beme adapter install codex
```

Effects (all idempotent, marker-delimited):

1. Installs the managed bootstrap block into the *user-level* Codex
   `AGENTS.md` (not any project's file — project files belong to their
   projects).
2. Prints the MCP server registration fragment for the user's Codex MCP
   config (Codex config is user-owned; the adapter never writes credentials
   or silently edits harness config outside its managed block).

The installer **does not install skills**. The optional
[skill template](templates/skill-beme.md) is a manual reference; verify the
installed harness's skill discovery and location before placing it. The CLI
does not detect skill support, copy the template or verify its discovery.

## Uninstall

```sh
beme adapter remove codex
```

Removes only the managed block; unrelated content and any manually installed
skills are preserved (FR-043). `beme adapter verify codex` checks that block,
not MCP registration or skill discovery. See
[INTEGRATIONS.md](../../docs/INTEGRATIONS.md#harness-adapters).

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

`personal` projection is configured by the operator directly for personal
projects; the adapter's default registration is work-safe, the narrower
ceiling.

## Assurance

`advisory` in v1: Codex has no verified prompt-aware hook that can prove
pre-decision resolution on every material decision. Per-harness hook
verification happens against installed versions at WP8 time (§13.8);
a surface is labeled `assured` only with measured 100% pre-decision use.

## Version sensitivity

Codex agent configuration, MCP, skills, and plugin behavior are
version-sensitive; revalidate against current official documentation when
installing (`docs/INTEGRATIONS.md` records tested versions).
