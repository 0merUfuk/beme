# Changelog

All notable changes. Format: Keep a Changelog; versioning: semantic.

## [Unreleased] — purge reliability (owner review)

### Fixed
- Projection purge deleted provenance by a convention-derived ID; it now
  removes every ref in the record payload, the refs in the purge plan, and
  rows matching the record's source identity.
- A purge that failed part-way could not be re-run (the second run reported
  not-found and stranded traces, observations, and canonical files). Purge is
  now resumable through a content-free journal and idempotent (`already
  purged`); `beme doctor` reports interrupted purges.
- The purge ledger stored unkeyed digests of source content and statement
  text — a dictionary oracle for low-entropy private data. Entries are now
  keyed HMAC fingerprints of record identity only, with the key in
  `ledger/purge.key` (git-ignored); a missing key fails closed (ADR-027).

### Added
- Failure-injection tests for every purge stage; provenance tests with
  multiple and nonconventional IDs; ledger minimality and key tests.
- Shared threat-case registry `privacycorpus.NewSuite` (the empty
  `AllCases()` placeholder is gone), runner `cmd/beme-threat-corpus` (exit
  0/1/3) run in CI on Linux, macOS, and Windows, and supplementary cases
  S1–S3.
- Docs checks D10 (threat matrix matches the registry), D11 (runner claim
  backed by the shared registry), D12 (docs keep the ledger content-free).

## [Unreleased] — purge, benchmark, harness verification, Windows

### Added
- `beme purge` / `app.PhysicalPurge` (FR-055, ADR-027): RED physical purge
  with typed confirmation and dry run. Erases the record from both
  projection stores (files rewritten), persisted traces, and restating
  observations; optionally deletes the source file; reports Git history and
  external backups as residuals.
- Durable tombstone ledger under the canonical root (ADR-027): forget and
  purge survive rebuild, corrupt-store recovery, migration rollback, and
  backup restore. Purges store fingerprints only.
- Privacy corpus: threat cases 18 and 30 now execute; no case is `not_run`.
- `internal/benchmark`, `cmd/beme-bench`, `make bench` (NFR-008) and the
  measured `evals/benchmarks/seed-baseline.json`.
- Retrieval recall/precision in `internal/evalrunner` (ACCEPTANCE §5
  mechanism), with explicit `not_run` when refs are unavailable.
- Opt-in installed-harness integration tests for Claude Code and Codex with
  four separately reported verification levels (docs/INTEGRATIONS.md).
- CI `test-windows` job (build, vet, full test suite).
- Docs-consistency checks D7 (requirements counts derived from rows), D8
  (runner readiness), D9 (referenced Go tests exist); D1 now also catches
  "Contracts-phase".

### Fixed
- CLI and MCP output printed a literal `\n` instead of newlines.
- `beme forget` swallowed every tombstone error (the check compared an error
  with itself).
- Ingestion on Windows ingested nothing: relative paths used backslashes, so
  include globs never matched. Paths and locators are now slash-separated.
- Linux and Windows default directories used the macOS `~/Library` layout
  (ADR-028: XDG on Linux, `%AppData%`/`%LocalAppData%` on Windows).
- Requirements matrix: FR-055 carried a non-canonical status cell, which hid
  one of the four `partial` rows from counts; statuses are now canonical and
  the summary line is CI-checked.

### Changed
- Removed stale "Contracts-phase document" banners (ARCHITECTURE,
  THREAT_MODEL); evals/README, ACCEPTANCE, ROADMAP, PRIVACY, OPERATIONS, and
  INTEGRATIONS reconciled with the implemented state.

## [Unreleased] — end-to-end verification hardening

### Added
- `TestMCPClientEndToEnd`: a real MCP client (official Go SDK) drives the
  real stdio server — initialize, tools/list (exactly four tools),
  resolve_context under work-safe, status (private sources hidden),
  quarantined feedback persisted, and a `profile` elevation attempt
  rejected at the protocol layer (threat case 1, strongest position).
- `TestCorruptStoreRecovery` + rebuild-recovery in `BuildProfile`: an
  unusable (corrupt) projection store is deleted and rebuilt from
  registered sources; canonical sources verified byte-identical (FR-065).
- `TestRebuildDoesNotResurrectForgotten`: forget/rebuild semantics pinned.
- `TestAdapterInstallRemoveByteExact` + real-config verification: the
  adapter install→remove cycle is now byte-exact on real harness configs
  (found and fixed a separator-newline residue bug in the process).

### Fixed
- Adapter install/remove byte-exactness (FR-043): install previously left
  separator newlines behind after removal; the managed-region design now
  makes removal the exact inverse of installation.
- Corrupt projection stores no longer brick `beme build`: the build path
  recreates them (derived data only; canonical sources untouched).

## [Unreleased] — documentation reconciliation (ADR-024)

### Fixed
- Removed stale lifecycle claims that failed the first fresh-agent
  documentation test: "contracts phase / pre-implementation" (README),
  "production implementation gated behind WP2B" (PROJECT_CONTEXT),
  "WP2B blocked on user" and "WP4–WP11 not authorized" (ROADMAP), stale
  fixture/test counts, and DEVELOPMENT.md's pre-ADR-023 description of
  private-corpus coupling in the public validation path.
