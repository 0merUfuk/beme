# Be Me — Data Model

Schemas are the contract: `schemas/` (JSON Schema, draft 2020-12). This
document summarizes; the schemas are normative.

## Records (normalized envelope)

`kind` (closed set: directive, fact, preference, principle, heuristic,
pattern, workflow, failure_mode, capability, precedent, unmapped_reference),
`status` (active|deprecated — supersession is a resolver state, not a
canonical status), `confidence` (observed|validated — no invented decimals),
`authority` (informational|recommended|default — assigned by the trusted
registration path only), `sensitivity` (public_general | personal_private |
work_restricted:<ws> | secret_never_ingest — monotonic during
normalization), `scope` (profiles, workspaces, task kinds, risk, excludes;
OR within a dimension, AND across), `validity` (effective/review/expires),
`relationships` (supersedes, challenges, conflicts_with, refines,
derived_from), `provenance_refs`, `declassified_from` (safe records only).

`observation`, `candidate`, `inference` are learned-model states, not
canonical kinds. Unknown source types stay `unmapped_reference` and can
never be normative guidance.

## Provenance

Lineage explanation: source, locator (repository-relative, never absolute),
revision, content hash, capture time, ingestion version, transformation
chain, approval event. Provenance never alone proves authority.

## Sources

Registration descriptor: root, purpose, trust ceiling, instruction
semantics, authority ceiling, sensitivity baseline, allowed profiles,
ingestion mode, include/exclude globs, revision policy. Registration sets
the ceiling; it never approves future revisions. The work-safe builder
ingests only `safe_declassified`-purpose sources (ADR-018).

## Workspace identity

Trusted registry object: canonical roots, expected remote, fingerprint,
approved revision policy, sensitivity namespace, bound project policy paths
with digests and ceilings. Request paths are matching signals only; a clone
never inherits trust; ambiguity fails closed.

## Stores

SQLite (WAL, CGo-free via modernc.org/sqlite) + FTS5. Tables: records,
records_fts, provenance, tombstones, meta. Fully rebuildable; one store
file per profile; personal and work-safe never share files (ADR-005).
Migrations are versioned and tested (FR-064).
