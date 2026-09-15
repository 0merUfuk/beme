# Be Me — Acceptance

> Owns mandatory/prohibited conclusions, release gates, and what counts as
> evidence. Nothing is "done" because a command exited zero; every gate below
> requires recorded evidence in `docs/HANDOFF.md` or an evidence bundle.
>
> **Two axes, never conflated:** `docs/ROADMAP.md` records per-work-package
> *scope* completion (code written and tested). This file records *release
> gates* (measured evidence thresholds). A WP can be scope-complete while
> its release gate is pending — that is the current state for §5–§7, and it
> is not a contradiction.

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
- [x] Seed golden decisions approved — under ADR-022 delegation
      (explicit standing instruction, 2026-09-14): 34 real decisions,
      provenance recorded as delegated, never silent. Delegation is
      reopenable by any subsequent user instruction.
- [x] Exact dependencies verified (ADR-004: go-sdk v1.7.0, modernc sqlite
      FTS5, go 1.25.6 — compiled verification).

## 3. Gate: contracts (WP3) — complete

- [x] Versioned schemas: source descriptor, workspace identity, normalized
      record, provenance, resolution request, context pack, golden case, run
      manifest.
- [x] All public fixtures validate (`make validate` exits 0, all checks
      pass, repo-local only; private-corpus validation is the separate
      owner-gated `make validate-private` gate per ADR-023).
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

## 5. Gate: core alpha (WP5) — engine tested; measurement mechanism ready, owner-run measurement pending

Evidence status (2026-09-15): the deterministic items below pass in CI
(`go test ./...`, migration fixtures, work-safe boundary, determinism,
provenance/selection-reason assertions). The retrieval *measurement*
mechanism is implemented — `internal/evalrunner` computes per-case and
micro-averaged recall/precision against `required_evidence_refs` with these
thresholds (`TestRunnerRetrievalMetrics`). Measuring the private corpus is an
owner-run step (ADR-023), so the two items stay unchecked until that evidence
bundle exists.

- [x] Schema and migration fixtures: 100% pass (CI-gated).
- [x] Policy/precedence tests: 100% pass (CI-gated).
- [x] Denied-scope leakage: 0 (work-safe boundary tested at construction +
      pack section assertions).
- [x] Unsupported authoritative personal claims: 0 (injection-clamp and
      unknown-preference negative-control tests).
- [ ] Required-record retrieval recall: ≥90% — mechanism implemented;
      owner-run private-corpus measurement pending.
- [ ] Context precision: ≥80% — mechanism implemented; owner-run
      private-corpus measurement pending.
- [x] Deterministic pack snapshots stable (structural-identity test).
- [x] Every selected item has valid provenance and a selection reason
      (pack schema requires both; contract-tested).

## 6. Gate: trusted local beta (WP10) — pending owner-run live-model evaluation

Every item below except the privacy-adversarial item requires the live-model
B4-vs-B0 evaluation (blind paired grading over the private corpus). That
requires paid model-API runs — an owner-gated action Be Me's delegation does
not cover. Those items stay unchecked until the runs exist as an evidence
bundle. The privacy item is deterministic and checked on its own evidence.

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
- [x] All privacy adversarial cases pass — every §19 threat case executes
      and passes in `TestPrivacyCorpusDeterministic`, none `not_run`
      (backup restore and physical-purge resurrection included, ADR-027).

## 7. Gate: public release candidate (WP11) — alpha published; gate items partially evidenced

`v0.1.0-alpha.1` is published as an *alpha* — explicitly not a claim that
this gate passed. Item-level status:

- [x] Clean-machine install and uninstall verified (alpha.1 recovery:
      fresh-home binary runs; tagged module install outside the checkout).
- [x] Upgrade, rollback, corrupt-index recovery, revoke, forget, rebuild
      verified (unit + e2e): migrations idempotent/rollback/failed-safe
      (`migrate_test.go`); corrupt store recovered by rebuild with canonical
      sources byte-identical and resolution restored
      (`TestCorruptStoreRecovery`); revoke/forget tombstones honored in
      resolution and their rebuild semantics pinned
      (`TestRebuildDoesNotResurrectForgotten`, `TestRevocationTombstone`).
- [x] ≥2 Tier 1 harness adapters verified end to end (2026-09-15):
      protocol level — a real MCP client (official Go SDK) connected to the
      real stdio server, listed exactly the four tools, resolved a pack
      under work-safe, read status, filed quarantined feedback, and had a
      `profile` elevation attempt rejected at the protocol layer
      (`TestMCPClientEndToEnd`); config level — `adapter install|verify|
      remove` exercised against the installed Codex (0.154.0) and Claude
      Code (2.1.271) user configs, byte-identical after the full cycle
      (`TestAdapterInstallRemoveByteExact` + real-config verification).
      Remaining `advisory` limitation: pre-decision use inside live harness
      sessions is not measured (FR-045), so no surface is labeled
      `assured`.
- [x] No private/user-specific data in repository, packages, fixtures, logs,
      or CI artifacts (scan gate in CI; committed-tree deep scans).
- [x] Documentation executed successfully by a fresh agent with no prior
      context (procedure in §9) — PASSED 2026-09-15: clean checkout @
      `8896df9`, documented bootstrap only (python3.13 fallback per docs),
      all six documented gates exit 0 with expected outputs
      (`not_run` semantics verified), zero contradictions, zero
      improvisation, ~3-minute total. Evidence: fresh-agent report +
      transcript (recorded with ADR-024).
- [x] Zero unresolved critical/high defects (none known at publication;
      alpha limitations documented).
- [x] License, compatibility matrix, support boundaries, threat model, and
      limitations published.
- [ ] User approval of external release (RED) — v0.1.0-alpha.1 publication
      was authorized by ADR-022 delegation; a *non-alpha* release still
      requires explicit approval.

## 7a. Unchecked-item classification (2026-09-15)

Every unchecked item above, sorted by what it needs. Mechanisms are never
classified as owner-gated merely because *executing* them on private data,
paid APIs, or a release needs the owner.

| Item | Class | What remains |
|---|---|---|
| §5 retrieval recall ≥90% | owner-run execution (private corpus, no paid API) | Run `evalrunner` with a `Refs` lookup against the private deployment (ADR-023) |
| §5 context precision ≥80% | owner-run execution (private corpus, no paid API) | Same run as recall |
| §6 blind paired B4-vs-B0 | owner-paid model execution | Wire a live provider into `evalrunner.Provider`; blind human grading |
| §6 general-task non-regression | owner-paid model execution | Same live run |
| §6 invented-preference rate 0 | owner-paid model execution | Same live run (engine negative control already passes) |
| §6 trusted project override accuracy | owner-paid model execution | Same live run |
| §6 escalation recall ≥95% | owner-paid model execution | Same live run |
| §6 avoidable-question rate | owner-paid model execution | Same live run |
| §6 cross-run stability ≥85% | owner-paid model execution | Same live run (repeat runs implemented) |
| §6 pre-decision use 100% on `assured` surfaces | owner-run live harness sessions | No surface is `assured` (FR-045 not-applicable-v1); measuring needs live model sessions |
| §7 external release approval | approval-gated (RED) | Explicit owner approval of a non-alpha release |

Implementation work the agent could complete was completed on 2026-09-15:
retrieval metrics, all threat cases executed, physical-purge workflow on
synthetic data, benchmark harness, Windows CI, and installed-harness
verification levels (see HANDOFF §4). None remains in this class.

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