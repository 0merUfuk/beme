# Be Me — Development Guide

> How to work in this repository. Read `AGENTS.md` first for guardrails.

## Environment

- Go ≥ 1.25 (engine; ADR-004).
- Python ≥ 3.11 with `jsonschema` and `pyyaml` for contract validation.
- SQLite 3.54+ tooling optional (inspecting derived stores).

## Validation environment bootstrap (clean checkout)

The public validation path is **repo-local only**: it reads no file outside
the repository and never requires the private evaluation corpus (ADR-023).

```sh
# 1. Isolated environment for validation deps — never modify your system
#    Python. Requires Python 3.11+:
python3 -m venv .venv
.venv/bin/pip install jsonschema pyyaml

# 2. Public contract validation (repo-local; safe on any clean checkout
#    and on CI runners):
PATH="$PWD/.venv/bin:$PATH" make validate

# 3. Go gates:
go build ./... && go vet ./... && go test ./...

# 4. Public/private separation regression suite + private-data scan:
PATH="$PWD/.venv/bin:$PATH" python3 scripts/test_ci_regression.py
python3 scripts/scan_private_data.py
```

**If your system `python3` is older than 3.11:** the venv step fails. Use
any Python 3.11+ interpreter explicitly instead, e.g.
`python3.13 -m venv .venv` (Homebrew: `brew install python@3.13`). Do not
`pip install --user` into a system Python.

**Private evaluation (owner-gated, never in public CI):**

```sh
# Same venv PATH prefix as every other Python gate:
PATH="$PWD/.venv/bin:$PATH" make validate-private
# with BEME_PRIVATE_EVAL_DIR unset → prints
#   "private eval: not_run — …" (validator exit 3; never a silent pass).
# Note: make wraps recipe failure as its own exit 2 and shows "Error 3" —
# the guaranteed semantic is the explicit not_run message + the validator's
# exit 3, both shown above.
```

The private corpus lives on the owner's machine only; its validation
reports an explicit `not_run` (exit 3) when unavailable — it never
silently passes and never blocks public CI. This behavior is itself
regression-tested (R3/R3b in `scripts/test_ci_regression.py`).

The public validator enforces the WP3 exit gate: positive fixtures must
pass, and elevation attempts (`profile`, `capability` fields) must be
**rejected** by the resolution-request schema.

## Layout

- `schemas/` — versioned JSON contracts. Changes require a schema_version
  bump or a reviewed in-place revision before any production code consumes
  them.
- `internal/` — Go runtime packages (policy, resolver, storage, ingestion,
  workspace, learning, projection, app). Domain logic never imports CLI,
  MCP, or storage-driver specifics (NFR-010).
- `cmd/beme/` — CLI entry point and MCP stdio server.
- `testdata/synthetic/` — public, fully synthetic fixtures. No real data.
- `testdata/injection/` — negative/injection regression fixtures.
- `evals/` — public evaluation assets. Real cases live privately (never here).
- `scripts/` — validation tooling, including the CI regression suite.
- `docs/` — normative project docs (see ownership rules in
  `docs/PROJECT_CONTEXT.md` §7).

## Documentation-consistency rules

- `docs/HANDOFF.md` §1 is the **single canonical current-status section**.
  Other documents reference it; they must not maintain independent phase
  descriptions.
- Prefer evidence-linked references over hard-coded counts: "public fixture
  checks via `make validate`", "test inventory via `go test -list '.*' ./...`".
  Where a count is useful, derive it from the command's actual output.
- `scripts/test_docs_consistency.py` (runs in CI) fails on the known stale
  patterns that broke the first fresh-agent documentation test: lifecycle
  phase contradictions, hard-coded evidence counts, and any doc
  reintroducing private-corpus validation into the public path. The exact
  pattern list lives in the script, not in this prose.

## Conventions

- Conventional commits (`feat:`, `fix:`, `docs:`, `test:`, `chore:`,
  `refactor:`, `ci:`, `perf:`). One logical change per commit.
- Schemas are the contract: prose describes them, never replaces them.
- Docs that duplicate canonical external content are forbidden — link, don't
  copy (blueprint rule; ADR-003).
- Every work package completion updates `docs/HANDOFF.md` with actual
  evidence (commands + output), per NFR-001.

## Adding a schema change

1. Draft/patch the schema in `schemas/<area>/`.
2. Add or update fixtures in `testdata/`.
3. Extend `scripts/validate_contracts.py` coverage.
4. `make validate` green → commit schema + fixtures + validator together.