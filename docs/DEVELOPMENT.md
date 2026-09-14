# Be Me — Development Guide

> How to work in this repository. Read `AGENTS.md` first for guardrails.

## Environment

- Go ≥ 1.25 (engine, once WP4+ begins; ADR-004).
- Python 3.11+ with `jsonschema` and `pyyaml` for contract validation.
- SQLite 3.54+ tooling optional (inspecting derived stores later).

## Validation (contracts phase)

```sh
make validate            # all fixtures vs versioned schemas; also validates
                         # the private candidate corpus if present locally
```

The validator enforces the WP3 exit gate: positive fixtures must pass, and
elevation attempts (`profile`, `capability` fields) must be **rejected** by
the resolution-request schema.

## Layout

- `schemas/` — versioned JSON contracts. Changes require a schema_version
  bump or a reviewed in-place revision before any production code consumes
  them.
- `testdata/synthetic/` — public, fully synthetic fixtures. No real data.
- `testdata/injection/` — negative/injection regression fixtures.
- `evals/` — public evaluation assets. Real cases live privately (never here).
- `scripts/` — validation tooling.
- `docs/` — normative project docs (see ownership rules in
  `docs/PROJECT_CONTEXT.md` §7).

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