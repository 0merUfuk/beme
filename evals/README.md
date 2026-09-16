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

- `internal/evalrunner` — the EVALUATION_CONTRACT.md §4 runner: every
  baseline (B0–B4) and every ablation (no-scope, no-provenance, no-unknowns,
  canonical-only, learned-only), each built from one serving session's
  inputs so all share the same capability boundary; deterministic rendered
  prompts; repeat runs; per-generation manifests with observed values plus
  owner-side evidence; blinded packaging with the generated text; retrieval
  recall/precision; explicit `passed`/`failed`/`not_run` — a zero-score
  blocker fails its unit and `Summary.ExitCode` returns 0/1/3. no-scope is
  refused in code outside a wholly synthetic deployment. Proven end to end
  with deterministic mock providers on public synthetic fixtures
  (`TestRunnerFullPipelineB0ThroughB4`,
  `TestRunnerBlockerFailsUnitAndPreservesText`, `TestRunnerRetrievalMetrics`,
  `TestArmsReceiveExactlyTheirConstruction`,
  `TestSyntheticArmFixturePositiveControls`,
  `TestArmsCannotMutateEachOthersInput`,
  `TestNoScopeRefusedOutsideSyntheticDeployments`,
  `TestManifestsRecordObservedValues`).
- `cmd/beme-eval` — the owner-run procedure (never CI against private data):
  `beme-eval retrieval --corpus DIR --config DIR` measures retrieval
  recall/precision locally; `beme-eval behavioral --corpus DIR --config DIR
  --provider command --command 'CMD ...'` pipes each rendered prompt to an
  external command's stdin and reads its stdout, with `--dry-run` reporting
  how many generations would run without executing anything. The corpus
  comes from `--corpus` or `BEME_PRIVATE_EVAL_DIR`; with neither it reports
  `not_run` and exits 3. Artifacts go to `--out` or a new OS temp directory,
  never inside this repository. Exit codes follow `Summary.ExitCode`
  (0/1/3; 2 usage).
- `internal/bootstrap` — the canonical managed bootstrap text every
  bootstrap-bearing arm and the adapter installer use, byte-identical to
  `adapters/common/skill/BOOTSTRAP.md`
  (`TestBootstrapMatchesCanonicalAdapterText`).
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