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
| `tombstones.json` | logical-forget keys; keyed purge fingerprints of record and observation identities (no content) | back up with the canonical root |
| `purge.key` | HMAC key, its key ID, and the generation of the last committed ledger write (0600) | back up with the ledger, from the same moment; never share or commit (git-ignored) |
| `pending/*.json` | journal of an interrupted purge (identifiers only) | transient; removed when the purge completes (git-ignored) |
| `.lock` | exclusive maintenance lock (forget, purge, build, learning writes) | transient (git-ignored) |

Missing `purge.key`, `pending/`, and `.lock` rules are added to an existing
`ledger/.gitignore` without touching other rules.

**Back up and restore both ledger files together** (ADR-030). They prove each
other: the key records the generation of the last committed ledger write, and
the ledger records which key it belongs to and carries an authentication tag
over its entries, so an edit that drops or adds tombstones is detected. Restoring one without the other,
restoring an older ledger over a newer key, or pairing a ledger with a
different key blocks every surface with an error naming both files — including
`beme build`, so a deployment stays unusable until they match. Removing both
files and every pending journal *together* is indistinguishable from a fresh
deployment: Be Me cannot detect that locally, which is why the canonical root
belongs in your backup set. An interrupted first purge (key created, ledger
not yet written) is recognized and recovers on its own.

Key custody: the key is per-deployment and only meaningful with its ledger.
Keep both out of shared checkouts (they are git-ignored). Losing the key while
purges exist blocks resolution, rebuild, and purge until it is restored, by
design — a purge that cannot be recognized cannot be enforced.

Durability (ADR-027 §5, ADR-030 §5): files are replaced by writing a temp
file, flushing it, renaming it, and flushing the parent directory on macOS and
Linux; on Windows the rename uses `MoveFileExW` with `MOVEFILE_WRITE_THROUGH`,
which is the documented equivalent (Windows exposes no user-mode directory
flush, so an unlinked name can reappear after power loss — the file's content
is zeroized and flushed before the unlink, so what can reappear is an empty
file). A purge erases nothing until the ledger and the journal have been
flushed; every deletion is zeroized, flushed, and its directory flushed, and
the projection stores are compacted with `synchronous=FULL` and flushed,
before the journal is removed. A retried purge completes flushes an
interrupted attempt could not. Any flush error aborts the purge, which stays
resumable. Storage that acknowledges flushes without performing them (volatile
drive caches, some network or virtualized filesystems) is outside this
guarantee, as are copies outside Be Me (backups, snapshots, filesystem
history), which the purge report lists as residuals.

Concurrency: `forget`, `purge`, `build`, and learning writes take the
exclusive `.lock`; a second one waits up to two minutes and then reports
"another Be Me maintenance operation is running". Read surfaces are never
blocked.

Deployment layout (`<config>`, `<data>`, and `<cache>` are the directories in
the table above; each already ends in `beme`):

```
<config>/
├── config.yaml          # canonical_root, data_dir, cache_dir (optional)
├── sources/             # source descriptors (registration = the trust act)
├── policies/            # workspace identity registry
└── ledger/              # durable tombstone ledger, purge key, pending purges, lock
<data>/projections/personal/store.db     # separate files per profile
<data>/projections/work-safe/store.db
<data>/observations/                     # quarantined feedback
<cache>/traces/                          # persisted resolver traces (explain)
```

## Registering a source

Registration is the trust act (FR-020): Be Me ingests a directory only after
you have written a descriptor for it by hand. **There is no `beme source
register` command** — you create one descriptor file per source under
`<config>/sources/` (`.yaml` or `.json`; `<config>` is the directory from the
table above, or whatever `--config` / `BEME_CONFIG_HOME` points at).

```yaml
# <config>/sources/my-notes.yaml
schema_version: "1"
source_id: my-notes                 # stable identifier; used in record IDs
type: directory                     # directory | git_repository
root: /absolute/path/to/knowledge   # absolute path to the source tree
purpose: [reusable_knowledge]
trust: canonical                    # canonical | trusted | untrusted
instruction_semantics: registered_files_only
authority_ceiling: default
sensitivity: personal_private       # personal_private | work_shareable | public_general
profiles_allowed: [personal]        # personal and/or work-safe
ingestion_mode: index_content       # index_content | reference_only
include: ["entries/**/*.md"]        # globs, relative to root
exclude: []                         # optional
```

The full contract is `schemas/source/source-descriptor.schema.json`; `make
validate` checks the fixtures against it.

**What becomes a record.** Ingestion reads the files matched by `include` and
normalizes Markdown entries with YAML frontmatter:

```markdown
---
id: PREF-001                 # required — a file without it is skipped
title: "Small reviewable changes"
type: preference             # preference | decision | constraint | precedent | knowledge
status: active               # only active entries are ingested
---

Keep changes small and reviewable.
```

The first meaningful body line becomes the record statement. Files without
frontmatter, without `id`, or whose `status` is not active are skipped
silently — `beme build` reports how many records each source contributed, so
compare that count with what you expect. Then:

```sh
beme build --profile personal     # ingest; re-run after editing sources
beme status                       # what is registered
```

## Lifecycle

```sh
beme doctor                       # health: sources, projections, findings
beme export --projection personal # visible records + provenance as JSON
beme explain --trace TRACE_ID     # why a pack selected what it did
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
- A purge that cannot inspect something it must erase — an unreadable or
  corrupt observation, an unreadable trace directory, a canonical path it
  cannot stat — stops with an error instead of reporting the step done. The
  failed command prints the steps that completed (`--json` includes the
  partial report) and says whether the purge is resumable.
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

`--work` (default: a temp dir removed on every exit) must be an absolute path
that is not a filesystem root, the home directory, the current directory, an
ancestor of either, or a repository root. Only `scale-N` directories carrying
the harness's marker file are ever deleted.

## Health states

`healthy` · `degraded` (missing/stale source) · `unavailable` (cannot
resolve) · `policy_blocked` (ambiguity, elevation attempt, corrupt policy) ·
`rebuild_required` (index unusable, sources intact). `beme doctor` reports
these with exact remediation: `policy_blocked` when the tombstone ledger or
purge key cannot be read, and `degraded` with "rebuild required" when a
projection holds records the ledger suppresses (for example after a backup
restore). Findings never name suppressed records or count them.
