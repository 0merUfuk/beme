# Changelog

All notable changes. Format: Keep a Changelog; versioning: semantic.

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
  owner-gated; runner and scoring contract ready).
- Learning loop ships observation intake + tombstones; batch review CLI
  (candidate approve/edit/merge/reject) is pending.
- macOS verified; Linux/Windows ported-unverified (NFR-007).
