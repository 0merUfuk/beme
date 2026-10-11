# Be Me — Decision Ledger

> ADR ledger. Append-only; supersede via a new ADR, never silently rewrite a
> ratified one. Every ADR records: alternatives, evidence, consequences,
> reversibility, decision owner, reopen conditions.
>
> Decision rights: GREEN = agent-decidable; YELLOW = agent decides with
> recorded assumption + rollback; RED = user-owned (see `AGENTS.md`).

**Owner:** the user owns all RED gates; the implementation agent
owns GREEN/YELLOW technical decisions within these ratified boundaries.

---

## ADR-001 — Be Me is a context runtime, not an agent or personality clone

**Status:** Ratified (blueprint §1–2) · **Reversibility:** RED (product identity)
**Decision:** Be Me resolves authorized evidence into scoped ContextPacks for
agents. It does not impersonate the user, run reasoning loops, or generate
style-cloned output.
**Alternatives rejected:** style/personality cloning (low value, high
hallucination risk); autonomous agent (out of scope).
**Evidence:** product definition and non-goals in the planning baseline;
`docs/PROJECT_CONTEXT.md` §1–2.
**Consequences:** success is measured as decision-quality uplift, not
similarity of tone. Learning is quarantined; nothing promotes without user
action.
**Reopen:** user redefines the product thesis.

## ADR-002 — Core is harness-independent; plugins are adapters/packaging

**Status:** Ratified · **Reversibility:** GREEN
**Decision:** `internal/` core packages import no harness SDKs. Codex/Claude/
Hermes/Cursor integration lives in `adapters/` and references only versioned
pack contracts.
**Alternatives rejected:** Codex/Claude plugin as the product (vendor lock-in,
packaging ≠ product boundary).
**Evidence:** blueprint §5 ownership table; adjacent harness-independent tools
in the user's portfolio.
**Consequences:** harness breakage never breaks the runtime; adapter contracts
must stay thin.
**Reopen:** a harness capability exists that cannot be met by pack contracts.

## ADR-003 — External systems retain ownership of their data

**Status:** Ratified · **Reversibility:** RED (ownership boundaries)
**Decision:** Canonical knowledge repos, project repos, and episodic-continuity
tools (Rifja) keep ownership. Be Me registers sources through adapters, builds
only rebuildable derived indexes, and never rewrites canonical content during
resolution.
**Alternatives rejected:** copying canonical knowledge sources into a second authoritative
profile database.
**Evidence:** live layer model verified in WP0 (Layer 1 operating model, Layer
2 approved entries, Layer 3 project docs, each with its own governance).
**Consequences:** Be Me is useless without registered sources; deletion of the
derived index is always safe.
**Reopen:** a source system is abandoned by the user.

## ADR-004 — Go core; human-readable sources; SQLite/FTS derived index

**Status:** Ratified (dependencies verified in WP0) · **Reversibility:** YELLOW
**Decision:** Go for CLI, runtime, builder, MCP process; Markdown/YAML for
human-authored sources; SQLite with FTS5 as the rebuildable operational index;
JSON as the canonical machine contract.
**Dependency verification (2026-09-14, live):**
- Go 1.25.6 darwin/arm64 — present.
- `github.com/modelcontextprotocol/go-sdk` v1.7.0 — official MCP Go SDK
  (Tier 1, maintained with Google), Apache-2.0, stdio transport; compiled and
  linked in a verification program.
- `modernc.org/sqlite` v1.40.0 — CGo-free SQLite translation; FTS5 virtual
  table created and queried successfully in the same program; no CGo in the
  build, so cross-compilation stays trivial.
**Alternatives considered:** `mattn/go-sqlite3` (CGo required — deployment
friction), `ncruces/go-sqlite` (WASM runtime dependency), embedded Go-native
stores without FTS.
**Consequences:** single static binary; no database server; FTS5 available for
Stage-B retrieval; `go.sum` pins the supply chain (NFR-013).
**Owner:** implementation agent. **Reopen:** an official SDK or driver change
breaks stdio/FTS contracts; or measured recall shows FTS is insufficient for
retrieval (that is ADR-012's reopen condition, not this one).

## ADR-005 — Separate immutable profile projections and serving processes

**Status:** Ratified · **Reversibility:** RED (privacy boundary)
**Decision:** `personal` and `work-safe` are separate projection stores, built
by separate builders, served by separate processes; each serving process opens
exactly one store with one immutable capability ceiling.
**Alternatives rejected:** single shared store + output filter (filter bugs and
debug/explain paths leak; the safe projection must exclude inaccessible data
by construction).
**Evidence:** blueprint §7.4; user's repeated public/private separation
principles.
**Consequences:** two build paths to maintain; capability changes require
rebuild; work-safe store must never reference personal roots.
**Reopen:** never for the boundary itself; implementation details may evolve.

## ADR-006 — Source content is data by default; trusted path assigns authority

**Status:** Ratified · **Reversibility:** RED (security thesis)
**Decision:** Ingested content never executes and cannot declare its own
authority, profile, source type, or instruction semantics. Authority comes from
the registered source role and trusted approval path. Raw excerpts are omitted
from agent-facing packs by default; necessary untrusted excerpts are labeled and
confined to evidence sections.
**Evidence:** blueprint §7.6; prompt-injection threat corpus.
**Consequences:** the ingestion boundary is a security boundary; extraction
transformations output untrusted candidates only.
**Reopen:** never.

## ADR-007 — Two-stage resolver: hard eligibility, then soft ranking

**Status:** Ratified · **Reversibility:** GREEN
**Decision:** Stage A (capability, workspace identity, sensitivity, lifecycle,
scope, revocation, precedence grouping) runs before Stage B (facet
classification, retrieval, ranking, budgeting). Never one weighted score.
**Evidence:** blueprint §9.1; the failure mode it prevents (relevance
outvoting policy) is threat case 3.
**Consequences:** retrieval quality can be tuned independently of policy;
policy tests are deterministic and enumerable.
**Reopen:** never for the split; stage contents may evolve.

## ADR-008 — Versioned JSON ContextPack is the canonical interface

**Status:** Ratified · **Reversibility:** GREEN
**Decision:** `ContextPack` is versioned JSON with deterministic ordering and a
trace digest. Harness-specific Markdown is a renderer output derived from the
same pack (FR-037).
**Alternatives rejected:** Markdown as the canonical form (unstable to diff,
hard to validate, renderer-coupled).
**Evidence:** blueprint §11; user's JSON-first CLI conventions in adjacent
projects (`--json` output with `schema_version`).
**Consequences:** schemas are contracts (`schemas/context-pack/`); renderers
must not invent fields.
**Reopen:** a harness integration proves JSON unusable on its surface.

## ADR-009 — Narrow agent MCP surface; admin CLI is separate

**Status:** Ratified · **Reversibility:** RED (capability boundary)
**Decision:** Agent-facing MCP exposes exactly: `beme.resolve_context`,
`beme.get_context_item`, `beme.report_feedback`, `beme.status`. No raw search, no
profile switching, no source administration, no canonical writes, no raw
evidence drill-down. Admin operations live in the CLI and are unreachable from
ordinary agent capability.
**Evidence:** blueprint §12.1; Rifja's precedent of exposing no destructive
operations to agents.
**Consequences:** the MCP tool list is an enumerable contract test (FR-040);
CLI and MCP evolve independently.
**Reopen:** a demonstrable agent use case requires a fifth read tool — needs
user approval (RED: capability surface).

## ADR-010 — Explicit learning and implicit observation are separate paths

**Status:** Ratified · **Reversibility:** RED (learning governance)
**Decision:** Explicit trusted user instructions may authorize canonical
changes through the proposal path. Implicit behavior creates quarantined
observations only; observations are non-normative, excluded from work-safe,
visible to the resolver only behind an explicit experimental setting, and never
promote without explicit user approval. Agent-generated recommendations not
explicitly accepted are never evidence of preference; repeated outputs from
one model/session/workflow are one evidence family.
**Evidence:** blueprint §14; anti-self-training invariant.
**Consequences:** the review queue exists; rejected candidates need tombstones
(FR-053).
**Reopen:** never for the invariant; workflow mechanics may evolve.

## ADR-011 — Evaluation corpus precedes resolver implementation

**Status:** Ratified · **Reversibility:** RED (process gate)
**Decision:** Golden cases, mandatory/prohibited conclusions, privacy corpus,
and scoring contract exist and are frozen (WP2B) before production resolver,
storage, MCP, or adapter code begins. Ground truth may come only from explicit
user decisions, repository ADRs with user confirmation, repeated approved
patterns, or user-approved decision sets.
**Evidence:** blueprint §18; this repository's WP2A outputs.
**Consequences:** WP ordering is enforced; the seed corpus (private) is
user-approved before the holdout is sealed.
**Reopen:** never; thresholds may be recalibrated only before holdout opening,
by the user.

## ADR-012 — No embeddings in the first vertical slice

**Status:** Ratified until recall evidence disproves · **Reversibility:** YELLOW
**Decision:** Retrieval uses task/topic maps, exact decision keys, scope
metadata, canonical-entry lookups, and FTS. Semantic retrieval is added only if
locked-eval recall tests demonstrate a gap.
**Evidence:** blueprint §6.2, §10.3, §18.8 (recall ≥90% gate is defined without
embeddings).
**Consequences:** the resolver is fully deterministic and offline; a semantic
layer, if ever added, enters through the same candidate interface and cannot
affect policy.
**Reopen:** locked eval proves unacceptable recall after deterministic
improvements are exhausted.

## ADR-013 — Codex and Claude Code are Tier 1 initial adapters

**Status:** Ratified for initial release · **Reversibility:** GREEN
**Decision:** Adapter order: Codex and Claude Code first; Hermes and Cursor
documented and fixture-tested before being called supported. Initial
production-credible release requires two Tier 1 adapters. `assured` labeling
requires measured 100% pre-decision use; everything else is `advisory`.
**Evidence:** blueprint §13.2; harness version revalidation list (§13.8) — to
be re-verified against installed versions at WP8 time.
**Consequences:** adapter work waits for stable MCP/pack contracts (WP7).
**Reopen:** harness capability changes; user reprioritizes harnesses.

## ADR-014 — V1 uses local stdio only; network transport deferred

**Status:** Ratified · **Reversibility:** RED (security boundary)
**Decision:** MCP is served over local stdio only. No TCP listener, no HTTP.
Network serving is deferred until a separate threat model covering
authenticated encryption, short-lived audience-bound credentials, replay
resistance, rotation, revocation, rate limits, and adversarial tests is
ratified.
**Evidence:** blueprint §7.7, FR-061; Rifja's identical stdio-only posture
verified in WP0.
**Consequences:** "whoever can spawn the process holds local operator
authority" holds by construction; `beme serve --transport stdio` is the only
transport flag in v1.
**Reopen:** ratified network threat model + user approval.

## ADR-015 — Public engine and private deployment/evals are physically separate

**Status:** Ratified · **Reversibility:** RED (privacy boundary)
**Decision:** The public repository contains no user-specific data: no real
names, personal paths, private repository identities, private evaluation
cases, or credentials. Deployment data lives under platform user-config/data
directories; the private regression pack never reaches public CI.
**Evidence:** blueprint §17.1, confidentiality header; the user's
public/private separation principle.
**Consequences:** synthetic examples only in-repo; publication requires a
clean-history + installed-artifact scan (WP11).
**Reopen:** never.

## ADR-016 — Task authority requires trusted harness origin and integrity

**Status:** Ratified · **Reversibility:** RED (authority boundary)
**Decision:** Ordinary MCP request fields are untrusted hints for retrieval
only. Tier-2 task authority is assigned only when the harness captures the
user-authored prompt before model transformation and integrity-binds it to the
request. Quoted repository text, tool output, model summaries and paraphrases
never inherit user authority. Regression tests cover forged task authority,
prompt substitution, and integrity-token modification.
**Evidence:** blueprint §10.1 task-origin rule; threat cases 21, 4.
**Consequences:** the request schema has no free `profile` field; the
capability ceiling is process-bound, not request-bound.
**Reopen:** never.

## ADR-017 — Trusted admin actions need out-of-band OS separation for `isolated-admin` claims

**Status:** Ratified (default is `cooperative-local`) · **Reversibility:** YELLOW
**Decision:** Default assurance is `cooperative-local`: application-level least
privilege (separate capabilities, stores, builders, policy) with no
confidentiality claim against a same-user process with arbitrary shell access.
`isolated-admin` is claimed only when canonical sources, approvals, personal
projections, and admin operations are verified unavailable through OS
permissions, a separate account, or a verified sandbox.
**Evidence:** blueprint §7.4; WP0 confirmed no OS isolation is configured on
the initial deployment machine — so v1 claims only `cooperative-local`.
**Consequences:** documentation must state the threat boundary honestly;
`beme.status` reports `os_isolation: none`.
**Reopen:** the user configures OS separation and asks for the stronger claim.

## ADR-018 — Work-safe projection built only from an approved safe manifest via explicit declassification

**Status:** Ratified · **Reversibility:** RED (declassification is user-owned)
**Decision:** The routine work-safe builder reads only the approved safe-source
manifest — never personal source roots. Moving knowledge into the manifest is
an explicit declassification event that creates a new, separately approved safe
record with safe provenance; it never relabels a personal record in place, and
is tested for content, metadata, lineage, count, hash, and locator leakage.
**Evidence:** blueprint §7.4, ADR-018 table entry; WP0 verified a
de-identified safe engineering-knowledge corpus exists in the canonical source
repo as the natural initial safe-manifest seed — its existence is confirmed;
its content was sampled for structure, not ingested.
**Consequences:** every work-safe record has declassification provenance;
personal→safe flow is one-way and user-gated.
**Reopen:** never for the gate; the manifest contents evolve with user
approval.

## ADR-019 — Episodic evidence enters only through Rifja's bounded contract

**Status:** Ratified (new in WP0) · **Reversibility:** YELLOW
**Decision:** Be Me integrates Rifja as an optional source adapter over its
existing stable surfaces: the MCP stdio tool server (`rifja mcp`) or the JSON
CLI (`rifja search/resume/... --json`). Be Me normalizes authorized results
into precedent/evidence references. Be Me builds no second transcript importer
and no second session-resume UX. If deeper sharing becomes desirable, an
explicit ADR precedes any ownership move.
**Alternatives rejected:** shared database coupling (couples internals, breaks
independent evolution); reimplementing transcript import (duplicates Rifja's
owned domain).
**Evidence:** WP0 verified Rifja's published agent surface (read tools +
bounded proposal tool; destructive operations agent-invisible; stdio-only;
`--json` with `schema_version`) at the pinned snapshot.
**Consequences:** Rifja availability is a degradation, not a failure; the
adapter is optional and fails closed for episodic content.
**Owner:** implementation agent. **Reopen:** Rifja exposes no stable surface,
or the products' overlap review finds indistinguishable scope.

