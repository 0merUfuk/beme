# Be Me

A local-first, harness-independent, provenance-aware **personal execution-context
runtime**. Be Me turns trusted personal operating principles, approved reusable
knowledge, project policy, and bounded historical evidence into scope-safe,
inspectable `ContextPack` objects for AI coding agents.

> Give an agent the right, authorized evidence about how you work and decide —
> without inventing preferences, leaking unrelated data, or making you restate
> context on every session.

## Project film

https://github.com/user-attachments/assets/78a1214e-08c6-47f5-8d94-e50f5df6cbd0

An illustrative project film about capability-bound evidence selection and a provenance-aware ContextPack for a coding decision. It uses reconstructed visuals and synthetic examples, not a live screen recording; adapter guidance remains advisory.

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

## V2: second brain

The same `beme` executable now includes the existing V2 plain-Markdown work
memory: decisions, open work, summaries and learnings; gated capture, Brain
Doctor, snapshots/restore/freeze, bounded briefings and optional harness hooks.
Notes remain non-canonical and **do not enter V1 ContextPacks**. The newer
runtime, privacy ledger, retrieval and evaluation tooling are preserved.
See [`docs/BRAIN.md`](docs/BRAIN.md), ADR-033–035, and
[`docs/HANDOFF.md`](docs/HANDOFF.md) §1 for candidate verification and limits.
Independent review and parent production-path replays closed the tracked audit
defects, including two corrective cycles (ADR-036–040). The accepted integration
is committed and proposed in [PR #3](https://github.com/0merUfuk/beme/pull/3).
A separately authorized local rollout verified installed CLI and fresh MCP
connections; that does not establish native model consumption or user benefit.
Generated answers remain `grading_pending` until independent grade import;
synthetic grades never establish live benefit. See
[`EVALUATION_USAGE.md`](docs/EVALUATION_USAGE.md) for the frozen-split and grading protocol.

## Status

**Alpha source version: `v0.1.0-alpha.2`.** Contracts, context runtime, narrow
MCP surface, CLI, learning review and non-canonical work memory are implemented.
The [GitHub releases](https://github.com/0merUfuk/beme/releases) list is the
authority for published versions and downloadable assets; a source version
alone is not a publication claim. See [`docs/HANDOFF.md`](docs/HANDOFF.md) §1
for current evidence and [`docs/ROADMAP.md`](docs/ROADMAP.md) for work-package
state. Adapters remain advisory and live-model benefit is unmeasured. Release
verification and the commit/push/versioned-package policy are documented in
[`RELEASE_VERIFICATION.md`](docs/RELEASE_VERIFICATION.md).

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