# Project Context — Be Me

> **Layer 3 project document.** This file is the repository's own context: what
> Be Me is, what it owns, what it deliberately does not do, and the rules any
> implementing agent must follow. Blueprint-derived. Maintained alongside
> `docs/DECISIONS.md`.

**Status:** Contracts phase (WP0–WP3 contracts + eval contract + schema work).
Production engine implementation is gated behind WP2B (evaluation corpus
freeze + user approval). See `docs/ROADMAP.md`.

## 1. Definition

Be Me is a local-first personal execution-context runtime that resolves
explicit user principles, approved knowledge, trusted project policy, and
historical evidence into bounded, provenance-aware, capability-scoped context
for AI agents.

It is **not** a personality clone, a chat-history dump, a new coding agent, an
orchestrator, or a replacement for existing knowledge repositories.

## 2. Problem

AI coding agents start with incomplete context. The user repeatedly restates
preferences, explains quality standards, reconstructs decisions, points agents
at the right knowledge, and catches generic or contradictory recommendations.
Existing memory systems commonly:

1. dump too much history into the prompt;
2. treat observations as authoritative preferences;
3. ignore project and work/personal scope boundaries;
4. produce personalized prose without measurable decision improvement.

## 3. Ownership boundaries (anti-duplication contract)

| System | Owns | Does not own |
|---|---|---|
| Canonical knowledge repositories | Canonical operating model, approved reusable knowledge, governance, evidence maps | Runtime indexes, per-session context packs |
| Project repositories | Project policy, decisions, constraints, learnings | Global personal model |
| Session-continuity tooling (e.g. Rifja) | Agent-session continuity, project/worktree identity, episodic evidence, resume/export | Canonical personal operating model, cross-source decision resolution |
| **Be Me** | Source registration, capability/scope enforcement, normalization, retrieval, precedence, context packs, resolver traces, candidate-learning workflow | Raw source ownership, agent reasoning/execution, transcript parsing |

Be Me reads external sources through adapters without taking ownership, never
copies them into a second authoritative database, and builds only rebuildable
derived indexes.

## 4. Product rules

- **Evidence over invention.** Unsupported personal claims and
  inferred-to-authoritative promotions must be zero in the locked evaluation
  and privacy corpora.
- **Capability before retrieval.** Profile/scope enforcement occurs before
  retrieval. The model cannot elevate its profile or source access through
  MCP arguments; a session capability fixes the ceiling; requests may narrow,
  never widen.
- **Work-safe by construction.** Work-safe serving uses a separately built safe
  projection whose routine build and serving paths never open personal sources.
  Moving knowledge into the safe manifest is an explicit declassification
  event with its own provenance, never an in-place relabel.
- **External content is data.** Ingested content never executes; it cannot
  supply authority, profile, source type, or instruction semantics for itself.
- **Learning is quarantined.** Agent feedback creates observations only; promotion
  to canonical requires explicit trusted user action. Agent-generated
  recommendations the user did not accept are never evidence of preference.
- **Local-first, honestly stated.** The core works offline; when a cloud model
  is used, the selected pack may leave the machine. Documentation must never
  claim "data never leaves the device."
- **MVP optimizes decision quality, not style.** Success is measured decision
  improvement over a plain-agent baseline, not stylistic similarity.

## 5. Private/public boundary

The public engine repository must remain generic. User-specific paths, profile
data, conversations, repositories, and evaluation cases are private deployment
data, never engine defaults. Deployment data lives under the platform
user-config/data directories (see `docs/OPERATIONS.md` once written), never in
this repo. A clean-history and installed-artifact scan is required before any
publication (WP11 gate).

## 6. Non-goals

Vector DB/knowledge graph (v1), cloud service/sync, web dashboard,
multi-user/enterprise auth, style cloning, general-purpose policy DSL. See
`docs/DECISIONS.md` for the full rejected/deferred table with reasons.

## 7. Documentation ownership

- `README.md` — public start-here.
- `docs/HANDOFF.md` — current execution snapshot and recovery entrypoint.
- `docs/DECISIONS.md` — decision ledger: ratified/open/superseded, alternatives, evidence, reversibility, reopen conditions.
- `docs/ACCEPTANCE.md` — mandatory/prohibited conclusions, release gates.
- `docs/THREAT_MODEL.md` — data-flow, attacker, boundary, misuse, mitigation, non-goals.
- Generated docs/indexes are marked rebuildable and never canonical.