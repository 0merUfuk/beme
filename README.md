# Be Me

A local-first, harness-independent, provenance-aware **personal execution-context
runtime**. Be Me turns trusted personal operating principles, approved reusable
knowledge, project policy, and bounded historical evidence into scope-safe,
inspectable `ContextPack` objects for AI coding agents.

> Give an agent the right, authorized evidence about how you work and decide —
> without inventing preferences, leaking unrelated data, or making you restate
> context on every session.

## What Be Me is

- A **context runtime**: it resolves *what an agent should know before a
  material decision* into a versioned, provenance-aware JSON pack.
- **Capability-bound**: privacy is bound to immutable serving capabilities
  (`personal`, `work-safe`), never to model requests.
- **Two-stage**: hard policy/scope eligibility runs first; relevance ranking
  second. The model can never widen its own authority.
- **Local-first**: no cloud service is required; MCP is served over stdio only.

## What Be Me is not

- Not a personality clone or style emulator.
- Not an agent, orchestrator, or reasoning loop.
- Not a chat-history dump, transcript importer, vector database, or knowledge
  graph.
- Not a replacement for your canonical knowledge repositories or session-
  continuity tooling — Be Me reads them through adapters and never takes
  ownership.

## Status

**Contracts phase — pre-implementation.** The engine design, trust model,
requirement traceability, decision ledger (ADR-001…021), evaluation contract,
and versioned JSON schemas are defined and fixture-validated. Engine code
begins only after the evaluation corpus freeze (see `docs/ROADMAP.md`).

```
beme/
├── docs/        # constitution, requirements, architecture, decisions, evaluation
├── schemas/     # versioned JSON contracts (source, record, policy, pack, eval)
├── testdata/    # synthetic fixtures + negative/injection fixtures
├── evals/       # public evaluation assets (synthetic cases, privacy invariants)
├── scripts/     # contract validation tooling
└── examples/    # synthetic deployment examples
```

## Quick start (contracts validation)

```sh
make validate     # validates all fixtures against the versioned schemas
```

## Documentation

Start at [`docs/PROJECT_CONTEXT.md`](docs/PROJECT_CONTEXT.md). Read order and
contribution rules are in [`AGENTS.md`](AGENTS.md) and
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).

## License

[MIT](LICENSE). This project is in a pre-release state; no compatibility
commitments exist yet.