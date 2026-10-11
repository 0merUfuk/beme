# Be Me — Requirements Traceability Matrix

> One row per requirement ID, expanded from the blueprint's range-level seed.
> No requirement is "implemented" because a row exists; evidence links are the
> production-code entry criteria. Update the Evidence column as work advances.

**Integration qualification (2026-10-09):** existing rows describe implemented
mechanisms and their listed tests, not whole-product acceptance. Independent
audits reopened runtime/evaluation and brain correctness work, including
false-success behavioral verdicts, restore, deduplication and scope failures.
The combined source now includes the audit regressions and corrections
(ADR-036–038). Independent review then reopened six adjacent brain findings and
evaluator evidence/blinding integrity. The bounded corrective pass (ADR-039–040)
adds exact regression and lifecycle evidence. Its final bounded review found
one remaining BFR-006 overlapping-publication defect. Corrective cycle 2 changes
only that accounting protocol and focused regressions; parent independent
source-bound rechecks now pass. See HANDOFF §1 for exact
synthetic gates. A separately authorized local rollout verified CLI and fresh
MCP connection mechanics; native model use and human decision benefit remain
unmeasured. Publication preparation, including platform-fixture and artifact
verification, is tracked separately under ADR-041–043 and RELEASE_VERIFICATION.md.

**Corrective evidence (ADR-039–040):** FR-073/074 use
`TestFinalBoundedBacklogKeepsLatestRecapture`,
`TestCorrectionRecaptureStopsPendingRetirement`,
`TestCorrectionCommittedMergeReceiptsSurviveManyRecaptures` and the unresolved
retry-capacity test. FR-075/086 use `TestFinalOldGlobalOpenNeverBroadcastsIDs` and
`TestCorrectionAllGlobalWarningPathsWithholdUnsupportedIDs`. FR-084 uses the
actual generated chain in `TestBrainExplicitWrapperSurvivesGeneratedChain`.
NFR-009 adds `TestFinalRepeatedRecapturesPublicLifecycleReadable` (800 real
recaptures) and `TestCorrectionWriterRefusesUnreadableOutputWithoutReplacingLive`.
NFR-001 evidence includes the evaluator semantic matrix, original grader controls
and content-only reviewer-packet test; local engineering acceptance is complete,
not installed-harness or human-benefit acceptance.

**Legend:** Status — `defined` (contract only) · `planned` (design exists, no
code) · `partial` · `implemented` (code + tests) · `verified` (gate evidence).

**Status counts** (derived from the rows below; CI-checked by
`scripts/test_docs_consistency.py` D7): 83 rows — 77 implemented · 2 partial · 2 not-applicable-v1 · 2 active.

## Core and ownership

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-001 | Core operates independently of harness packaging | WP1/WP3 | Boundary ADR; dependency-graph audit (no harness SDK imports in core packages) | ADR-002 + verified: no harness-SDK imports in `internal/` (grep); MCP SDK only in `cmd/beme`| implemented |
| FR-002 | Register external sources without changing ownership | WP4 | Source adapter contract tests | `internal/app` registration = the trust act; adapters point at sources, never copy (ADR-003)| implemented |
| FR-003 | Optional episodic-evidence adapter; no duplicate transcript ingestion | WP4 | Adapter contract test + boundary ADR review | ADR-019: contract documented; no transcript importer exists in Be Me (code audit)| implemented |
| FR-004 | Derived indexes/records rebuildable from authorized sources | WP4 | Rebuild test: delete index, rebuild, compare digest | `Store.Wipe()` + rebuild; `TestWipeRebuildable`| implemented |
| FR-005 | Public engine code contains no user-specific profile data or private fixtures | WP10/WP11 | Private-data scan of repo + artifacts; public-fixture provenance audit | `scripts/scan_private_data.py` CI gate; committed-tree deep scans; fresh-agent verified no private paths| implemented |

