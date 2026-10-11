# Be Me V2 — Second Brain (architecture note and usage flow)

> Short note for operators and agents. Decisions: ADR-033 (MVP), ADR-034
> (completion), ADR-035 (`briefing` read-only fix) in [`DECISIONS.md`](DECISIONS.md).
> **Locally verified source candidate; independent review and parent recheck complete.**
> ADR-036 corrects restore, equivalence, chronology, scope, bounded reads and
> accounting. Synthetic regressions exercise these production paths; installation,
> native harness delivery and real task benefit remain `not_run`/unmeasured.
> Canonical current status: [`HANDOFF.md`](HANDOFF.md) §1.

## What changed

V1 resolves *who you are and how you decide* (canonical, read-only). V2 adds
*what happened and what is open*: a plain-markdown **brain vault** that
carries decisions, work summaries, unfinished work, and learnings from one
agent session to the next.

```text
Agent ── beme brain capture ──▶ inbox/ (proposal) ──▶ gate ──▶ Brain Doctor ──▶ projects/<slug>/… (live)
                                   ▲                    │        │  snapshot first  ├─ archive/ (superseded · merged · done)
              refused: code, diff, trace, secret,       │        └─ log.md · index.md
              transcript, injection, no-why             ▼
                                             (rejected/ — pruned after 30 days)

SessionStart hook ──▶ doctor pass ──▶ bounded briefing (normal · economy · manual) ──▶ next session
PreCompact / SessionEnd hooks ──▶ doctor --quiet
```

| Piece | Where / command | Rule |
|---|---|---|
| Vault | `<data>/brain/` (`brain_dir`, `BEME_BRAIN_HOME`) | markdown + yaml only, 0700/0600 on Unix; Windows ACL privacy unverified, `brain.yaml` schema version; no code, no symlinks |
| Note types | `decision` (needs *why*) · `open` · `summary` · `learning` | one file each; frontmatter `id`, `type`, `project`, `topic`, `status`, `confidence` |
| Capture | `beme brain capture` | lock-free proposal; synchronous gate; agents never write live notes |
| Brain Doctor | `beme brain doctor [--dry-run] [--commit] [--strict]` | promote/reject, merge exact equivalence only, supersede by `topic` and capture time, newest 3 summaries, flags, `index.md`; snapshot before first change |
| Session start | `beme brain session-start --format claude-hook` | maintenance pass, then briefing; **fail-open** |
| Modes | `brain_mode` in `config.yaml` or `--mode` | `normal` ≤ 9 KB · `economy` ≤ 3 KB · `manual` two-line pointer |
| Manual pull | `beme brain briefing --mode normal [--doctor]` | true read by default (no writes; safe under a read-only sandbox); `--doctor` also promotes pending captures, best-effort — a failure there degrades to a notice, never fails the read |
| Audit trail | `log.md` | append-only, `## [date] op \| detail`; never injected |
| Reversibility | `beme brain backup / restore / freeze` | checksummed snapshots in `<vault>.backups/`; restore validated/staged before live replacement, with rollback on install error; freeze gates writers (explicit restore is a recovery exception) |
| Projects | `beme brain project add` | explicit longest real-path root first; verified reciprocal Git metadata for worktree aliases; ties fail closed |
| Operator view | `beme brain status` | counts, snapshots, frozen, identified sessions and separate invocations, last log line |

Implemented contracts with synthetic regression evidence (not live-use guarantees):

- **Selective persistence.** Only distilled outcomes enter; code, patches,
  traces, transcripts, secrets, and instruction-shaped text are refused at
  capture *and* re-checked by Doctor and by every briefing (identity card included).
- **No silent overwrite, no silent merge.** A new decision with an existing
  `--topic` supersedes the old one (history in `archive/`) — unless the old one
  is `confidence: validated` (edit the frontmatter by hand): then both stay and
  a read-only scope audit also flags the conflict. Only conservative exact
  equivalence of content/rationale/topic is a recapture; scalar changes, identifier
  case, different next steps and paraphrases are preserved for review. Recapture
  is not independent evidence. RFC3339Nano capture instants, not maintenance
  times or filename order, select the latest summary and topic revision.
  `last_captured` also preserves an exact recapture's semantic freshness across
  the 128-proposal inbox pages and retry reconciliation. Older/ambiguous revisions
  remain reviewable rather than retiring a later recapture. Completed merges
  retain archive receipts; live retry metadata holds only unresolved IDs (at most
  128), never an evicting lifetime deduplication window. Exhausted retry capacity
  defers merges. Successful note writes must remain readable within 64 KiB.