- `docs/HANDOFF.md` §1 is now the single canonical current-status section;
  every other lifecycle statement derives from it.
- README and DEVELOPMENT now document a reproducible clean-checkout
  validation bootstrap: Python 3.11+ requirement, isolated venv creation,
  jsonschema + pyyaml install, behavior when the system Python is older,
  and the exact commands that work from a clean checkout.
- Evidence claims use command-derived references instead of hard-coded
  counts (drift-prone prose).

### Added
- `scripts/test_docs_consistency.py` (D1–D6, CI-gated): phase
  contradictions, stale hard-coded counts, public/private validation
  semantics, bootstrap documentation presence, link integrity, and the
  canonical status anchor.
- ADR-024: binding record of the failed documentation gate, remediation,
  and reopen condition.

## [Unreleased] — learning and governance (WP9)

### Added
- `internal/learning`: the observation → candidate → batch-review pipeline
  (blueprint §14). Observations carry hypothesis, evidence family, family
  count, inherited sensitivity, and candidate scopes from creation.
- Evidence-family dedup: repeated feedback from one model/session/workflow
  increments a family count instead of creating independent observations
  (§14.4 anti-self-training; FR-052).
- Rejected-proposal tombstones with normalized fingerprints: equivalent
  re-proposals (case/whitespace-insensitive) are refused, not re-filed
  (§14.5; FR-053, threat case 13).
- Batch-review CLI `beme candidate list|inspect|review` with all §14.5
  actions: approve, edit, merge, reject, defer, situational, scope_limit,
  counterexample. Approve records the decision; the canonical knowledge write
  remains the owning repository's proposal path (Be Me has no
  canonical-write API by design, FR-051).
- `beme.report_feedback` (MCP) now writes through the learning store:
  durable data dir, family dedup, tombstone enforcement, capability-derived
  inherited sensitivity (FR-054).

## [0.1.0-alpha.1] — 2026-09-14

Verification-recovery release for v0.1.0-alpha. No engine behavior changes.

### Fixed
- Public contract validation is repo-local: it no longer reads the
  operator's home directory or any private evaluation-corpus path. On a
  clean checkout or CI runner it previously failed; it now passes with an
  empty HOME (ADR-023).
- Private evaluation is a separate owner-gated gate (`make
  validate-private`, corpus via `BEME_PRIVATE_EVAL_DIR` only) that reports
  an explicit `not_run` (exit 3) when the corpus is unavailable — never a
  silent pass, never a public-CI dependency.
- CI installs Python validation deps in a venv (PEP 668-safe on GitHub's
  Homebrew-managed macOS runners) and pins current action majors (v7),
  removing Node-20 deprecation warnings.
- CI runs a 7-check regression suite reproducing the exact v0.1.0-alpha
  failure and the public/private separation invariants.
- Distribution: release binaries ship as tarballs so the executable bit
  survives download (bare binaries downloaded from the v0.1.0-alpha page
  arrive non-executable; `chmod +x` works around it there).

### Documentation
- Release and handoff claims corrected to match observable evidence:
  19 public fixture checks (private 34 reported separately, owner-run);
  the eval runner is documented as not-yet-implemented; v0.1.0-alpha
  release notes carry a post-publish correction.

## [0.1.0-alpha] — 2026-09-14


First alpha: contracts + engine + narrow MCP surface. Pre-release; no
compatibility commitments.

### Added
- Versioned JSON contracts: source descriptor, workspace identity, normalized
  record, provenance, resolution request, context pack, golden case, run
  manifest (fixture-validated; elevation structurally unrepresentable).
- Two-stage resolver: Stage-A policy (sensitivity, profile, validity, scope,
  trust, revocation) before Stage-B retrieval/ranking/budgeting.
- SQLite/FTS5 projection stores (CGo-free), separate personal/work-safe
  files; work-safe builder reads only the approved safe manifest.
- Safe ingestion: registered roots only, no code execution, hard excludes,
  secret scan, symlink containment, size/depth/time bounds, `**` globs.
- Workspace identity registry: real-path matching, ambiguity fails closed,
  clone-never-inherits-trust.
- ContextPack emission with deterministic ordering, conflict preservation,
  mandatory-content budget protection, unknowns (never invented).
- Quarantined feedback path (observations; promotion is user-owned).
- Admin CLI: status, doctor, build, preview/resolve, forget, adapter
  install/remove/verify, serve (stdio MCP; 4 tools).
- Harness adapters: codex + claude-code (Tier 1), hermes + cursor (Tier 2
  documented); marker-delimited idempotent managed blocks.
- Evaluation contract: 0–4 blind paired rubric, P1–P18 privacy invariants,
  reproducibility manifest; 34-case private golden corpus (ADR-022
  delegation recorded), frozen splits.
- Documentation set: architecture, threat model, acceptance gates, data
  model, operations, integrations, security, privacy.

### Known limitations (honest)
- Advisory-only adapter assurance; no `assured` surface in v1 (no verified
  prompt-bound hook measured at 100%).
- No live-model behavioral evaluation yet (requires paid API runs —
  owner-gated; scoring contract and frozen splits ready; runner not yet
  implemented).
- Learning loop ships observation intake + tombstones; batch review CLI
  (candidate approve/edit/merge/reject) is pending.
- macOS verified; Linux/Windows ported-unverified (NFR-007).