## Profiles and policy

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-010 | Each serving process binds one immutable capability ceiling | WP5 | Capability binding test; server startup refuses unbound | `app.Serve()` binds one capability per process; capability is not request-settable| implemented |
| FR-011 | Requests may narrow, never widen capability | WP5 | Elevation-attempt regression (threat cases 1, 21) | Request schema structurally lacks elevation fields (validate_contracts negative fixtures); capability process-bound| implemented |
| FR-012 | Work-safe serving/build uses only safe projection + approved manifest | WP4 | Safe-builder path test: work-safe build succeeds with personal roots absent | `TestWorkSafeBoundaryAtConstruction`: personal source excluded from work-safe build AND pack| implemented |
| FR-013 | Repository-local config only narrows | WP5 | Repo-config widening regression test | Profile eligibility comes only from the trusted descriptor; requests cannot add sources/profiles| implemented |
| FR-014 | Ambiguous workspace identity disables project/personal resolution | WP5 | Ambiguity fixture; fail-closed test | `TestAmbiguousPathsFailClosed`; `Serve.Resolve` returns policy-blocked on ambiguity| implemented |
| FR-015 | Denied records invisible across resolve/expand/explain/error/metadata | WP5/WP7 | Denial-invisibility regression (threat cases 6–8, 26) | Policy exclusions happen before retrieval; expansion is pack-bound with one indistinguishable refusal (ADR-029, `TestExpandItemIsPackBound`); explain drops steps naming non-visible records (`TestRestoredBackupHiddenOnEveryReadSurface`); threat cases 8 and 26| implemented |
| FR-016 | Trusted project policy requires out-of-band registration, allowed paths, approved revision/digest, scope, authority ceiling | WP4/WP5 | Project-policy registration test; changed-revision pending test (threat case 22) | `schemas/policy/workspace.schema.json` bound_project_policy requires out-of-band approved_digest; registration is the trust act| implemented |

## Sources and indexing

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-020 | Only registered roots ingested | WP4 | Unregistered-root ingestion test | `internal/app` loads only `<config>/sources/`; walker walks registered roots only| implemented |
| FR-021 | Ingestion never executes repository code/hooks/packages/macros/generated commands | WP4 | No-exec ingestion test (threat case 16) | Ingestion is pure file reading; no `os/exec` import in `internal/ingestion`. The explicit evaluator command provider is separate from ingestion.| implemented |
| FR-022 | Real-path and symlink checks prevent escape from registered roots | WP4 | Symlink-escape fixtures | `TestSymlinkEscapeSkipped` + real-path containment in walker| implemented |
| FR-023 | Secret/never-ingest paths and patterns excluded before indexing | WP4 | Secret-pattern fixtures (threat case 17) | `TestSecretScanBlocksCredentials` + `TestHardExcludesNeverIngested`| implemented |
| FR-024 | Every derived item retains revision, locator, hash, transformation, trust ceiling, sensitivity | WP4 | Provenance round-trip test | Provenance records locator/revision/hash/transformation; `TestPutAndFetchRecords`| implemented |
| FR-025 | Missing/moved/revoked/changed sources produce explicit stale/unavailable state | WP4 | Stale-state fixtures | doctor reports missing sources/roots as degraded with remediation; `Session.Health` rereads failed-source metadata; partial packs incomplete, empty projections unavailable, missing/corrupt health stale (`TestPartialBuildDegradationSurvivesServingAndRecovery`)| implemented |
| FR-026 | Source deletion/revocation invalidates derived indexes, caches, observations, authorized expansion refs | WP4/WP9 | Revoke lifecycle test (threat cases 10, 18, 24) | `Store.Tombstone` + durable ledger applied on every read and learning surface (ADR-027, ADR-030); `TestRevocationTombstone`, `TestForgetSurvivesRestoreAndCorruptRecovery`, `TestExpandItemIsPackBound` (expansion refused after revocation or rebuild), `TestRestoredObservationBackupStaysHidden` and `TestRestoredObservationHiddenOnCLI` (purged observations restored from a backup stay hidden and are erased by build); threat cases 10, 18, and 24 executed| implemented |

