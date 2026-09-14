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

**Alpha.** `v0.1.0-alpha.1` is released: contracts, runtime, narrow MCP
surface, CLI, adapter contracts, and the learning-review pipeline are
implemented and tested. See [`docs/HANDOFF.md`](docs/HANDOFF.md) §1 for the
canonical current status and [`docs/ROADMAP.md`](docs/ROADMAP.md) for
work-package state. Known alpha limitations (advisory-only adapter
assurance, no live-model behavioral evaluation yet, Windows
ported-unverified) are listed in the
[release notes](https://github.com/0merUfuk/beme/releases/tag/v0.1.0-alpha.1).

```
beme/
├── docs/        # constitution, requirements, architecture, decisions, evaluation
├── schemas/     # versioned JSON contracts (source, record, policy, pack, eval)
├── internal/    # Go runtime: policy, resolver, storage, ingestion, workspace, learning
├── cmd/beme/    # CLI + MCP stdio server
├── adapters/    # harness adapter assets (Codex, Claude Code, Hermes, Cursor)
├── evals/       # public evaluation assets (synthetic cases, privacy invariants)
├── testdata/    # synthetic fixtures + negative/injection fixtures
├── scripts/     # contract validation tooling
└── examples/    # synthetic deployment examples
```

## Quick start (validation from a clean checkout)

```sh
# 1. Python deps for contract validation (Python 3.11+; isolated venv —
#    never modify your system Python):
python3 -m venv .venv
.venv/bin/pip install jsonschema pyyaml

# 2. Public contract validation (repo-local; runs on any clean checkout):
PATH="$PWD/.venv/bin:$PATH" make validate

# 3. Go build + tests (Go 1.25+):
go build ./... && go test ./...
```

If your system `python3` is older than 3.11, use any Python 3.11+
interpreter explicitly (e.g. `python3.13 -m venv .venv`). See
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) for the full environment
guide and the owner-gated private-evaluation workflow.

## Documentation

Start at [`docs/PROJECT_CONTEXT.md`](docs/PROJECT_CONTEXT.md). Read order and
contribution rules are in [`AGENTS.md`](AGENTS.md) and
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).

## License

[MIT](LICENSE). This project is in a pre-release state; no compatibility
commitments exist yet.