# Be Me — Verified release delivery

This document defines the release contract, not the live publication state.
Use the exact GitHub tag, release, asset checksums and CI run to establish what
has shipped. The current source targets `v0.1.0-alpha.2`; earlier published
tags remain immutable. ADR-041 records standing approval for verified alpha
delivery, without overriding any separate merge hold or non-alpha gate.

## 0. Completion standard

Completed product changes must be committed and pushed. Every verified delivery
package must be versioned and released; a local installation or an open PR alone
does not complete delivery. Stage only reviewed public source, never private
planning, deployment data or operation receipts. Preserve existing PR ancestry
and unrelated work. Authenticate publication as `0merUfuk`, not another account.

For each package, record the exact source commit, PR, required CI results, tag,
release and assets. A tag/dispatch workflow may automate packaging and publishing
only after its validated tag target is on `main` and that same immutable SHA
passes the required CI and artifact checks. A dispatch must not validate one
tag while testing the moving default branch. The pipeline must not auto-merge
or overwrite a published release. The owner must pre-create a reviewed draft
before the publisher job runs; the pipeline retains that draft and its authorship
while verifying and uploading assets. It never creates a replacement release.

If access, an explicit merge hold or a required check blocks publication, record
that state and complete independent safe prerequisites. Do not treat an empty
check list, an earlier green run or an unanswered approval question as consent
or success. Real model benefit is a separate evidence level and remains unmeasured.

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
| CLI version, tag and selected CHANGELOG version agree; all user-visible changes included | release preflight and review |
| All five distribution archives execute on their native runners before upload | release artifact matrix, not cross-compilation alone |
| Owner-authored draft and exact source-bound assets verified before publication | publisher readback; no overwrite of existing mismatched or published assets |
| Owner approval for a non-alpha version | explicit decision |

## 2. Verification run (on the release commit, clean checkout)

Set `TMPDIR` to an existing writable scratch directory if outputs must stay
under a specific root. The recipes pass full path templates to `mktemp` so a
platform's default temporary-directory selection cannot override that root.

```sh
git clone https://github.com/0merUfuk/beme && cd beme && git checkout <sha>
python3 -m venv .venv && .venv/bin/pip install jsonschema pyyaml   # needs network
export PATH="$PWD/.venv/bin:$PATH"             # keep the same interpreter for every Python gate
make check                                    # public Go, contract, scan and release-helper gates
python3 scripts/test_docs_consistency.py
python3 scripts/test_ci_regression.py
go run ./cmd/beme-threat-corpus --repo .
go test -race ./internal/brain ./internal/app ./internal/durable ./internal/learning ./internal/storage ./internal/evalrunner
bench_out="$(mktemp -d "${TMPDIR:-/tmp}/beme-bench.XXXXXX")"
go run ./cmd/beme-bench --seed testdata/synthetic/records.json --out "$bench_out/benchmark.json"
GOOS=windows go vet ./... && GOOS=linux go vet ./...
```

Then execute the same source-install smoke used by the
`installed-binary-smoke` CI job, followed by a real native archive:

```sh
export GOBIN="$(mktemp -d "${TMPDIR:-/tmp}/beme-bin.XXXXXX")"
go install ./cmd/beme ./cmd/beme-bench ./cmd/beme-threat-corpus ./cmd/beme-eval
python3 -B scripts/smoke.py --bin-dir "$GOBIN"
artifact_out="$(mktemp -d "${TMPDIR:-/tmp}/beme-artifacts.XXXXXX")"
python3 -B scripts/release.py package --out "$artifact_out" --smoke
```

These helpers use isolated synthetic homes, check every native exit, run the
context lifecycle and brain capture/Doctor/recall, and invoke the threat corpus,
benchmark and evaluator help. They do not run a model. `release-check` alone may
skip the explicitly opt-in native build/source-install controls and PowerShell
when unavailable; CI supplies the installed binaries and executes archive smoke
separately, and Windows must execute the real PowerShell fail-then-success control.
Set `BEME_RELEASE_NATIVE_TEST=1` for the unittest's additional real packaging
control, or `BEME_SMOKE_BIN_DIR` for its source-install control.

### Automated alpha publication

`.github/workflows/release.yml` runs on a new numbered-alpha tag or an owner
dispatch from `main`. Its preflight verifies the actual remote tag, main ancestry,
CLI version and CHANGELOG, then forwards the peeled immutable SHA to reusable
CI and every artifact/publisher checkout. All four executables, LICENSE and a
source-bound `MANIFEST.json` are packaged for macOS amd64/arm64, Linux
amd64/arm64 and Windows amd64. Each target is built and exercised natively;
only then are the five archives combined into an exact `SHA256SUMS` inventory.