## ADR-020 — Evidence snapshot is re-pinned per work-package phase

**Status:** Ratified (new in WP0) · **Reversibility:** GREEN
**Decision:** Repository-derived architecture claims are pinned to explicit
source revisions. WP0 (2026-09-14) re-verified the planning snapshot: the
pinned commits resolve locally for all seven normative evidence repositories;
two previously GitHub-only repos were cloned read-only for verification.
Drift observed: one evidence repo advanced past its pinned commit; dirty
worktrees exist in several evidence repos; these do not invalidate the
architecture conclusions, which are re-checkable per claim. The private
snapshot manifest lives outside this repository (deployment data).
**Consequences:** future agents compare live revisions before relying on
repository-derived claims; public docs never cite private repo identities.
**Owner:** implementation agent. **Reopen:** live evidence contradicts a pinned
claim — then update the affected ADR, don't ignore the drift.

## ADR-021 — Repository identity: `beme`, MIT, local-only until publication gates pass

**Status:** Ratified (new in WP0) · **Reversibility:** YELLOW
**Decision:** Engine repository name `beme` (CLI `beme`); MIT license matching
the user's portfolio convention; no remote is created and nothing is published
until WP11 gates pass and the user approves external release. Name-availability
checks at WP0: `beme` is unused in the user's GitHub namespace; public search
shows no high-signal conflicting project by that exact name; registry checks
before release remain required (blueprint §25).
**Evidence:** WP0 name checks (2026-09-14); portfolio license convention (MIT
across the user's public tooling).
**Consequences:** everything lives on a local `main` until the publication
gate; publishing, tags, releases, and visibility are separate RED gates.
**Owner:** implementation agent for the local repo; user owns publication.
**Reopen:** a naming conflict is confirmed at registry-check time.

---

## ADR-022 — User delegation of RED gates via explicit instruction (2026-09-14)

**Status:** Ratified · **Reversibility:** YELLOW (recorded; user may re-assert gates anytime)
**Decision:** On 2026-09-14 the user issued an explicit standing instruction:
"Everything you need to make the core decisions is already documented in the
Blueprint Handoff. If anything remains ambiguous, do not ask me. Use the
sources referenced in the handoff, think as I would, validate the decision
yourself, and proceed… Complete all development, integrations, testing, and
fixes. Handle everything from pushing to GitHub and deployment through
distribution, if required. Do not wait for additional guidance."
This delegates, for this execution: (1) golden-case approval (self-validate
with provenance recorded as delegated, not silent); (2) WP2B threshold/split
freeze at design targets; (3) GitHub publication of the audited-clean public
repo; (4) release tagging with honest alpha labeling. Explicitly NOT
delegated: paid actions (none arise), credential creation (none), physical
purge (nothing to purge), and any *future* widening of privacy scope.
**Evidence:** the instruction itself (recorded verbatim above); blueprint
§3.2 "current explicit task instructions always override older workflow
preferences."
**Consequences:** gold provenance marks `approved_by_user: delegated_explicit_instruction_2026-09-14` — never presented as silent user review; live-model behavioral gates remain honestly pending operator runs; publication proceeded only after private-data deep scan passed.
**Owner:** user (delegated to implementation agent for this execution).
**Reopen:** any subsequent user message re-asserting a gate.

---

## ADR-023 — Public validation is repo-local; private evaluation is owner-gated and explicitly not_run when unavailable

**Status:** Ratified (2026-09-14, recovery of the v0.1.0-alpha CI failure) · **Reversibility:** GREEN
**Decision:** `make validate` (public contract validation) may depend only on
files inside the repository. It never reads the operator's home directory,
environment-specific paths, or private evaluation corpora. Private
evaluation data is validated by `scripts/validate_private_eval.py`, which
locates its corpus exclusively via `BEME_PRIVATE_EVAL_DIR` (no hard-coded
paths), is never invoked by public CI, and exits 3 (`not_run`) when the
corpus is unavailable — a private evaluation must never silently pass
because its data is missing, and its absence must never fail public CI.
**Alternatives rejected:** (a) keep validating the private corpus in public
CI with a skip-if-absent fallback — rejected: it fails closed on public
runners (the observed failure) and, worse, a silent skip would let private
validation silently pass; (b) upload the corpus to CI — rejected: private
evaluation data never reaches public runners (ADR-015).
**Evidence:** CI run 34794708408 failed at `make validate` with
`private candidates found — none in /home/runner/.config/beme/evals/<private>/candidates`;
regression suite `scripts/test_ci_regression.py` (R1–R5) reproduces and
guards the exact failure and the separation invariants; both run in public
CI.
**Consequences:** public "fixture checks" counts refer only to public
fixtures (19 as of this ADR); private-corpus validation (34 cases) is an
owner-run gate reported separately. The published validator previously
embedded the private corpus path name — that string was removed from the
public tree (private-path leak, in addition to the CI breakage).
**Reopen:** a verified need for public CI to exercise private data (never,
per ADR-015) or a schema change that couples public and private validation.

## ADR-024 — Documentation gate failed; canonical status single-sourced; docs consistency CI-gated

**Status:** Ratified (2026-09-14) · **Reversibility:** GREEN
**Decision:** The first fresh-agent documentation test (blueprint §24 /
ACCEPTANCE §9) FAILED and its verdict is binding: the runtime, CI, release
recovery, and public/private separation passed, but a fresh agent could not
determine the current project phase or run the documented validation path
without undocumented recovery work. Verified stale claims removed:
"Contracts phase — pre-implementation" (README), "production implementation
gated behind WP2B" (PROJECT_CONTEXT), "WP2B Blocked on user" and "WP4–WP11
not authorized" (ROADMAP), "fixtures 35/35" (ROADMAP), "34 tests across 7
packages" (HANDOFF), and DEVELOPMENT.md's "make validate … also validates the
private candidate corpus if present locally" — the pre-ADR-023 coupling that
caused the original CI failure. Remediation: HANDOFF §1 is the single
canonical status section (other docs reference it); README/DEVELOPMENT
document a reproducible validation bootstrap (Python 3.11+ requirement,
isolated venv, jsonschema+pyyaml, behavior when system Python is older,
exact clean-checkout commands); hard-coded evidence counts are replaced by
command-derived references; `scripts/test_docs_consistency.py` (D1 phase
contradictions, D2 stale counts, D3 public/private validation semantics,
D4 bootstrap presence, D5 link integrity, D6 status anchor) runs in CI.
**Alternatives rejected:** patching only the failing strings without a
regression gate (the drift would recur); moving status to ROADMAP (HANDOFF
is the blueprint's recovery entrypoint and already the most-recently-updated
document).
**Evidence:** fresh-agent transcript + report (2026-09-14, checkout @
`15bbda7`): `make validate` failed first run with FATAL jsonschema missing
(no documented bootstrap); stale claims listed above; final verdict "Yes,
with the caveat that status claims are stale".
**Consequences:** docs edits that reintroduce these classes fail CI.
Lifecycle claims must derive from HANDOFF §1. No new release for docs-only
changes; `v0.1.0-alpha.1` source retains the stale docs (recorded in its
release notes) and the next release carries the remediated set.
**Outcome (2026-09-15):** remediation verified. Re-test at clean checkout
`8896df9` (isolated agent, documented commands only): system python3 3.9.6
handled by the documented python3.13 fallback; all six documented gates
exit 0; `not_run` semantics reproduced exactly; no contradictions; no
improvisation; ~3 minutes total. Verdict: PASS. One residual imprecision
fixed post-test (make's exit-2 wrapping of the validator's exit 3 now
documented precisely).
**Reopen:** a future fresh-agent test failure re-opens this ADR.

## ADR-026 — Explicit config dir is a self-contained deployment root (test/production data isolation)

**Date:** 2026-09-15
**Status:** accepted (YELLOW — material, reversible)
**Context:** The privacy threat-case corpus (§19 threat model verification)
exposed cross-contamination: `app.Load(explicitCfg)` with no `config.yaml`
defaulted `DataDir` to the operator's real data home
(`~/Library/Application Support/beme`). Test deployments (threat corpus,
recovery tests, MCP e2e) silently wrote synthetic records, tombstones, and
observations into the real operator store. On this machine the entire store
was synthetic residue from agent tests (verified by direct SQLite inspection:
`rec_priv-001`, `rec_safe-001`, `rec_f-001` + two tombstones + one
observation); the directory was removed and the machine restored to a
pristine no-beme-state baseline. The operator's real configuration lives
under `~/.config/beme` (private eval corpus only) and was never touched.
**Decision:** an explicit config dir is a self-contained deployment root —
when `Load()` is called with a non-empty override, `DataDir`/`CacheDir`
default INSIDE it (`<cfg>/data`, `<cfg>/cache`), never to the user data
home. The user-home default applies only to `Load("")` (the real CLI
deployment case). Pinned by `TestExplicitConfigDirIsSelfContained`
(`internal/app/isolation_test.go`).
**Alternatives rejected:** requiring every caller to set `data_dir`
explicitly (silent foot-gun for every future test); making `Load` fail when
no config.yaml exists (breaks legitimate empty deployments).
**Consequences:** a test deployment that omits `data_dir` and `cache_dir`
cannot write outside its config root by construction. Explicit `data_dir` or
`cache_dir` values are still honored and may point anywhere, including the
user data home, so the guarantee covers defaulted paths only.
**Rollback:** restore the old defaulting in `Load` (one block).
**Reopen:** a test or subprocess path that writes to the default data home
again.

## ADR-027 — Physical purge workflow and durable tombstone ledger

**Date:** 2026-09-15 (revised the same day after owner review)
**Status:** accepted (mechanism implemented; executing it on real data stays RED/owner-owned)
**Context:** §7.8 and FR-055 require a physical purge distinct from logical
forget, leaving only a non-content anti-resurrection tombstone. Before this
ADR there was no purge workflow (threat case 30 was `not_run`), and
tombstones lived only inside projection stores, which get wiped, recovered
from corruption, rolled back, or restored from backup, silently reactivating
forgotten records (threat case 18 was `not_run`).
Owner review of the first implementation found three defects, fixed in this
revision: (a) projection purge deleted provenance by a convention-derived ID
(`prov_` + record suffix), missing records with several or nonconventional
provenance refs; (b) a purge that failed after its first deletion could not
be re-run — the second run found no records and reported not-found,
stranding traces, observations, and canonical files; (c) the ledger held
unkeyed SHA-256 digests of the source content and of the normalized
statement text, an offline dictionary oracle for low-entropy private data
(a guessed sentence or a short file could be confirmed from the ledger
alone).
**Decision:**
1. **Durable ledger** at `<canonical_root>/ledger/tombstones.json`
   (operator-owned configuration, never inside the data dir). `forget`
   writes the store tombstone and a ledger revocation; resolution and
   rebuild merge both. An unreadable ledger fails closed.
2. **Ledger minimality.** Purge entries are `hmac-sha256` fingerprints under
   a random 32-byte per-deployment key, over record identity
   (source ID + record ID) and over the purge key — nothing else. No content
   or text digests, no plain IDs, no timestamps or reasons; entries are
   sorted so order reveals no chronology; plain revocations superseded by a
   purge are removed. The key lives in a separate file,
   `ledger/purge.key` (0600), and `ledger/.gitignore` excludes the key and
   pending journals so a Git-tracked canonical root never commits them.
   Purge entries without a readable key fail resolution, rebuild, and purge
   closed (`ErrPurgeKeyMissing`).
3. **Provenance.** Projection purge removes the union of the record
   payload's `ProvenanceRefs`, the refs recorded in the purge plan, and every
   provenance row with the record's `(source_id, source_record_id)`. No ID is
   derived by convention. Canonical removal follows every locator of every
   ref.
4. **Idempotent, resumable execution.** Order: ledger fingerprints → journal
   → per-store purge + compact (`secure_delete`, FTS `optimize`, `VACUUM`,
   WAL truncate) → persisted traces → restating observations → canonical
   files (`--remove-canonical`, root-contained) → journal removal. The
   journal (`ledger/pending/`) holds identifiers only — record/source IDs,
   provenance refs, relative locators, trace file names, observation IDs —
   never record text. Every step skips work already done, so re-running the
   same `beme purge` resumes from the journal; a completed key reports
   `already_purged` (exit 0). `beme doctor` reports pending purges.
   `PurgeRequest.FailAt` is a verification hook (never set by the CLI or
   MCP) that injects failures at every stage in tests.
5. **Durability boundary.** The ledger, the purge key, and the journal are
   written by durable replacement: temp file → fsync (`F_FULLFSYNC` on macOS)
   → rename followed by an fsync of the parent directory on Unix; on Windows,
   which has no directory fsync, `MoveFileExW` with
   `MOVEFILE_WRITE_THROUGH`, which returns only after the move is flushed. No
   erasure begins until both the ledger fingerprints and the journal have
   crossed that boundary; any flush error aborts the purge before erasure.
   This does not protect against storage that acknowledges flushes it does not
   perform (volatile drive caches, some network or virtualized filesystems).
   The first revision only fsynced the file and described the ordering more
   strongly than the implementation supported.
6. **Inspection failures abort.** A projection, trace directory, observation
   store (unreadable directory, unreadable or corrupt observation file), or
   canonical path that cannot be inspected fails the purge — during planning,
   before anything is written, or during execution with the journal left for
   a resume. No step is reported `done` over data that was not inspected.
   Observations planned for removal are deleted by ID on resume even if their
   files became corrupt in between.
7. **Read surfaces.** Every surface that can reveal projection content,
   provenance, counts, or metadata reads through `internal/app` and applies
   store tombstones plus the durable ledger at read time; an unusable ledger
   or purge key fails each closed (`ErrLedgerUnusable`; CLI exit 3, MCP
   `policy_blocked`). `TestReadSurfacesUseLedgerFilter` fails the build if
   product code outside `internal/app`, `storage`, `projection`, or
   `resolver` reads projection rows directly.

   | Surface | Entry point | Rule |
   |---|---|---|
   | resolve, preview, MCP `resolve_context` | `Session.Resolve` | Stage A excludes revoked and purged records |
   | MCP `get_context_item` | `Session.ExpandItem` | pack-bound (ADR-029), visibility, Stage A |
   | MCP `status` record count | `Session.VisibleCount` | counts visible records only |
   | `beme export` | `Runtime.ExportProjection` | visible records and their provenance only |
   | `beme explain` | `Runtime.LoadTrace` | steps naming non-visible records dropped; pack counts withheld |
   | `beme doctor` | `Runtime.ProjectionFindings` | generic "rebuild required"; no IDs or counts |
   | pack degradation notice | `Runtime.Serve` | generic "out of date"; never mentions a purge |
   | `beme build` (admin write) | `Runtime.BuildProfile` | reports only a purge-blocked count of entries from operator-owned sources |
   | `beme candidate` | learning store | observations, not projection records — see consequences |
8. **Ledger ignore rules** are merged into an existing `ledger/.gitignore`:
   missing `purge.key` and `pending/` rules are appended, unrelated rules are
   preserved byte for byte.
9. Git history and external backups are out of reach; the report lists them
   as residuals with the remediation instead of claiming erasure.
**Evidence:** `TestPurgeRecordsUsesPayloadProvenanceRefs`,
`TestPhysicalPurgeRemovesEveryProvenanceRef`,
`TestPhysicalPurgeResumesAfterFailureAtEveryStage`,
`TestPhysicalPurgeIsIdempotent`, `TestPurgeLedgerIsKeyedAndContentFree`,
`TestPurgeLedgerMatchesIdentityNotContent`, `TestMissingPurgeKeyFailsClosed`,
`TestPhysicalPurgeErasesAndBlocksResurrection`,
`TestForgetSurvivesRestoreAndCorruptRecovery`, `TestCorruptLedgerFailsClosed`;
threat cases 18, 30, S1–S4 in `TestPrivacyCorpusDeterministic` and
`TestThreatCorpusRunner`; revision 3: `TestPurgePlanningFailsOnCorruptObservation`,
`TestPurgeResumeWithObservationStorageFailures`,
`TestPurgeFailsOnUnreadableTracesAndCanonicalPaths`,
`TestPurgeErasesNothingBeforeDurabilityBoundary`,
`TestFlushFailuresAreReported`,
`TestRestoredBackupHiddenOnEveryReadSurface`,
`TestUnusableLedgerFailsClosedOnEveryReadSurface`,
`TestRestoredBackupCannotResurrectOnAnySurface`,
`TestExistingLedgerGitignoreGainsRules`, `TestReadSurfacesUseLedgerFilter`. Each of the three review defects was reintroduced
in a scratch copy and the tests failed.
**Alternatives rejected:** unkeyed or salted fast content hashes (a
per-entry salt still allows a cheap dictionary test per entry); slow-KDF
content hashes (rebuild cost grows with records × purges); keeping content
matching to block re-keyed copies (defends against deliberate re-authoring at
the price of an offline oracle — accidental resurrection via sync, restore,
rollback, or rebuild preserves record identity); the key inside the ledger
file; OS keychain storage (platform-specific; deferred); a ledger inside the
data dir (restored with the store); Be Me rewriting Git history (outside the
ownership boundary, §5).
**Consequences:**
- Re-authoring the same words under a new record ID is not blocked — a
  deliberate operator act (pinned by `TestPurgeLedgerMatchesIdentityNotContent`).
- Whoever holds both the ledger and the key can test guesses of record
  identities (not content); keep the key out of shared or committed copies.
- Losing the key while purges exist blocks resolution and rebuild until it
  is restored, or until the operator deliberately removes the ledger entries
  (re-authorization).
- A source-level purge also blocks records later added under that source ID.
- A pending journal exposes identifiers until its purge completes.
- The number of entries reveals how many records and purge keys were purged.
- The pre-release v1 ledger format (content-derived purge entries) is
  rejected with an explicit error; it was never released.
- A restored pre-purge store still holds bytes until the next `beme build`;
  every read surface filters them, packs carry a generic "out of date"
  notice, and `beme doctor` reports that a rebuild is required.
- Observations restored from a backup of the data dir are recognized by
  identity since ADR-030; observations that merely paraphrase purged content
  under a new ID are not, because the ledger holds no content to match them
  against.
- Refusal paths are not timing-equalized.
**Rollback:** delete `internal/app/purge.go`/`ledger.go` and the ledger merge
in `Session.Resolve`; existing ledgers become inert files.
**Reopen:** a resurrection path the identity fingerprint does not cover, a
key-custody requirement (e.g. OS keychain), or a requirement for Be
Me-managed Git history rewriting.
**Amended by ADR-030** (2026-09-16): §2's "unreadable key fails closed" is
extended to full state verification (key ID, generation, entry shape,
missing-file asymmetry); §4's `secure_delete`/`VACUUM` compaction gains
`synchronous=FULL` plus explicit file, WAL, and directory flushes; §5's
durability boundary is extended from the ledger and journal to every
deletion; §7's read-surface table gains the learning surfaces; §8's ignore
rules gain `.lock`. `beme doctor` is the documented exception to the exit-code
table: it is a diagnostic that reports the most severe state it finds in its
output and exits 0, so a monitoring script reads `status`, not the exit code.

## ADR-028 — Platform directories per OS; Windows runtime verification in CI

**Date:** 2026-09-15
**Status:** accepted (YELLOW — behavior change on Linux/Windows defaults)
**Context:** `DefaultDirs` used the macOS `~/Library` layout on every OS, so
Linux and Windows deployments wrote to a nonsensical `~/Library` tree.
NFR-007 kept Windows `ported-unverified`: no Windows runtime had ever run the
test suite. Adding a `windows-latest` job immediately exposed that ingestion
produced backslash-separated relative paths, so include globs matched nothing
and Windows ingested zero records.
**Decision:** macOS keeps `~/Library/Application Support/beme` and
`~/Library/Caches/beme`; Linux follows XDG (`$XDG_CONFIG_HOME`,
`$XDG_DATA_HOME`, `$XDG_CACHE_HOME`, defaulting to `~/.config`,
`~/.local/share`, `~/.cache`); Windows uses `%AppData%\beme` and
`%LocalAppData%\beme`. `BEME_*_HOME` overrides win everywhere. Ingestion
relative paths and provenance locators are slash-separated on every OS. CI
runs build, vet, and the full Go test suite on `windows-latest`.
**Evidence:** `TestDefaultDirsPerPlatform`; CI jobs `test (macos-latest)`,
`test (ubuntu-latest)`, `test-windows`.
**Alternatives rejected:** `os.UserConfigDir` alone (no data-dir notion on
Linux); keeping `~/Library` everywhere (wrong on two of three platforms).
**Consequences:** a Linux/Windows alpha.1 deployment that relied on the old
`~/Library` default must move its files or set `BEME_*_HOME`. Windows is now
test-suite verified in CI; released-binary behavior inside Windows harnesses
is still not exercised.
**Rollback:** restore the single-layout `DefaultDirs` and drop the CI job.
**Reopen:** a platform convention change or a Windows failure CI cannot see.

## ADR-029 — Pack-bound context-item expansion

**Date:** 2026-09-15
**Status:** accepted (YELLOW — MCP tool contract change: `pack_id` is now required)
**Context:** `beme.get_context_item` found the record in the raw store, ran
an unrelated resolution, and returned the full record whenever that
resolution produced a pack ID. It never checked that the record had been
selected into a pack the caller received, nor whether the record was revoked
or purged. Reproduced against `597bfc4` over a real MCP client: a forgotten
record and a purged record from a restored backup were both returned in full.
**Decision:** each serving session keeps a bounded registry (256 packs,
30-minute TTL) of the packs it issued: the random pack ID, the selected
record IDs, the trusted task context, and the projection build generation.
Expansion requires `pack_id` and `record_id` and succeeds only when the pack
is known to this session and unexpired, the record was selected into it, the
projection generation is unchanged (every build stamps a new random
generation; a restored store carries an older one), the record is visible
under store tombstones and the durable ledger, and it still passes Stage-A
policy with the pack's task context. Every refusal is the single
`ErrItemUnavailable` ("context item not available"). An unusable ledger
returns `policy_blocked`, independent of the requested record. Work-safe
expansions omit source ID, source record ID, and provenance refs (FR-039).
**Evidence:** `TestExpandItemIsPackBound`,
`TestWorkSafeExpansionOmitsSourceIdentity`,
`TestRestoredBackupHiddenOnEveryReadSurface`,
`TestRestoredBackupCannotResurrectOnAnySurface`, `TestMCPClientEndToEnd`;
threat cases 8, 24, 26, S4.
**Alternatives rejected:** signed expansion tokens (they survive restarts but
add key management and still need revocation and generation checks);
re-resolving the original task at expansion time (budget-dependent and
costly); record-only expansion with a Stage-A re-check (the original defect —
eligible is not the same as selected).
**Consequences:** packs do not survive a server restart; clients re-resolve
after a restart, a rebuild, or 30 minutes. Refusal paths are not
timing-equalized. Clients that sent only `record_id` must add `pack_id`
(pre-release contract).
**Rollback:** restore record-only lookup in `mcp.go` (reintroduces the leak).
**Reopen:** a need for expansion across sessions or restarts.

## ADR-030 — Verified enforcement state: ledger generations, observation tombstones, durable erasure, maintenance lock

**Date:** 2026-09-16
**Status:** accepted
**Context:** owner review of `6c5b7d7` found four gaps in ADR-027's
mechanism, each reproduced against that head before it was fixed:
(a) a missing `ledger/tombstones.json` with `ledger/purge.key` still present
loaded as an empty ledger, so deleting one file silently disabled every
purge tombstone, and a `hmac-sha256:` prefix with any tail was accepted as a
fingerprint; (b) only the ledger, key, and journal writes were flushed —
traces, observations, and canonical files were unlinked without flushing the
directory entry, and a retried purge skipped files an earlier attempt had
already unlinked, so an outstanding flush was never completed;
(c) observations deleted by a purge returned in full (list, inspect, review,
dedup) after a data-dir backup restore, because nothing recorded that they
had been purged; (d) concurrent `forget`, `purge`, and `build` runs each
loaded, modified, and saved the ledger, so 24 concurrent forgets kept one
revocation.
**Decision:**
1. **Enforcement state is two files that prove each other.** `purge.key`
   (schema 3) holds the key, a `key_id` derived from it, the `generation` of
   the last committed ledger write, and `committed`; `tombstones.json`
   (schema 3) holds the same `key_id`, its own `generation`, and a `mac`
   authenticating its entry set. Loading fails closed when: the ledger is
   missing while the key is committed (or is a legacy key, which only ever
   existed alongside a ledger); the key is missing or malformed while the
   ledger needs it; `key_id` differs; the ledger's generation is below the
   generation the key records (partial rollback or restore); an entry is not
   exactly `hmac-sha256:` plus 64 lowercase hex characters; or the `mac`
   does not match the entries present.
2. **The entry set is authenticated, not just the file shape.** `mac` is an
   HMAC under the purge key over the schema, the key binding, the
   generation, and every revocation, purge and observation entry in a
   canonical order, written atomically with the ledger. Deleting or adding
   entries in place — which leaves `key_id`, `generation` and every
   remaining entry valid — is therefore detected, where shape and generation
   checks alone accepted it. Someone who holds the purge key can recompute
   the tag; that is the same boundary as deleting both files, which is
   documented below as locally undetectable.
3. **A pending journal must be explained by the ledger.** A journal is
   written only after the purge's fingerprints are in the ledger, so a
   journal alongside a ledger that holds no purge fingerprints (or with no
   usable key) is a partial restore that dropped enforcement, and fails
   closed — not only the case where the ledger file is absent. A journal
   directory that cannot be read is an error on every surface that reports
   pending purges, never "none pending".
4. **Write protocol.** Create the key uncommitted → write the ledger at the
   next generation with the key ID → rewrite the key as committed at that
   generation. A crash after step 1 (key uncommitted, no ledger) is
   recognizable, loads as clean, and reuses the key, so an interrupted first
   initialization does not brick the deployment. A crash after step 2 leaves
   a ledger newer than the key, which loads and enforces normally.
5. **Legacy compatibility.** A bare-hex key with a schema-2 ledger keeps
   enforcing and migrates on the next ledger write with the same key bytes,
   so existing fingerprints keep matching. After migration, restoring the
   pre-migration ledger over the committed key fails closed.
6. **Observation tombstones.** A purge records `hmac-sha256` fingerprints of
   the observation IDs it erases. Deployment surfaces open the store through
   `Runtime.OpenLearning`, which fails closed on an unverifiable ledger and
   hides purged IDs from list, list-all, inspect, review, family counts,
   feedback dedup, and the rejection-tombstone index — so a restored backup
   reveals neither the content nor that a rejection once existed. `beme
   build` erases restored copies and `beme doctor` reports their count
   without IDs. Zero-length observation files (what a crash between zeroize
   and unlink can leave) are treated as erased remnants.
7. **Durable erasure.** Erasure decides from the open handle: the file is
   opened without following a final symlink (`O_NOFOLLOW`, or
   `FILE_FLAG_OPEN_REPARSE_POINT` on Windows) and must prove from that handle
   that it is a regular file, not a reparse point, and has no other hard
   link, before anything is written — so an entry swapped in after inspection
   cannot redirect the truncate. Device+inode identity is deliberately not
   compared: a filesystem may reuse a just-freed inode, so it is not a sound
   check. Removal is bound to that same handle: while it is still open the
   name is re-checked against it (device+inode on Unix, volume serial plus
   file index on Windows) and the unlink is skipped when the name no longer
   refers to it, so a file a writer created at that name after the open is
   not deleted in its place. **Residual:** between that re-check and the
   unlink there remains a window that cannot be closed without holding a
   lock on the parent directory, which Be Me does not own for canonical
   roots that are edited independently of it. Every file a purge removes is
   zeroized, flushed,
   unlinked, and its parent directory flushed (`internal/durable`), and an
   already-absent file still flushes its directory, so a retry completes the
   flush an earlier attempt could not. Projections are compacted with
   `synchronous=FULL` and their file, WAL, and directory flushed. All of
   this precedes journal removal, so a finalized purge means every deletion
   crossed the boundary. A canonical file with other hard links is reported
   as a residual instead of being zeroized, because its content is shared
   with names the purge was not asked to remove.
8. **Maintenance lock.** `forget`, `purge`, `build`, and learning writes take
   an exclusive inter-process lock at `<canonical_root>/ledger/.lock`
   (`flock` / `LockFileEx`, two-minute bounded wait, git-ignored), so
   concurrent operations cannot lose each other's ledger updates and a build
   cannot re-ingest what a purge is erasing.
**Evidence:** `TestLedgerIntegrityFailsClosedOnEverySurface` (thirteen
damaged or partially restored states — missing ledger, missing key, rolled
back, foreign key, malformed entry, entries edited in place, a revocation
dropped, a journal without a ledger, a journal beside a purge-free ledger,
an unreadable journal directory — each against twelve surfaces, each with a
positive control),
`TestInterruptedFirstLedgerWriteRecovers`,
`TestCrashBeforeKeyCommitKeepsEnforcement`,
`TestLedgerRemovedTogetherIsUndetectable`,
`TestLegacyLedgerMigratesAndRollbackIsDetected`,
`TestLearningSurfacesRequireVerifiedLedger`,
`TestRestoredObservationBackupStaysHidden`,
`TestRestoredObservationHiddenOnCLI` (real binary),
`TestPurgeFlushesEveryDeletionBeforeFinalize`,
`TestPurgeRetryCompletesOutstandingFlushes` (per deletion class),
`TestPurgeRefusesToEraseHardLinkedCanonicalFile`,
`TestMaintenanceOperationsDoNotLoseUpdates` (also under `-race`),
`TestMaintenanceLockTimesOut`, `TestEraseZeroizesBeforeUnlink`,
`TestEraseRefusesSymlinksAndHardLinks`, `TestFlushFailuresAreReported`,
`TestLockSerializesHolders`, threat cases S3 and S4. Every assertion was
proved by reintroducing the defect in a scratch copy (mutation log in PR #1).
**Mutation coverage note:** two checks are defense in depth and are recorded
as such rather than claimed as mutation-proven. Removing the open handle's
regular-file check is not detectable on Unix, where the `O_NOFOLLOW` open
already refuses a swapped-in symlink (that path is mutation-proven); it is
the load-bearing check on Windows, where the entry is opened as a reparse
point, and its evidence is the same test running in Windows CI. Removing the
`PendingPurges()` error propagation is not independently detectable — the same unreadable journal directory
already fails `LoadLedger` closed, which every surface goes through. It is
kept as defense in depth for callers that report pending purges without
loading the ledger, and is recorded here rather than claimed as proven.
**Alternatives rejected:** a single file holding both key and ledger (a
leaked or committed copy would carry its own key); a monotonic counter in a
third file (the same asymmetry problem with one more file to lose);
detecting rollback by timestamps (restores preserve or reset them);
content-derived observation fingerprints (an offline dictionary oracle, the
defect ADR-027 revision 2 removed); an advisory in-process mutex instead of
a file lock (does not serialize separate CLI and MCP processes); refusing to
start when the lock is held (a long build would make the MCP feedback tool
fail rather than wait).
**Consequences:**
- Removing the ledger, the key, and every pending journal *together* is
  indistinguishable from a fresh deployment and stays undetectable locally
  (pinned by `TestLedgerRemovedTogetherIsUndetectable`). Detecting it needs
  state Be Me does not own — for example a backup or an external attestation.
- Restoring one file without the other now blocks every surface until both
  come from the same backup; the error names both files.
- The key file is rewritten on every ledger write (same key bytes, new
  generation), so key backups older than the current generation fail closed
  until the ledger is restored with them.
- A held lock makes a concurrent maintenance command wait and then fail with
  a clear "another Be Me maintenance operation is running"; reads are never
  blocked.
- `beme build` and learning writes now require a writable canonical root
  (the lock and the ledger directory live there).
- Observation tombstones reveal the number of purged observations, like
  record fingerprints do.
- Erasure is file-level: copy-on-write filesystems, SSD wear levelling,
  snapshots, and backups outside Be Me keep their own copies (reported as
  residuals).
**Rollback:** revert to ADR-027 behavior by loading the ledger without the
key-state checks and removing the lock; existing schema-3 files still load
(the extra fields are ignored by a schema-2 reader only after the version
field is lowered, so a rollback needs a ledger rewrite).
**Reopen:** a durability boundary a platform documents differently, a
requirement to detect wholesale removal of enforcement state, or key custody
moving to an OS keychain.

## ADR-031 — The source-size limit counts selected files; traversal has its own cap

**Date:** 2026-09-17
**Status:** accepted
**Context:** the ingestion walker counted every file it examined toward
`MaxFiles` (5,000) before applying the descriptor's include patterns. A source
whose root is an ordinary repository and whose `include` selects a handful of
entry files therefore aborted with "file count limit exceeded" as soon as the
repository held more than 5,000 files anywhere, and contributed zero records.
Reproduced against a real personal knowledge repository: root at the
repository, `include: ["knowledge/entries/*.md"]` selecting 8 files, 0 records
ingested; the same files ingested when the root was narrowed to the entries
directory.
**Decision:** `MaxFiles` bounds the files a source contributes — only files
that pass the hard excludes, the descriptor excludes and the include patterns
count. Traversal itself is bounded separately by `MaxVisited` (default
200,000 file entries examined), so a narrow include over a very large tree
stays bounded, and the per-file size, total size, depth and timeout limits are
unchanged.
**Evidence:** `TestNarrowIncludeUnderLargeTree` (a narrow include under a tree
exceeding `MaxFiles` ingests its matches; selecting more than `MaxFiles`
still aborts; examining more than `MaxVisited` aborts), `TestBoundsEnforced`
and threat case 29 unchanged. Reintroducing the old counting, or removing the
traversal cap, fails the test.
**Alternatives rejected:** raising `MaxFiles` (moves the cliff instead of
removing it); pruning directories by analysing include globs (correct but a
larger change to the glob matcher, and `**` patterns defeat most pruning);
documenting "narrow the root" only (leaves a silent zero-record failure on the
natural configuration).
**Consequences:** a source may now examine up to 200,000 file entries before
aborting; examining is cheap (no read) and the timeout still applies.
**Diagnostics (same change):** a limit or any other ingestion error used to
appear only as a "skipped" line while `beme build` exited 0 and `beme doctor`
reported healthy. A source registered for a profile that does not ingest is
now a *failure*: `beme build` names it on stderr and exits 1 (the sources that
did ingest still serve), the failure list is persisted in the projection
(`build_source_failures` meta), and `beme doctor` reports each failed source
as degraded until a build ingests it. An unbuilt projection that has a
registered source is degraded as well. Evidence:
`TestFailedSourceIsReportedAndKeepsDoctorUnhealthy` (missing root and an
oversized file; persisted across processes; cleared by a clean build),
`TestIngestionFailureIsNeverReportedHealthy` (real binary).
**Rollback:** move the count back before the include check.
**Reopen:** a traversal-cost problem on real trees within the cap.

## ADR-032 — Stage B retrieves: advisory records need task evidence, and absence is reported

**Date:** 2026-09-17
**Status:** accepted
**Context:** ARCHITECTURE §2 specifies Stage B as retrieval — "retrieve
eligible candidates… rank by relevance… budget advisory records… compute
unknowns" — and FR-034 requires task-material unknowns. The resolver instead
scored eligible records and emitted *every* one of them. Reproduced with a
two-record synthetic source: a question neither record addresses ("which
database should the billing service use") returned both, with
`completeness=complete` and no unknown, so an agent reading the pack could
take unrelated personal guidance as context for the decision. With a small
personal corpus every pack contained every entry.
**Decision:**
1. Records that always apply are never filtered by relevance: mandatory
   constraints (trusted project policy; default-authority directives or
   principles with high criticality) and globally applicable principles
   (foundation-role records; principles with no task, workspace, path or
   technology restriction).
2. Every other record is advisory and enters the pack only with task
   evidence: a task or workspace scope (Stage A already proved those match),
   a named technology, or at least one shared content word. Content words
   are whole normalized words — lower-cased, English function words and
   generic request words ("should", "use", "choose", "best", …) removed,
   inflections folded by a light stem — never substrings, so ubiquitous words
   cannot create relevance. There is no score threshold.
3. A record sharing a `decision_key` with a relevant record is retained: it
   is an alternative for the same decision, so precedence and conflict
   reporting still see it.
4. Every relevance exclusion is a trace step (`relevance`, per record), so
   `beme explain` shows what was left out and why.
5. When no advisory record remains, the pack carries the unknown "No
   recorded preference, precedent or guidance addresses this task" with
   `proceed_with_assumption`, so absence of evidence is stated instead of
   implied by an empty or principles-only pack.
**Evidence:** `TestRetrievalDropsIrrelevantAdvisoryKeepsMandatoryAndPrinciples`,
`TestRetrievalKeepsRelevantAdvisoryWithoutNoEvidenceUnknown` (including
inflection matching), `TestGenericWordsDoNotCreateRelevance`,
`TestScopedAndDecisionAlternativesAreRetained`; existing precedence, budget,
determinism and unknowns tests unchanged. Evaluation arms: B2 still receives
every eligible record; B4 now drops the fixture's irrelevant record, and its
budget control uses long *relevant* records. Threat cases 9, 24 and S4 now
resolve tasks about their own fixtures instead of relying on
include-everything. Each rule was proved by reintroducing the old behavior
(mutation log in the PR).
**Alternatives rejected:** a numeric score cutoff (arbitrary, and it would
also cut mandatory or scoped context that scores low); top-k selection (hides
required context when many records apply, pads with irrelevant ones when few
do); embeddings or model-based relevance (network and model dependency in a
local, deterministic resolver).
**Consequences:** retrieval is lexical — an entry phrased with different words
than the task can be missed, and one shared topical word (for example
"service") is enough to make an advisory record relevant. Unscoped principles
appear in every pack by design; a principle that only applies to one domain
should carry a task scope. The no-evidence unknown appears on many ordinary
tasks, which is the honest state for a small corpus.
**Rollback:** return `scored` unchanged from `retainRelevant` and drop the
no-evidence unknown.
**Reopen:** measured recall loss on the evaluation corpus (E5), or a need for
semantic retrieval.

## ADR-033 — V2 second brain: a writable, non-canonical work-memory vault with hook-driven session continuity

**Date:** 2026-09-19 (adopted into the integrated candidate 2026-10-09)
**Status:** accepted (YELLOW — user-directed V2 scope; ADR-001 not reopened)
**Context:** V1 resolves personal principles and approved knowledge
read-only from canonical sources. Nothing carried *work memory* across agent
sessions — why an architecture was chosen, what is unfinished — so every new
session started cold. The user directed a V2 with four requirements: session
continuity by an automatic start mechanism, a lightweight hygiene loop that
keeps only durable decisions, a hard "no code in the brain" separation, and a
pragmatic (~1 day) MVP. The research dossier (Karpathy LLM-wiki, hook-based
injection, selective persistence, Brain Doctor, coding/memory separation)
converged on plain markdown + hooks + lint, and warned that agents writing
their own mid-task output into long-term memory poisons it.
**Decision:**
1. **A brain vault** — plain markdown/yaml, Obsidian-readable, git-diffable —
   is an **additional writable, non-canonical store** alongside the runtime's derived and enforcement stores. It is *non-canonical work memory*:
   every note is `informational`, never enters the normative precedence tiers,
   and can never override user instructions, project policy, or canonical
   knowledge. It does not copy canonical sources (ADR-003): the identity card
   holds a short role card and *pointers*.
2. **Selective persistence.** Agents write *proposals* (`beme brain capture`,
   at task completion, per the protocol injected at session start). A
   deterministic gate refuses code, diffs, stack traces, secrets,
   transcripts, instruction-shaped text, and under-specified notes (a
   decision requires its `why`) — synchronously at capture and again inside
   Doctor. Only **Brain Doctor** promotes proposals to live notes.
3. **Brain Doctor** is a lightweight deterministic loop (no model calls):
   promote/reject, merge reworded duplicates, supersede by `topic`, retain
   the newest summaries, and raise stateless flags (conflict, overlap, stale
   open items, cap of 30, lint, invalid, foreign file). A human-`validated`
   decision is never silently overwritten — a conflicting proposal is
   promoted alongside and flagged. Retired notes move to `archive/`;
   nothing is destroyed except rejected proposals after 30 days. Runs are
   lock-protected, idempotent, support `--dry-run`, and optionally
   `--commit` to a vault git repository (the diff is the human review).
4. **Session continuity by hooks.** `SessionStart` runs
   `beme brain session-start --format claude-hook`: maintenance pass, then a
   bounded (≤ 9,000 bytes; the test contract reserves headroom below 10,000) briefing
   of identity, the project's latest summary, open work, decisions in force,
   learnings, Doctor flags, and the capture protocol. It is **fail-open**
   (a broken deployment degrades to one honest line, never blocks a session).
   `SessionEnd` runs `beme brain doctor --quiet`. Adapters stay `advisory`
   (FR-045/046 unchanged).
5. **Scope safety.** A briefing carries only the resolved project plus
   `global`; other projects are never mentioned. Projects are registered in
   the vault (longest real-path root wins, linked git worktrees resolve
   through their main checkout by file reads, ties fail closed), deliberately
   *separate from the V1 trusted workspace registry* so a brain registration
   can never widen V1 trust.
6. **Non-changes:** the MCP surface stays exactly four tools (ADR-009);
   no transcript ingestion (ADR-019 — the brain stores distilled outcomes,
   not conversations); no embeddings (ADR-012); no `os/exec` in `internal/`
   (FR-021 — git runs in `cmd/beme` only); no network (FR-060). The vault
   follows the ADR-026 isolation rule (`brain_dir` defaults inside the data
   dir; `BEME_BRAIN_HOME` never applies to an explicit config root).
**Alternatives rejected:** LLM distillation of session transcripts (paid
model calls, a second transcript importer against ADR-019, and the highest
poisoning risk); agents writing live notes directly (memory poisoning, no
review point); new MCP capture tools (breaks the ADR-009 contract; the CLI
is already reachable and allow-listable); reusing the V1 workspace registry
for projects (agent-runnable registration would widen V1 trust); indexing
brain notes into V1 projections now (needs scope-narrowing ingestion
changes; deferred, notes already carry `id`/`type` frontmatter so V1 can
ingest them later); an idle-time background daemon (platform dependency —
hooks plus a maintenance pass at both session boundaries need none).
**Consequences:** notes are agent-authored and can be stale or wrong. That
is mitigated, not eliminated: the briefing frames them as data and instructs
verification against the repository; validated decisions are protected;
every change is a reviewable git diff. Capture depends on the agent
following the injected protocol (advisory) — there is no forced capture, so
sessions that never capture leave no memory (the honest limit; evaluated separately by
`scripts/brain_live_e2e.sh`). Briefing recall by a real model is owner-run
evidence (paid/quota), recorded as `not_run` when unavailable (ADR-023
semantics).
**Rollback:** remove the hooks from harness settings; delete
`internal/brain`, `cmd/beme/brain.go`, and the `brain` route in `main.go`.
The vault remains ordinary markdown; `brain_dir` in `config.yaml` is
optional and additive.
**Reopen:** measured capture-compliance in live runs is too low (consider a
`Stop`-hook nudge); a vault outgrows a few hundred notes (add FTS over the
vault); pack integration with the V1 resolver is wanted (ingest brain notes
as scope-narrowed `precedent` records); multi-user use.

## ADR-034 — V2 completion: reference-parity hardening and the deliberate non-adoptions

**Date:** 2026-09-19 (adopted into the integrated candidate 2026-10-09)
**Status:** accepted (YELLOW — extends ADR-033; no ADR-001 change)
**Context:** ADR-033 shipped the MVP from the research dossier. A second pass
against the reference systems' primary sources (Karpathy's LLM-wiki gist,
nanobrain, open-second-brain, avenoxbeyin) found mechanisms the dossier's
summary omitted, and an independent review of the MVP found defects.
**Decision — adopted** (each with tests, FR-080..089):
1. **Append-only `log.md`** (Karpathy/nanobrain): `## [date] op | detail`, one
   greppable line per capture/promotion (with content hash)/retirement/backup/
   restore/freeze; never injected into a briefing.
2. **Backup-first maintenance** (open-second-brain snapshots, nanobrain
   checkpoints, Avenox backup-before-update): Doctor snapshots the vault once
   before its first mutation of a run; snapshots are checksummed tar.gz files
   in a sibling `<vault>.backups/` (never inside the vault or its git history),
   newest 10 kept. `restore` verifies the checksum, refuses traversal/links/
   oversized entries, and is itself preceded by a snapshot.
3. **`freeze on|off`** (open-second-brain): locks every writer; reads and the
   briefing keep working and say so. Restore stays possible while frozen.
4. **Schema guard:** `brain.yaml` carries `schema_version`; a newer vault is never
   written by an older binary. Engine updates never touch the vault (data lives
   outside the binary), which is the Avenox update-without-overwriting-notes
   guarantee.
5. **Vault guards** (V1 FR-022 in the brain): symlinked notes are refused and
   flagged; vault files are private (0700/0600); `identity.md` is screened for
   secrets/instruction-shaped text like any note and withheld with a notice.
6. **Modes** (Avenox normal/economy/manual): `brain_mode` in config or
   `--mode`; economy ≤ 3,000 bytes, manual is a two-line pull pointer;
   `beme brain briefing` pulls the full briefing (progressive disclosure).
7. **Session-silence nudge:** the brain counts session starts per project since
   the last capture and tells the agent when ≥ 2 recorded nothing — closing the
   advisory-capture gap without a blocking `Stop` hook.
8. **Lint parity** (Karpathy/nanobrain/open-second-brain hygiene): `aging`
   (decisions/learnings untouched > 180 days), `broken_ref`, `identity_pointer`,
   `identity_unsafe`, contradiction detection (negation-aware: an opposite claim
   is never merged as a duplicate), `doctor --strict` for CI, and `status`.
   Summaries always replace (latest state), never dedupe.
9. **Harness integration:** `PreCompact` settles the vault before compaction
   (the post-compaction `SessionStart` re-injects); `install-hooks` merges the
   fragment idempotently into a Claude Code settings file (foreign settings
   untouched, original kept as `.beme-bak`, `--remove`, `--dry-run`); a
   byte-exact managed instruction block (`beme brain adapter`, own markers) serves
   harnesses without hooks; Codex/Hermes/Cursor hook mechanics remain unverified.
10. **Compounding synthesis** (Karpathy answer filing): the injected protocol
    tells agents to capture the *distilled conclusion* of a durable analysis as
    a `learning` — never the transcript.
**Decision — deliberately not adopted:**
- *LLM distillation of transcripts, an idle-time background daemon, a `Stop`-hook
  raw queue* (nanobrain): paid model calls, a second transcript importer against
  ADR-019, the highest memory-poisoning risk, and a platform daemon. Replaced by
  agent-authored proposals + deterministic Doctor + the silence nudge. Reopen if
  measured capture compliance stays low.
- *Daily notes, rules, skills, reference documents as brain content* (Avenox):
  the owner's direction is that the brain holds only architectural decisions,
  work summaries, and experience; identity holds pointers, never copies.
- *Hash-chained ledgers, per-write byte diffs* (open-second-brain): git history
  (`--commit`) plus snapshots and the log give the audit trail at this scale.
- *Vector/FTS search over the vault, wikilink graph builder*: ADR-012; the
  note-count warning and generated index support small curated vaults; the
  warning is not a hard storage or traversal limit.
- *Brain notes inside V1 resolver packs:* deferred (privacy-corpus invariants
  need scope-narrowing ingestion changes); nothing in the references requires it.
**Consequences:** the brain is now reversible end to end (log, snapshots,
restore, freeze) and its failure modes are visible (flags, nudge). The cost is
more surface (`beme brain` has 16 verbs); each verb is small and covered by a
real-binary test.
**Rollback:** each capability is independent; removing one leaves ADR-033's
core intact. **Reopen:** measured capture compliance too low; vault grows past a
few hundred notes; Codex/Hermes expose a verified hook contract.

## ADR-035 — `briefing` is a true read; hook-less adapters request maintenance explicitly

**Date:** 2026-09-20 (adopted into the integrated candidate 2026-10-09)
**Status:** accepted (GREEN — source bug fix, no product-boundary change)
**Context:** A briefing routed through session-start attempts maintenance and
lock-file creation. That prevents a read-only caller from consuming already
promoted memory. Without hooks, captures also need an explicit maintenance
invocation before they can appear in future briefings.
**Decision:** Preserve the existing V2 separation: `brain briefing` performs
no writes by default; `--doctor` opts into best-effort maintenance and reports
a skipped pass without failing the read. `session-start` still attempts
maintenance by default. The hook-less instruction block requests
`briefing --doctor` at start and `doctor --quiet` at end; these are advisory
instructions, not evidence that an agent actually executes them.
**Alternatives rejected:** aliasing briefing to session-start (read requires
write permission); promoting proposals during every read (same defect);
adding new MCP tools or weakening a harness approval policy (out of scope).
**Evidence:** `TestBriefingIsReadOnlyUnderARealReadOnlySandbox` exercises a
synthetic vault with directory writes denied on Unix. The adopted CLI and
adapter tests cover maintenance and block installation. No live harness,
private deployment, or model-use acceptance is claimed for this candidate.
**Consequences:** read-only briefings can be one maintenance cycle stale;
notes remain informational and must be checked against current project state.
**Rollback:** revert the separate briefing path and adapter instructions; the
read-only failure would return. Existing Markdown notes remain readable.
**Reopen:** a verified native hook replaces the advisory maintenance path.

## ADR-036 — Brain correctness, bounded reads and honest session accounting

**Date:** 2026-10-09 · **Status:** implemented; local independent review/recheck complete
**Owner:** integration agent (YELLOW; user-authorized source/synthetic scope).
**Supersedes:** ADR-033/034 fuzzy merging, chronology and invocation-count
semantics; extends ADR-035 without changing its true-read default. Their
historical text remains intact; this entry is the current correction.
**Decision:** restore pins checksum-verified bytes and stages/validates all
entries before retention/live replacement, with rollback on ordinary install
errors. Exact content/rationale/topic equivalence alone is a recapture; changed
claims remain versions/conflicts, never independent confirmations. Capture
instants use RFC3339Nano; maintenance does not invent semantic freshness.
Verified reciprocal Git metadata can alias registered roots; explicit roots
win and forged metadata fails closed. Scope selection precedes bounded
regular-file reads; identity/metadata use the same no-symlink reader. Global
summary/open captures are rejected, legacy ones retained and flagged without
broadcast. Read-only audits protect warnings before optional content.
Mutation reports describe committed actions, failed retirements retain retry
intent, and proposals publish with 128-bit random IDs plus atomic no-clobber.
Registration/bookkeeping/init obey freeze/schema gates; dry-run needs no write
lock. Hook/manual/protocol commands quote the selected executable and config;
manual pull explicitly selects normal mode. Startup/resume/fork identities
are deduplicated in a per-project 512-hash window, separately from unidentified
invocations; hook-less accounting is explicit opt-in. Capture is not use.
**Evidence:** brain reliability/equivalence/mutation/guard/state suites;
`TestBrainReliabilityEndToEnd`, `TestBrainRealWorktreeIdentity`,
`TestBrainHooksIncludeForkSource`; original parent-audit probes replayed outside
the repository. HANDOFF §1 records current combined gates.
**Alternatives rejected:** fuzzy confirmations, trusting a gitdir string,
read-time maintenance/accounting, unbounded whole-vault reads, and implicit
global fallback; each recreates an evidenced defect.
**Consequences:** paraphrases may require review; old coarse timestamps cannot
recover lost chronology. Restore is entry-wise, not power-loss-atomic; retain
recovery directories. Same-user races and stalled filesystem syscalls are not
OS-isolated. Evicted IDs can count again. No native harness/task benefit claim.
**Reversibility:** source changes can be reverted independently; existing notes
remain ordinary files. Do not restore unsafe merging or destructive restore
ordering merely to recover old behavior.
**Reopen:** reproducible loss/scope/boundedness failure, measured useful recall
loss, or separately authorized native harness evidence.

## ADR-037 — Production-path evidence, full-pack budgets and honest traces

**Date:** 2026-10-09 · **Status:** implemented; local independent review/recheck complete
**Owner:** integration agent (YELLOW); privacy boundaries unchanged.
**Decision:** normalize actual Summary/Decision paragraphs, not navigation,
headings, comments or fenced examples. Fences close only on the opening
character with sufficient marker length and no trailing content (RF02).
Bounded metadata can narrow applicability, never self-assign authority.
Infer task facets before Stage A; explicit mismatches remain narrowing.
Persist build-health classes and reread on every request. Admit advisory items
by relevance across sections with full compact JSON byte/4 accounting;
mandatory overflow never admits an unlimited tail. Unknowns and provenance
reflect delivered compact items, including learned evidence.
`Session.Resolve` records sanitized, policy-bound traces for every frontend;
cache failure leaves reads working but clears the handle with reduced
assurance. Retention is by age. Normalized records, source-byte provenance and
effective policy bind snapshot identities. Work-safe withholds raw fingerprints;
provenance order uses visible fields only (RF01).
The pack schema gains exactly `policy_digest: "withheld"` and `trace_ref: ""`
alternatives to its existing valid strings. Empty trace is emitted only for
honestly reported persistence failure; generated personal/work-safe/cache-failure
schema tests and invalid controls enforce the shared contract. Default preview/evaluator
capability IDs now satisfy the unchanged capability regex; explicit labels are
not silently normalized.
**Evidence:** `TestGeneratedRuntimePackSchema`,
`TestMCPSourceEvidenceTraceAndHealth`, `TestParentMultiSourceProvenanceDeterminism`,
`TestWorkSafeProvenanceOrderUsesOnlyVisibleTuple`,
`TestParentMarkdownMixedFencesDoNotReplaceSummary`, `TestMarkdownFenceBoundaries`,
runtime budget/health/snapshot/trace suites, original external audit probes.
**Alternatives rejected:** widening scope for recall, weakening fingerprint
regexes, fabricating cache handles, and source-label hashes.
**Consequences:** budgets are deterministic estimates, not model tokens;
mandatory metadata may overflow with explicit incompleteness. Optional trace
storage may be unavailable. Trace content still obeys current revocation.
**Reversibility:** targeted source/schema rollback; rebuild derived indexes
if snapshot format changes. **Reopen:** a changed-input hash collision in the
contract, a denied-data leak, or a reproduced budget/observability mismatch.

## ADR-038 — Frozen evaluation inputs and independent offline paired grades

**Date:** 2026-10-09 · **Status:** mechanism implemented; live benefit `not_run`
**Owner:** integration agent (YELLOW mechanism); corpus approval, independent
judgments and paid/live execution remain owner-gated.
**Decision:** validate JSON/YAML/YML, lifecycle, capability and exact corpus
coverage; require an approved frozen byte/hash/family/split manifest before
generation. Complete-request evaluation uses `RunRequest` →
`EvalInputsRequest`; legacy task-only wrappers remain explicit and evidence
records only forwarded fields. All delivered sections use `Pack.DeliveredItems`.
Required/optional/forbidden labels, aliases and empty-gold controls have honest
denominators; degraded retrieval fails instead of disappearing from coverage.
Command generation is `grading_pending`/exit 3. Templates contain no judgments;
complete blinded independent hash-bound grades and nonwaivable safety booleans
are imported offline. Pair by case/repeat; report decision/reasoning separately,
whole-family seeded bootstrap intervals, quality tolerance and high-risk
pass→fail guards. Weak B0 may legitimately improve to a good B4. Synthetic
ratings can pass machinery gates but never become measured benefit.
Every run, including retrieval-only, preserves original corpus/split bytes,
actual requests, eligible/delivered content and source/binary identities.
B3/B4 remains a bundled comparison: B3 flat item-text estimates and B4 full
JSON overhead differ. Provider/network/harness declarations are not observed
isolation or release assurance.
**Evidence:** evaluator corpus/split/evidence/grades/comparison/CLI tests,
`TestRetrievalEvidenceBindsRiskAndHarness`, independent original audit probes and
synthetic executable grade-import controls. Protocol: EVALUATION_USAGE.md.
**Alternatives rejected:** default keyword behavioral verdicts, split labels
without membership, silent YAML omission, averaging safety into scores, and
calling model settings observed when only supplied by a provider.
**Consequences:** existing private split formats require explicit owner
adaptation; no automatic approval. Reviewer independence is declared, not
identity authenticated. Hash/exclusive-write bundles are not tamper-proof
against their owner. A paired endpoint alone is not all trusted-beta gates.
**Reversibility:** API wrappers retained; old evidence remains historical,
never relabeled as newly graded. **Reopen:** invalid evidence accepted, blinded
condition leakage, or a reproducible gate false-success/false-failure.

## ADR-039 — Close adjacent brain chronology, write-gate and bookkeeping failures

**Date:** 2026-10-09 · **Status:** source correction accepted locally after independent recheck
**Owner:** corrective-pass sole writer (YELLOW); no deployment or permission changes.
**Decision:** retain the last semantic capture instant separately from maintenance
updates, including conservative legacy recapture timestamps. Use it both for
supersession and persisted-intent reconciliation across bounded inbox pages.
Filter global warning identifiers by supported, safely read note scope, not
warning kind. Strictly distinguish an absent legacy schema marker from an
unreadable, linked, oversized, malformed, unknown or newer marker; inspect it
before repairing even a partial vault. Completed merges retain archive receipts;
only unresolved retry IDs remain on a live note, capped at 128 without evicting
pending operations. Successful note writes must pass the production reader's
format and 64 KiB bound. Carry explicit wrapper identity through manual pull,
capture and its generated Doctor command. A lock-free accounting intent precedes
publication; accepted capture with a failed reset stays accepted, with a warning
and unknown counters, never an ordinary failure inviting duplicate retries.
**Evidence:** original BFR-001..006 reproductions, the real 800-recapture
Capture→Doctor lifecycle, `TestCorrection*`, and
`TestBrainExplicitWrapperSurvivesGeneratedChain`. HANDOFF §1 records final gates.
**Alternatives rejected:** Created-only ordering, warning-kind-only redaction,
marker-error fallback to v1, lifetime retry lists or eviction of unresolved IDs,
wrapper rediscovery via the executable, and failing a capture after publication.
**Consequences:** older/ambiguous revisions stay reviewable; excessive unresolved
cleanup delays more merges instead of inflating counts. A subsequent accepted
capture can reconcile completed publications, but not interrupted/in-flight
intents (see the corrective-cycle-2 amendment below). Old archive receipts remain ordinary
files; no lifetime archive-growth or crash/power-loss isolation claim.
**Reversibility:** scoped source rollback; preserve notes, receipts and prior
evidence. Do not roll back to silent false acceptance or unavailable live notes.
**Reopen:** a reproducible chronology, retry double-count, reader-validity,
write-authorization, identifier-disclosure or accounting-truth counterexample.

**Corrective-cycle-2 amendment (BFR-006 only):** independent review reproduced
two overlapping CLI captures where the single shared marker let the second
completion hide the first's late accepted publication and failed reset. Replace
that last-writer-wins marker with up to 128 exclusive per-project intent slots.
An owner appends completion only after publication. Reset acknowledges only a
snapshot of completed slots, commits state, and reclaims those slots while still
holding the accounting lock. File/allocation identity prevents an old delayed
reset from acting on a reused slot; already-acknowledged resets leave newer
events alone. A monotonic reset epoch and read-side snapshot recheck prevent
combining old counters with newly cleared slots. Publication errors/collisions
cancel only their own unaccepted attempt. No exported API, Doctor lock or
global-project scan is introduced.

**Alternatives rejected:** rewriting the same shared marker later (still racy),
unbounded per-capture live files, expiring pending intents by age, or converting
an accepted publication into an ordinary error. **Tradeoff:** full capacity
refuses before publication; partial/interrupted/refused or legacy-mismatched
intents remain conservatively unknown and require inspection after all writers
stop. A later successful capture clears only completed obligations. Known legacy
state remains readable; concurrent old/new writers and retroactive reconstruction
of already-lost legacy obligations are not promised. Rollback must stop all writers
and retain accounting evidence, not run an old binary that ignores new slots.

**Evidence:** unchanged public-CLI two-process reproduction observed RED then
GREEN, both hitting the real pause-before-publication boundary, with two accepted
notes, warning/unknown/no false nudge and later safe recovery. Added
`TestCaptureOverlapLatePublicationRemainsUnknown`, all six three-capture completion
orders, delayed-reset/slot-reuse, collision/error, bounded interruption and
concurrent stress controls; original sequential accounting test remains unchanged.
This amendment does not reopen BFR-001..005, E1/E2 or runtime/RF01/RF02. Parent
reviewed the new production delta and independently repeated the exact public-CLI
interleaving, safe recovery and 20-repeat race run successfully. Local acceptance
is recorded in HANDOFF §1; no deployment is authorized by that result.

## ADR-040 — Reconcile evidence before grading; share a content-only reviewer packet

**Date:** 2026-10-09 · **Status:** source correction accepted locally after independent recheck
**Owner:** corrective-pass sole writer (YELLOW mechanism); owner still approves
corpus, reviewers and live/paid execution.
**Decision:** checksums are necessary but do not establish semantic consistency.
Before template issuance, reviewer export or grade import, revalidate the stored
corpus and frozen split, derive the selected population, require the exact
configured case×arm×repeat matrix and artifact layout, and join raw text/prompts,
arm key/construction, request, retained context, manifests, gold and summary.
Reconstruct arm inputs from retained evidence; record repeat overrides and
full-versus-legacy request mode explicitly. Old task-only/default-repeat bundles
are checked as such, not relabeled as full requests. Reject invalid bootstrap
settings normally, before analysis. A new `reviewer-packet` export is the only
shareable review surface: one shuffled content-only JSON file carrying samples
and the existing hash-bound grade template. The owner bundle, including `blinded/`,
is not shared; its creation timestamps are not claimed normalized.
**Evidence:** all seven original E1 mutations reject at template/import while
original positive/mixed/regression controls retain their intended population;
`TestReindexedContradictoryEvidenceRejectedBeforeTemplate`,
`TestEveryConfiguredArmReconstructsForGrading`, repeat-override controls, and
`TestReviewerPacketHasNoConditionOrderMetadata` (predictions recorded before labels).
**Alternatives rejected:** checking only the reindexed hashes, inferring missing
population from submitted grades, fixing only mtime while exposing birth times,
or calling a directory blinded because its filenames are random.
**Consequences:** incomplete or unreconstructible historical bundles cannot become
gradeable by silently guessing configuration. These checks reject contradictory
pre-anchor bundles, not a malicious owner's wholly consistent fabrication; keep
the original index digest independently. Nonwaiver/completeness covers **the
reviewers supplied to this import**, not a frozen expected roster. Reviewer-specific
versions are recorded, not preauthorized by a frozen version set. Authentication
and roster enforcement are not claimed. Answer wording can still suggest an arm.
**Reversibility:** existing grade format/hash anchors remain; retain old evidence
as historical, export a new reviewer packet without changing its source bundle.
**Reopen:** contradictory evidence becomes gradeable, metadata predicts condition
order on the actual shared packet, or separately authorized roster requirements.

## Open decisions (tracked, none blocking contracts work)

| Question | Default action | Escalate when |
|---|---|---|
| Exact harness hook mechanics per harness | Installed Claude Code 2.1.271 and Codex 0.154.0 verified at config-lifecycle level (both) and harness-connection level (Claude Code); pre-decision use needs live model sessions | No safe pre-decision path exists on a claimed supported surface |
| Performance budget | Measured on the public seed corpus (`evals/benchmarks/seed-baseline.json`); warm p95 far below the 1 s NFR-008 target up to 200× scale | Meeting a usable SLO requires architecture expansion |
| Promotion UX | Batch CLI first (ADR-010 governance) | CLI friction makes the learning loop unusable in dogfood |
| Physical purge workflow | Implemented, resumable, idempotent, keyed content-free ledger; tested on synthetic data (ADR-027); running it on real data is an owner-run RED action | The user requests actual erasure |