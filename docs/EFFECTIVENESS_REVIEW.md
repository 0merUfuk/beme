# Be Me effectiveness review and remediation contract

**Review date:** 2026-10-09. **State:** local engineering acceptance complete; live benefit unmeasured.
Canonical current delivery state is [HANDOFF.md](HANDOFF.md) §1. This review
separates deterministic product correctness from actual task benefit. It is
not release, deployment, or paid-evaluation approval.

## Verdict on the original problem

The product is intended to reduce repeated context reconstruction and improve
scope-safe decisions. Its components can capture, retrieve and deliver
context, but those mechanics do not establish fewer corrections, better
answers or less user effort. Existing seeded recall and deterministic mock
scores are integration evidence, not a measured treatment effect.

The review found a stronger problem than missing live measurements: the old
evaluator could declare success despite a severe regression, and production
paths could lose, stale, omit or mis-scope the evidence being evaluated.
These are implementable defects. Paying for model calls before fixing them
would not make the measurement valid.

This public document contains only synthetic reproductions and generic
engineering conclusions. Real deployment inventories, notes, session text,
identities and usage receipts remain outside the repository.

## Evidence standard

Use this ladder; do not promote evidence across levels:

1. **Implemented:** a production path exists.
2. **Configured:** the installed harness points at the intended executable,
   deployment and immutable capability.
3. **Invoked:** a real hook or tool actually ran (not merely a generated config).
4. **Delivered:** authorized, current, relevant items reached that invocation's
   output. A trace handle resolves to the actual sanitized trace.
5. **Used:** an attributable observation says how the agent considered an item.
   Self-reports must be labelled; delivery alone is not use.
6. **Beneficial:** independently graded paired task outcomes meet frozen safety
   and non-regression gates with reported uncertainty and missing data.

Absence of an explicit note citation does not prove no implicit benefit.
Conversely, a successful tool exit, model recall of a seeded sentence,
recapture count or high mock score does not prove real benefit.

## Root causes and acceptance matrix

The parent independently replayed the newly authored failing regressions,
including real CLI behavior. Existing tests passed on the same baselines.
Every row below needs its own corrected production-path regression; a passing
whole-suite total alone cannot close the row.

| ID | Confirmed baseline failure | Required durable correction |
|---|---|---|
| B01 | Restore can prune its selected input or remove live data before archive validation finishes | Pin input; validate/stage before replacement; preserve live data on refusal/failure |
| B02 | A retry-limit change from 3 to 5 becomes a confirmation of 3 | Conservative equivalence; changed statements/rationale are versions or conflicts, not confirmations |
| B03 | Same-second or batched captures can present older state as newest | High-resolution semantic chronology independent of maintenance timestamps |
| B04 | An unrelated directory can claim project memory with a nonexistent git-worktree marker | Verify reciprocal worktree metadata and scope; fail closed on forged/stale markers |
| B05 | Failed retirement is reported as completed and not recovered | Report only committed transitions; preserve retryable operations and error state |
| B06 | Absolute-path hook succeeds but advertises an unavailable bare executable | Bind every generated follow-up command to the executable and deployment actually selected |
| B07 | True reads omit conflict warnings; budget pressure removes them | Read-only scoped audit; protected warning budget before optional content |
| B08 | Manual-mode pull returns another manual pointer | Generated pull explicitly requests a full bounded briefing |
| B09 | Hook maintenance failure is silent | Bounded sanitized degradation notice, without blocking or promoting pending notes |
| B10 | Short capture IDs allow accepted proposals to overwrite one another | Collision-resistant, atomic no-clobber writes with deterministic collision tests |
| B11 | An unrelated FIFO can hang a scoped read | Bounded regular-file reads and scoped traversal before content loading |
| B12 | Symlinked identity content bypasses the note guards | Shared guarded read policy for identity, metadata and notes |
| B13 | Freeze misses registration and bookkeeping writers | Complete public-mutator inventory; consistent freeze/schema checks |
| B14 | Resumes inflate session counts; hook-less reads are absent from them | Idempotent explicit session identity, honest legacy/unknown counts, write-free reads by default |
| R01 | Real CLI accepts high-risk B0=4, B4=1 as passed | Generation/grading/gate states separated; independent blinded grade import and paired safety/benefit checks |
| R02 | Loader drops YAML and accepts invalid, retired or unapproved cases; split flag is only a label | Validated formats and lifecycle, exact coverage and frozen split membership |
| R03 | A Markdown heading becomes the entire decision while the Summary is lost | Test real supported source shape through ingestion, not only normalized store fixtures |
| R04 | Partial source failure looks complete to the agent | Capability-safe degradation propagated to packs, status and evaluation |
| R05 | Natural-language task facets are inferred after matching scoped records were excluded | Shared request derivation before the task-scope test, without widening privacy authority |
| R06 | ID-order budgeting defeats relevance; overflow and omitted evidence corrupt accounting | Relevance-first admission, protected mandatory sections, delivered-item provenance/unknowns |
| R07 | Learned content and empty-gold negative controls evade retrieval measurement | Complete delivered-item view; explicit negative-control and precision denominators |
| R08 | MCP emits a trace handle but discards the trace | Consistent bounded safe persistence and honest observability failure |
| R09 | Content changes do not change source/index digests; retrieval runs have no input manifest | Content/policy-bound identities and immutable retrieval run/case evidence |
| P01 | Newer runtime and brain exist on divergent implementation lines | One combined candidate retaining both sets of behavior and tests |
| P02 | Global project-state captures are accepted but not recallable | Reject new wrongly scoped summary/open captures; flag legacy notes without deleting, moving or broadcasting them |
| P05 | SessionStart matcher omits the documented `fork` source | Cover forked sessions and preserve session-ID idempotence; no automatic installation |

