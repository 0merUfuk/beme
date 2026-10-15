# Example deployment profile (synthetic)

A fully synthetic example of a Be Me deployment configuration. All names,
paths, and sources are fictional. Real deployment configuration lives
outside this repository, under the platform user-config directory.

Synthetic deployment layout; see [OPERATIONS.md](../../docs/OPERATIONS.md)
for executable setup instructions:

```
<beme-config>/
├── config.yaml          # canonical source roots, capability definitions
├── sources/             # source descriptors (schema: schemas/source/)
├── policies/            # workspace identity registry (schema: schemas/policy/)
├── capabilities/        # personal / work-safe capability definitions
└── adapters/            # harness adapter installation state
```

See `schemas/source/source-descriptor.schema.json` and
`schemas/policy/workspace.schema.json` for the contracts these files follow.
