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

Baseline for this revision: every open item below was first reproduced against
PR head `6c5b7d7`, then fixed and re-verified at the current head. Each
"verified" row names the test or command that exercises the real boundary;
every new assertion was additionally proved by reintroducing the defect in a
scratch copy (mutation evidence in PR #1).

### Readiness (reported independently)

| Category | State | Evidence / blocker |
|---|---|---|
| Engineering ready for merge | yes | S1–S7, E1–E4, B1–B2 and D1 verified at head `419fb18` with mutation evidence; three-platform CI green including the installed-binary smoke (run [35052381926](https://github.com/0merUfuk/beme/actions/runs/35052381926)); every review thread answered with evidence and resolved; an independent adversarial review's four majors fixed. Merging remains the owner's decision |
| Release verification complete | no | P1, P2, P4 and P5 are verified. P3 was re-run independently at `9a9a6fa`: the core install-to-deployment path is improvisation-free, and all eight remaining findings are fixed at the final head — but certifying a full zero-improvisation run needs one more independent pass. P6 is an owner decision |
| Personal effectiveness demonstrated | blocked | live-model evaluation (E6), private-corpus retrieval measurement (E5) and live-session pre-decision use (H3–H5) need owner approval; runnable procedures are prepared |

### Safety and data-integrity invariants

| ID | Requirement / invariant | Current implementation and evidence | Remaining work | Status | Verification and acceptance |
|---|---|---|---|---|---|
| S1 | Ledger fails closed on detectable partial, unreadable, or malformed state, without bricking an interrupted first-time initialization | schema-3 ledger and key prove each other (key ID + generation + committed flag, ADR-030); missing, mismatched, rolled-back, malformed-entry, and pending-journal-without-ledger states fail closed on all twelve surfaces; interrupted first write recovers; legacy formats migrate | none | verified | `TestLedgerIntegrityFailsClosedOnEverySurface` (8 states × 12 surfaces, positive controls), `TestInterruptedFirstLedgerWriteRecovers`, `TestCrashBeforeKeyCommitKeepsEnforcement`, `TestLegacyLedgerMigratesAndRollbackIsDetected`, `TestLedgerRemovedTogetherIsUndetectable` (documented boundary); mutations M1–M6, M15 |
| S2 | Purge deletions reach the documented durability boundary before the journal is finalized, including on retry | every deletion is zeroized, flushed, unlinked and its directory flushed; projections compacted with `synchronous=FULL` and flushed; retries complete outstanding flushes; hard-linked canonical files reported as residuals | none | verified | `TestPurgeFlushesEveryDeletionBeforeFinalize`, `TestPurgeRetryCompletesOutstandingFlushes` (per deletion class), `TestPurgeRefusesToEraseHardLinkedCanonicalFile`, `TestEraseZeroizesBeforeUnlink`, `TestFlushFailuresAreReported`; platform limits from vendor docs in ADR-030; mutations M7–M9, M16–M17 |
| S3 | Conflicting purge/forget/build operations cannot lose updates or resurrect content | exclusive inter-process maintenance lock (`ledger/.lock`) taken by forget, purge, build and learning writes | none | verified | `TestMaintenanceOperationsDoNotLoseUpdates` (16 concurrent forgets + purge + build, also under `-race`), `TestMaintenanceLockTimesOut`, `TestLockSerializesHolders`; mutation M12 |
| S4 | Observations in a completed purge stay hidden after a data-dir backup restore | purge tombstones observation identities; list, list-all, inspect, review, family counts, feedback dedup and the rejection tombstone index hide them; build erases them; doctor counts them without IDs | none | verified | `TestRestoredObservationBackupStaysHidden`, `TestRestoredObservationHiddenOnCLI` (real binary, CLI + doctor + build), `TestLearningSurfacesRequireVerifiedLedger`; mutations M10, M11, M14 |
| S5 | Every projection read surface applies the ledger and fails closed | resolve, export, explain, doctor, MCP status/expansion and the learning surfaces filtered; guard test fails the build on direct reads and on traversal errors | none | verified | `TestRestoredBackupHiddenOnEveryReadSurface`, `TestRestoredBackupCannotResurrectOnAnySurface`, `TestUnusableLedgerFailsClosedOnEveryReadSurface`, `TestLedgerIntegrityFailsClosedOnEverySurface`, `TestReadSurfacesUseLedgerFilter`, `TestReadSurfaceGuardCatchesEvasions` |
| S6 | Context-item expansion is pack-bound with one refusal | ADR-029; `TestExpandItemIsPackBound`, MCP e2e | none | verified | suites pass at the current head |
| S7 | Physical purge keeps minimal non-content tombstones, confirmation, dry run, partial reports, resumability | ADR-027/ADR-030; purge suites; threat cases 30 and S1–S4 | none | verified | suites pass at the current head |

### Evaluation

| ID | Requirement | Current implementation and evidence | Remaining work | Status | Verification and acceptance |
|---|---|---|---|---|---|
| E1 | Arms B0–B4 follow the contract (B1 real bootstrap; B2 all eligible, no selection/precedence/truncation; B3 pre-precedence retrieval without provenance) | every arm built from one session's `Session.EvalInputs` behind the same Stage-A gate; B1 uses the real managed bootstrap text (`internal/bootstrap`) | none | verified | `TestArmsReceiveExactlyTheirConstruction` (10 subtests), `TestSyntheticArmFixturePositiveControls`, `TestArmsCannotMutateEachOthersInput`, `TestBootstrapMatchesCanonicalAdapterText`; mutations M1–M2, M5, M7 of the evaluation set |
| E2 | Ablations no-scope, no-provenance, no-unknowns, canonical-only, learned-only | all five implemented; no-scope refuses outside a synthetic, network-disabled deployment (`Runtime.NoScopeRefusal`); canonical-only is role-based and `not_run` on an unknown role | none | verified | `TestNoScopeRefusedOutsideSyntheticDeployments`, ablation assertions in `TestArmsReceiveExactlyTheirConstruction`; mutations M3–M4 of the evaluation set |
| E3 | Manifests identify real model settings, prompts, corpus revisions, arm construction | observed provider settings, prompt and fixture hashes, corpus revisions and arm construction recorded; build-info versions reported honestly as `unknown` in test binaries | `BEME_EVAL_GIT_COMMIT` must be set for owner runs in a worktree (Go stamps the main checkout HEAD) | verified | `TestManifestsRecordObservedValues`; mutation M6 of the evaluation set |
| E4 | Blockers fail units; generated text reaches raw and blinded artifacts | `TestRunnerBlockerFailsUnitAndPreservesText` | none | verified | suite passes at the current head |
| E5 | Retrieval recall ≥90% / precision ≥80% on the locked corpus (core alpha) | mechanism proven on synthetic fixtures; `beme-eval retrieval --corpus DIR --config DIR` runs it without a model | owner-authorized local run on the private corpus | blocked-approval | procedure in [INTEGRATIONS.md](INTEGRATIONS.md) "Owner-run procedure for the evaluation gates"; evidence bundle kept outside the repository |
| E6 | Blind paired B4-vs-B0 and the trusted-beta behavioral gates | runner proven with deterministic mocks; `beme-eval behavioral … --provider command --command '…'` pipes prompts to an external harness CLI (`--dry-run` counts generations without running them) | paid live-model runs | blocked-approval | same procedure; §6 thresholds unchanged |

### Benchmark, integration, platform, and documentation

| ID | Requirement | Current implementation and evidence | Remaining work | Status | Verification and acceptance |
|---|---|---|---|---|---|
| B1 | Benchmark seed identifiers cannot escape owned dirs or inject metadata; explicit seed; accurate failure reporting | whole seed validated before anything is written (ID allowlist, Windows device names, enum and control-character checks, round-trip through the real parser, case-insensitive collisions); `--seed` required; cleanup and output-write failures surface in the exit code | Windows-specific ID cases verified on macOS only; three-platform CI covers the rest | verified | `internal/benchmark/seed_safety_test.go`, `cmd/beme-bench/{seed_flag,cleanup}_test.go`; 11 mutations caught |
| B2 | NFR-008 measurement with environment conditions | `evals/benchmarks/seed-baseline.json` regenerated at the current head (darwin/arm64, go1.25.6, 14 cpus): warm resolve p95 0.30 / 5.65 / 27.20 ms at 1×, 20×, 200× against the 1 s target | re-measure per release commit | verified | `make bench` output recorded in the baseline file |
| D1 | Diagnostics: doctor never downgrades `policy_blocked`; error labels and exit codes match failure classes; guard tests fail on traversal errors | severity-preserving `raise()`; explain maps trace→4, unusable ledger→3, other→1; guard propagates walk/read errors; `beme candidate --config` no longer falls back to the real deployment | none | verified | `TestDoctorKeepsMostSevereStatus`, `TestExplainErrorClasses`, `TestReadSurfaceGuardCatchesEvasions`, `TestRestoredObservationHiddenOnCLI`; mutations M13–M14 |
| H1 | Harness configuration lifecycle (Claude Code, Codex) | re-run at the final head on 2026-09-16 against installed Claude Code 2.1.271 and Codex 0.154.0 in isolated config homes, no model calls: `TestClaudeCodeHarnessIntegration` (add → get → remove), `TestCodexHarnessIntegration` (add → list/doctor → remove), `TestAdapterInstallRemoveByteExact` | none | verified | `BEME_HARNESS_INTEGRATION=1 go test ./cmd/beme -run HarnessIntegration` — all pass |
| H2 | MCP protocol connection | re-run at the final head on 2026-09-16: `TestMCPClientEndToEnd` (official Go MCP client against the real stdio server) and Claude Code reporting `Connected` for the spawned server | none | verified | same run; Codex has no model-free MCP health check, which is why its connection level stays unverified |
| H3 | A real agent session retrieves context | not run | live session with synthetic data | blocked-approval | runnable procedure in [INTEGRATIONS.md](INTEGRATIONS.md) (H3–H5); session transcript shows `beme.resolve_context` |
| H4 | Retrieval happens before a material decision | not run | same session | blocked-approval | same procedure; tool call precedes the decision in the transcript |
| H5 | Relevant context used; no invented preferences | not run | same session with negative-control prompt | blocked-approval | same procedure; graded transcript, counts reported here |
| H6 | `assured` surfaces with 100% pre-decision use | no surface claimed (FR-045) | none in v1 | out-of-scope | — |
| P1 | Full test suite on macOS, Linux, Windows | CI run [35052381926](https://github.com/0merUfuk/beme/actions/runs/35052381926) green on code head `419fb18`: `test (macos-latest)`, `test (ubuntu-latest)`, `test-windows`, all three `installed-binary-smoke` jobs, and `private-data-scan`. Locally at the same head: build, vet (darwin/linux/windows), full suite, `-race` on app/durable/learning/storage/evalrunner, threat corpus 34/34, docs 15/15, validate 19/19, CI regression 7/7, scan clean | none | verified | the linked run; a later docs-only commit has its own run linked in PR #1 |
| P2 | Installed-binary smoke tests per OS at the release candidate | `installed-binary-smoke` green on macOS, Linux and Windows in run [35052381926](https://github.com/0merUfuk/beme/actions/runs/35052381926): `go install`, then doctor/build/status/preview/candidate/forget/purge --dry-run plus the threat-corpus and benchmark binaries, run from outside the repository in an isolated deployment | none | verified | the linked run |
| P3 | Fresh-checkout documentation test at the current head | re-run on `9a9a6fa` by a second independent agent (clone, isolated HOME and `BEME_*` overrides, public docs only). The **core install-to-deployment path is improvisation-free**: install → doctor → register a source → build → status → preview (human and `--json`) → export → explain → forget → purge (dry-run and real, including resume) all followed the docs literally from outside the repo. Verdict was still **no** on three non-core steps (verifying MCP without a harness, the origin of review-queue observations, interrupting a purge) plus five documentation errors, two of them factual: the frontmatter `type` list named kinds the code maps to `unmapped_reference`, and the status rule claimed only `active` is ingested when only `deprecated` is skipped | all eight findings fixed at the final head: harness-free MCP smoke check and `--capability` semantics, observation provenance, the real kind and status rules with their consequences, the corrected "every surface" claim (`doctor` and `status` are deliberately not ledger-gated), a "Registering a workspace" section, and `--help`/`--version` now exit 0 instead of 2 | implemented-unverified | a third independent pass over the fixed steps; verified means one full run with zero improvisations |
| P4 | Release notes and release-verification procedure | [RELEASE_VERIFICATION.md](RELEASE_VERIFICATION.md): preconditions, verification run, rollback guidance, and draft notes (unpublished) | owner review; publishing stays an owner action | verified | document reviewed in PR #1; nothing tagged or published |
| P5 | No private data in public artifacts | `scripts/scan_private_data.py` clean at the current head with the local extra-terms list | re-scan on the final head in CI | verified | scan output ("private-data scan: clean") |
| P6 | Non-alpha release approval | — | owner decision | blocked-approval | — |

### Consolidated approval request (owner decisions)

Everything below is blocked only on an explicit owner decision; nothing here
has been attempted. Each row states exactly what would run and what it costs.

| # | Decision | What runs | Cost / risk | Unblocks |
|---|---|---|---|---|
| 1 | Merge PR #1 into `main` | the reviewed branch only; no tag, no publish | none beyond the merge itself | the release-verification track (P1–P4 on a `main` commit) |
| 2 | Live harness sessions on a synthetic deployment | the H3–H5 procedure in [INTEGRATIONS.md](INTEGRATIONS.md), run with `HOME` pointed at the isolated deployment: 3 task sessions + 1 negative control | spends your model usage. Proposed cap: 4 sessions, one harness (Claude Code), synthetic sources only — no private corpus and no canonical personal knowledge is read | H3, H4, H5 and any future `assured` claim |
| 3 | Private-corpus retrieval measurement | `make validate-private`, then `beme-eval retrieval --corpus "$BEME_PRIVATE_EVAL_DIR" --config DIR --out DIR --json` | no model spend, no network. Reads the private corpus read-only on your machine; artifacts are written to `--out` outside the repository and never committed | E5 |
| 4 | Blind paired B4-vs-B0 behavioral run | `beme-eval behavioral --corpus DIR --config DIR --provider command --command '<harness CLI>' --arms B0,B4 --repeats N --out DIR`; run `--dry-run` first, which prints the exact generation count and executes nothing | paid model runs through your own harness CLI. Proposed cap: 2 arms × the locked split × 3 repeats, confirmed by the `--dry-run` count before any spend; prompts carry private corpus content to whatever model that CLI calls, so choose the provider accordingly | E6 and the trusted-beta gates |
| 5 | Publish a release (tag + notes) | the procedure in [RELEASE_VERIFICATION.md](RELEASE_VERIFICATION.md) | public artifact; irreversible tag | P6 |

Not requested, and not needed for any acceptance row: running a physical
purge on real personal data. Destructive behavior is demonstrated on
disposable synthetic fixtures only (S2, S4, S7, threat cases 30 and S1–S4).

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