- **Scope-safe.** A briefing contains the matched project plus global decisions
  and learnings. New global `summary`/`open` captures are rejected. Unsupported
  legacy global project-state notes remain intact and are flagged without
  broadcasting their text or IDs. Scope is selected before note content reads.
- **Data, not instructions.** Notes are `informational`; the briefing says so and
  tells the agent to verify against the repo. Memory can be stale.
- **Capture pressure without blocking.** Identified startup/resume/fork events
  are deduplicated within a retained 512-ID hash window per project. Missing IDs
  and legacy counts are labelled invocations, not distinct sessions. The nudge
  reports starts since an accepted capture, not promotion, use or benefit.
  Default `briefing` remains write-free; hook-less tracking requires explicit
  `--record-session [--session-id ID]`. No `Stop` hook blocks work.
- **Recoverable.** Doctor snapshots before mutation. Restore pins verified input
  bytes, validates/stages everything before retention/live changes, snapshots the
  prior vault, then installs with rollback on ordinary filesystem errors. This
  is an entry-wise protocol, not power-loss-atomic whole-tree replacement; retain
  any recovery directory on rollback failure. `--commit` records changed runs
  in the vault repository only; retired notes are archived.
- **Hygiene flags** (recomputed each run): `conflict`, `overlap`, `stale_open`
  (30 d), `aging` (180 d), `cap_exceeded` (30), `broken_ref`, `lint`,
  `invalid_note`, `foreign_file`/symlink, `identity_pointer`, `identity_unsafe`,
  `unregistered_project`, `backup_failed`.

## Usage flow

```sh
# once — vault, project, hooks
beme brain init --git                          # vault + git (prints the vault path)
#   fill in <vault>/identity.md: a short role card + "## Pointers" to canonical docs (never copies)
beme brain project add myproj --root /abs/path/to/myproj
beme brain install-hooks --user --dry-run      # review, then run without --dry-run
#   merges SessionStart/PreCompact/SessionEnd into ~/.claude/settings.json (other settings untouched,
#   original saved as settings.json.beme-bak; --remove undoes it)
#   allow the agent to record in Claude Code permissions:
#     Bash(beme brain capture:*)  Bash(beme brain done:*)  Bash(beme brain archive:*)  Bash(beme brain show:*)
# hook-less harness (Codex, …): beme brain adapter install codex   (adds a managed instruction block)

# every session (automatic)
#   SessionStart → doctor pass + briefing injected → agent works → agent captures at task completion →
#   PreCompact/SessionEnd → doctor settles the vault

# by hand, when needed
beme brain capture --type decision --title "…" --body "…" --why "…" --topic storage
beme brain status        beme brain list --project myproj [--all]       beme brain show <id>
beme brain done <id>     beme brain archive <id> --reason "…"           beme brain doctor --dry-run
beme brain backup [--list]   beme brain restore <snapshot> --force   beme brain freeze on|off
```

## How V2 relates to the reference systems

| Reference mechanism | Status |
|---|---|
| Hook injection of identity, recent work, open tasks (Avenox, nanobrain) | adopted (Claude Code hooks; text/adapter fallback elsewhere) |
| Selective persistence + gatekeeper queue (dossier) | adopted: proposal → gate → Doctor |
| Brain Doctor: dedupe, contradictions, freshness (Avenox/Karpathy lint) | adopted, deterministic, no model calls |
| `index.md` with one-line summaries and `log.md` (Karpathy) | adopted |
| Snapshots, rollback, freeze, path/symlink guards (open-second-brain) | adopted (hash-chained ledgers declined) |
| Economy/normal/manual modes (Avenox) | adopted (bytes, not characters) |
| Backup-before-update (Avenox) | adopted as backup-before-mutate; vault is independent of the binary |
| Answer filing / compounding synthesis (Karpathy) | adopted as protocol guidance (`learning` = distilled conclusion) |
| Idle-time LLM distillation, `Stop`-hook raw queue (nanobrain) | **declined** (ADR-034): paid, transcript import, poisoning risk |
| Daily notes, rules, skills, reference docs as brain content (Avenox) | **declined**: the brain holds decisions, summaries, experience only |
| Vector/FTS search over the vault | declined (ADR-012) |
| Brain notes inside V1 resolver packs | deferred (ADR-034) |
| Codex/Hermes/Antigravity native hooks | unverified; hook-less instruction block provided — tells the agent to run `briefing --mode normal --doctor --record-session` at start and `doctor --quiet` at end, closing the promotion loop without a real hook (ADR-035) |

