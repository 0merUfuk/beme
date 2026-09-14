# AGENTS.md — Be Me Repository Guardrails

Working in this repository means executing the ratified blueprint phases in
order. This file sets the operating rules; the product contract lives in
`docs/`.

## Read order (before writing anything)

1. [`README.md`](README.md)
2. [`docs/PROJECT_CONTEXT.md`](docs/PROJECT_CONTEXT.md) — product definition, non-goals, ownership boundaries
3. [`docs/ROADMAP.md`](docs/ROADMAP.md) — work packages, gates, current position
4. [`docs/REQUIREMENTS.md`](docs/REQUIREMENTS.md) — per-ID traceability matrix
5. [`docs/DECISIONS.md`](docs/DECISIONS.md) — ADR ledger and decision rights
6. [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md)
7. [`docs/HANDOFF.md`](docs/HANDOFF.md) — current execution snapshot and exact next step

## Hard rules

1. **No private data in this repository.** No real names, personal paths,
   private repository names/commits, credentials, or real evaluation cases.
   Deployment-specific data lives outside the repo, under the private
   deployment directory. The confidential planning baseline at the repo root is
   git-ignored and must never be committed, quoted, or published.
2. **Respect the implementation gate.** Production resolver, storage, MCP, and
   adapter code is not authorized until the evaluation corpus freeze
   (WP2B) is complete. Contract/schema/eval work is the authorized scope
   before that gate.
3. **Never duplicate canonical sources.** Be Me reads external canonical
   sources through adapters; it never copies them into a second authoritative
   store, and repository docs never restate them as if authoritative here.
4. **Evidence beats prose.** Completion claims require the actual command
   output. A passing unit test is not end-to-end evidence when the acceptance
   criterion is end-to-end.
5. **Conventional commits.** `feat:`, `fix:`, `docs:`, `test:`, `chore:`,
   `refactor:`, `ci:`, `perf:`. One logical change per commit. No AI
   attribution lines.
6. **Preserve unrelated changes.** Never reset, clean, or force-push branches
   that may contain other work.

## Decision rights (GREEN / YELLOW / RED)

- **GREEN** — low-risk, local, reversible: decide, implement, verify, record.
- **YELLOW** — material but reversible: research, pick one approach, record
  assumption + rollback in `docs/DECISIONS.md`, proceed. Do not send option
  menus to the user.
- **RED** — stop and request a narrow user decision:
  - irreversible/destructive actions (physical purge, history rewrite);
  - privacy scope widening or declassification approvals;
  - canonical promotion, authority or global-scope changes;
  - credentials, paid services, or new external accounts;
  - public release, external messages, license commitments;
  - unresolved equal-authority material conflicts after research;
  - golden-decision approval in the evaluation corpus.

## After completing work

- Run `make validate` and include the actual output in your report.
- Update `docs/HANDOFF.md` (status, completed/not-completed, do-not-redo,
  next step, evidence).
- Update `docs/DECISIONS.md` for any new decision, with alternatives,
  consequences, reversibility, and reopen conditions.
- Update `docs/REQUIREMENTS.md` evidence links for any requirement you
  advanced.