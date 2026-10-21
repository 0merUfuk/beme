# Be Me — Handoff (current execution snapshot)

**Revision:** 25 — 2026-10-12
**Position:** accepted integration committed and pushed; `v0.1.0-alpha.2`
publication preparation follows ADR-041. The separately authorized local rollout
verified installed CLI and fresh configured MCP connections, not model use.
Canonical source/evidence status is in §1. Live merge, CI and publication state
must be read from the linked GitHub objects; historical results are not evidence
for a different release commit.

**Rev 26 — 2026-10-21 (behaviour delivery, local branch `feat/always-on-and-capture-nudge`,
not pushed):** a transcript baseline showed agents received the briefing but did not
call the resolver (Claude Code) and rarely captured. ADR-044 adds MCP initialize
usage instructions and a Stop-hook capture reminder (`beme brain stop-check`).
Open: mean extra advisory records per relevant task rose from 1.00 to 3.00
between the pre-integration and integrated builds (budget change in `bbb63fe`;
relevance filter unchanged) — measure on the larger private corpus before tuning.
Effect on behaviour is unproven until the baseline is re-run on sessions started
after rollout. Nothing here is published.
Rollout (private deployment): revision `behavior-eb7a45b` is live and the Stop hook
is installed; both are verified (handshake 0 -> 743 instruction characters, Layer-1
6/6 + 6/6 paraphrase + 4/4 negative controls, settings diff limited to `hooks.Stop`).
Open findings, not fixed: (1) `beme brain status` may print all-zero live notes while
other sessions run maintenance — it should report "unavailable", not zeros;
(2) five tests are time-budget sensitive and fail identically on unmodified `a29ddda`
under heavy load (`TestAuditRefsDoesNotParseTheWholeArchive`,
`TestFinalBoundedBacklogKeepsLatestRecapture`,
`TestFinalRepeatedRecapturesPublicLifecycleReadable`, `TestSessionStartLatencyAtScale`,
`TestBrainReliabilityEndToEnd/cross-process-session-accounting`); a quiet-machine
`make validate` run was not obtained.

### Historical V1 continuation notes

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
or published, and no real user data was touched. This historical continuation
record does not establish current readiness: the 2026-10-09 independent audits
reopened deterministic engineering work before any owner-run evaluation or
release decision (see §1).

## 1. Status

