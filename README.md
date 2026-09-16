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
work-package state. Known alpha.1 limitations (advisory-only adapter
assurance, no live-model behavioral evaluation yet) are listed in the
[release notes](https://github.com/0merUfuk/beme/releases/tag/v0.1.0-alpha.1);
since alpha.1 the full test suite also runs on Windows in CI (ADR-028).

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

## Install and first deployment

```sh
# Install the binaries (Go 1.25+):
go install ./cmd/beme ./cmd/beme-bench ./cmd/beme-eval ./cmd/beme-threat-corpus
export PATH="$(go env GOPATH)/bin:$PATH"   # or set GOBIN to a directory on PATH

beme doctor            # health check; it names what is missing
```

A fresh deployment has no sources: registration is a file-authoring act (it
is the trust act — Be Me has no command that registers a source for you). See
[**Registering a source**](docs/OPERATIONS.md#registering-a-source) in
[`docs/OPERATIONS.md`](docs/OPERATIONS.md), which also covers the lifecycle
(`build`, `preview`, `forget`, `purge`, `candidate`), platform directories,
and ledger backup and key custody. Harness/MCP setup is in
[`docs/INTEGRATIONS.md`](docs/INTEGRATIONS.md).

## Documentation

Start at [`docs/PROJECT_CONTEXT.md`](docs/PROJECT_CONTEXT.md). Read order and
contribution rules are in [`AGENTS.md`](AGENTS.md) and
[`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).

## License

[MIT](LICENSE). This project is in a pre-release state; no compatibility
commitments exist yet.