The B3/B4 baseline contrast also needs an honest causal label: different
candidate filtering/budgeting cannot be presented as an isolated
precedence/provenance ablation. Declared provider/network configuration is
not observed execution isolation.

The primary [official Claude Code changelog](https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md),
section 2.1.214, was fetched on 2026-10-09: fork starts report `fork` rather
than `resume`. Direct hook-page extraction was unavailable; this is not a
claim that the hook reference page was read. The matcher now covers that event.
The first-hand [upstream report #93458](https://github.com/anthropics/claude-code/issues/93458)
describes a fork hook running while its additional context fails to reach the
model. That report is unverified locally, not a confirmed Be Me defect or a
blocker to synthetic implementation. Hook invocation and native delivery remain
distinct; synthetic matcher/input tests do not establish native fork acceptance.

## Implementation plan

1. **Converge source, not real data.** Adopt the existing brain onto the newer
   runtime in an isolated candidate. Preserve all original branches and
   deployments. Keep brain notes non-canonical and outside V1 packs.
2. **Repair memory correctness first.** Restore safety, semantic updates,
   chronology, scope, file liveness and truthful mutation outcomes take
   priority over increasing the amount of captured context.
3. **Repair the delivery path.** Source normalization, partial health,
   task-scope derivation, budget selection and trace/snapshot evidence must
   agree across CLI, MCP and evaluation.
4. **Make evaluation fail honestly.** Model/command generation without
   independent grades is pending, not a behavioral pass. Validate the exact
   corpus/split, import hash-bound blinded grades, preserve missingness and
   compute paired gates without penalizing an intentionally weak baseline.
5. **Prove the combined candidate.** Re-run original failures, adjacent failure
   paths, process-level workflows, races, privacy corpus, public validators,
   documentation checks and fresh artifacts. Bind results to source hashes.
6. **Then measure real effectiveness.** Separately authorized paired real tasks
   and independent grading establish benefit or an inconclusive/negative
   result. No improvement percentage is invented from synthetic fixtures.

## Operational and privacy boundaries

- Memory is optional assistance, not a new task-blocking dependency. A denied
  capability remains denied; never change approval policy just to green a probe.
- Capture only durable outcomes. Do not fill the vault merely to improve a
  capture-rate counter or treat repetitions from one source as independent
  confirmation.
- Keep default reads write-free. A hook-less session must explicitly opt into
  operational accounting; missing identity is unknown, not a fabricated session.
- Do not solve missing recall by dumping all projects or unsupported global
  state into every prompt. Preserve unmatched-project and work-safe boundaries.
- Supported adapter code, installed configuration, real connection,
  pre-decision use and task benefit are separate claims for each harness.
- No real note migration, purge, transcript importer, new personal-context
  disclosure surface, live paid evaluation, merge or release is authorized by
  this engineering review.

## Completion record

Independent review of the first integrated candidate reopened the adjacent
findings below despite its existing green suites. The corrective pass first
replayed the original failures, then repaired their production boundaries
(ADR-039–040). Parent independently reviewed the final production delta and
replayed the full suite, original failures, actual overlapping-publication CLI
scenario, safe recovery and repeated races. All tracked deterministic findings
are closed within this tested scope. This is **not installed delivery or measured
human benefit**; the failed earlier snapshots remain preserved.

| Finding | Correction and observed synthetic evidence |
|---|---|
| BFR-001 | Last semantic capture survives both 128-proposal pagination and persisted supersession retry; delayed older revisions no longer erase a later exact recapture |
| BFR-002 | All global warning references are filtered by supported note identity/scope, not only `unsupported_scope`; original stale-open leak and warning-kind matrix pass |
| BFR-003 | Unknown/refused/malformed/oversized/linked schema metadata refuses writes before partial Init; genuine absent legacy marker remains supported |
| BFR-004 | Unresolved retry IDs are bounded without eviction; completed archive receipts prevent replay double counting; writer/reader format and size agree; all 800 actual Capture→Doctor recaptures stay readable |
| BFR-005 | Explicit wrapper identity persists through exact generated hook→manual pull→capture→Doctor commands, including quoted paths and no PATH alias |
| BFR-006 | Sequential correction passed, but final review reproduced a remaining overlapping-publication failure. Cycle 2 uses bounded independent obligations: unmodified public CLI observed RED→GREEN; both notes survive, failed reset warns and remains unknown, false nudge suppressed, later safe recovery demonstrated; six three-capture orders and delayed-reset/slot-reuse stress pass |
| E1 | Full semantic evidence/matrix reconciliation precedes template/export/import; all seven retained mutation rows reject at both template and import, bootstrap zero is a normal error, original positive/mixed/regression controls keep their 20-pair denominator |
| E2 | Only a standalone shuffled content-only reviewer packet is shared; no per-sample filesystem metadata. Predictions were persisted before owner labels: old directory 40/40, packet 16/40; hash-linked grades remain intact |

The original CLI probe compared an unescaped wrapper pathname with its
shell-quoted command string. Its unchanged corrected-side run remains recorded
as a false negative; the adapted assertion compares the shell-decoded executable
exactly and was replayed RED on the original binary, GREEN on the correction.
No product safety assertion was removed. The new generated-chain regression
also found and fixed the capture acknowledgment's next-command continuity.

E2 does not normalize or authorize sharing the owner `blinded/` directory, and
does not defeat inference from answer content. E1 does not authenticate a
malicious owner's wholly consistent fabricated run; keep original digest custody.
Completeness and safety nonwaiver cover supplied reviewers only, not an enforced
frozen roster or allowed version set. No new authentication system is claimed.

HANDOFF §1 records the fresh full/race/CLI/validation results and remaining gates.

The bounded final review found no additional blocker in the other listed
corrections; this second corrective cycle does not reopen them. Its public-CLI
probe changes only the observation path from the old shared marker to the first
independent intent slot; process pause, publication, acceptance and false-nudge
assertions are preserved. A missed pause is not a pass. Default briefing/status
reads were separately checked for unchanged bytes and metadata before a real
subsequent capture recovered known counters. Interrupted or refused intents are
not auto-expired; full capacity refuses before publication. Legacy mismatches
need quiescent inspection, not an unsupported promise of automatic recovery.

### Earlier integration record (before adjacent independent review)

The source-integration smoke was independently re-run successfully: one
executable handles brain capture/promotion/briefing while preserving newer
positive/negative retrieval, work-safe separation, trace explanation and
forget-across-rebuild behavior. That is **P01 integration evidence only**.

B01–B14, R01–R09, P01/P02/P05 now have integrated source corrections and
synthetic regression coverage (ADR-036–038). Parent followups **RF01** (stable
visible-only provenance order) and **RF02** (mixed/short fence markers cannot
replace a real Summary) were separately observed RED then GREEN.
`TestGeneratedRuntimePackSchema` also exposed default preview/evaluator capability IDs outside
the schema; the emitters were corrected rather than relaxing the regex. Shared
schema alternatives and complete-request evaluator delivery are covered by
production-path regressions. Original probes are replayed with only truthful
frozen-corpus/API/budget-contract adaptations, never lowered assertions.

HANDOFF §1 records the combined command results and completed parent independent
source-bound acceptance. Installed delivery, paid/private runs and actual human
decision benefit remain `not_run`/unmeasured. Historical green suites alone did
not close any row; the original failures and final correction were replayed.
