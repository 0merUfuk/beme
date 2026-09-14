# Be Me — Contributing

## Ground rules

1. Read `AGENTS.md` and `docs/PROJECT_CONTEXT.md` first; the ownership
   boundaries and decision rights are binding.
2. `make validate` and `go test ./...` must pass before any commit.
3. Conventional commits (`feat:`, `fix:`, `docs:`, `test:`, `chore:`,
   `refactor:`, `ci:`, `perf:`). One logical change per commit. No AI
   attribution lines.
4. Never add private data (real names, paths, repositories, evaluation
   cases) to this repository. Synthetic fixtures only.
5. Schema changes require fixture updates + validator coverage in the same
   commit; schemas are contracts.
6. New decisions go in `docs/DECISIONS.md` with alternatives, evidence,
   consequences, reversibility, and reopen conditions.
7. Security-relevant changes must extend the regression tests (see
   `evals/EVALUATION_CONTRACT.md` §7 invariants).

## Development

```sh
make validate      # contract fixtures vs schemas
go test ./...     # engine tests
go vet ./...
```

See `docs/DEVELOPMENT.md` for layout and conventions.
