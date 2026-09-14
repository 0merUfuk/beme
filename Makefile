.PHONY: help validate validate-private test build vet fmt scan check

help:
	@echo "validate          — public contract fixtures vs schemas (repo-local only, CI-safe)"
	@echo "validate-private  — private eval corpus vs golden-case schema (owner-gated; not_run if unset)"
	@echo "test / build / vet — Go toolchain gates"
	@echo "scan              — private-data leakage scan (public patterns + local terms)"
	@echo "check             — full public gate: validate + build + vet + test + scan"

# Public contract validation. Repo-local only: no home-directory reads, no
# private corpus dependency (ADR-023). Safe on any clean checkout/CI runner.
validate:
	python3 scripts/validate_contracts.py

# Owner-gated private evaluation. Explicitly NOT part of CI: unset
# BEME_PRIVATE_EVAL_DIR yields exit 3 (not_run), never a silent pass and
# never a public-CI failure.
validate-private:
	python3 scripts/validate_private_eval.py

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

fmt:
	gofmt -l . && test -z "$$(gofmt -l .)"

scan:
	python3 scripts/scan_private_data.py

check: validate build vet test scan