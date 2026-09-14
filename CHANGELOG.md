# Changelog

All notable changes. Format: Keep a Changelog; versioning: semantic.

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
