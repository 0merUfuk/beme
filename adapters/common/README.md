# Be Me — Shared Adapter Assets (common)

The common block every harness adapter installs. Managed content is minimal,
idempotently installable/removable, and never copies the user's profile —
it points at the runtime (blueprint §13.1).

## Managed bootstrap block (v1)

The canonical text lives in `skill/BOOTSTRAP.md`. Installers copy it into a
managed region delimited by markers:

```markdown
<!-- BEGIN beme:managed -->
...bootstrap text...
<!-- END beme:managed -->
```

Rules:

- Install/verify/remove operate only inside the markers (FR-043).
- Unrelated user content outside markers is preserved byte-for-byte.
- `beme adapter verify <harness>` reports drift between installed blocks and
  the canonical text.

## MCP configuration fragment

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

The `--capability` value is deployment configuration, owned by the operator,
never by a repository. A repository-local config may point at the same
command but must not change the projection or capability (FR-013).

## Assurance labels

- `advisory` — bootstrap + MCP tool availability encourage pre-decision use;
  the harness cannot guarantee timing. Default label for all surfaces in v1.
- `assured` — requires a verified harness boundary that resolves and
  integrity-binds context before every material decision on the claimed
  surface, with measured 100% pre-decision use (FR-045). No surface is
  labeled assured without that measurement (FR-046).

See `docs/INTEGRATIONS.md` (in the repo docs set) for per-harness mechanics.