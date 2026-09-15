# Be Me — Operations

## Directories (FR-062/063; platform APIs, no hard-coded ~/.beme)

| Purpose | macOS | Linux (XDG) | Windows | Override |
|---|---|---|---|---|
| config | `~/Library/Application Support/beme` | `$XDG_CONFIG_HOME/beme` (`~/.config/beme`) | `%AppData%\beme` | `BEME_CONFIG_HOME` |
| data (projections) | `~/Library/Application Support/beme` | `$XDG_DATA_HOME/beme` (`~/.local/share/beme`) | `%AppData%\beme` | `BEME_DATA_HOME` |
| cache (indexes, packs, temp) | `~/Library/Caches/beme` | `$XDG_CACHE_HOME/beme` (`~/.cache/beme`) | `%LocalAppData%\beme` | `BEME_CACHE_HOME` |

The durable tombstone ledger lives under the canonical root
(`<config>/ledger/`, ADR-027), never in the data dir, so restoring or
rebuilding derived data cannot resurrect forgotten or purged records:

| File | Contents | Handling |
|---|---|---|
| `tombstones.json` | logical-forget keys; keyed purge fingerprints (no content) | back up with the canonical root |
| `purge.key` | HMAC key for purge fingerprints (0600) | back up with the ledger; never share or commit (git-ignored) |
| `pending/*.json` | journal of an interrupted purge (identifiers only) | transient; removed when the purge completes (git-ignored) |

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
beme forget rec_xxx "reason"      # logical forget (store + durable ledger)
beme purge --confirm rec_xxx --dry-run rec_xxx            # show what a purge would erase
beme purge --confirm rec_xxx --remove-canonical rec_xxx   # RED: irreversible physical purge
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
- `beme forget` tombstones a record in its projection and in the durable
  ledger; rebuilds, corrupt-store recovery, migration rollback, and restored
  backups do not resurrect it. The content stays on disk until purged.
- `beme purge` (RED, typed `--confirm`) erases the record from both
  projection stores (files rewritten), persisted traces, and restating
  observations, optionally deletes the source files, and leaves only a keyed
  identity fingerprint. It reports what it cannot erase — Git history,
  external backups — with the remediation. Exit codes: 3 unconfirmed,
  4 key not found.
- An interrupted purge (crash, full disk, locked file) leaves a journal;
  `beme doctor` reports it as degraded. Re-run the same `beme purge` command
  to finish; the report shows `resumed`. Re-running a completed purge prints
  `already purged` and changes nothing.
- If `ledger/purge.key` is lost while purge entries exist, resolution,
  rebuild, and purge fail closed until the key is restored from backup.
- Logs are structured and sanitized: no raw task text, personal record text,
  source excerpts, absolute personal paths, or credentials (NFR-014).
- Telemetry: none. Network: none (offline by default; FR-060).

## Performance benchmark (NFR-008)

```sh
make bench    # go run ./cmd/beme-bench --seed testdata/synthetic/records.json --out evals/benchmarks/seed-baseline.json
```

Builds disposable deployments from the public seed corpus at 1×, 20×, and
200× scale, then records projection build time and warm resolution latency
(p50/p95/p99/max) with the machine's OS, architecture, CPU count, and Go
version. Timings are machine-dependent evidence, so `bench` is not part of
`make check`; `--enforce` exits 1 when a scale misses the p95 target.

## Health states

`healthy` · `degraded` (missing/stale source) · `unavailable` (cannot
resolve) · `policy_blocked` (ambiguity, elevation attempt, corrupt policy) ·
`rebuild_required` (index unusable, sources intact). `beme doctor` reports
these with exact remediation.
