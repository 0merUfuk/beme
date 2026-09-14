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

## Status snapshot (2026-09-14)

| WP | Scope | Status |
|---|---|---|
| WP0 | Live evidence and drift validation | **Complete** (private evidence report; snapshot re-verified; 2 read-only verification clones; 18 gold candidates identified) |
| WP1 | Constitution, decision ledger, traceability | **Complete** (`docs/PROJECT_CONTEXT.md`, `docs/DECISIONS.md` ADR-001…021, `docs/REQUIREMENTS.md` per-ID matrix) |
| WP2A | Evaluation contract + candidate corpus | **Complete** (`evals/EVALUATION_CONTRACT.md`; 18 private candidate cases, all `status: candidate`) |
| WP3 | Contracts and schemas | **Complete** (8 versioned schemas; fixtures 35/35; elevation structurally unrepresentable) |
| WP2B | Frozen fixtures + thresholds | **Blocked on user (RED):** candidate gold approval; then splits/threshold freeze |
| WP4–WP11 | Production code → release | Not authorized until WP2B exit gate passes |

## What each remaining WP owns

- **WP2B:** user-approved locked seed corpus, frozen splits, exact scoring
  contract, calibrated thresholds, sealed holdout. *Entry needs nothing more
  from the agent; approval is the user's.*
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