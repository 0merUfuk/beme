# Be Me — Requirements Traceability Matrix

> One row per requirement ID, expanded from the blueprint's range-level seed.
> No requirement is "implemented" because a row exists; evidence links are the
> production-code entry criteria. Update the Evidence column as work advances.

**Legend:** Status — `defined` (contract only) · `planned` (design exists, no
code) · `partial` · `implemented` (code + tests) · `verified` (gate evidence).

## Core and ownership

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-001 | Core operates independently of harness packaging | WP1/WP3 | Boundary ADR; dependency-graph audit (no harness SDK imports in core packages) | `docs/DECISIONS.md` ADR-002 | defined |
| FR-002 | Register external sources without changing ownership | WP4 | Source adapter contract tests | `docs/ARCHITECTURE.md` §adapters | defined |
| FR-003 | Optional episodic-evidence adapter; no duplicate transcript ingestion | WP4 | Adapter contract test + boundary ADR review | ADR-019 in `docs/DECISIONS.md` | defined |
| FR-004 | Derived indexes/records rebuildable from authorized sources | WP4 | Rebuild test: delete index, rebuild, compare digest | — | defined |
| FR-005 | Public engine code contains no user-specific profile data or private fixtures | WP10/WP11 | Private-data scan of repo + artifacts; public-fixture provenance audit | `scripts/` scan tooling (planned) | defined |

## Profiles and policy

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-010 | Each serving process binds one immutable capability ceiling | WP5 | Capability binding test; server startup refuses unbound | ADR-005 | defined |
| FR-011 | Requests may narrow, never widen capability | WP5 | Elevation-attempt regression (threat cases 1, 21) | `evals/` invariants | defined |
| FR-012 | Work-safe serving/build uses only safe projection + approved manifest | WP4 | Safe-builder path test: work-safe build succeeds with personal roots absent | ADR-018 | defined |
| FR-013 | Repository-local config only narrows | WP5 | Repo-config widening regression test | — | defined |
| FR-014 | Ambiguous workspace identity disables project/personal resolution | WP5 | Ambiguity fixture; fail-closed test | — | defined |
| FR-015 | Denied records invisible across resolve/expand/explain/error/metadata | WP5/WP7 | Denial-invisibility regression (threat cases 6–8, 26) | — | defined |
| FR-016 | Trusted project policy requires out-of-band registration, allowed paths, approved revision/digest, scope, authority ceiling | WP4/WP5 | Project-policy registration test; changed-revision pending test (threat case 22) | — | defined |

## Sources and indexing

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-020 | Only registered roots ingested | WP4 | Unregistered-root ingestion test | — | defined |
| FR-021 | Ingestion never executes repository code/hooks/packages/macros/generated commands | WP4 | No-exec ingestion test (threat case 16) | — | defined |
| FR-022 | Real-path and symlink checks prevent escape from registered roots | WP4 | Symlink-escape fixtures | `testdata/injection/` | defined |
| FR-023 | Secret/never-ingest paths and patterns excluded before indexing | WP4 | Secret-pattern fixtures (threat case 17) | `testdata/injection/` | defined |
| FR-024 | Every derived item retains revision, locator, hash, transformation, trust ceiling, sensitivity | WP4 | Provenance round-trip test | `schemas/record/` | defined |
| FR-025 | Missing/moved/revoked/changed sources produce explicit stale/unavailable state | WP4 | Stale-state fixtures | `testdata/synthetic/` | defined |
| FR-026 | Source deletion/revocation invalidates derived indexes, caches, observations, authorized expansion refs | WP4/WP9 | Revoke lifecycle test (threat cases 10, 18, 24) | — | defined |

## Resolver and pack

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-030 | Policy/scope eligibility runs before retrieval and ranking | WP5 | Two-stage ordering test; eligibility-first trace assertions | ADR-007 | defined |
| FR-031 | Precedence is lexicographic by tier, not a single relevance score | WP5 | Precedence fixtures; deterministic tie-break tests | `docs/ARCHITECTURE.md` §precedence | defined |
| FR-032 | Resolver supports multiple task facets | WP5 | Multi-facet fixtures | — | defined |
| FR-033 | Resolver preserves resolved/shadowed/unresolved/stale-suspected/scope-separated conflict states | WP5 | Conflict-state fixtures | — | defined |
| FR-034 | Resolver reports task-material unknowns and source/retrieval incompleteness | WP5 | Unknowns fixtures; completeness states | — | defined |
| FR-035 | Mandatory constraints/conflicts/unknowns/degradations never silently removed by budgeting | WP5 | Budget-truncation regression (threat case 14) | — | defined |
| FR-036 | ContextPack is versioned JSON with deterministic ordering and trace digest | WP5 | Pack snapshot stability tests | `schemas/context-pack/` | defined |
| FR-037 | Harness renderers derive from the same canonical pack | WP5/WP8 | Renderer derivation tests | — | defined |
| FR-038 | Explain output generated from the actual resolver trace | WP6 | Trace-explain equivalence test | — | defined |
| FR-039 | Work-safe provenance is a safe scoped view; omits private identifiers, locators, paths, repo names, denied relationship metadata, correlatable hashes | WP5 | Safe-provenance regression (threat case 25) | — | defined |

