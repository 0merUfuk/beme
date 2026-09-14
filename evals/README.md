# Be Me — Evals

Public evaluation assets for the engine's claims.

- `EVALUATION_CONTRACT.md` — how every claim is measured: ground-truth rules,
  baselines/ablations, scoring, evaluator governance, thresholds, privacy
  invariants (P1–P18), reproducibility manifest, approval gates.
- `schemas/` — shared evaluation schemas (golden case, run manifest) also
  published under `schemas/evaluation/` at the repo root (canonical location).
- `public/` — synthetic, anonymized public cases and privacy/policy
  regression fixtures. **No real user data ever lives here.**
- `runners/` — evaluation runners. **Not yet implemented.** The behavioral
  evaluation protocol (baselines, rubric, manifest) is defined in
  `EVALUATION_CONTRACT.md`; the executable runner arrives with the first
  owner-gated live-model evaluation session.

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