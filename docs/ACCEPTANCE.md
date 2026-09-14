# Be Me — Acceptance

> Owns mandatory/prohibited conclusions, release gates, and what counts as
> evidence. Nothing is "done" because a command exited zero; every gate below
> requires recorded evidence in `docs/HANDOFF.md` or an evidence bundle.

## 1. Universal acceptance rules

- **Mandatory:** every completion claim includes exact scope, actual command
  output, and verification method. Partial completion is reported as partial.
- **Mandatory:** privacy/policy invariants have 100% acceptance — one
  failure blocks release regardless of behavioral scores.
- **Prohibited:** claiming completion from successful commands without
  verified outcomes; waiving blockers; lowering thresholds; "production-ready"
  language before gates pass; averaging privacy failures into scores.
- **Prohibited:** treating agent inference as gold truth; agent-marked PASS
  on blockers; public-release approval by anyone other than the user.

## 2. Gate: blueprint readiness (complete)

- [x] Product definition and non-goals explicit.
- [x] Existing source ownership explicit.
- [x] User-specific principles and evidence gaps separated.
- [x] Scope, trust, and capability boundaries specified.
- [x] Context and learning contracts specified.
- [x] Threat model and release-blocking cases defined.
- [x] Evaluation strategy and work-package order defined.
- [x] Core repository-derived claims pinned to evidence snapshot revisions.
- [x] No implementation performed during planning.
- [x] Live local state validated by the implementation agent (WP0 complete,
      2026-09-14; report in the private evidence directory).
- [ ] Seed golden decisions approved (RED — user).
- [x] Exact dependencies verified (ADR-004: go-sdk v1.7.0, modernc sqlite
      FTS5, go 1.25.6 — compiled verification).

## 3. Gate: contracts (WP3) — complete

- [x] Versioned schemas: source descriptor, workspace identity, normalized
      record, provenance, resolution request, context pack, golden case, run
      manifest.
- [x] All seed fixtures validate (`make validate` → 35/35 pass).
- [x] Elevation-negative fixtures are structurally rejected by the request
      schema — the contract cannot represent model-selected authority
      elevation (WP3 exit gate).
- [x] Evaluation contract drafted (claims, ground-truth rules, baselines,
      scoring, evaluator governance, thresholds, privacy invariants,
      reproducibility manifest).

## 4. Gate: WP2B freeze — PASSED (2026-09-14, under ADR-022 delegation)

- [x] Candidate gold corpus: 34 real decisions approved under delegated
      authority (ADR-022); provenance recorded as delegated, never silent.
- [x] Frozen splits: calibration 13 / development 13 / locked-holdout 8;
      decision families (supersession chains) never cross splits.
- [x] Thresholds locked at blueprint design targets.
- [x] Gold lifecycle recorded; holdout sealed (FREEZE-WP2B.yaml, private).
- [x] Private regression pack never referenced by public CI.
- **Exit:** production code authorized. WP4–WP7 begin.

## 5. Gate: core alpha (WP5)

- [ ] Schema and migration fixtures: 100% pass.
- [ ] Policy/precedence tests: 100% pass.
- [ ] Denied-scope leakage: 0.
- [ ] Unsupported authoritative personal claims: 0.
- [ ] Required-record retrieval recall: ≥90%.
- [ ] Context precision: ≥80%.
- [ ] Deterministic pack snapshots stable.
- [ ] Every selected item has valid provenance and a selection reason.

## 6. Gate: trusted local beta (WP10)

- [ ] Blind paired B4-vs-B0: B4 wins ≥60%, loses ≤20%, bootstrap 95% CI
      excludes a negative effect (decision and reasoning reported separately).
- [ ] General-task non-regression: within 0.25 points; no high-risk case
      pass→fail.
- [ ] Invented-preference rate on negative controls: 0.
- [ ] Trusted project override accuracy: 100%.
- [ ] Required escalation recall: ≥95%.
- [ ] Avoidable-question rate improves over baseline.
- [ ] Non-ambiguous cross-run decision stability: ≥85%.
- [ ] Pre-decision context-use rate: 100% on every `assured` surface.
- [ ] All privacy adversarial cases pass.

## 7. Gate: public release candidate (WP11)

- [ ] Clean-machine install and uninstall verified.
- [ ] Upgrade, rollback, corrupt-index recovery, revoke, forget, rebuild
      verified.
- [ ] ≥2 Tier 1 harness adapters verified end to end.
- [ ] No private/user-specific data in repository, packages, fixtures, logs,
      or CI artifacts.
- [ ] Documentation executed successfully by a fresh agent with no prior
      context (procedure in §9).
- [ ] Zero unresolved critical/high defects.
- [ ] License, compatibility matrix, support boundaries, threat model, and
      limitations published.
- [ ] User approval of external release (RED).

## 8. Evidence bundles

Each completed gate produces an immutable bundle: inputs, revisions,
environment manifest, raw outputs, grading records, summaries, artifact
digests. Bundles live outside the public repo for anything touching private
data; only digests/counts appear in public evidence.

## 9. Fresh-agent documentation test

A clean checkout + a new agent with only public repo documents plus synthetic
fixtures must: identify the next authorized work package; reproduce the
ownership/trust model; generate the expected plan and traceability entries;
execute documented non-secret validation steps; and produce a handoff —
without prior conversation history or private profile data. Questions,
incorrect assumptions, setup time, and documentation defects are recorded
as test evidence.

## 10. Failure definition (reminder)

The project has failed even if the plugin technically runs when any of these
remain true: it is effectively a long static profile prompt; it sounds
personal but cannot prove decision improvement; a repository can elevate its
own authority; a model can request a broader profile; work-safe filtering
happens only after personal data entered a shared pack; any source acquires a
competing source of truth; agent-produced choices are learned as preferences
without acceptance; completion is claimed from commands rather than verified
outcomes.