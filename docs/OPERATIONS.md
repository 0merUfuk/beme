# Be Me — Operations

## Directories (FR-062/063; platform APIs, no hard-coded ~/.beme)

| Purpose | Default (macOS) | Override |
|---|---|---|
| config | `~/Library/Application Support/beme` | `BEME_CONFIG_HOME` |
| data (projections) | `~/Library/Application Support/beme` | `BEME_DATA_HOME` |
| cache (indexes, packs, temp) | `~/Library/Caches/beme` | `BEME_CACHE_HOME` |

Deployment layout:

```
<config>/beme/
├── config.yaml          # canonical_root, data_dir, cache_dir (optional)
├── sources/              # source descriptors (registration = the trust act)
├── policies/            # workspace identity registry
├── capabilities/        # (reserved) capability definitions
└── adapters/            # (reserved) adapter installation state
<data>/beme/projections/personal/store.db     # separate files per profile
<data>/beme/projections/work-safe/store.db
<cache>/beme/observations/                     # quarantined feedback
```

## Lifecycle

```sh
beme doctor                       # health: sources, projections, findings
beme build --profile all         # rebuild projections from registered sources
beme status                       # registered sources + workspaces
beme preview --task "..." --workspace "$PWD"   # human pack preview
beme preview --task "..." --json # machine pack
beme forget rec_xxx "reason"      # logical tombstone (revokes from resolution)
beme adapter install codex        # managed bootstrap block

# Learning review (batch, low-friction):
beme candidate list                       # quarantined observations pending review
beme candidate inspect obs_xxxxxxxx
beme candidate review obs_xxxxxxxx --action reject --note "reason"
# actions: approve|edit|merge|reject|defer|situational|scope_limit|counterexample
# approve records the decision; the canonical write happens in the owning
# repository's proposal path. Be Me never writes canonical knowledge.
```

## Recovery

- Projection stores are derived and rebuildable: delete them and re-run
  `beme build` (FR-004/065). Canonical sources are never touched by a rebuild.
- `beme forget` tombstones a record; the next rebuild does not resurrect it
  (tombstones live in the store; a full store wipe + rebuild from a source
  whose content changed requires re-registration review).
- Logs are structured and sanitized: no raw task text, personal record text,
  source excerpts, absolute personal paths, or credentials (NFR-014).
- Telemetry: none. Network: none (offline by default; FR-060).

## Health states

`healthy` · `degraded` (missing/stale source) · `unavailable` (cannot
resolve) · `policy_blocked` (ambiguity, elevation attempt, corrupt policy) ·
`rebuild_required` (index unusable, sources intact). `beme doctor` reports
these with exact remediation.
