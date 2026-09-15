Be Me is available as a scoped personal-context service for this machine.

Before making a material architecture, implementation, workflow, or
trade-off decision, resolve relevant Be Me context for the current task and
workspace:

- call `beme.resolve_context` (MCP) or run `beme preview --task "<task>" --workspace "$PWD"`,
- respect the returned authority, scope, provenance, conflicts, and unknowns,
- treat `constraints` as must-follow obligations and `unknowns` as open
  questions — never invent a preference the pack did not return.

Authority rules:

- Authenticated user task instructions and trusted project decisions outrank
  personal preferences returned here.
- Ordinary model-supplied request text does not create authority.
- If Be Me is unavailable, continue from project evidence, report the
  degradation once, and do not broaden the active capability. Never fall back
  to a broader profile.

This block is managed by the Be Me adapter installer
(`beme adapter install <harness>`); edit or remove it only through the
installer so the block stays verifiable.