**Source delivery (2026-10-11):** the accepted integration is recorded in
[`bbb63fe`](https://github.com/0merUfuk/beme/commit/bbb63feaa4a4728fd6c3a05f4db0aa4f9dca4128)
(functional source) and
[`ed7de4f`](https://github.com/0merUfuk/beme/commit/ed7de4f1e0e39203aeeb90d158600123588e6e28)
(contracts and acceptance docs), proposed in
[PR #3](https://github.com/0merUfuk/beme/pull/3). Both remote commit identities
were read back. The branch preserves all public PR #1/#2 dependencies rather
than cherry-picking their dependents onto older main.

**Publication is a separate gate:** the source version is `v0.1.0-alpha.2`.
Use the [release list](https://github.com/0merUfuk/beme/releases) and exact-head
CI to establish what is actually published. The publication review identified
POSIX-only fixtures accidentally selected on Windows, missing intermediate
native-command exit checks in Windows smoke, and an inherited private-marker
residue. Their corrections and release automation require separate verification;
the earlier source seal does not cover those new bytes. The historical explicit
merge hold must be lifted by the owner before landing the package on main.

**First native publication CI:** run
[38154574257](https://github.com/0merUfuk/beme/actions/runs/38154574257) tested
the PR at `ed7de4f`. Linux's full suite passed. macOS exposed an output-path
alias assertion; Windows exposed the executable-extension fixture, evaluator
manifest-path handling, refused intent-directory handling, identity-pointer
lint and concurrent event accounting. The old installed-smoke jobs passed but
did not establish coverage of those new paths. These failures are retained as
RED evidence; a corrected exact-head matrix must pass before release, without
skipping portable cases or weakening their invariants.

**Corrections prepared at `f8b0954` (ADR-042):** evaluator wire identifiers and
output containment now respect native filesystem identity without relaxing evidence
joins. Brain intent allocation preserves refused occupants while using other
slots; failed replacement cleans its temporary file only. Native path fixtures
and explicit pre-write accounting refusals are tested rather than hidden. All
implementation writers released their scopes and parent verified their source
hashes and inspected the changes. The fresh combined local run reports
`go test -json -count=1 -timeout=300s ./...`: 361 top-level tests, 672 passing
test/subtest events and 18 packages, with two opt-in installed-harness tests
skipped. Public validation reports `19 passed, 0 failed`; docs, synthetic
separation, release-helper checks, build, vet, format, private scan, threat corpus
and the combined brain/app/durable/learning/storage/evaluator race run exit 0.
The late release-instructions edit is documentation-only; production/test bytes
remain bound to that run. The bounded independent release-automation review also
passed its ten-file scope, including real native archive execution and adversarial
controls; parent verified the same file hashes. New exact-head native CI remains
a separate gate. The original failed run remains evidence, not a green result.

**Follow-up evidence at `f8b0954`:** the portability fixes, release automation
and version/docs preparation are pushed in `a4553b8`, `0bd2e45` and `f8b0954`.
Native run [38161550878](https://github.com/0merUfuk/beme/actions/runs/38161550878)
passed Linux/macOS full tests, all three installed-smoke jobs and the private
scan. Windows full Go still reported a lock-creation `Access is denied` error in
`TestConcurrentSessionInvocationsDoNotLoseUpdates`; a passing local run did not
close it. The fresh-reader documentation gate separately found an unsafe legacy
grader-directory instruction, a stale §8 continuation and an obsolete example
lifecycle statement. Those instructions are corrected here; the regression suite
also guards explicit temporary-directory templates after a platform-specific
scratch-containment discrepancy. A corrected source needs fresh exact-head CI
and a repeated clean-checkout documentation gate. Failed receipts stay preserved;
the live GitHub objects establish subsequent completion, not this older evidence.

**Acquisition follow-up (ADR-043):** Windows access/sharing-denied exclusive
creates now retry only within the unchanged 500 ms deadline, preserving the
original error on persistent failure. No callback/publication is retried and no
denied create grants ownership. The twelve-event assertion remains unchanged.
Parent verified the six-file delivery hashes and independently exercised host
replay plus false-success/permanent-error mutation controls. The combined parent
`go test -json -count=1 -timeout=300s ./...` run reports 364 top-level tests,
681 passing test/subtest events and 18 packages; the two installed-model tests
remain opt-in. `make validate` reports `19 passed, 0 failed`; docs consistency
reports 22/22. Build, vet, format, scan, synthetic separation, release helpers,
threat corpus and the combined six-package race run exit 0. This evidence
addition is documentation-only and preserves those production/test bytes.
Real delete-pending and DACL tests are part of the Windows suite; the corrected
combined source must still pass native CI and the repeated clean-checkout
documentation gate.

**Pre-tag target coverage:** PR CI now uses the same explicit five runner/target
pairs as the release artifact matrix, with actual source-install and archive
execution. A workflow regression binds those matrices and requires explicit
native targets. It does not replace the tag-bound release gates or the owner
merge boundary.

**Native/docs follow-up at `0d86f0c`:** run
[38167202919](https://github.com/0merUfuk/beme/actions/runs/38167202919)
passed all five native source/archive targets, Linux/macOS full suites and the
privacy scan. Windows full Go failed only the new delete-pending fixture's
precondition: its initial exclusive create returned no error. The log did not
report failures in the original twelve-event or permanent-ACL controls. The
fixture now requests classic `FileDispositionInfo` explicitly with `DELETE`
access rather than assuming `DeleteFile` retains a blocked name. Its strict
access-denial precondition and exactly-one recovery remain, with both callbacks
checked before release. Parent verified the API contract, changed-file hash,
unchanged permanent-denial controls and Windows-targeted compile/vet. This is a
Windows-test-only correction, not another production change; it still requires
native execution. The failed run remains RED evidence, not release acceptance.

The independent reader also found an unsupported managed Cursor command in a
secondary guide. Parent traced the shared target selector and replayed all four
adapter names with the exact-source binary in disposable homes. Codex/Claude
write only their instruction block and print the manual MCP example; they do
not install skills or print prompt hooks. Cursor/Hermes refuse without writing.
All adapter guides now distinguish these effects from manual contracts, and
the docs gate scans the complete Git-tracked Markdown inventory, including
adapters/templates. This corrects instructions, not adds another installer.
Native fixture acceptance and corrected-revision fresh-reader acceptance remain
required before publication.

**Native acceptance at `d62b30d`:** run
[38170961639](https://github.com/0merUfuk/beme/actions/runs/38170961639)
completed all nine jobs successfully: full macOS/Linux/Windows suites, all five
native source/archive targets and the privacy scan. This includes the strict
Windows fixture correction; earlier failed runs remain unchanged evidence.
The independent fresh reader passed every executed source/bootstrap/install/
synthetic/archive path and accepted the adapter corrections. Its remaining HOLD
was the evaluation contract's unqualified original WP2B approval wording.
That design/history text now references the completed original freeze and this
current status, without accessing or changing any corpus, threshold or gold.
The lifecycle regression was RED first and is now covered by the docs gate.
The independent corrected-source documentation gate is **PASS at `99295c8`**:
the prose, changed links and regression were independently checked in a fresh
public checkout. Unchanged earlier runtime executions retain their original
source identity; they are not relabeled as new-head executions.

**Concurrent-read test correction:** the next exact-head run
[38173348267](https://github.com/0merUfuk/beme/actions/runs/38173348267)
passed eight jobs, including Windows, but exposed a macOS stress-test assumption.
A legitimate `document changed during read` refusal caused the test worker to
exit early, then label its unattempted captures as lost notes. A controlled
native reproduction preserved every actually accepted note. The test-only
correction keeps all eight workers/eight captures, public error-or-unknown
behavior, error-free quiescent reads, read immutability, exactly 65 unique
unchanged notes and safe recovery; arbitrary errors still fail. Parent replayed
the original RED and corrected GREEN, public/write-caller fault controls and
repeated race checks. Production, Windows fixtures and the original twelve-event
control remain byte-identical. Parent full local gates passed: 365 top-level
tests / 685 passing test events across 18 packages, six-package race checks,
public validation (19 passed, 0 failed), docs (25/25), separation, release-helper,
build, vet, formatting, privacy scan and threat corpus. Only the two opt-in live
harness integration tests were skipped. The correction still requires its own
exact-head native CI. Documentation acceptance does not lift the owner merge
hold or claim a published release.

**Local installation:** the earlier accepted snapshot was installed through
existing entrypoints with rollback copies. Installed CLI, a synthetic
capture/Doctor/briefing and fork-hook fixture, a read-only real briefing, and
fresh connections through both configured MCP routes passed. Existing real
note bytes and metadata were preserved. Existing AI sessions were not killed;
old loaded servers require a session restart. Private receipts remain outside
this repository. These checks do not prove native model consumption or task
benefit (`not_run`).

**Engineering baseline (2026-10-09): parent independent review and local
acceptance complete.** The full Go suite and public static gates were rerun
successfully before publication preparation on 2026-10-11. The detailed
cycle-2 evidence below remains bound to its original accepted snapshot.
The bounded final review and parent both reproduced one remaining BFR-006
overlap on the first corrective seal: A paused after intent, B completed, two
starts were recorded, then A published but failed its reset. Both notes survived,
yet the shared marker hid A's uncertainty and allowed a false silence nudge.
Only accounting, its focused tests and these truth documents change in cycle 2;
BFR-001..005, E1/E2, runtime/RF01/RF02 and existing tests remain byte-preserved.

ADR-039's cycle-2 amendment records bounded independent publication obligations,
completed-only reconciliation and delayed-reset/slot-reuse handling. Pending or
interrupted intents are never acknowledged merely because another capture
finished. Default reads remain write-free and fail unknown on inconsistent
state/intent snapshots. Capture remains independent of Doctor's lock. Full
intent capacity refuses before publication; interrupted/legacy-mismatched
bookkeeping requires quiescent inspection, not fabricated precision.

Fresh cycle-2 evidence (local macOS arm64; fresh synthetic HOME/TMPDIR/BEME
roots, offline caches, host Git configuration and signing disabled):

| Gate | Observed cycle-2 result |
|---|---|
| Original unmodified public-CLI overlapping-capture reproduction | RED then GREEN; both runs observed the real pause-before-publication boundary on the first attempt; A exit 0 + warning, both accepted notes survive; GREEN reports unknown and suppresses the false nudge |
| Public-CLI recovery and read-only control | repeated briefing/status preserve bytes and metadata; a later accepted capture reconciles completed obligations, restores known zero counters, and all three notes survive |
| New deterministic adjacent accounting tests | 8 top-level tests / 20 passing events: all six three-capture completion orders, delayed reset and slot reuse, per-attempt collision cancellation, publication failure, capacity/interruption/refused-file bounds, and concurrent stress |
| Repeated scheduling/concurrency race run | 20 repetitions; 200 passing events / 80 top-level executions, no race report |
| `go test -json -count=1 -timeout=300s ./...` | exit 0; 542 passing test/subtest events, 343 top-level tests, 18 passing packages; 2 installed Claude Code/Codex tests intentionally skip |
| `go test -race -count=1 -timeout=300s ./internal/brain ./cmd/beme` | exit 0; no race report; includes unchanged sequential accepted/unknown/reset regression |
| Original BFR tests and CLI | original Go assertions replayed unchanged as replacement overlay: 15 top-level / 39 passing events; existing quoted-command property adaptation retained, CLI 11/11 |
| Evaluator/runtime no-drift smoke | unchanged 37-command built-binary acceptance passes; all 14 retained E1 invalid operations reject normally; positive/mixed/regression controls preserve denominator 20 and benefit `not_run` |
| Public/static gates | `make validate`: 19 passed/0 failed; docs 19/19, CI separation 7/7; build, vet, fmt, diff check and private scan exit 0 |
| Linux/Windows cross-builds | exit 0; not native execution evidence |

Exact new source hashes, the narrow delta versus the frozen 232-file baseline,
full uncommitted patch, command logs and repeated `-trimpath` binary comparisons
are in the external cycle-2 receipt. The concurrency probe changes only the
marker observation path; no product pause hook or test bypass was added. Its
missed-pause exit cannot count as success. The first adjacent-control run had a
fixture-only entry-count error (`inbox/rejected/` is not a proposal); the failed
receipt is retained and corrected checks count actual note files.

**Parent acceptance:** all 233 source files and all 244 packaged file entries
were hash-verified against the delivered cycle-2 seal. Parent reviewed the two
changed production files and focused scheduling tests, then independently reran
the full suite (542 passing events / 343 top-level tests / 18 packages), original
15 Go and 11 CLI regressions, all 14 malformed-evidence rejections and original
positive/mixed/regression controls. The public-CLI concurrency probe hit its real
pause boundary, preserved both accepted notes, marked accounting unknown and
suppressed the false nudge; subsequent safe recovery preserved all three notes.
Read-only calls preserved bytes and metadata. The 20-repeat concurrency race run
also passed. `make validate` returned `19 passed, 0 failed`; docs, CI separation,
private scan, vet and fmt passed. Final closure edits are documentation-only and
are separately sealed; product code/tests and exercised executable bytes remain
unchanged. The final external receipt binds these identities and reused evidence.

**Not claimed / not_run:** native model consumption or native harness/fork delivery,
live/private-corpus/model benefit, power-loss
transactionality, hostile same-user isolation, mixed-version concurrent writers,
retroactive reconstruction of lost legacy obligations, or native Linux/Windows
runtime acceptance from the historical local suite alone. The original local
engineering pass performed no publication or deployment; the later independently
authorized installation and source publication are recorded above. The bounded
deterministic review closed its tracked defects, not every future portability
or release concern. Final release CI, artifact verification and real effectiveness
measurement remain separate evidence levels.

### Historical first corrective pass (superseded by cycle 2 above)

**First corrective candidate (2026-10-09; final review reopened BFR-006 overlap).**
The first integrated seal was independently rejected on six adjacent brain
findings (BFR-001..006) and evaluator E1/E2. ADR-039–040 record that first bounded
corrective pass. Earlier green counts below are preserved as historical evidence,
not retroactive acceptance. Runtime/ingestion/RF01/RF02, platform adaptations and
config-only APIs remain intact; there was no product Git write, deployment,
settings/vault change, credential access, network/model call or release.

Fresh synthetic results on the corrected source (local macOS arm64, isolated
HOME/TMPDIR/config, offline Go modules and disabled host Git config/signing):

| Gate | Observed corrective-pass result |
|---|---|
| `go test -count=1 -json -timeout=240s ./...` | exit 0; 522 passing test/subtest events, 335 top-level tests, 18 passing packages; 2 installed Claude Code/Codex tests intentionally skip |
| Targeted races across brain/app/ingestion/projection/resolver/evalrunner and both CLIs | exit 0; no race report, including the real 800-recapture lifecycle |
| Original BFR Go and generated-command regressions | original core failures now pass; CLI 11/11 after replacing a raw quoted-path substring check with exact shell-decoded executable identity; the original false-negative log is retained |
| BFR adjacent controls | delayed/paginated recapture and retirement retry, all global warning paths, malformed/absent schema, 128 unresolved retry capacity, old receipt replay, writer bounds and accepted/unknown accounting pass |
| Original E1 bundles at template and import | all 14 invalid-evidence assertions reject normally; bootstrap zero no longer panics; original positive/mixed/regression controls retain 20 pairs (mixed: 6 wins/14 losses) |
| Evaluator malformed-bundle matrix and configured-arm controls | 24 template mutation cases pass; configured repeat overrides and all ten arms reconstruct; original anchored-grade rejection and independent positive/safety/nonregression controls pass |
| E2 actual shared surface | standalone shuffled reviewer packet, no per-sample filesystem metadata; metadata-only predictions written before labels: 16/40 correct versus 40/40 on the old owner directory; packet-based grades remain hash-bound |
| Combined built CLI acceptance | 37 checked commands pass, including pending/import/high-risk/quality/blocker controls; original generation bundle unchanged; benefit `not_run` |
| Build, vet, fmt, public validation, CI separation and private scan | exit 0; `make validate`: 19 passed/0 failed; CI separation 7/7; clean scan |
| Original parent audit overlay, privacy corpus and benchmark | pass; threat corpus 34/34; seed warm-p95 within target at all scales; benchmark output external only |
| Linux/Windows cross-builds | exit 0, not native runtime evidence |

The external correction receipt binds the exact delta from the rejected seal,
complete source inventory, `-trimpath` binary hashes and command logs. No private
paths or raw receipts belong in this repository. A separately retained bundle
index remains necessary: semantic consistency is not malicious-owner-proof
integrity. The owner directory (including `blinded/`) is **not** shareable with
reviewers; only `reviewer-packet` is. Reviewer completeness/nonwaiver covers the
supplied review set, not a frozen expected roster or authenticated identities.

**Still not_run / not claimed:** parent independent acceptance of this new seal,
installed native/fork delivery, live model/private-corpus outcomes, real note
migration, Linux/Windows runtime acceptance, deployment/release or human benefit.
Next: independently replay the new sealed source and every review finding;
obtain separate owner authorization for any real-data, installed or paid action.

### Historical first-integration verification (before corrective review)

**First integrated candidate (superseded by the corrective pass above):**
One `beme` executable combines V2 brain and the newer runtime. Verified brain,
runtime and evaluation deliveries were integrated as scoped deltas, preserving
Windows-aware harness adaptations, config-only `ResolveConfig`, ADR-033–035
and the newer durable/privacy paths. No commit, staging, publication,
installation, real settings/vault access or provider run occurred.

Current corrections are ADR-036 (brain reliability), ADR-037 (runtime evidence),
and ADR-038 (honest evaluation). Full generated-pack schema tests cover personal,
work-safe and trace-cache failure with invalid controls. The evaluator uses
`Pack.DeliveredItems` and complete-request APIs; B3's flat text estimate and
B4's full compact JSON budget remain a **bundled comparison**. RF01 visible-only
provenance determinism and RF02 matching fence markers passed their original
parent regressions after observed failures. Generated default preview/evaluator capability
IDs were corrected to match the unchanged schema regex.

Fresh combined evidence (local macOS arm64, offline Go dependencies, synthetic
HOME/config/temp roots; host Git config/signing disabled):

| Gate | Observed result |
|---|---|
| `go test ./... -count=1 -json` | exit 0; 438 passing test/subtest events across 18 passing packages, including both temporary-vault Git commit tests; 2 installed Claude Code/Codex tests intentionally skip |
| `go test -race ./internal/brain ./internal/app ./internal/ingestion ./internal/projection ./internal/resolver ./internal/evalrunner ./cmd/beme ./cmd/beme-eval -count=1` | exit 0; no race report |
| Built combined CLI + evaluator synthetic acceptance script | 37 checked commands: positive/negative retrieval, schema/cache failure, brain capture/maintenance/fork/freeze, corpus formats and pending/no-op/degraded/high-risk/quality/safety grade controls; source bundle unchanged, benefit not_run |
| Real-binary source/brain/MCP/evaluator suites | capture/promote/briefing, restore/error/freeze, protected warnings and scope, Markdown→build→pack, task parity, partial/recovered health, trace→explain and cache failure all pass |
| `TestGeneratedRuntimePackSchema` | actual personal/work-safe/cache-failure CLI packs validate; malformed trace/digest controls fail |
| Independent parent RF01/RF02 tests | both observed RED then GREEN; mixed/shorter/longer fence and work-safe ordering controls also pass |
| `make validate` | exit 0: 19 passed, 0 failed (historical public fixtures; generated states are separately tested above) |
| `go build ./...`, `go vet ./...`, `make fmt`, `git diff --check` | all exit 0 |
| `GOOS=windows go build ./...`, `GOOS=linux go build ./...` | exit 0; cross-build only, not native runtime/harness evidence |
| `python3 scripts/test_docs_consistency.py` | exit 0: 19/19 checks pass; D17/D19 observed RED before shared-doc correction |
| `python3 scripts/test_ci_regression.py`, `python3 scripts/scan_private_data.py` | exit 0: 7/7 separation checks and clean scan |
| Original independent audit probes via external Go overlay | exit 0: all 12 original brain probes, 13 runtime/evaluation groups and the B3 observation control pass; only frozen-fixture setup and full-JSON cap accounting adapted |
| Built `beme-threat-corpus --repo .` | exit 0: 34 passed, 0 failed, 0 not_run |
| Scratch-output `beme-bench --seed testdata/synthetic/records.json --out <scratch>/benchmark.json` | exit 0; all seed scales under the 1,000 ms warm-p95 target; historical repository baseline unchanged |

The exact command logs, original/adapted probe source, final source inventory
and executable hashes live in the operator's external integrated evidence
bundle, not in this repository. A binary's inherited alpha version string is
not its candidate source identity: use that bundle's SHA-256 seals. Build and
final verification results are bound there to the uncommitted candidate.

**Not run / not claimed:** installed native hooks (including native fork
delivery), live model use, paid evaluation, private corpus/vault changes,
deployment, Linux/Windows runtime acceptance or actual human decision benefit.
Synthetic supplied grades can pass the mechanism but retain `benefit: not_run`.
No `assured` label or release readiness follows from delivery tests. Brain
backups/logs/archive remain independent of V1 purge; Windows ACL privacy and
hostile same-user races are outside the demonstrated boundary.

**Next gate:** parent independent review of exact source, external evidence and
unresolved items. Passing synthetic gates is not merge/deployment approval.

### Historical source-adoption evidence (superseded by remediation above)

The following snapshot describes the earlier adoption-only candidate, not
current failures or current readiness. Its command evidence is preserved.

**Historical source-adoption snapshot (before remediation, 2026-10-09):** source integration is complete; this is an
**integrated baseline pending remediation**, not accepted product readiness. One `beme` binary has both the brain CLI and the newer runtime. Shared
edits are limited to brain dispatch, config-only loading and independent
managed adapter blocks. Public durable enforcement, purge, source-failure
diagnostics, negative relevance, trace handling, four-tool MCP surface and
B0–B4/ablation evaluation implementations remain on the newer code. Existing
`beme-eval`, `beme-bench` and `beme-threat-corpus` commands remain separate tools.

The adopted brain ADRs are **033–035**; public ADR-027–032 keep their identities.
FR-070..089 describes brain scope; live harness/model recall (FR-076) stays
**partial**. The vault remains separate, non-canonical and never enters V1
packs. See [BRAIN.md](BRAIN.md) and [THREAT_MODEL.md](THREAT_MODEL.md) §8.

Fresh evidence and remaining work:
- The new `TestIntegratedBrainAndRuntime` failed first because the public
  binary lacked `brain` (exit 2), then passed after source integration. The
  same executable captures/promotes/reads a synthetic note and proves newer
  positive/negative retrieval, no-evidence unknown, trace explain, work-safe
  exclusion, durable forget across rebuild and isolated config defaults.
- Adopted real-binary tests exercise synthetic hook JSON, read-only briefing,
  operator lifecycle, gate rejection and adapter coexistence. An ADR collision
  regression (docs D16) failed before the new entries and passes with the
  renumbered ledger. These are local fixtures, not installed-harness proof.
- Fresh local gates (macOS arm64; isolated synthetic HOME/config/temp roots,
  installed Go dependency cache only; no provider calls):

  | Command | Observed result |
  |---|---|
  | `go test ./... -count=1` | exit 0, all packages pass |
  | `go test -race ./internal/brain ./internal/app ./internal/resolver ./internal/evalrunner ./cmd/beme -count=1` | exit 0, all selected packages pass |
  | `go build ./...` and `go vet ./...` | each exit 0 |
  | `go build -o <scratch>/bin/ ./cmd/beme ./cmd/beme-eval ./cmd/beme-bench ./cmd/beme-threat-corpus` | exit 0; deliverable executables built, not installed |
  | Built `beme` on a disposable synthetic deployment | capture/promote/briefing + positive/negative runtime retrieval + explicit no-evidence unknown + no brain in packs + work-safe exclusion all pass |
  | Built `beme-threat-corpus --repo .` | exit 0: 34 passed, 0 failed, 0 not_run |
  | `make validate` (Python 3.13 venv, `jsonschema pyyaml`) | exit 0: 19 passed, 0 failed |
  | `python3 scripts/test_docs_consistency.py` | exit 0: 16/16 checks pass |
  | `python3 scripts/test_ci_regression.py` | exit 0: 7/7 checks pass; private corpus intentionally absent |
  | `python3 scripts/scan_private_data.py` | exit 0: clean, including additive untracked files |
  | `GOOS=windows go build ./...`, `GOOS=linux go build ./...` | each exit 0; cross-build only, not runtime verification |
  | `bash -n scripts/brain_live_e2e.sh` | syntax only, exit 0; script not executed |
  | `git diff --check`, `gofmt -l cmd/beme internal/brain internal/app/app.go` | clean/no formatting drift |

  The source-adoption comparison confirms only three existing Go files changed
  (`main.go`, `adapter.go`, `app.go`); newer resolver/eval/ingestion/durable/
  storage/privacy/MCP implementations are unchanged. Added brain code is the
  existing implementation, with ADR comments and the stale briefing-help line
  corrected. Test harness adaptations reuse the public Windows-aware builder
  and isolate temporary homes; Unix permission tests explicitly skip Windows.
- **Not run:** `brain-live`, any paid/provider call, private corpus/vault access,
  real hook installation, live model recall or repeated real-task effectiveness.
  Historical live results are deliberately not imported as acceptance.
- **Confirmed blockers (independent audits, not fixed by this integration):**
  the behavioral evaluator can report `passed` for a high-risk comparison
  with B4 scoring 1 versus B0 scoring 4; corpus/measurement and runtime
  evidence-delivery gaps remain. V2 restore can remove a healthy live vault
  before extraction succeeds; fuzzy deduplication can falsely confirm a
  materially changed decision; unverified worktree metadata can select another
  project's memory. Chronology, failure reporting, file-read safety, boundedness,
  freeze and generated-command issues also require isolated remediation.
  Existing green tests do not cover those new failure cases. Paid runs alone
  cannot close these engineering gaps. No remediation was attempted here.
- **Review required:** parent-directed remediation and new regressions must
  precede product acceptance or deployment. Hook counters do not measure
  task-level capture/use. V1 purge does not erase the independent brain vault,
  logs, archive or backups. Windows ACL privacy is unverified; Unix permission
  tests do not establish it. No deployment or release approval is implied.

### Historical runtime/release evidence

**Historical (rev 13):** PR #1 is open against `main`, unmerged at the owner's
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
  paid model-API runs — owner-gated. Generation remains grading_pending until independent offline grades;
  frozen corpus/split bytes and complete paired coverage are mandatory.
  See EVALUATION_USAGE.md; no private manifest migration is automatic.
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

1. Continue from [the current status in §1](#1-status), its linked PR/CI records
   and the selected candidate's exact source identity. Do not replay initial
   PR/authentication setup or closed historical audits from older receipts.
2. Apply [RELEASE_VERIFICATION.md](RELEASE_VERIFICATION.md) to that candidate:
   resolve evidenced blockers, require exact-head CI and fresh-checkout docs
   acceptance, obtain any outstanding explicit merge clearance, then verify the
   owner-authored release and downloaded source-bound assets. A pushed branch
   or local pass is not a published release.
3. Keep installation and real-task benefit separate. Any further authorized
   rollout must preserve real data, quiesce mixed-version writers and retain
   rollback evidence. Independently graded real tasks require their own scope
   approval; source tests do not authorize privacy expansion, paid calls or
   destructive operations.


## 9. Historical V1 acceptance evidence (prior continuation)

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