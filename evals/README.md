# Be Me — Evals

Public evaluation assets for the engine's claims.

- `EVALUATION_CONTRACT.md` — how every claim is measured: ground-truth rules,
  baselines/ablations, scoring, evaluator governance, thresholds, privacy
  invariants (P1–P18), reproducibility manifest, approval gates.
- `schemas/` — shared evaluation schemas (golden case, run manifest) also
  published under `schemas/evaluation/` at the repo root (canonical location).
- `public/` — synthetic, anonymized public cases and privacy/policy
  regression fixtures. **No real user data ever lives here.**
- `benchmarks/` — NFR-008 performance evidence on the public seed corpus
  (`make bench` regenerates `seed-baseline.json`).

## Executable runners (implemented; live execution owner-gated)

The runners live in Go packages so they share the production runtime path:

- `internal/evalrunner` — the EVALUATION_CONTRACT.md runner: B0–B4 arms,
  implemented ablations (no-scope and canonical-only report `not_run`),
  repeat runs, immutable manifests, blinded packaging with the generated
  text, retrieval recall/precision, explicit `passed`/`failed`/`not_run`; a
  zero-score blocker fails its unit and `Summary.ExitCode` returns 0/1/3.
  Proven end to end with deterministic mock providers on public synthetic
  fixtures (`TestRunnerFullPipelineB0ThroughB4`,
  `TestRunnerBlockerFailsUnitAndPreservesText`, `TestRunnerRetrievalMetrics`).
- `internal/privacycorpus` + `cmd/beme-threat-corpus` — the shared threat-case
  registry (`NewSuite`): every §19 case plus supplementary cases S1–S4, each
  behind a positive control, executed by `TestPrivacyCorpusDeterministic` and by the runner
  (`go run ./cmd/beme-threat-corpus --repo .`; exit 0 all passed, 1 failed,
  3 not_run).
- `internal/benchmark` + `cmd/beme-bench` — the reproducible performance
  harness.

Running the behavioral evaluation against live models (paid API use) and
against the private corpus is owner-gated: wire a real provider into
`evalrunner.Provider` and run it outside public CI (ADR-023).

## Hard rules

1. Real golden cases and the private regression pack live **outside this
   repository** (deployment data). They are never committed, synced to CI, or
   referenced by path from public files.
2. Gold ground truth may come only from: explicit user decision + rationale;
   repository ADR with user confirmation; repeated cross-context pattern
   later approved; a user-approved decision set. Agent inference is never
   gold.
3. Privacy, capability-elevation, fabricated-authority, and hard-policy
   violations are binary release blockers — never averaged into scores.
4. Locked runs use no live network unless the case is explicitly an
   integration test. The no-scope ablation runs only on synthetic data in an
   isolated, no-network environment.