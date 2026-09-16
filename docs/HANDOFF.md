# Be Me — Handoff (current execution snapshot)

**Revision:** 14 — 2026-09-16
**Position:** `v0.1.0-alpha.1` published. Continuation work is on branch
`feat/eval-runner-and-privacy-corpus`, PR
[#1](https://github.com/0merUfuk/beme/pull/1). Every safely implementable
prerequisite from the owner's 14-item continuation list is implemented and
verified: the evaluation runner (now with retrieval recall/precision); the
privacy threat corpus with all 30 §19 cases executed and passing (none
`not_run`); the physical-purge workflow and durable tombstone ledger
(ADR-027); the NFR-008 benchmark harness with a measured seed baseline;
Windows CI runtime verification with per-OS directories (ADR-028);
installed-harness integration tests at four explicit verification levels;
and the requirements/docs reconciliation with an expanded docs-consistency
gate (D1–D9).

Remaining work is owner-gated only (ACCEPTANCE §7a): live-model B4-vs-B0
behavioral evaluation (paid API runs); private-corpus retrieval measurement
(owner-run, ADR-023); pre-decision-use measurement in live harness sessions
before any `assured` label; running a physical purge on real data (RED);
non-alpha release approval.

**Rev 12 — purge reliability (owner review blockers, PR #1 not to be
merged until the owner says so):** projection purge now removes provenance
through the record's actual refs (never a convention-derived ID); purge is
idempotent and resumable after a failure at any stage (content-free journal,
failure injection at every stage); the ledger keeps only keyed HMAC
fingerprints of record identity — no content or text digests (ADR-027
revised); the privacy corpus is one shared registry
(`privacycorpus.NewSuite`) executed by both `TestPrivacyCorpusDeterministic`
and the runner `cmd/beme-threat-corpus`, with supplementary cases S1–S3;
docs checks D10–D12 and a CI runner step guard these.

**Rev 14 — end-to-end completion pass (owner mandate; PR #1 still not to be
merged, nothing tagged or published):** every open finding was first
reproduced at `6c5b7d7`, then fixed and proved by reintroducing the defect
(17 mutations, all caught). Enforcement state is integrity-verified —
`ledger/tombstones.json` and `ledger/purge.key` carry a key ID and a
generation and prove each other, an interrupted first write recovers, legacy
formats migrate, and removing both files together is documented as locally
undetectable (ADR-030). Purged observations survive no backup restore: they
are tombstoned by identity and hidden on every learning surface, erased by
build, counted by doctor. Every purge deletion is zeroized, flushed and
directory-flushed before the journal is finalized, retries complete
outstanding flushes, and hard-linked canonical files are reported instead of
zeroized. Forget, purge, build and learning writes are serialized by a
maintenance lock (`-race` clean). Evaluation arms B0–B4 are built from real
primitives with all five ablations implemented and observed-value manifests
(`cmd/beme-eval`). The benchmark validates its seed before writing and
requires `--seed`. Diagnostics keep the most severe doctor state, map error
classes to exit codes, and `beme candidate --config` no longer falls back to
the operator's real deployment. Added: `installed-binary-smoke` CI job on
three OSes and [RELEASE_VERIFICATION.md](RELEASE_VERIFICATION.md)
(procedure, rollback, draft notes — unpublished). An independent
fresh-checkout test (public docs only, isolated HOME) then failed the docs,
not the runtime: installing the CLI and registering a source were
undocumented. Both are now documented (README install section, OPERATIONS
"Registering a source" with descriptor and entry formats), the two dead
cross-references are fixed, and the CLI no longer advertises a `source`
command it does not implement or hide four commands it does. Owner-gated work and the
consolidated approval request are in [ACCEPTANCE.md](ACCEPTANCE.md) §7a.

**Rev 13 — release-blocking correctness and privacy gaps (owner review; PR
#1 still not to be merged):** every issue was first reproduced against
`597bfc4`, then fixed. Purge inspection failures (observation store, traces,
canonical paths) now abort instead of reporting done; ledger, key, and
journal writes cross a documented durability boundary before any erasure.
Every read surface — resolve, export, explain, doctor, MCP status and
expansion — applies the ledger and fails closed (ADR-027 §7), and
`beme.get_context_item` is bound to packs the same session issued (ADR-029).
The evaluation runner fails blocker units, keeps response text, and reports
unimplemented ablations `not_run`. Every threat case has a positive control;
S4 covers restored backups across all read surfaces. `**` globs with
multi-segment tails match. The benchmark refuses unsafe work dirs. Existing
ledger `.gitignore` files gain the required rules. All CodeRabbit review
comments were dispositioned in the PR description.

**Continuation state (rev 14, for whoever picks this up):** branch
`feat/eval-runner-and-privacy-corpus`. The last commit that changes product
code is `c2ae7b9`; it has green three-platform CI (run [35126605518](https://github.com/0merUfuk/beme/actions/runs/35126605518)) and
passed the fresh-checkout onboarding test (P3) with zero improvisations,
performed by an independent agent on that exact commit. Later commits are
evidence-only (documentation). PR #1 is open and unmerged by owner
instruction; every review thread is answered and resolved. Nothing is tagged
or published, and no real user data was touched. What remains is owner-gated
only — the decisions in [ACCEPTANCE.md](ACCEPTANCE.md) §7a "Consolidated
approval request"; no engineering item is open.

## 1. Status

**Current (rev 13):** PR #1 is open against `main`, unmerged at the owner's
instruction, with CI on macOS, Ubuntu, and Windows plus the private-data
scan. Nothing from this continuation is released or tagged; `main` is
unchanged until the owner merges. The alpha
release history below remains accurate.

v0.1.0-alpha shipped with a release-verification failure (public CI read a
private corpus path; both CI runs failed). Recovery is complete and
verified: public validation is repo-local (ADR-023), private evaluation is
owner-gated with explicit `not_run` semantics, a 7-check regression suite
guards the failure, and `v0.1.0-alpha.1` is published with green CI on
macOS + Ubuntu, verified artifacts, and honest platform claims.
v0.1.0-alpha's release notes carry a post-publish correction; the tag was
never moved.

Verified for `v0.1.0-alpha.1` (release commit 8cee5b9):
- CI run 34845025922: test(macos) ✓ test(ubuntu) ✓ private-data-scan ✓, zero annotations.
- Fresh download of every tarball + checksums.txt: all checksums OK.
- Exec bit survives tarball extraction (0o755); darwin-arm64/amd64 run
  natively; linux-amd64/arm64 run via Docker; Windows PE-verified only
  (runtime untested — ported, unverified).
- Tagged module install `go install …@v0.1.0-alpha.1` verified outside
  the checkout; installed binary runs; transport elevation rejected (exit 3).

**Documentation gate (blueprint §24 / ACCEPTANCE §9):** the first
fresh-agent test (2026-09-14, clean checkout @ `15bbda7`) verified
runtime/CI/separation behaviors but FAILED the documentation gate: stale
lifecycle phase claims in three docs, an undocumented validation
bootstrap, and a stale description of pre-ADR-023 validation coupling.
Remediation: one canonical status section (here, §1), all stale claims
removed, bootstrap documented in README + DEVELOPMENT, hard-coded counts
replaced with command-derived evidence, and a documentation-consistency
regression suite wired into CI so these classes cannot regress. The
clean-checkout re-test at `8896df9` (2026-09-15) returned PASS: documented
bootstrap only (python3.13 fallback per docs on a 3.9.6 system), all six
documented gates exit 0, `not_run` semantics reproduced exactly, zero
contradictions, zero improvisation, ~3 minutes total. Findings and the
verbatim stale strings are recorded in DECISIONS.md (ADR-024).

## 2. Verified evidence snapshot

Pinned evidence repositories re-verified locally at WP0 (report in the
private evidence directory). Nothing since contradicts the architecture.

## 3. Decisions made this session (full ledger: docs/DECISIONS.md)

ADR-001…021 from the blueprint, ratified with live verification. New:
- **ADR-022** — user delegated RED gates for this execution via explicit
  standing instruction (recorded verbatim; reopen on any re-assertion).
- Dependency verification baked into ADR-004 (MCP go-sdk v1.7.0, modernc
  sqlite FTS5, CGo-free, go1.25.6).
- **ADR-026** — explicit config dir is a self-contained deployment root
  (test/production data isolation).
- **ADR-027** — physical purge workflow + durable tombstone ledger.
- **ADR-028** — per-OS platform directories; Windows runtime verification
  in CI.

## 4. Work completed (engine)

- **Contracts** (`schemas/`, 8 versioned JSON Schemas; public fixture checks
  pass repo-locally via `make validate` — zero failures by gate; private-corpus validation is a separate owner-gated
  gate; elevation structurally unrepresentable in the request schema).
- **Stage-A policy** (`internal/policy`): sensitivity monotonicity,
  profile/work/task scope, lifecycle/validity, trust, revocation
  tombstones — every exclusion carries a reason.
- **Stage-B resolver** (`internal/resolver`): facets, scoring, lexicographic
  precedence (§9.2 tiers), conflict preservation (shadowed states
  visible), budget (mandatory never silently dropped → `completeness:
  incomplete` + degradation), unknowns (never invented — negative control
  tested), deterministic structural packs.
- **Storage** (`internal/storage`): SQLite WAL + FTS5 (CGo-free), separate
  store files per profile, tombstones, rebuildable (Wipe), meta digests.
- **Ingestion** (`internal/ingestion`): registered roots only, no code
  execution, hard excludes (.git/.env/keys/node_modules/credentials),
  secret scan (case-normalized), symlink containment, size/count/depth/
  timeout bounds, `**`-glob includes/excludes (gitignore semantics),
  frontmatter parsing, authority clamping (content self-assignment →
  informational; untrusted → never normative).
- **Workspace identity** (`internal/workspace`): trusted registry,
  real-path matching, child-path matching, ambiguity fails closed,
  clone-never-inherits-trust, symlink resolution.
- **App runtime** (`internal/app`): platform dirs (BEME_*_HOME overrides),
  trusted registration loading, profile builds, capability-bound sessions.
- **Work-safe boundary at construction** (tested end-to-end): the safe
  builder ingests only `safe_declassified` sources; personal content
  provably absent from the work-safe store and every pack section.
- **CLI** (`cmd/beme`): status, doctor, build, preview/resolve (human +
  JSON), forget (tombstone), adapter install/remove/verify (marker-delimited
  idempotent blocks preserving unrelated config), serve/mcp (stdio only).
- **MCP server**: exactly four tools (ADR-009 contract test), stdio only
  (ADR-014, transport rejection test), quarantined feedback writing
  observation files, work-safe status hiding private source names.
- **Adapters** (`adapters/`): common canonical bootstrap + MCP fragment;
  codex + claude-code (Tier 1) install targets; hermes + cursor (Tier 2)
  documented contracts; all `advisory` in v1 (no measured assured surface).
- **CI** (`.github/workflows/ci.yml`): macOS + Linux — contract validation,
  vet, tests, private-data scan.
- **Private-data scan** (`scripts/scan_private_data.py`): generic public
  patterns; owner inventory in git-ignored local terms file; exit-1 gate.
- **Evaluation runner** (`internal/evalrunner`): B0–B4, ablations, repeats,
  immutable manifests, blinded packaging, explicit states, and retrieval
  recall/precision against `required_evidence_refs` (`RunConfig.Refs`).
  Proven with deterministic mocks (`TestRunnerFullPipelineB0ThroughB4`,
  `TestRunnerRetrievalMetrics`).
- **Privacy threat corpus** (`internal/privacycorpus`): all 30 §19 cases run
  on isolated synthetic deployments, all passing (`TestPrivacyCorpusDeterministic`).
- **Forget + physical purge** (`internal/app/purge.go`, `ledger.go`,
  `internal/storage/purge.go`, `beme purge`): ADR-027. Provenance removed via
  actual refs; idempotent and resumable (journal under `ledger/pending/`,
  `beme doctor` reports pending purges); keyed content-free ledger with the
  key in `ledger/purge.key`. Verified at raw-byte level on synthetic data,
  with failure injection at every stage; no resurrection through sync,
  backup restore, migration rollback, or corrupt-store recovery.
- **Read surfaces + expansion** (`internal/app/visibility.go`, `expand.go`):
  ADR-027 §7 and ADR-029; guarded by `TestReadSurfacesUseLedgerFilter`.
- **Durable writes** (`internal/app/durable*.go`): file + parent-directory
  flush on Unix, `MOVEFILE_WRITE_THROUGH` on Windows.
- **Threat corpus registry** (`internal/privacycorpus/cases.go`,
  `cmd/beme-threat-corpus`): one registry for the Go test and the runner;
  §19 cases plus S1–S3; runner exit 0/1/3 (`TestThreatCorpusRunner`).
- **Benchmark** (`internal/benchmark`, `cmd/beme-bench`, `make bench`):
  `evals/benchmarks/seed-baseline.json` — on the development machine
  (darwin/arm64) warm resolution p95 is well under a millisecond at 1× seed
  scale and about 10 ms at 200× (1,000 records); build about 74 ms at 200×.
- **Harness integration** (`cmd/beme/harness_integration_test.go`, opt-in):
  Claude Code 2.1.271 config lifecycle + harness connection verified; Codex
  0.154.0 config lifecycle verified; see INTEGRATIONS "Verification levels".
- **Portability** (ADR-028): per-OS directories; slash-separated ingestion
  paths; `test-windows` CI job.
- **CLI fixes:** output newlines (literal `\n` before); `forget` error
  handling.

## 5. Work NOT completed (honest gaps)

- **Live-model behavioral evaluation** (B4-vs-B0 blind paired grading):
  paid model-API runs — owner-gated. The runner, rubric, splits, and
  thresholds are ready; `evalrunner.Provider` is the single integration
  point.
- **Private-corpus retrieval measurement:** owner-run (ADR-023). Mechanism
  ready: pass a `Refs` lookup that maps record IDs to the refs the private
  gold uses.
- **Pre-decision use / `assured` surfaces:** needs live model sessions in
  each harness. No surface is `assured`.
- **Codex harness connection:** Codex has no MCP health check without a model
  session (`codex exec`); config lifecycle is verified, connection is not.
- **Hermes and Cursor:** documented contracts only; installed-harness tests
  were not run (Cursor is not installed).
- **Physical purge on real data:** the mechanism is done; executing it is a
  RED owner action. Git history rewriting stays out of scope (ADR-027).
- **Purge key custody:** `ledger/purge.key` sits beside the ledger (git-ignored,
  0600); OS-keychain storage is deferred. Losing it while purges exist fails
  resolution closed until restored.
- **Windows released-binary use inside harnesses:** the test suite runs on
  Windows CI; a Windows harness session has not been exercised.
- **Release:** nothing from PR #1 is released; a non-alpha release needs
  explicit owner approval.


## 6. Do not redo

- Do not re-verify ADR-004 dependencies without new contrary evidence.
- Do not reopen ratified ADRs without live contradicting evidence per
  their reopen conditions.
- Do not rebuild the private corpus — 34 traceable approved cases exist;
  extend, never replace.
- Do not add embeddings (ADR-012) without locked-eval recall evidence.
- Do not publish releases/tags without the owner (ADR-022 covers this
  repo's initial publication; future releases with private-data deltas
  re-run the scan first).
- Do not move tombstones back into projection stores only — the durable
  ledger is what makes forget/purge survive restore and rebuild (ADR-027).
- Do not put content or text digests, plain IDs, or timestamps into purge
  ledger entries, and do not derive provenance IDs by convention (ADR-027
  revision; D12 and the purge tests guard both).
- Do not reintroduce OS-separated ingestion paths; globs and locators are
  slash-separated everywhere (ADR-028).


## 7. Known drift/risks

- Harness hook mechanics are version-sensitive; verify against installed
  versions before labeling any surface `assured` (§13.8 list).
- The private regression pack must never be committed or referenced by
  public CI; the scan gate enforces the repository side.
- Learned observations are opt-in and non-normative; promotion is
  user-owned (ADR-010); the batch-review CLI is implemented.

## 8. Exact next step

1. **Owner:** review and merge PR #1 once its CI (macOS, Ubuntu, Windows,
   private-data scan) is green.
2. **Owner:** run the live behavioral evaluation (B0–B4 + ablations) against
   the private corpus — wire a paid provider into `evalrunner.Provider`;
   protocol in `evals/EVALUATION_CONTRACT.md` §4–§6.
3. **Owner:** measure retrieval recall/precision on the private corpus with
   `evalrunner.Run(..., RunConfig{Refs: ...})` and record the evidence bundle
   against ACCEPTANCE §5.
4. **Owner:** measure pre-decision use in live Claude Code/Codex sessions
   before labeling any surface `assured`; verify the Codex harness connection
   in the same session.
5. **Then:** dogfood (`beme build`, `beme preview` in a real registered
   workspace); extend the corpus in thin categories.


## 9. Acceptance evidence (this session)

| Check | Result |
|---|---|
| Contract fixtures vs schemas (`make validate`) | exit 0, all public checks pass (repo-local); separation regression suite green |
| Private eval corpus (owner-run `make validate-private`) | exit 0 with `BEME_PRIVATE_EVAL_DIR` set, all cases valid; `not_run`/exit 3 when absent — never in public CI |
| Elevation fixtures rejected by request schema | 2/2 rejected (P1 cases) |
| `go build ./...` | OK (macOS darwin/arm64) |
| `go vet ./...` | OK |
| `go test ./...` | all packages pass, exit 0 (inventory via `go test -list '.*' ./...`; zero failures by gate) |
| Work-safe boundary end-to-end (construction + pack + store) | PASS — personal content provably absent |
| Injection clamp (self-assigned authority → informational) | PASS |
| Secret scan + symlink containment + bounds | PASS (ingestion tests) |
| MCP tool surface contract | PASS — exactly 4 tools |
| stdio-only transport rejection | PASS |
| Adapter install idempotency + unrelated-config preservation | PASS |
| Private-data scan (public patterns + local inventory) | clean |
| Deterministic pack structure (same inputs → identical structure) | PASS |
| Mandatory-content budget protection (FR-035) | PASS |
| Unknown-preference negative control (never invented) | PASS |
| Clean history for publication | Single squashed root commit from audited tree (no private-term history) |
| Documentation consistency at the 2026-09-15 fresh-agent re-test (historical; the gate had D1–D6 then) | all six checks passed, exit 0 (CI-gated) |
| Released module install | `go install github.com/0merUfuk/beme/cmd/beme@v0.1.0-alpha.1` → exit 0; installed binary runs (`status --json` OK; transport elevation rejected, exit 3) |
| Privacy threat corpus (`TestPrivacyCorpusDeterministic`) | all 30 §19 cases executed and passing; none `not_run` |
| Physical purge on synthetic data (`TestPhysicalPurgeErasesAndBlocksResurrection`) | purged text absent from every file under data, cache, and canonical root; no resurrection via sync, re-key, restore, rollback |
| `beme purge` / `beme forget` CLI on a synthetic deployment | unconfirmed → exit 3; unknown key → exit 4; dry run changes nothing; JSON report carries no content; ledger holds fingerprints only |
| Benchmark (`make bench`) | report written; every scale within the NFR-008 target |
| Retrieval metrics (`TestRunnerRetrievalMetrics`) | per-case + aggregate recall/precision; `not_run` without refs |
| Installed harnesses (`BEME_HARNESS_INTEGRATION=1`) | Claude Code: config lifecycle + connection verified; Codex: config lifecycle verified |
| Windows (`test-windows` CI job) | build, vet, full test suite |
| Purge provenance (`TestPurgeRecordsUsesPayloadProvenanceRefs`, `TestPhysicalPurgeRemovesEveryProvenanceRef`) | refs with nonconventional IDs and mismatched source record IDs removed; convention-looking ID owned by another record kept |
| Purge resumability (`TestPhysicalPurgeResumesAfterFailureAtEveryStage`, `TestPhysicalPurgeIsIdempotent`) | failure injected at every stage (mid-stage for traces/observations); second run completes all cleanup; third run `already_purged` |
| Ledger minimality (`TestPurgeLedgerIsKeyedAndContentFree`, `TestMissingPurgeKeyFailsClosed`) | no IDs, content digests, or timestamps; foreign key matches nothing; missing key fails closed |
| Threat corpus runner (`go run ./cmd/beme-threat-corpus --repo .`, CI step on all three OSes) | every case passed; exit 3 without a checkout (case 19 `not_run`) |
| Purge inspection failures (`TestPurgePlanningFailsOnCorruptObservation`, `TestPurgeResumeWithObservationStorageFailures`, `TestPurgeFailsOnUnreadableTracesAndCanonicalPaths`) | planning and resume abort over uninspectable storage; no step reported done; purge completes once storage is readable |
| Durability boundary (`TestPurgeErasesNothingBeforeDurabilityBoundary`, `TestFlushFailuresAreReported`) | injected ledger or journal flush failure → nothing erased; completes after flushes succeed |
| Read surfaces after a restored backup (`TestRestoredBackupHiddenOnEveryReadSurface`, `TestRestoredBackupCannotResurrectOnAnySurface`, `TestUnusableLedgerFailsClosedOnEveryReadSurface`) | resolve, export, explain, doctor, MCP status/resolve/expansion hide purged and revoked records; missing key or corrupt ledger fails each closed |
| Pack-bound expansion (`TestExpandItemIsPackBound`) | eligible-but-unselected, unknown, replayed, expired, rebuilt, revoked, nonexistent → one refusal |
| Evaluation runner (`TestRunnerBlockerFailsUnitAndPreservesText`, `TestSummaryExitCodeContract`) | blocker fails unit and exit; text in results, raw and blinded artifacts |
| Glob tails (`TestMatchGlobTable`, `TestWalkIncludeExcludeNestedTails`) | direct and nested `a/**/b/*.md` include and exclude |
| Benchmark guard (`TestBenchmarkRefusesUnsafeWorkDir`, `TestBenchCleansUpOnEveryExit`) | unsafe work dirs rejected, no user data deleted, temp dir removed on every exit |
| Mutation evidence | each blocker's defect reintroduced in a scratch copy; its tests fail (recorded in the PR description) |
| Documentation consistency (D1–D15) | `python3 scripts/test_docs_consistency.py` → all checks pass |


## 10. Reply/ownership venue

This handoff is the recovery entrypoint. RED gates return to the owner
(from this point on): live-model evaluation, any privacy-scope widening,
declassification approvals, release tagging beyond this initial publication.
Contract-phase technical decisions remain agent-owned within ratified ADRs.