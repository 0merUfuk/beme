# Be Me — Threat Model

> Data-flow, attacker, boundary, misuse, mitigation, and non-goal analysis
> for the implemented runtime. The 30 ship-blocking regression cases map
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

Implementation (ADR-027): the durable ledger lives under the canonical root,
outside derived data. Purge tombstones are keyed HMAC fingerprints of record
identity only, so a leaked ledger cannot confirm guessed private content or
IDs without the separately stored key. A purge that fails part-way leaves a
content-free journal; re-running it completes every remaining deletion, and
completed purges are idempotent. Provenance is removed through the record's
actual refs, never an ID convention. Misuse and failure cases covered: an
interrupted purge stranding derived copies, resurrection through
multi-provenance records, and offline dictionary tests against the ledger
(corpus cases S1–S3).

Read surfaces (ADR-027 §7, ADR-029): resolve, export, explain, doctor, MCP
status counts, and context-item expansion all apply the ledger at read time,
so a restored pre-purge backup cannot surface purged or revoked records in
content, provenance, counts, or metadata, and an unusable ledger or purge key
fails every one of them closed (case S4). Expansion is bound to a pack the
same session issued: guessed, replayed, unselected, expired, rebuilt, and
revoked combinations share one refusal (case 24). The ledger and journal
cross a documented durability boundary (file and parent-directory flush;
write-through rename on Windows) before any erasure; storage that
acknowledges flushes it does not perform is outside that guarantee.
Residuals: restored observations are hidden by identity (ADR-030); a
paraphrase recreated under a new identity cannot be matched to purged content
(the ledger holds none). Refusal timing is not equalized.

## 7. Non-goals (explicitly not claimed)

- No protection against a compromised OS or a deliberate same-user process
  with arbitrary shell/filesystem access, under `cooperative-local`.
- No guarantee downstream models obey returned items.
- No guarantee data stays on-device after a pack is sent to a cloud model.
- No universal impossibility proof: zero observed failures in the locked
  corpus is evidence for the defined threat set only.

## 8. Brain boundary (V2)

Brain memory is a separate non-canonical CLI surface (ADR-033–035), not a new
MCP tool or a new source in either V1 projection. Agent-authored notes are
untrusted evidence. Capture/Doctor/briefing screens are heuristic; a plausible
false note can pass. A same-user filesystem writer can bypass capture. Review
notes and identity pointers before giving a briefing to a cloud harness.

Brain logs, archive and backups retain content; the V1 purge ledger does not
cover them. Checksums are corruption checks, not authenticated snapshots.
Unix file modes do not establish Windows ACL isolation. Fail-open session
startup means context may be unavailable, not that V1 capability or project
trust expands. Synthetic regression coverage is not proof of live use or
protection against every injection or filesystem race.

ADR-036 adds component-aware no-symlink/regular-file guards for identity,
metadata and notes, scope selection before content loading, scan and byte
budgets, complete registration/bookkeeping freeze/schema gates and atomic
no-clobber proposals. Cooperative-local pathname checks cannot defeat a
hostile same-user process replacing components concurrently. Scan deadlines
are checked between operations, not preemptions of stalled filesystem calls.

Restore stages checksum-verified bytes before retention or live mutation.
Ordinary install errors roll back and preserve recovery data on rollback
failure. Entry-wise installation is not a whole-tree power-loss transaction;
operator recovery may be necessary after process/power interruption. Snapshots
and logs retain content independently of the V1 purge boundary.

## 9. Evidence integrity and optional observability

Generated ContextPacks and independently imported grades must be distinguished
from real task benefit. ADR-037/038 bind delivered evidence and frozen corpus
bytes, reject absent/invalid review coverage, and keep safety blockers separate
from aggregate scores. Approval and independent-review declarations are not
identity-authenticated signatures; application-level exclusive/hashed bundles
are not OS tamper-proof storage. Provider/network declarations are not a
sandbox or proof the baseline lacked other memory/tools.

Work-safe output withholds correlatable policy/index/source fingerprints and
sorts provenance by visible fields only. Trace-cache failure advertises no
usable handle and returns class-level reduced assurance rather than inventing
observability. Brain hook invocation, harness delivery, agent use and causal
benefit remain separate evidence levels.