## Interfaces

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-040 | MCP exposes only scoped resolve, authorized expand, safe status, quarantined feedback | WP7 | Tool-surface enumeration test | ADR-009 | defined |
| FR-041 | Admin operations and unrestricted evidence inspection unreachable via ordinary agent MCP capability | WP7 | Admin-reachability regression (threat case 23) | — | defined |
| FR-042 | CLI supports human + JSON output, stable exit codes, preview/explain/doctor/rebuild/source lifecycle/profile build/review/export/forget | WP6 | CLI contract tests | — | defined |
| FR-043 | Adapter install/removal idempotent; preserves unrelated user configuration | WP8 | Install/uninstall round-trip tests on synthetic harness configs | — | defined |
| FR-044 | Runtime unavailability produces documented degradation without broader-profile fallback | WP7/WP8 | Unavailability fixtures | — | defined |
| FR-045 | Surfaces labeled `assured` prove pre-decision context resolution (100% measured) | WP8 | Pre-decision context-use measurement | — | defined |
| FR-046 | Surfaces without that proof are labeled `advisory` with visible limitation | WP8 | Adapter tier/assurance matrix in docs | — | defined |
| FR-047 | Snapshot reuse only when capability, workspace, projection revision, task origin, age, stale state valid and explicit | WP8 | Snapshot-validity tests (threat case 9) | — | defined |

## Learning

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-050 | Agent feedback and inferred behavior create quarantined observations only | WP9 | Feedback-path test (threat case 11) | — | defined |
| FR-051 | Canonical promotion and authority/scope changes require trusted user action | WP9 | Promotion lifecycle test | — | defined |
| FR-052 | Evidence families deduplicated; correlated repetitions never appear independent | WP9 | Evidence-family fixtures (threat case 12) | — | defined |
| FR-053 | Rejected candidates tombstoned against equivalent re-proposal | WP9 | Tombstone regression (threat case 13) | — | defined |
| FR-054 | Work-restricted observations never become global personal knowledge without de-identification and approval | WP9 | Cross-namespace promotion regression | — | defined |
| FR-055 | Canonical changes transactional, auditable, reversible; physical purge is distinct, RED, irreversible, tombstone-only | WP9 | Transaction/undo tests; purge workflow documentation | — | defined |

## Portability and operation

| ID | Requirement (summary) | Owner WP | Verification method | Evidence | Status |
|---|---|---|---|---|---|
| FR-060 | Core functionality works without network access | WP4/WP6/WP10 | Offline build + resolve in network-disabled environment | ADR-014 | defined |
| FR-061 | V1 exposes MCP over local stdio only; network serving deferred pending ratified threat model | WP7 | Transport enumeration test; no listener in binary surface | ADR-014 | defined |
| FR-062 | Config, durable data, cache, runtime state use platform-appropriate directories and remain separable | WP4 | Platform-dir tests (macOS verified first) | — | defined |
| FR-063 | Canonical profile/source path configurable; no hard-coded `~/.beme` | WP4 | Config-resolution tests | — | defined |
| FR-064 | Schema and index migrations versioned, atomic, rollback-aware, tested | WP4 | Migration fixtures | `testdata/migrations/` | defined |
| FR-065 | Corrupt operational state recoverable through rebuild without changing canonical sources | WP4/WP10 | Corruption-recovery test | — | defined |

## Non-functional requirements

| ID | Requirement (summary) | Verification method | Evidence | Status |
|---|---|---|---|---|
| NFR-001 Reliability | No completion claim without required evidence; incomplete runs cannot mark state complete | Process gate: evidence recorded in HANDOFF | `docs/HANDOFF.md` | active |
| NFR-002 Determinism | Identical source revisions, capability, request, policy, index version → structurally stable packs | Pack snapshot stability tests (WP5) | — | defined |
| NFR-003 Privacy | Forbidden profile leakage rate zero in the defined regression corpus | Privacy corpus runner (WP10) | `evals/` | defined |
| NFR-004 Security | Authority and scope assigned by trusted config/ingestion paths, never source content or model arguments | Threat corpus 100% pass (WP10) | `evals/` | defined |
| NFR-005 Inspectability | Every selected material item and exclusion class has a traceable reason | Trace assertions in resolver tests | — | defined |
| NFR-006 Reversibility | Configuration, projection build, adapter install, candidate review, schema migration have rollback/rebuild paths | Rollback tests per area | — | defined |
| NFR-007 Portability | macOS verified first; Linux/Windows claims remain ported/unverified until independently tested | Platform matrix | — | defined |
| NFR-008 Performance | Interactive resolution targets sub-second on seed corpus after warm index; correctness gates outrank; budgets set at calibration | Benchmark evidence (WP10) | — | defined |
| NFR-009 Boundedness | File ingestion and context output enforce explicit size/time/depth limits | Bounds fixtures (threat case 29) | — | defined |
| NFR-010 Maintainability | Domain logic independent of CLI, MCP, storage driver, harness adapter | Dependency-direction audit | `internal/` layout (planned) | defined |
| NFR-011 Extensibility | Source adapters and pack renderers have internal interfaces; no public plugin SDK in v1 | Interface tests | — | defined |
| NFR-012 Documentation | A new implementation agent can execute each work package from repository docs without prior conversation access | Fresh-agent documentation test (release-candidate gate) | `docs/HANDOFF.md` | active |
| NFR-013 Supply chain | Dependencies and CI actions pinned per project policy; licenses and update behavior documented | Dependency audit record | ADR-004 | defined |
| NFR-014 Observability | Logs and traces expose revisions, timings, decisions, failure classes without raw personal content | Log-sanitization regression (threat case 7) | — | defined |