Only the publisher **job** receives write permission; that permission also
applies to its checkout/download actions, not just the final shell step.
Explicit final-step `GH_TOKEN` injection is not step-level token isolation.
The publisher reads back the owner draft,
uploads missing matching assets, verifies their digests, and finally publishes
the prerelease. Published releases and mismatched existing assets refuse writes.
Local dirty snapshots can be packaged for inspection but cannot be finalized
as a release. Asset names are isolated to the current workflow run and attempt.

The authorized order is: clear the merge hold → land verified source on main →
verify exact-head CI → push a new immutable tag → create the owner-authored draft
with reviewed notes before the publisher → complete native artifact gates and
publication readback. A failed draft can be retried using the **whole workflow**:

```sh
gh workflow run release.yml --ref main -f tag=v0.1.0-alpha.2
```

Do not rerun only the publisher: a new attempt must not reuse earlier-attempt
artifacts. Missing drafts, failed gates or mismatched assets are blockers, not
permission to recreate a release or move a tag. The release page, not this
procedure, establishes that a particular execution actually succeeded.
Changes to toolchains, workflows, permissions or draft assets can legitimately
make a later retry refuse; inspect the actual error rather than expanding
credentials, overwriting assets or weakening verification.

## 3. Rollback

- **Before a tag exists:** nothing to roll back; revert the merge commit on
  `main` (`git revert -m 1 <merge sha>`) and re-open the PR.
- **After publication:** preserve the tag and existing assets. Issue a new
  version or correction notice rather than deleting, retagging or silently
  replacing the published package. An operator may select a previously verified
  binary only if its data-format compatibility permits it; the ledger caveat
  below is mandatory.
- **Data:** no migration runs on upgrade, and **downgrading to a binary older
  than the schema-3 ledger (ADR-030) is unsupported.** The older binary
  cannot read `ledger/tombstones.json` and fails closed — which is the
  correct behavior, because there is no downgrade path that preserves the
  purge and revocation state recorded since the upgrade. Do not "fix" it by
  restoring an older ledger: the newest `ledger/tombstones.json` and
  `ledger/purge.key` are the enforcement state and must be kept together and
  kept newest. Pairing a newer key with an older ledger fails closed by
  design, and the older ledger is missing tombstones the deployment has
  enforced. If you must run an older binary, point it at a separate
  deployment (its own `--config`, data and cache), never at a canonical root
  whose ledger has been upgraded.
- **Deployment:** `beme adapter remove <harness>` restores harness
  instruction files; projections are derived and can be deleted and rebuilt.

## 4. Release-note template

Review this template against the selected CHANGELOG section before creating the
draft. This template is not a claim that the release has been published.

> ### Be Me v0.1.0-alpha.2
>
> Alpha. Local-only, offline, no telemetry. Not a stable contract.
>
> **Integrated work memory and effectiveness corrections**
> - The same executable now serves the context runtime and non-canonical
>   Markdown work memory: gated capture, Doctor, bounded briefing, snapshots,
>   restore/freeze and optional harness hooks. Brain notes do not become
>   canonical sources or silently enter ContextPacks.
> - Corrected chronology/recapture, scope disclosure, unsafe schema write
>   gates, bounded metadata and overlapping-capture accounting. Accepted notes
>   survive degraded bookkeeping; ordinary reads remain write-free.
> - Fixed Markdown evidence selection, provenance ordering, complete-pack
>   budgets, partial-source health and persisted trace/explain consistency.
> - Generation is `grading_pending`, not behavioral success. Frozen evidence
>   is reconciled before independent grading; reviewer packets omit revealing
>   owner-directory metadata. Synthetic success does not establish real benefit.
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
>   provider-declared model settings and actual prompt/corpus hashes. The current
>   `beme-eval` requires frozen splits and independent offline grades; generation
>   alone stays grading_pending (ADR-038).
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
>   the next ledger write. Downgrading afterwards to a pre-schema-3 binary is
>   unsupported: it cannot read the upgraded ledger and fails closed, and no
>   downgrade path preserves the purge and revocation state recorded since.
>   Keep `ledger/tombstones.json` and `ledger/purge.key` together and newest.
> - `beme build` and learning writes now need a writable canonical root (the
>   ledger directory and its lock live there).
> - MCP `beme.get_context_item` requires `pack_id` (since alpha.1).
