# Changelog

All notable changes. Format: Keep a Changelog; versioning: semantic.

## [Unreleased]

### Added
- Verified enforcement state (ADR-030): `ledger/purge.key` and
  `ledger/tombstones.json` carry a key ID and a generation and prove each
  other, and the ledger carries an authentication tag over its entry set, so
  tombstones edited out in place are detected. A missing, mismatched,
  rolled-back, tampered, or malformed file fails every surface closed with an
  error naming both files; a pending purge journal that the ledger cannot
  explain fails closed too; an interrupted first write recovers; legacy
  bare-hex keys and schema-2 ledgers migrate in place.
- Observation anti-resurrection: a purge tombstones the observation IDs it
  erases, so copies restored from a data-dir backup stay hidden on list,
  inspect, review, family counts, feedback dedup, and the rejection
  tombstone index. `beme build` erases them; `beme doctor` reports how many
  without naming them.
- `internal/durable`: durability, erasure (zeroize → flush → unlink →
  directory flush), and an exclusive inter-process maintenance lock that
  serializes forget, purge, build, and learning writes.
- Positive controls for every threat case; supplementary case S4 (restored
  backup across every read surface, fail-closed without the key).
- `TestReadSurfacesUseLedgerFilter`, durable-write primitives,
  `Summary.ExitCode`, mutation-tested regression suites for every fix below,
  and docs checks D13 (MCP tool table), D14 (no unqualified crash-safety
  claims), D15 (single Unreleased section).
- Failure-injection tests for every purge stage; provenance tests with
  multiple and nonconventional IDs; ledger minimality and key tests.
- Shared threat-case registry `privacycorpus.NewSuite` (the empty
  `AllCases()` placeholder is gone), runner `cmd/beme-threat-corpus` (exit
  0/1/3) run in CI on Linux, macOS, and Windows, and supplementary cases
  S1–S3.
- Docs checks D10 (threat matrix matches the registry), D11 (runner claim
  backed by the shared registry), D12 (docs keep the ledger content-free).
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
- `scripts/test_docs_consistency.py` (D1–D6, CI-gated): phase
  contradictions, stale hard-coded counts, public/private validation
  semantics, bootstrap documentation presence, link integrity, and the
  canonical status anchor.
- ADR-024: binding record of the failed documentation gate, remediation,
  and reopen condition.
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

### Fixed
- Purge durability: traces, observations, and canonical files are erased
  through zeroize → flush → unlink → directory flush, and a retry completes
  flushes an interrupted attempt could not; projections are compacted with
  `synchronous=FULL` and flushed — all before the purge journal is removed.
  A canonical file with other hard links is reported as a residual instead
  of being zeroized.
- Concurrent `forget`, `purge`, and `build` no longer lose each other's
  ledger updates.
- `beme doctor` keeps the most severe health state instead of letting a
  later, milder finding downgrade `policy_blocked`.
- `beme explain` maps failures to their classes: unavailable trace exits 4,
  unusable ledger exits 3 (policy blocked), everything else exits 1.
- `beme candidate --config DIR` no longer ignores the override and fall back
  to the operator's real deployment.
- Purge planning ignored observation-store read failures and could finish
  while matching observations remained; unreadable trace directories,
  canonical paths, and projections were likewise reported done. Each now
  aborts the purge (resumable) and no step is reported done over data that was
  not inspected.
- `beme export`, `beme explain`, MCP status counts, and
  `beme.get_context_item` read projection stores without the durable ledger,
  so a restored pre-purge backup exposed purged and revoked records. Every
  read surface now applies the ledger and fails closed (exit 3 /
  `policy_blocked`) when the ledger or purge key cannot be read (ADR-027).
- `beme.get_context_item` returned any stored record — including revoked and
  purged ones — whenever an unrelated resolution succeeded. It now requires a
  `pack_id` + `record_id` from a pack issued by the same session (ADR-029).
- Evaluation runner: a zero-score (prohibited) generation was reported
  `passed`; generated text was missing from results and raw/blinded
  artifacts; no-scope and canonical-only ablations graded the full pack.
- Threat corpus: the secret fixture was never written (unchecked writes),
  case 12 could panic, case 16 used a global `/tmp` marker, and cases 2, 3, 15,
  17, 22, 23, 24, 26, and 29 could pass without exercising their threat.
- `**` patterns followed by a multi-segment tail (`a/**/b/*.md`) never
  matched, so such includes and excludes were silently ignored.
- Ledger, key, and journal writes did not flush the parent directory, and the
  docs overstated the crash durability of the ledger-first ordering.
- `benchmark.Run` with an empty or unsafe `WorkDir` deleted `scale-N`
  directories under the working directory; `beme-bench` leaked its temp dir
  on error exits and ignored report write failures.
- An existing `ledger/.gitignore` was left without the `purge.key` and
  `pending/` rules.
- `beme build` omitted purge-blocked counts in human output; a failed
  `beme purge` hid its partial report; explain accepted trace IDs containing
  path separators.
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
- Adapter install/remove byte-exactness (FR-043): install previously left
  separator newlines behind after removal; the managed-region design now
  makes removal the exact inverse of installation.
- Corrupt projection stores no longer brick `beme build`: the build path
  recreates them (derived data only; canonical sources untouched).
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

### Changed
- `beme.get_context_item` input now requires `pack_id` (ADR-029); work-safe
  expansions omit source identity.
- CHANGELOG consolidated into a single Unreleased section.
- Removed stale "Contracts-phase document" banners (ARCHITECTURE,
  THREAT_MODEL); evals/README, ACCEPTANCE, ROADMAP, PRIVACY, OPERATIONS, and
  INTEGRATIONS reconciled with the implemented state.

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
