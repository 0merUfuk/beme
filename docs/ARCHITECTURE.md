# Be Me — Architecture

> Component boundaries and the resolution pipeline as implemented in
> WP4–WP8. Schemas referenced here live
> in `schemas/` and are normative.

## 1. Component map

```text
Trusted user config/approvals ──▶ Canonical sources ──┐
Registered repos + Rifja ──────▶ Untrusted ingestion ──┤
                                                       ▼
                                        Privileged personal builder
                                                       ▼
                                    Personal projection store
Canonical sources ──▶ Explicit declassification gate ──▶ Safe-source manifest
                                                       ▼
                                            Work-safe builder
                                                       ▼
                                    Work-safe projection store
(personal store) ──▶ Personal resolver process ──┐
(work-safe store) ─▶ Work-safe resolver process ─┼──▶ Versioned ContextPack ──▶ Agent
                                                  └──▶ Resolver trace
Agent feedback ──▶ Quarantined observations ──▶ Batch review ──▶ Canonical (user-gated)
```

1. **Source Registry** — declares roots, purpose, trust ceiling, sensitivity,
   profile eligibility, include/exclude, revision, ingestion mode
   (`schemas/source/source-descriptor.schema.json`).
2. **Ingestion & Normalization** — reads allowed content without executing
   repository code; produces normalized records with monotonic sensitivity and
   provenance (`schemas/record/`).
3. **Adapters** — canonical knowledge (markdown/frontmatter), project policy
   (ADR-style docs), Rifja contract (MCP stdio or CLI JSON, ADR-019). Adapters
   point to canonical sources; they never duplicate full contents.
4. **Projection Builders** — privileged personal builder and constrained
   work-safe builder compile physically separate stores. The work-safe builder
   reads only the approved safe-source manifest (ADR-018).
5. **Operational Index** — SQLite + FTS5 (CGo-free, ADR-004), revision
   metadata, fully rebuildable.
6. **Policy Evaluator** — non-bypassable privacy, capability, validity, status,
   scope, and trust rules. Runs before retrieval (Stage A).
7. **Context Resolver** — facets, retrieval, precedence/conflicts, budgeting,
   unknowns, pack + trace (Stage B).
8. **Admin CLI** — lifecycle, builds, preview, explain, review, export,
   forget, doctor, recovery.
9. **MCP Server** — narrow typed read surface + quarantined feedback, stdio
   only (ADR-009/014).
10. **Harness Adapters** — bootstrap, MCP config, hooks/wrappers, pack
    rendering; no business logic.
11. **Evaluation Harness** — deterministic policy/retrieval tests; behavioral
    baseline comparisons (`evals/EVALUATION_CONTRACT.md`).
12. **Brain (V2, ADR-033–035)** — an additional writable Markdown work-memory
    vault, independent of the resolver pipeline. Capture writes proposals;
    Doctor promotes; a separate briefing reads matched-project and global
    notes. Brain project registration never grants V1 workspace/source trust.

## 2. Two-stage resolution (ADR-007)

Task facets are derived from lexical task evidence before Stage A. Explicit
hints still narrow; an unclassified task is not silently called implementation.
CLI/MCP and `EvalInputsRequest` share request derivation, including risk/harness
hints. `evalrunner.RunRequest` forwards the full case request; task-only legacy
wrappers remain compatible and record only the narrower request they send.

**Stage A — hard eligibility (deterministic, enumerable):**
1. bind immutable session capability/profile ceiling;
2. resolve trustworthy workspace identity (fail closed on ambiguity);
3. deny prohibited sources, sensitivities, namespaces, relationships;
4. validate source revision and availability;
5. filter lifecycle, validity, scope;
6. apply revocation and purge tombstones;
7. group conflicts and supersession;
8. apply lexicographic precedence.

**Stage B — soft retrieval and budgeting:**
1. use the task/decision facets already derived before scope eligibility;
2. retrieve eligible candidates via task/topic maps → exact decision keys →
   canonical entries → bounded evidence refs → FTS fallback;
3. rank by relevance, scope specificity, evidence quality, utility,
   freshness (where applicable), redundancy;
4. reserve mandatory content (constraints, conflicts, unknowns, degradations
   are never budget-dropped);
5. admit advisory records by relevance across sections, then stable presentation
   order; budget the entire compact JSON pack (rounded bytes/4, not a model
   tokenizer), including unknowns, provenance and degradation notices;
6. compute unknowns and completeness;
7. derive unknowns/provenance from `Pack.DeliveredItems`, including experimental
   learned evidence, then emit the pack and actual sanitized resolver trace.

B3 retains flat item-text estimates at the same numeric requested cap; B4
includes the compact JSON envelope. Their contrast is a bundled pipeline
comparison, not an equal-overhead causal precedence/provenance ablation.

