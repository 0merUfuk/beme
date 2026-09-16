# Be Me — Release verification procedure and draft notes

Nothing here is published. This is the procedure a release runs through and
the draft notes for the next tag; both are owner-gated (ADR-021, ADR-022).
Publishing a release, tagging, or pushing a package is an owner action.

## 1. Preconditions

| Precondition | How it is checked |
|---|---|
| Branch merged to `main` by the owner | GitHub PR state |
| Three-platform CI green on the exact release commit | CI run linked in the PR |
| Installed-binary smoke green on macOS, Linux, Windows | `installed-binary-smoke` CI job on that commit |
| Fresh-checkout documentation test passed on that commit | recorded run (docs only, isolated HOME) |
| Private-data scan clean | `make scan` (CI job `private-data-scan`) |
| Threat corpus 100% with no `not_run` | `go run ./cmd/beme-threat-corpus --repo .` |
| Docs consistency and CI regression suites green | `scripts/test_docs_consistency.py`, `scripts/test_ci_regression.py` |
| CHANGELOG `Unreleased` section describes every user-visible change | review |
| Owner approval for a non-alpha version | explicit decision |

## 2. Verification run (on the release commit, clean checkout)

```sh
git clone https://github.com/0merUfuk/beme && cd beme && git checkout <sha>
python3 -m venv .venv && .venv/bin/pip install jsonschema pyyaml   # needs network
PATH="$PWD/.venv/bin:$PATH" make check          # validate + build + vet + test + scan
python3 scripts/test_docs_consistency.py
python3 scripts/test_ci_regression.py
go run ./cmd/beme-threat-corpus --repo .
go test -race ./internal/app ./internal/durable ./internal/learning ./internal/storage ./internal/evalrunner
make bench                                       # records evals/benchmarks/seed-baseline.json
GOOS=windows go vet ./... && GOOS=linux go vet ./...
```

Then the installed-binary smoke in an isolated deployment (the
`installed-binary-smoke` CI job performs exactly these steps per OS):

```sh
export BEME_HOME="$(mktemp -d)"
export BEME_CONFIG_HOME="$BEME_HOME/cfg" BEME_DATA_HOME="$BEME_HOME/data" BEME_CACHE_HOME="$BEME_HOME/cache"
go install ./cmd/beme ./cmd/beme-bench ./cmd/beme-threat-corpus ./cmd/beme-eval
export PATH="$(go env GOPATH)/bin:$PATH"
cd "$BEME_HOME"   # run from outside the repository
beme doctor --json --config "$BEME_CONFIG_HOME"
# register a synthetic source (docs/OPERATIONS.md "Registering a source"),
# then: build → status → preview → candidate list → forget → purge --dry-run
```

## 3. Rollback

- **Before a tag exists:** nothing to roll back; revert the merge commit on
  `main` (`git revert -m 1 <merge sha>`) and re-open the PR.
- **After a tag:** delete the pre-release tag and its GitHub release; a
  published tag is never rewritten. Users pin the previous version
  (`go install github.com/0merUfuk/beme/cmd/beme@<previous tag>`).
- **Data:** no migration runs on upgrade. Downgrading past the ledger
  schema-3 change (ADR-030) makes a schema-3 `ledger/tombstones.json`
  unreadable to the older binary, which fails closed rather than ignoring
  tombstones — restore the canonical root from backup (ledger and key
  together) if you must run an older binary.
- **Deployment:** `beme adapter remove <harness>` restores harness
  instruction files; projections are derived and can be deleted and rebuilt.

## 4. Draft release notes (next tag — NOT published)

> ### Be Me v0.1.0-alpha.2 (draft)
>
> Alpha. Local-only, offline, no telemetry. Not a stable contract.
>
> **Privacy and data integrity**
> - Enforcement state is now integrity-verified: `ledger/tombstones.json` and
>   `ledger/purge.key` carry a key ID and a generation and prove each other.
>   A missing, mismatched, rolled-back or malformed file fails every surface
>   closed with an error naming both files. Back them up together.
> - Purged observations can no longer be resurrected by restoring a data-dir
>   backup: they stay hidden on every learning surface, `beme build` erases
>   them, and `beme doctor` reports how many without naming them.
> - Every deletion a purge performs is zeroized, flushed and directory-flushed
>   before the purge journal is removed; a retried purge completes flushes an
>   interrupted attempt could not.
> - Forget, purge, build and learning writes are serialized by a maintenance
>   lock, so concurrent runs cannot lose each other's ledger updates.
>
> **Evaluation**
> - Evaluation arms B0–B4 are built from real primitives (real bootstrap text,
>   all eligible records for B2, pre-precedence retrieval without provenance
>   for B3); the no-scope and canonical-only ablations are implemented, with
>   no-scope refusing to run outside a synthetic deployment; manifests record
>   observed model settings, prompt and corpus hashes. New `beme-eval` CLI.
>
> **Benchmark and CLI**
> - `beme-bench` requires `--seed`, validates seed identifiers and metadata
>   before writing anything, and reports cleanup and output-write failures in
>   its exit code.
> - `beme doctor` keeps the most severe health state; `beme explain` maps
>   failures to distinct exit codes (4 unavailable trace, 3 policy blocked,
>   1 other); `beme candidate --config DIR` honors the override.
>
> **Upgrade notes**
> - Legacy `purge.key`/schema-2 ledgers keep working and migrate in place on
>   the next ledger write.
> - `beme build` and learning writes now need a writable canonical root (the
>   ledger directory and its lock live there).
> - MCP `beme.get_context_item` requires `pack_id` (since alpha.1).