## Resolver and pack

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-030 | Policy/scope eligibility runs before retrieval and ranking | WP5 | Two-stage ordering test; eligibility-first trace assertions | `internal/policy` Stage-A runs before retrieval in `resolver.Resolve`; ADR-007| implemented |
| FR-031 | Precedence is lexicographic by tier, not a single relevance score | WP5 | Precedence fixtures; deterministic tie-break tests | `resolvePrecedence` lexicographic tiers + stable ID tie-break; `TestProjectPolicyOutranksPersonalPreference`| implemented |
| FR-032 | Resolver supports multiple task facets | WP5 | Multi-facet fixtures | shared request derivation before Stage A; CLI/MCP/evaluation parity (`TestNaturalTaskFacetsReachScopedSource`, `TestRetrievalEvidenceBindsRiskAndHarness`)| implemented |
| FR-033 | Resolver preserves resolved/shadowed/unresolved/stale-suspected/scope-separated conflict states | WP5 | Conflict-state fixtures | shadowed conflicts preserved in pack (`TestProjectPolicyOutranksPersonalPreference`); states enumerated in schema| implemented |
| FR-034 | Resolver reports task-material unknowns and source/retrieval incompleteness | WP5 | Unknowns fixtures; completeness states | `deriveUnknowns` + `TestUnknownsNeverInvented` negative control| implemented |
| FR-035 | Mandatory constraints/conflicts/unknowns/degradations never silently removed by budgeting | WP5 | Budget-truncation regression (threat case 14) | `TestMandatoryContentNotBudgetDropped` → incomplete + degradation, never silent omission| implemented |
| FR-036 | ContextPack is versioned JSON with deterministic ordering and trace digest | WP5 | Pack snapshot stability tests | `TestDeterministicPacks`, `TestParentMultiSourceProvenanceDeterminism`; real generated personal/work-safe/cache-failure schema test `TestGeneratedRuntimePackSchema`| implemented |
| FR-037 | Harness renderers derive from the same canonical pack | WP5/WP8 | Renderer derivation tests | CLI human render + JSON come from one pack struct; no separate renderer state exists| implemented |
| FR-038 | Explain output generated from the actual resolver trace | WP6 | Trace-explain equivalence test | `Session.Resolve` persists sanitized traces for CLI/MCP/evaluation; cache failure clears handle without blocking reads (`TestMCPSourceEvidenceTraceAndHealth`, `TestTraceFailureIsHonestAndReadStillServes`)| implemented |
| FR-039 | Work-safe provenance is a safe scoped view; omits private identifiers, locators, paths, repo names, denied relationship metadata, correlatable hashes | WP5 | Safe-provenance regression (threat case 25) | work-safe provenance omits source IDs/revisions/hashes (`provenanceManifest`); task_summary echoes requester text only| implemented |