## Verification and limits

The adopted source runs in the same `beme` executable as the newer runtime.
`TestIntegratedBrainAndRuntime` drives capture → promotion → read in separate
processes, then uses that exact binary for positive retrieval, PR2 negative
relevance (no unrelated advisory context + explicit unknown), trace explain,
work-safe exclusion, and durable forget across rebuild. It also proves a
malformed V1 source registration does not block brain reads. Brain registration
never registers a V1 source; no brain content enters either V1 projection.
The evaluation runner remains the separate existing `beme-eval` command; its
B0–B4 and ablation implementation is preserved, not replaced with the old fork.

- Deterministic and real-binary tests are local synthetic acceptance, not proof
  of installed hook invocation, agent compliance, or improvement on real tasks.
  Current command results are in [`HANDOFF.md`](HANDOFF.md) §1.
- `scripts/brain_live_e2e.sh` (`make brain-live`) is an **owner-run model/quota
  action**, not a safe local smoke test. Stage 2 already invokes `claude -p`;
  stages 3–5 also invoke a real model. It was **not_run** for this integration.
  Historical source-branch live results are not candidate acceptance evidence.
- The vault is plain files: a same-user writer can bypass `capture`. The safety
  screen is heuristic, not a proof of truthful notes or an OS isolation boundary.
  Review content before passing it to a harness; a cloud model may receive it.
- Backup checksums detect corruption, not malicious replacement of both the
  archive and its checksum. Snapshots/logs/archive retain private content; they
  are independent of the V1 physical-purge ledger and require their own custody.
- Capture remains advisory. Identified sessions and invocation counts are
  operational indicators, never task-level compliance, recall, use or benefit.
  IDs evicted from the 512-entry window may count again. The 30-note flag is a
  warning; independent regular-file byte/enumeration/time limits bound scans.
  A stalled filesystem syscall cannot be preempted by between-operation time checks.
- Capture uses 128-bit random suffixes and atomic no-clobber publication.
  Briefings run nonmutating scoped audits; warnings precede optional notes and
  survive economy budgets. Maintenance failure is a bounded class-level notice,
  never a false promotion. Identity and metadata share guarded no-symlink reads.
- Missing legacy schema metadata is distinct from invalid, linked, oversized,
  unreadable or newer metadata: the latter refuse writes, including partial
  `init` repairs. Unsupported global open/summary identifiers are withheld on
  every warning path, not just the scope warning.
- Each capture reserves its own accounting intent before publishing a proposal.
  Up to 128 fixed slots per project bound live bookkeeping; exhausted capacity
  refuses a new capture **before** publication, rather than evicting uncertainty.
  Completed slots are reclaimed, so ordinary lifetime captures do not accumulate
  live accounting history. This path never takes Doctor's maintenance lock.
- After publication, the owner marks only its intent completed. A successful
  reset reconciles completed intents observed under the accounting lock, writes
  state, then removes those intents under that same lock. It cannot acknowledge
  another still-in-flight publication. An already-acknowledged delayed reset
  does not erase newer session events. Readers check a reset epoch/snapshot and
  occupied slots without writing, scanning other projects, or reading note history.
- Once queued, a failed reset remains an accepted capture (exit 0), with
  `accounting_warning`, stderr notice, and `accounting_unknown_projects` in
  status. Briefings suppress false silence nudges. A later accepted capture can
  reconcile completed obligations, even after Doctor has moved the notes; it
  cannot clear a pending, partial, refused or interrupted intent. Do not retry
  an accepted capture merely to repair bookkeeping.
- Interrupted intents have no age-based eviction: a paused process might still
  publish. Such intents (or a mismatched legacy shared marker) remain unknown
  until inspected with **all capture processes stopped**, including older
  binaries. Reconcile actual accepted notes before any manual bookkeeping repair;
  deleting intents merely to silence a nudge is unsafe. Mixed-version concurrent
  writers and retroactive recovery of obligations lost by the old shared marker
  are not supported. This is process-interruption bookkeeping, not a new
  power-loss transaction or hostile same-user filesystem isolation guarantee.
- Generated hook/manual/protocol/adapter commands quote the running executable
  (or explicit wrapper) and configuration path. Manual pointers request
  `--mode normal` and retain `--binary` through capture and its next Doctor
  command; they do not loop back into manual mode. The SessionStart
  matcher includes `fork`, but native fork delivery has not been accepted here.
- New integrated runtime results on Linux/Windows and installed harnesses must
  be measured separately; cross-compilation is not execution. Unix permission
  tests do not establish Windows ACL privacy.
