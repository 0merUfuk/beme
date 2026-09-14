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

## Open decisions (tracked, none blocking contracts work)

| Question | Default action | Escalate when |
|---|---|---|
| Exact harness hook mechanics per harness | Verify installed versions at WP8; use wrapper where hooks insufficient | No safe pre-decision path exists on a claimed supported surface |
| Performance budget | Measure on seed corpus at WP10 calibration | Meeting a usable SLO requires architecture expansion |
| Promotion UX | Batch CLI first (ADR-010 governance) | CLI friction makes the learning loop unusable in dogfood |
| Physical purge workflow | Document as RED destructive workflow (§7.8) | The user requests actual erasure |