## Interfaces

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-040 | MCP exposes only scoped resolve, authorized expand, safe status, quarantined feedback | WP7 | Tool-surface enumeration test | `TestToolSurfaceContract`: exactly the four tools in `contracts.MCPTools`; expansion authorized only for items selected into a pack issued by the same session (ADR-029)| implemented |
| FR-041 | Admin operations and unrestricted evidence inspection unreachable via ordinary agent MCP capability | WP7 | Admin-reachability regression (threat case 23) | No admin verb is registered on the MCP server (surface test); admin lives in CLI only| implemented |
| FR-042 | CLI supports human + JSON output, stable exit codes, preview/explain/doctor/rebuild/source lifecycle/profile build/review/export/forget | WP6 | CLI contract tests | CLI: status/doctor/build/preview/resolve/explain/export/forget/adapter/candidate (+JSON, versioned exit codes); explain/export E2E-verified| implemented |
| FR-043 | Adapter install/removal idempotent; preserves unrelated user configuration | WP8 | Install/uninstall round-trip tests on synthetic harness configs | `TestAdapterInstallRemoveIdempotent` preserves unrelated config| implemented |
| FR-044 | Runtime unavailability produces documented degradation without broader-profile fallback | WP7/WP8 | Unavailability fixtures | Empty/unbuilt store → honest degraded packs (stale_projection notice), never broader profile; `Serve` degradations| implemented |
| FR-045 | Surfaces labeled `assured` prove pre-decision context resolution (100% measured) | WP8 | Pre-decision context-use measurement | No surface labeled `assured` in v1 (nothing to prove); measurement protocol documented in ACCEPTANCE §6| not-applicable-v1 |
| FR-046 | Surfaces without that proof are labeled `advisory` with visible limitation | WP8 | Adapter tier/assurance matrix in docs | All adapters documented `advisory` with visible limitation (adapters/*, INTEGRATIONS)| implemented |
| FR-047 | Snapshot reuse only when capability, workspace, projection revision, task origin, age, stale state valid and explicit | WP8 | Snapshot-validity tests (threat case 9) | No snapshot reuse implemented in v1 (each resolve is fresh); contract documented for future work| not-applicable-v1 |

## Learning

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-050 | Agent feedback and inferred behavior create quarantined observations only | WP9 | Feedback-path test (threat case 11) | `internal/learning/store_test.go` TestObserveNeverCanonical; MCP rewired to learning store | implemented |
| FR-051 | Canonical promotion and authority/scope changes require trusted user action | WP9 | Promotion lifecycle test | `candidate review --action approve` records decision only; canonical write is the owning repo's proposal path; no canonical-write API exists in Be Me | implemented |
| FR-052 | Evidence families deduplicated; correlated repetitions never appear independent | WP9 | Evidence-family fixtures (threat case 12) | `internal/learning/store_test.go` TestFamilyDedupNeverIndependent | implemented |
| FR-053 | Rejected candidates tombstoned against equivalent re-proposal | WP9 | Tombstone regression (threat case 13) | `internal/learning/store_test.go` TestRejectedTombstoneRefusesEquivalent (normalized fingerprints) | implemented |
| FR-054 | Work-restricted observations never become global personal knowledge without de-identification and approval | WP9 | Cross-namespace promotion regression | `observationSensitivityFor`: work-safe feedback inherits public_general; TestSensitivityInherited | implemented |
| FR-055 | Canonical changes transactional, auditable, reversible; physical purge is distinct, RED, irreversible, tombstone-only | WP9 | Transaction/undo tests; purge workflow documentation | Observation writes atomic (tmp+rename) + review audit trail; physical purge `beme purge` / `app.PhysicalPurge` (RED: typed confirmation, dry run, on-disk erasure through actual provenance refs, resumable journal, idempotent, keyed content-free ledger, durability boundary before erasure, inspection failures abort, Git/backup residuals reported) — `TestPhysicalPurgeErasesAndBlocksResurrection`, `TestPhysicalPurgeResumesAfterFailureAtEveryStage`, `TestPurgeResumeWithObservationStorageFailures`, `TestPurgeErasesNothingBeforeDurabilityBoundary`, `TestPhysicalPurgeCoversBothProjectionStores`, `TestPurgeLedgerIsKeyedAndContentFree`, threat cases 30 and S1–S4 (ADR-027); integrity-verified enforcement state, observation tombstones, durable erasure of every deletion before journal removal, and a maintenance lock (ADR-030) — `TestLedgerIntegrityFailsClosedOnEverySurface`, `TestInterruptedFirstLedgerWriteRecovers`, `TestLegacyLedgerMigratesAndRollbackIsDetected`, `TestPurgeFlushesEveryDeletionBeforeFinalize`, `TestPurgeRetryCompletesOutstandingFlushes`, `TestPurgeRefusesToEraseHardLinkedCanonicalFile`, `TestMaintenanceOperationsDoNotLoseUpdates`; running it on real data is an owner action| implemented |

## Portability and operation

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-060 | Core functionality works without network access | WP4/WP6/WP10 | Offline build + resolve in network-disabled environment | Zero network imports in `internal/` (grep-verified); stdio-only MCP| implemented |
| FR-061 | V1 exposes MCP over local stdio only; network serving deferred pending ratified threat model | WP7 | Transport enumeration test; no listener in binary surface | `TestStdioOnlyTransport` rejects non-stdio; no listener in binary| implemented |
| FR-062 | Config, durable data, cache, runtime state use platform-appropriate directories and remain separable | WP4 | Platform-dir tests (macOS verified first) | `app.DefaultDirs()` per OS — macOS `~/Library`, Linux XDG, Windows `%AppData%`/`%LocalAppData%` — + BEME_*_HOME overrides; `TestDefaultDirsPerPlatform` (ADR-028)| implemented |
| FR-063 | Canonical profile/source path configurable; no hard-coded `~/.beme` | WP4 | Config-resolution tests | Config root from env/config only; no `~/.beme` constant in code| implemented |
| FR-064 | Schema and index migrations versioned, atomic, rollback-aware, tested | WP4 | Migration fixtures | `migrate.go` + `TestMigrationsFreshAndIdempotent`/`RollbackAndReapply`/`FailedMigrationLeavesPriorIntact`| implemented |
| FR-065 | Corrupt operational state recoverable through rebuild without changing canonical sources | WP4/WP10 | Corruption-recovery test | `Wipe()` rebuild; doctor flags rebuild-needed; rebuild never touches sources| implemented |

## Brain (V2 second brain, ADR-033)

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-070 | The brain vault is plain markdown/yaml, initialized idempotently; non-document files (code) are flagged, never ingested | WP12 | Vault init + foreign-file audit tests | `internal/brain` `Init`, `foreignFiles`; `TestDoctorFlagsCapOverlapLintForeignAndInvalid` | implemented |
| FR-071 | Capture is selective: code, diffs, stack traces, secrets, transcripts, instruction-shaped text, and under-specified notes (a decision without its why) are refused | WP12 | Gate table test + positive controls | `TestGateRejectsWhatTheBrainMustNeverHold`, `TestGateAcceptsLegitimateNotes`; CLI exit 3 in `TestBrainSessionContinuityEndToEnd` | implemented |
| FR-072 | Agents write proposals only; Brain Doctor is the sole promoter; capture never waits on maintenance | WP12 | Lock + inbox tests | `TestMaintenanceLockExcludesConcurrentDoctor`, `TestProposalFailingGateAtDoctorTimeIsRejectedNotPromoted` | implemented |
| FR-073 | Doctor merges only conservative exact equivalence and supersedes by topic/capture chronology; paraphrases remain reviewable; validated decisions are protected | WP12 | Dedupe/supersede/conflict tests | `TestDoctorPreservesRewordedClaimsForReview`, `TestReliabilityScalarRevisionMustReplacePriorDecision`, `TestTopicSupersedesOldDecision`, `TestValidatedDecisionIsNeverSilentlyOverwritten` | implemented |
| FR-074 | Doctor is idempotent, lock-protected, retries persisted partial mutations, supports dry-run, retains bounded summaries, and raises stateless hygiene flags | WP12 | Idempotence/dry-run/recovery/flag tests | `TestDoctorPromotesCleanProposalsAndIsIdempotent`, `TestDryRunWritesNothing`, `TestInterruptedPromotionIsRecovered`, `TestStaleLockIsBroken`, `TestSummaryRetentionKeepsNewestThree` | implemented |
| FR-075 | The session briefing is bounded (≤ 9,000 bytes), scope-safe (matched project + global only), data-framed, single-line-sanitized, and never injects unsafe live notes | WP12 | Briefing tests | `TestBriefingCarriesDecisionsAndOpenWorkForItsProjectOnly`, `TestBriefingNeverInjectsUnsafeLiveNotesAndFlattensText`, `TestBriefingIsBounded` | implemented |
| FR-076 | SessionStart injection works under a real harness and is fail-open; a clean session recalls decisions and open work | WP12 | Hook-contract tests + owner-run live recall | `TestBrainSessionContinuityEndToEnd` verifies the real binary and synthetic hook JSON; installed-harness execution, model-level recall and repeated real-task benefit are not_run for this candidate | partial |
| FR-077 | Project resolution: longest real-path root, worktree via main checkout, ambiguity fails closed; registration is separate from the V1 workspace registry | WP12 | Resolution tests | `TestResolveProject`, `TestBrainRealWorktreeIdentity`, `TestOneWayWorktreeMarkerDoesNotGrantProjectScope`, `TestReliabilityFakeWorktreeMustNotAcquireProjectMemory`, `TestProjectRegistrationValidation` | implemented |
| FR-078 | The vault follows the deployment-root isolation rule (ADR-026) | WP12 | Isolation e2e | `TestBrainRespectsDeploymentIsolation` | implemented |
| FR-079 | Every Doctor run can be recorded as one git commit in the vault (reviewable diff); refuses non-repository vaults; no empty commits | WP12 | Git e2e | `TestBrainDoctorCommit` | implemented |
| FR-080 | Append-only, greppable brain log records captures, promotions (with hash), retirements, snapshots, restores, freezes; a no-op run appends nothing; never injected | WP12 | Log tests | `TestLogIsAppendOnlyAndGreppable` | implemented |
| FR-081 | Doctor snapshots once before its first mutation; snapshots are checksummed, retained (10), outside the vault, never overwritten; restore is verified, traversal/link-safe, and itself preceded by a snapshot | WP12 | Backup/restore tests incl. adversarial archives | `TestDoctorSnapshotsBeforeFirstMutationOnly`, `TestBackupRetention`, `TestRestoreRoundTripIsVerifiedAndReversible`, `TestRestoreRefusesTamperedOrUnverifiedArchives`, `TestRestoreRefusesTraversalAndLinks`, `TestBackupsWithinOneSecondDoNotOverwriteEachOther`, `TestListBackupsSurvivesCorruptSidecar` | implemented |
| FR-082 | `freeze` locks every writer while reads work; unknown/refused/newer schema metadata cannot authorize writes | WP12 | Freeze/schema tests | `TestFreezeLocksEveryWriter`, `TestNewerSchemaIsNeverWrittenByOlderBinary`, `TestFinalPartialNewSchemaInitMustNotWrite`, `TestFinalUnreadableSchemaCannotAuthorizeWrites`, `TestCorrectionSchemaAbsenceIsNotInvalidity`; CLI in `TestBrainOperatorLifecycleEndToEnd` | implemented |
| FR-083 | Vault guards: symlinked documents refused and flagged, private permissions, identity card screened like a note | WP12 | Guard tests | `TestSymlinksNeverEnterTheVaultOrABriefing`, `TestVaultFilesArePrivate`, `TestIdentityCardIsScreenedLikeAnyOtherInjectedText`; Unix permission bits tested, Windows ACL privacy not verified | partial |
| FR-084 | Briefing modes normal/economy/manual (config `brain_mode` or `--mode`); `briefing` pulls the full text, as a true read by default, with best-effort `--doctor` | WP12 | Mode + read-only-sandbox tests | `TestModes`; config/flag path in `TestBrainOperatorLifecycleEndToEnd`; `TestBriefingIsReadOnlyUnderARealReadOnlySandbox` (ADR-035) | implemented |
| FR-085 | Session-silence nudge distinguishes deduplicated identified sessions (512-ID window) from invocations; accepted capture resets counts or visibly marks accounting unknown; overlapping in-flight obligations cannot be erased by another reset; default reads do not account | WP12 | Nudge/degradation tests | `TestCaptureNudgeAfterSilentSessions`, `TestIdentifiedSessionAccountingIsIdempotent`, `TestConcurrentSessionInvocationsDoNotLoseUpdates`, unchanged `TestCorrectionAcceptedCaptureAccountingUnknownUntilReset`; `TestCaptureOverlapLatePublicationRemainsUnknown`, `TestCaptureOverlapThreeCompletionOrders`, `TestCaptureOverlapDelayedResetCannotEraseNewEvents`, `TestCaptureAccountingConcurrentStressPreservesPending`; observed unmodified public-CLI overlap RED→GREEN + safe recovery (HANDOFF §1); real hook JSON in `TestBrainOperatorLifecycleEndToEnd` | implemented |
| FR-086 | Lint parity: aging, broken references, identity pointers, unsafe identity, contradictions; `doctor --strict` exits 1 on any flag; `status` reports the vault | WP12 | Lint + CLI tests | `TestExtendedLint`, `TestContradictionIsNotMergedAsDuplicate`; strict/status in `TestBrainOperatorLifecycleEndToEnd` | implemented |
| FR-087 | Duplicate detection preserves all material content/rationale differences; summaries select the latest parsed capture instant | WP12 | Semantics tests | `TestContradictionIsNotMergedAsDuplicate`, `TestNewerSummaryIsNeverDroppedAsDuplicate` | implemented |
| FR-088 | `install-hooks` merges idempotently into a Claude Code settings file preserving foreign settings and shapes (`.beme-bak`, `--remove`, `--dry-run`, refuses a go-run binary); a byte-exact instruction block serves hook-less harnesses; PreCompact hook | WP12 | Settings/adapter tests | `TestInstallSettingsHooksPreservesEverythingElse`, `TestInstallSettingsHooksEdgeCases`, `TestStripBrainHooksLeavesUnknownShapesAlone`, `TestInstallHooksRefusesTemporaryBinary`, `TestBrainAdapterBlockCoexistsWithV1BlockAndIsByteExact` | implemented |
| FR-089 | Doctor + briefing has a synthetic latency regression target over 300 notes; production hook latency is measured separately | WP12 | Latency regression | `TestSessionStartLatencyAtScale`, `TestAuditRefsDoesNotParseTheWholeArchive`; no new production latency claim | implemented |


## Non-functional requirements

| ID | Requirement (summary) | Verification method | Evidence | Status |
|---|---|---|---|---|
| NFR-001 Reliability | No completion claim without required evidence; incomplete runs cannot mark state complete | Process gate: evidence recorded in HANDOFF | `docs/HANDOFF.md`, `docs/RELEASE_VERIFICATION.md`, release workflow/helper regressions and checked native-command failure controls (ADR-041); `scripts/test_docs_consistency.py` covers tracked-public guide sharing, continuation, temporary roots and actual managed-adapter targets/effects alongside the independent fresh-reader gate | active |
| NFR-002 Determinism | Identical source revisions, capability, request, policy, index version → structurally stable packs | Pack snapshot stability tests (WP5) | `TestDeterministicPacks`| implemented |
| NFR-003 Privacy | Forbidden profile leakage rate zero in the defined regression corpus | Privacy corpus runner (WP10) | Shared threat registry (`privacycorpus.NewSuite`): all §19 cases plus supplementary S1–S4 executed and passing behind positive controls, none `not_run` (`TestPrivacyCorpusDeterministic`, runner `TestThreatCorpusRunner`); work-safe construction test; scan gate| implemented |
| NFR-004 Security | Authority and scope assigned by trusted config/ingestion paths, never source content or model arguments | Threat corpus 100% pass (WP10) | Injection-clamp + elevation-rejection + capability-binding tests| implemented |
| NFR-005 Inspectability | Every selected material item and exclusion class has a traceable reason | Trace assertions in resolver tests | Every Stage-A exclusion carries a reason; pack items carry selection_reason (schema-required)| implemented |
| NFR-006 Reversibility | Configuration, projection build, adapter install, candidate review, schema migration have rollback/rebuild paths | Rollback tests per area | Migrations rollback-aware; Wipe rebuild; adapter uninstall preserves; candidate review reversible| implemented |
| NFR-007 Portability | macOS, Linux, and Windows verified by the full test suite in CI; released-binary behavior inside Windows harnesses stays unverified until tested | Platform matrix | Native CI matrix and source/archive smoke; portable wire paths, directory-identity controls, occupied intent nodes and explicit accounting refusals (ADR-042); native delete-pending and permanent-ACL controls for bounded acquisition retries (ADR-043). Each final source needs its own green CI; historical/cross-build results do not satisfy that gate. Windows harness-model use remains unmeasured | implemented |
| NFR-008 Performance | Interactive resolution targets sub-second on seed corpus after warm index; correctness gates outrank; budgets set at calibration | Benchmark evidence (WP10) | `internal/benchmark` + `cmd/beme-bench` (`make bench`); `evals/benchmarks/seed-baseline.json` — warm resolution p95 far below the 1 s target at 1×, 20×, and 200× seed scale| implemented |
| NFR-009 Boundedness | File ingestion and context output enforce explicit size/time/depth limits | Bounds fixtures (threat case 29) | Ingestion Limits + `TestBoundsEnforced`; brain bounded regular-file/scoped scans, identity guards, archive limits (ADR-036)| implemented |
| NFR-010 Maintainability | Domain logic independent of CLI, MCP, storage driver, harness adapter | Dependency-direction audit | `internal/` domain packages import no CLI/MCP/SDK (grep-verified); MCP SDK isolated to `cmd/beme`| implemented |
| NFR-011 Extensibility | Source adapters and pack renderers have internal interfaces; no public plugin SDK in v1 | Interface tests | Source ingestion via descriptor; renderers derive from pack; no public SDK| implemented |
| NFR-012 Documentation | A new implementation agent can execute each work package from repository docs without prior conversation access | Fresh-agent documentation test (release-candidate gate) | `docs/HANDOFF.md` | active |
| NFR-013 Supply chain | Dependencies and CI actions pinned per project policy; licenses and update behavior documented | Dependency audit record | go.sum pins; actions pinned to major v7; licenses documented (ADR-004)| implemented |
| NFR-014 Observability | Logs and traces expose revisions, timings, decisions, failure classes without raw personal content | Traces carry stage/outcome/reasons without content; no personal-text logging paths exist | `internal/resolver` TraceStep | implemented |