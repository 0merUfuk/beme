# Be Me — Roadmap

> Work packages in dependency order (blueprint §20). Current position is
> tracked in `docs/HANDOFF.md`.

```text
WP0 Live evidence/drift ─▶ WP1 Constitution/ADRs ─▶ WP2A Eval contract + candidates
        └────────────────────────────────────────────────┐
WP3 Contracts/schemas ─▶ WP2B Frozen fixtures + thresholds ─▶ [PRODUCTION CODE GATE]
                                                              │
                     WP4 Sources/projections ─▶ WP5 Resolver ─▶ WP6 Admin CLI
                                                    │        └▶ WP7 MCP boundary
                                                    └▶ WP8 Harness adapters
                     WP9 Learning/review ─▶ WP10 Hardening/dogfood ─▶ WP11 Public release
```

## Status snapshot

> The single canonical current-status section is
> [`docs/HANDOFF.md`](HANDOFF.md) §1. This table records per-work-package
> scope and completion state; refresh it from HANDOFF when a package lands.
> Counts below are evidence-linked, not prose promises: contract checks are
> re-derived by `make validate`, test counts by `go test -list '.*' ./...`.

| WP | Scope | Status |
|---|---|---|
| WP0 | Live evidence and drift validation | **Complete** (2026-09-14; private evidence report; 2 read-only verification clones; gold candidates identified) |
| WP1 | Constitution, decision ledger, traceability | **Complete** (ADR ledger in DECISIONS.md; per-ID requirements matrix with CI-checked status counts) |
| WP2A | Evaluation contract + candidate corpus | **Complete** (evaluation contract; 34-case private corpus approved under ADR-022 delegation) |
| WP3 | Contracts and schemas | **Complete** (8 versioned schemas; public fixture checks via `make validate`; elevation structurally unrepresentable) |
| WP2B | Frozen fixtures + thresholds | **Complete** (2026-09-14, ADR-022 delegation; splits 13/13/8; thresholds at design targets; private freeze manifest) |
| WP4 | Source and projection architecture | **Complete** (registry, safe ingestion, adapters, separate stores, revocation; work-safe boundary tested at construction) |
| WP5 | Resolver vertical slice | **Complete** (two-stage policy→resolution; determinism, budget, unknowns tested) |
| WP6 | Administrative CLI | **Complete** (status/doctor/build/preview/forget/adapter/candidate) |
| WP7 | MCP boundary | **Complete** (4-tool stdio server; surface contract + elevation + transport tests) |
| WP8 | Harness adapters | **Complete** (codex + claude-code install targets; hermes + cursor documented contracts; all `advisory` — see ADR-013) |
| WP9 | Learning and review | **Complete** (observation→candidate→batch-review pipeline; §14.5 actions; tombstones; FR-050..055 tested; physical-purge workflow proven on synthetic data, ADR-027) |
| WP10 | Hardening | **Complete for this alpha** (CI on macOS/Ubuntu/Windows, migrations FR-064, private-data scan, regression suite, all §19 threat cases executed, NFR-008 benchmark; behavioral beta gates pending live-model runs) |
| WP11 | Public release | **v0.1.0-alpha.1 published** (verification defects of alpha corrected post-publish; clean history, tarball artifacts, honest platform labels) |

**V2 integration:** WP12 adopts the existing brain source onto this newer
runtime (ADR-033–035), with FR-070..089 tracked separately. The integration is
committed in PR #3; synthetic gates are not live adoption evidence. Brain notes
inside V1 packs, embeddings, new MCP tools and provider runs are out of scope.
See HANDOFF §1 for the exact verification state.

**Current engineering state:** the 2026-10-09 tracked audit defects have source
fixes and synthetic regressions in the combined candidate (ADR-036–040).
Independent review and parent source-bound rechecks are complete, including the
overlapping-capture correction. A separately authorized local rollout verified
installed CLI and fresh MCP connections. Publication preparation follows the
standing commit/push/verified-release policy in ADR-041; exact-head native CI,
any explicit merge hold and artifact verification remain separate gates.
Previous WP completion records describe narrower historical gates, not measured
personal effectiveness.

**Separately owner-gated, after remediation:** live-model B4-vs-B0 behavioral evaluation (paid
API runs); private-corpus retrieval measurement (owner-run, ADR-023);
`assured` adapter surfaces (requires measured 100% pre-decision use in live
harness sessions); non-alpha release approval. See ACCEPTANCE §7a.

## What each WP owns (scope record)

All work packages through WP11 have shipped scope as recorded in the
status table above; this section is the scope definition each was held to.

- **WP2B:** user-approved locked seed corpus, frozen splits, exact scoring
  contract, calibrated thresholds, sealed holdout. *Executed 2026-09-14
  under ADR-022 delegation.*
- **WP4:** source registry, safe ingestion, adapters (canonical-knowledge style, project
  policy, optional Rifja contract), projection builders, separate
  personal/work-safe stores, revocation/deletion semantics.
- **WP5:** capability binding, workspace identity, facets, Stage-A policy
  filter, retrieval, precedence, conflicts, budgeting, unknowns, pack, trace.
  Exit: core-alpha gates without embeddings.
- **WP6:** admin CLI — status, doctor, source, profile, preview, explain,
  inspect, rebuild, export, forget; human + JSON output; stable exit codes.
- **WP7:** narrow MCP tools over stdio; capability binding; timeout and
  degradation contract; protocol compatibility tests.
- **WP8:** harness adapters — Codex and Claude Code first (Tier 1), Hermes
  and Cursor next; bootstrap blocks, skills, config integration,
  hooks/wrappers; install/uninstall idempotency; assurance labeling
  (`assured` requires measured 100% pre-decision use).
- **WP9:** observations, evidence-family dedup, candidate generation, batch
  review, approval events, rejection tombstones, undo/audit lifecycle.
- **WP10:** concurrency, corruption, migration, backup/restore, deletion,
  performance, security, privacy, real-task correction-rate, cross-harness
  evidence. Exit: trusted-local-beta gates over multiple runs.
- **WP11:** clean public history, synthetic examples, public evals, full
  documentation set, packages, compatibility evidence, user release approval.

## Parallelization rules (unchanged)

Documentation/synthetic fixtures alongside schema work; security regression
authoring throughout; Codex+Claude adapters in parallel after MCP/pack
contracts stabilize. Never: adapters before stable contracts; implicit
learning before governance contracts; retrieval tuning before the locked
golden set; publishing before private-data and clean-history audits.