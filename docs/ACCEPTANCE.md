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
- [x] All privacy adversarial cases pass — every §19 threat case and the
      supplementary cases S1–S4, each behind a positive control proving its
      fixture is present and detectable, execute and pass from the
      shared registry, none `not_run` (`TestPrivacyCorpusDeterministic`;
      runner `cmd/beme-threat-corpus`, `TestThreatCorpusRunner`; ADR-027).

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
      (`TestRebuildDoesNotResurrectForgotten`, `TestRevocationTombstone`);
      revoked and purged records stay hidden on every read surface after a
      backup restore (`TestRestoredBackupHiddenOnEveryReadSurface`,
      `TestRestoredBackupCannotResurrectOnAnySurface`).
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

## 7a. Completion ledger

The single completion-tracking record for Be Me. Per-requirement evidence stays
in [`REQUIREMENTS.md`](REQUIREMENTS.md); current snapshot and next step in
[`HANDOFF.md`](HANDOFF.md) §1. Statuses: **verified** (behavior proven by a
test or command that exercises the real boundary), **implemented-unverified**,
**not-implemented**, **blocked-approval**, **blocked-access**,
**out-of-scope**. Nothing moves to verified without recorded evidence, and no
threshold is lowered here.

Baseline for this revision: PR head `6c5b7d7` (CI run 35019956360 green).
Every not-implemented safety item below was reproduced against that head.

### Readiness (reported independently)

| Category | State | Evidence / blocker |
|---|---|---|
| Engineering ready for merge | no | safety items S1–S5, evaluation items E1–E3, benchmark item B1, diagnostics item D1 open |
| Release verification complete | no | installed-binary smoke tests and fresh-checkout test not run at the current head; release notes not prepared |
| Personal effectiveness demonstrated | blocked | live-model evaluation and private-corpus measurement need owner approval |

### Safety and data-integrity invariants

| ID | Requirement / invariant | Current implementation and evidence | Remaining work | Status | Verification and acceptance |
|---|---|---|---|---|---|
| S1 | Ledger fails closed on detectable partial, unreadable, or malformed state, without bricking an interrupted first-time initialization | fails closed on unreadable/corrupt JSON and missing key | missing `tombstones.json` with `purge.key` present was accepted as empty; `hmac-sha256:broken` accepted (both reproduced) | not-implemented | tests for each detectable state on every read, learning, and rebuild surface; recoverable interrupted init; documented undetectable cases |
| S2 | Purge deletions reach the documented durability boundary before the journal is finalized, including on retry | ledger and journal writes durable | trace, observation, and canonical unlinks not directory-flushed; retries skip flushes for already-absent files | not-implemented | flush-failure injection per deletion class; retry completes outstanding flushes; platform limits stated from vendor docs |
| S3 | Conflicting purge/forget/build operations cannot lose updates or resurrect content | none | 24 concurrent forgets kept 1 revocation (reproduced) | not-implemented | concurrency tests (incl. `-race`) for forget/purge/build/learning writes |
| S4 | Observations in a completed purge stay hidden after a data-dir backup restore | observations deleted at purge | restored observations listed, inspected, reviewable (reproduced) | not-implemented | restored-backup test across candidate list/inspect/review, dedup, MCP feedback, CLI |
| S5 | Every projection read surface applies the ledger and fails closed | resolve, export, explain, doctor, MCP status/expansion filtered; `TestRestoredBackupHiddenOnEveryReadSurface`, `TestRestoredBackupCannotResurrectOnAnySurface` | extend to learning surfaces (S4) and new ledger states (S1) | implemented-unverified | same tests extended to S1/S4 states |
| S6 | Context-item expansion is pack-bound with one refusal | ADR-029; `TestExpandItemIsPackBound`, MCP e2e | re-check after S1/S3 changes | verified | tests pass at final head |
| S7 | Physical purge keeps minimal non-content tombstones, confirmation, dry run, partial reports, resumability | ADR-027; purge test suites; mutation evidence in PR #1 | preserve through S1–S4 changes | verified | suites pass at final head |

### Evaluation