## 3. Precedence order (strongest → weakest)

1. non-bypassable runtime privacy/consent/safety policy;
2. current explicit user instruction authenticated by the trusted harness
   boundary (ADR-016); otherwise task text is a retrieval hint;
3. trusted, approved repository/project decision or constraint;
4. profile-scoped canonical directive;
5. global canonical directive;
6. scoped approved principle/preference;
7. global approved principle/preference;
8. validated precedent/pattern/workflow/heuristic/failure mode;
9. observed pattern (experimental, personal-only, explicit opt-in);
10. inference;
11. generic agent default.

Within one tier: narrower scope → explicit supersession → newer explicit
revision (evolving/time-bound items only) → stronger evidence → retrieval
relevance → stable tie-break by record ID. Stable principles do not decay by
age.

## 4. Conflict states

`resolved`, `shadowed`, `unresolved`, `stale_suspected`, `scope_separated` —
preserved in the pack's `conflicts` section. Conflict detection uses
`decision_key` and explicit relationships; similarity alone never declares a
normative conflict. Equal-tier material conflict on a reversible decision:
prefer project status quo or explicit fallback, record the assumption, proceed.
On a RED decision: escalate narrowly.

## 5. Work-safe boundary (ADR-005/018)

- Separate stores, builders, serving processes, caches for `personal` vs
  `work-safe`.
- The work-safe builder reads only the approved safe-source manifest; moving
  knowledge into it is an explicit declassification event creating a new safe
  record with safe provenance (`declassified_from` registry link).
- Work-safe provenance is a safe scoped view: omits private source IDs, repo
  names, absolute paths, private locators, denied relationship IDs/counts,
  correlatable content hashes. `policy_digest` and `index_revision` are
  `withheld` in work-safe packs; source digests stay absent. Provenance
  ordering uses only the already-visible/redacted tuple, never hidden IDs.
- Ambiguous workspace identity → no project/personal resolution (fail closed).
- Assurance mode is explicit: `cooperative-local` (v1 default) vs
  `isolated-admin` (requires verified OS separation; not claimed in v1).

## 6. Degradation contract

`status: ok | partial | unavailable | policy_blocked | rebuild_required`,
with `capability`, `resolver_scope_enforced`, `os_isolation`, and
`projection_revision` always reported. Unavailability never falls back to a
broader profile. Each request rereads persisted build-health metadata: partial
source failures make packs incomplete, empty projections unavailable, missing
or corrupt health stale. Notices are class-level without failed-source IDs.
No last-known-pack reuse is implemented (FR-047).

`Session.Resolve` persists policy-bound sanitized traces across CLI/MCP/eval.
Optional trace-cache failure leaves reads and expansion operational, clears
`trace_ref` and adds reduced assurance. A nonempty handle must resolve; retention
keeps the newest 200 by age, not random ID order. Legacy trace arrays are
filtered against current policy/revocation. Runtime fingerprints bind normalized
records, source-byte provenance and effective registration/session policy, not
labels; issued expansion tokens still bind the separate build generation.

## 7. Storage & layout

- Derived stores: SQLite (WAL), FTS5 index, per-projection; migrations
  versioned, atomic, rollback-aware (`testdata/migrations/`).
- Platform directories: config / data / cache separable (FR-062); canonical
  profile/source path configurable (FR-063).
- Logs: structured, sanitized — no raw task content, personal record text,
  source excerpts, absolute personal paths, credentials, or full packs.
- Offline by default; telemetry off; network transport absent in v1 (ADR-014).

## 8. Technology (ADR-004, verified)

Go 1.25; `github.com/modelcontextprotocol/go-sdk` v1.7.0 (stdio);
`modernc.org/sqlite` (CGo-free, FTS5 verified); YAML for human sources; JSON
for all machine contracts. No embeddings in v1 (ADR-012).

## 9. Brain alongside the runtime

See [`BRAIN.md`](BRAIN.md). Existing V2 source is integrated additively, not
used to replace the newer V1 runtime. `app.ResolveConfig` loads configuration
without source registrations; `app.Load` still assembles the full runtime.
Both use the newer per-platform directory defaults and explicit-config
isolation (ADR-026/028). Brain configuration adds only `brain_dir` and
`brain_mode`; a missing brain configuration creates no vault during V1 use.

The brain's separate project registry, Markdown vault, log and snapshots do
not share V1's precedence, projection stores or physical-purge ledger. The
MCP surface stays four tools. V1 build/resolve/export/purge and evaluation
arms remain on the newer runtime paths. `brain briefing` is a true read;
`session-start` and opt-in `briefing --doctor` can maintain the vault. Hook
failure is fail-open for starting a session, not permission to widen V1 scope.
