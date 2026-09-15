# Be Me — Requirements Traceability Matrix

> One row per requirement ID, expanded from the blueprint's range-level seed.
> No requirement is "implemented" because a row exists; evidence links are the
> production-code entry criteria. Update the Evidence column as work advances.

**Legend:** Status — `defined` (contract only) · `planned` (design exists, no
code) · `partial` · `implemented` (code + tests) · `verified` (gate evidence).

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
| FR-015 | Denied records invisible across resolve/expand/explain/error/metadata | WP5/WP7 | Denial-invisibility regression (threat cases 6–8, 26) | Policy exclusions happen before retrieval; get_context_item re-checks Stage-A; not-found and denied indistinguishable (explain/expand paths)| implemented |
| FR-016 | Trusted project policy requires out-of-band registration, allowed paths, approved revision/digest, scope, authority ceiling | WP4/WP5 | Project-policy registration test; changed-revision pending test (threat case 22) | `schemas/policy/workspace.schema.json` bound_project_policy requires out-of-band approved_digest; registration is the trust act| implemented |

## Sources and indexing

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-020 | Only registered roots ingested | WP4 | Unregistered-root ingestion test | `internal/app` loads only `<config>/sources/`; walker walks registered roots only| implemented |
| FR-021 | Ingestion never executes repository code/hooks/packages/macros/generated commands | WP4 | No-exec ingestion test (threat case 16) | Ingestion is pure file reading; zero `os/exec` imports in `internal/` (grep-verified)| implemented |
| FR-022 | Real-path and symlink checks prevent escape from registered roots | WP4 | Symlink-escape fixtures | `TestSymlinkEscapeSkipped` + real-path containment in walker| implemented |
| FR-023 | Secret/never-ingest paths and patterns excluded before indexing | WP4 | Secret-pattern fixtures (threat case 17) | `TestSecretScanBlocksCredentials` + `TestHardExcludesNeverIngested`| implemented |
| FR-024 | Every derived item retains revision, locator, hash, transformation, trust ceiling, sensitivity | WP4 | Provenance round-trip test | Provenance records locator/revision/hash/transformation; `TestPutAndFetchRecords`| implemented |
| FR-025 | Missing/moved/revoked/changed sources produce explicit stale/unavailable state | WP4 | Stale-state fixtures | doctor reports missing sources/roots as degraded with remediation; empty projection → stale_projection degradation (`Serve` + `doctor`)| implemented |
| FR-026 | Source deletion/revocation invalidates derived indexes, caches, observations, authorized expansion refs | WP4/WP9 | Revoke lifecycle test (threat cases 10, 18, 24) | `Store.Tombstone`/`RevokedSet` + `TestRevocationTombstone` + policy revocation check| implemented |

## Resolver and pack

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-030 | Policy/scope eligibility runs before retrieval and ranking | WP5 | Two-stage ordering test; eligibility-first trace assertions | `internal/policy` Stage-A runs before retrieval in `resolver.Resolve`; ADR-007| implemented |
| FR-031 | Precedence is lexicographic by tier, not a single relevance score | WP5 | Precedence fixtures; deterministic tie-break tests | `resolvePrecedence` lexicographic tiers + stable ID tie-break; `TestProjectPolicyOutranksPersonalPreference`| implemented |
| FR-032 | Resolver supports multiple task facets | WP5 | Multi-facet fixtures | `classifyFacets` multi-facet; facets echoed in pack| implemented |
| FR-033 | Resolver preserves resolved/shadowed/unresolved/stale-suspected/scope-separated conflict states | WP5 | Conflict-state fixtures | shadowed conflicts preserved in pack (`TestProjectPolicyOutranksPersonalPreference`); states enumerated in schema| implemented |
| FR-034 | Resolver reports task-material unknowns and source/retrieval incompleteness | WP5 | Unknowns fixtures; completeness states | `deriveUnknowns` + `TestUnknownsNeverInvented` negative control| implemented |
| FR-035 | Mandatory constraints/conflicts/unknowns/degradations never silently removed by budgeting | WP5 | Budget-truncation regression (threat case 14) | `TestMandatoryContentNotBudgetDropped` → incomplete + degradation, never silent omission| implemented |
| FR-036 | ContextPack is versioned JSON with deterministic ordering and trace digest | WP5 | Pack snapshot stability tests | `TestDeterministicPacks` structural identity; pack schema versioned with trace_ref| implemented |
| FR-037 | Harness renderers derive from the same canonical pack | WP5/WP8 | Renderer derivation tests | CLI human render + JSON come from one pack struct; no separate renderer state exists| implemented |
| FR-038 | Explain output generated from the actual resolver trace | WP6 | Trace-explain equivalence test | `beme explain --trace` reads the persisted actual trace (FR-038 E2E verified)| implemented |
| FR-039 | Work-safe provenance is a safe scoped view; omits private identifiers, locators, paths, repo names, denied relationship metadata, correlatable hashes | WP5 | Safe-provenance regression (threat case 25) | work-safe provenance omits source IDs/revisions/hashes (`provenanceManifest`); task_summary echoes requester text only| implemented |