| ID | Requirement | Current implementation and evidence | Remaining work | Status | Verification and acceptance |
|---|---|---|---|---|---|
| E1 | Arms B0–B4 follow the contract (B1 real bootstrap; B2 all eligible, no selection/precedence/truncation; B3 pre-precedence retrieval without provenance) | B2 reused the B4 pack; B3 flattened a post-precedence pack; placeholder bootstrap | implement arm construction from resolver primitives | not-implemented | fixtures where arms must differ; per-arm input assertions; mutation evidence |
| E2 | Ablations no-scope, no-provenance, no-unknowns, canonical-only, learned-only | no-scope and canonical-only `not_run`; no-provenance leaves provenance manifest | implement all; no-scope synthetic-only guard | not-implemented | same |
| E3 | Manifests identify real model settings, prompts, corpus revisions, arm construction | placeholder hashes and hardcoded settings | record real values | not-implemented | manifest assertions |
| E4 | Blockers fail units; generated text reaches raw and blinded artifacts | `TestRunnerBlockerFailsUnitAndPreservesText` | keep through E1–E3 | verified | suite passes at final head |
| E5 | Retrieval recall ≥90% / precision ≥80% on the locked corpus (core alpha) | mechanism proven on synthetic fixtures | owner-authorized local run on the private corpus; runnable command | blocked-approval | evidence bundle kept outside the repository |
| E6 | Blind paired B4-vs-B0 and the trusted-beta behavioral gates | runner proven with deterministic mocks | paid live-model runs | blocked-approval | §6 thresholds unchanged |

### Benchmark, integration, platform, and documentation

| ID | Requirement | Current implementation and evidence | Remaining work | Status | Verification and acceptance |
|---|---|---|---|---|---|
| B1 | Benchmark seed identifiers cannot escape owned dirs or inject metadata; explicit seed; accurate failure reporting | work-dir guard and marker ownership | seed ID traversal/injection; repo-relative default seed; cleanup error swallowed | not-implemented | traversal, separator, drive, control-char, collision, symlink tests |
| B2 | NFR-008 measurement with environment conditions | `evals/benchmarks/seed-baseline.json` | re-measure after B1 | verified | report regenerated at final head |
| D1 | Diagnostics: doctor never downgrades `policy_blocked`; error labels and exit codes match failure classes; guard tests fail on traversal errors | doctor downgraded `policy_blocked` to `degraded` (reproduced); explain labels all errors "policy blocked"; guard test swallows walk errors | fix and test | not-implemented | CLI-level tests |
| H1 | Harness configuration lifecycle (Claude Code, Codex) | opt-in installed-CLI tests in isolated homes (2026-09-15) | re-run at final head | verified | `BEME_HARNESS_INTEGRATION=1` tests pass |
| H2 | MCP protocol connection | `TestMCPClientEndToEnd`; Claude Code `mcp get` Connected | re-run | verified | e2e passes |
| H3 | A real agent session retrieves context | not run | live session with synthetic data | blocked-approval | session transcript shows `beme.resolve_context` call |
| H4 | Retrieval happens before a material decision | not run | same session | blocked-approval | tool call precedes the decision in the transcript |
| H5 | Relevant context used; no invented preferences | not run | same session with negative-control prompt | blocked-approval | graded transcript |
| H6 | `assured` surfaces with 100% pre-decision use | no surface claimed (FR-045) | none in v1 | out-of-scope | — |
| P1 | Full test suite on macOS, Linux, Windows | CI 35019956360 | re-run at final head | verified | final-head CI |
| P2 | Installed-binary smoke tests per OS at the release candidate | alpha.1 binaries verified; current head not | CI smoke job or recorded runs | not-implemented | `go install` + CLI smoke on each OS |
| P3 | Fresh-checkout documentation test at the current head | passed at `8896df9` (stale) | re-run with public docs only | not-implemented | recorded run with zero improvisation |
| P4 | Release notes and release-verification procedure | CHANGELOG Unreleased | draft notes + procedure (no publish) | not-implemented | reviewed draft |
| P5 | No private data in public artifacts | scan clean at `6c5b7d7` | re-scan at final head | verified | scan output |
| P6 | Non-alpha release approval | — | owner decision | blocked-approval | — |

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