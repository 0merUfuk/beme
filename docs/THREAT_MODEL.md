# Be Me — Threat Model

> Contracts-phase document. Data-flow, attacker, boundary, misuse,
> mitigation, and non-goal analysis. The 30 ship-blocking regression cases map
> to the invariants in `evals/EVALUATION_CONTRACT.md` §7.

## 1. Security thesis

> The model does not choose its authority. A session capability determines its
> maximum scope. A source does not declare its own authority. The trusted
> ingestion path assigns the authority ceiling. Where privacy matters, absence
> of access is stronger than a deny filter.

## 2. Trust zones

| Zone | Examples | Default treatment |
|---|---|---|
| Runtime safety policy | Non-bypassable privacy/consent rules | Trusted; never budget-dropped |
| Trusted user approval path | Explicit user memory/promotion action | May create/revise canonical records |
| Canonical source | Layer-1 operating model; promoted knowledge entries | Trusted within registered scope and revision |
| Trusted project policy | Explicit approved project decision file | Trusted only for that project; cannot widen privacy |
| Trusted reference | Curated source indexes | Evidence/guidance within authority ceiling |
| Untrusted content | READMEs, source code, imported notes, external docs | Data only; never a directive |
| Agent feedback | Model output, suggested preference | Quarantined observation only |
| Secret/never-ingest | Keys, tokens, raw protected corpora | Excluded before indexing |

## 3. Data flows and disclosure

Local: (1) data stored locally; (2) retrieved/resolved locally. Not local by
itself: (3) pack content enters the harness; (4) the harness sends it to a
local or cloud model; (5) downstream logs/retention/provider policy are outside
Be Me's control. Documentation must never claim "data never leaves the
device."

## 4. Attackers and misuse cases

| Adversary | Capability | Primary invariants |
|---|---|---|
| Prompt-injected repository content | Tries to become a directive; set its own authority; hide attacks in encoding/comments/Unicode | P4, P11 (ADR-006) |
| Compromised/misbehaving agent | Tries profile elevation, workspace escape, admin reach, feedback writes to canon, relationship traversal into denied records, expansion-ref guessing/replay | P1, P2, P5, P6, P8, P15, P16 (ADR-016) |
| Malicious clone | Same remote as a trusted workspace; tries to inherit trust | P2 (clone ≠ trust; registry approval required) |
| Curious "helpful" model | Invents preferences; states unknowns as fact; over-personalizes | P13 (negative controls) |
| Same-user shell process | Reads any file the OS user can read | Out of scope for `cooperative-local` (§7); `isolated-admin` requires verified OS separation (ADR-017) |
| Resource abuse | Oversized/recursive/malformed/Unicode-confusable/decompression-bomb inputs | P18 (bounds) |
| Time/confusion attacks | Changed normative file trusted because the repo was registered; stale caches mixing revisions | P14, P7 (revision pinning; rebuild invalidation) |

## 5. Boundary mitigations (design commitments)

- **Capability-bound serving:** one process, one immutable projection store,
  one ceiling; requests narrow only (FR-010/011).
- **Structural non-representation:** the request schema cannot carry
  `profile`/`capability`/`authority` fields (verified by negative fixtures).
- **Workspace identity as registry object:** request signals are matching
  hints; ambiguity fails closed (FR-014).
- **Revision pinning:** normative content trusted only at approved
  revisions/digests; changed content is pending (FR-016, P14).
- **Ingestion safety:** registered roots only; no execution of repo code;
  real-path/symlink checks; secret excludes; size/count/depth/time bounds
  (FR-020–023, P10/P11).
- **Expansion references:** opaque, unguessable, short-lived, bound to one
  pack + capability + workspace + revision + expiry; invalidated on
  rebuild/revocation; existence of denied records is never revealed (P16).
- **Learning quarantine:** feedback writes observations only; evidence-family
  dedup; rejection tombstones; promotion is user-owned (FR-050–055, P8).
- **Budget discipline:** mandatory sections are never silently truncated
  (FR-035, P9).
- **Transport:** stdio only; no listener exists to attack remotely (ADR-014).
- **Logs/traces:** sanitized; class-level failure info without personal
  content (NFR-014, P5).

## 6. Forget/deletion semantics

`revoke` (immediate stop) → `logical forget` (tombstone; never resolved) →
`derived purge` (projection stores, FTS, caches, traces, pending observations) →
`physical purge` (canonical Git history/backups where feasible — RED,
intentionally irreversible, non-content anti-resurrection tombstone only, no
reactivation path). Restore/rollback never reactivates revoked/purged data
without deliberate reauthorization (FR-055, P7.5).

## 7. Non-goals (explicitly not claimed)

- No protection against a compromised OS or a deliberate same-user process
  with arbitrary shell/filesystem access, under `cooperative-local`.
- No guarantee downstream models obey returned items.
- No guarantee data stays on-device after a pack is sent to a cloud model.
- No universal impossibility proof: zero observed failures in the locked
  corpus is evidence for the defined threat set only.