## Interfaces

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-040 | MCP exposes only scoped resolve, authorized expand, safe status, quarantined feedback | WP7 | Tool-surface enumeration test | `TestToolSurfaceContract`: exactly 4 tools (cmd/beme)| implemented |
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
| FR-055 | Canonical changes transactional, auditable, reversible; physical purge is distinct, RED, irreversible, tombstone-only | WP9 | Transaction/undo tests; purge workflow documentation | Observation writes are atomic (tmp+rename) + full review outcome audit trail; physical purge remains RED (documented) | partial (physical-purge workflow owner-gated by design) |

## Portability and operation

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-060 | Core functionality works without network access | WP4/WP6/WP10 | Offline build + resolve in network-disabled environment | Zero network imports in `internal/` (grep-verified); stdio-only MCP| implemented |
| FR-061 | V1 exposes MCP over local stdio only; network serving deferred pending ratified threat model | WP7 | Transport enumeration test; no listener in binary surface | `TestStdioOnlyTransport` rejects non-stdio; no listener in binary| implemented |
| FR-062 | Config, durable data, cache, runtime state use platform-appropriate directories and remain separable | WP4 | Platform-dir tests (macOS verified first) | `app.DefaultDirs()` platform dirs + BEME_*_HOME overrides; clean-machine test verified| implemented |
| FR-063 | Canonical profile/source path configurable; no hard-coded `~/.beme` | WP4 | Config-resolution tests | Config root from env/config only; no `~/.beme` constant in code| implemented |
| FR-064 | Schema and index migrations versioned, atomic, rollback-aware, tested | WP4 | Migration fixtures | `migrate.go` + `TestMigrationsFreshAndIdempotent`/`RollbackAndReapply`/`FailedMigrationLeavesPriorIntact`| implemented |
| FR-065 | Corrupt operational state recoverable through rebuild without changing canonical sources | WP4/WP10 | Corruption-recovery test | `Wipe()` rebuild; doctor flags rebuild-needed; rebuild never touches sources| implemented |

## Non-functional requirements

| ID | Requirement (summary) | Verification method | Evidence | Status |
|---|---|---|---|---|
| NFR-001 Reliability | No completion claim without required evidence; incomplete runs cannot mark state complete | Process gate: evidence recorded in HANDOFF | `docs/HANDOFF.md` | active |
| NFR-002 Determinism | Identical source revisions, capability, request, policy, index version → structurally stable packs | Pack snapshot stability tests (WP5) | `TestDeterministicPacks`| implemented |
| NFR-003 Privacy | Forbidden profile leakage rate zero in the defined regression corpus | Privacy corpus runner (WP10) | Boundary tests zero-leak (work-safe construction test, scan gate); full regression corpus pending eval harness| partial |
| NFR-004 Security | Authority and scope assigned by trusted config/ingestion paths, never source content or model arguments | Threat corpus 100% pass (WP10) | Injection-clamp + elevation-rejection + capability-binding tests| implemented |
| NFR-005 Inspectability | Every selected material item and exclusion class has a traceable reason | Trace assertions in resolver tests | Every Stage-A exclusion carries a reason; pack items carry selection_reason (schema-required)| implemented |
| NFR-006 Reversibility | Configuration, projection build, adapter install, candidate review, schema migration have rollback/rebuild paths | Rollback tests per area | Migrations rollback-aware; Wipe rebuild; adapter uninstall preserves; candidate review reversible| implemented |
| NFR-007 Portability | macOS verified first; Linux/Windows claims remain ported/unverified until independently tested | Platform matrix | macOS verified; Ubuntu CI-verified; Windows ported-unverified (release notes matrix)| partial |
| NFR-008 Performance | Interactive resolution targets sub-second on seed corpus after warm index; correctness gates outrank; budgets set at calibration | Benchmark evidence (WP10) | Not yet measured; correctness gates outrank (documented)| partial |
| NFR-009 Boundedness | File ingestion and context output enforce explicit size/time/depth limits | Bounds fixtures (threat case 29) | Ingestion Limits + `TestBoundsEnforced`| implemented |
| NFR-010 Maintainability | Domain logic independent of CLI, MCP, storage driver, harness adapter | Dependency-direction audit | `internal/` domain packages import no CLI/MCP/SDK (grep-verified); MCP SDK isolated to `cmd/beme`| implemented |
| NFR-011 Extensibility | Source adapters and pack renderers have internal interfaces; no public plugin SDK in v1 | Interface tests | Source ingestion via descriptor; renderers derive from pack; no public SDK| implemented |
| NFR-012 Documentation | A new implementation agent can execute each work package from repository docs without prior conversation access | Fresh-agent documentation test (release-candidate gate) | `docs/HANDOFF.md` | active |
| NFR-013 Supply chain | Dependencies and CI actions pinned per project policy; licenses and update behavior documented | Dependency audit record | go.sum pins; actions pinned to major v7; licenses documented (ADR-004)| implemented |
| NFR-014 Observability | Logs and traces expose revisions, timings, decisions, failure classes without raw personal content | Traces carry stage/outcome/reasons without content; no personal-text logging paths exist | `internal/resolver` TraceStep | implemented |