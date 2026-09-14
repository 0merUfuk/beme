.PHONY: validate help

help:
	@echo "validate — validate all contract fixtures against versioned schemas"

validate:
	python3 scripts/validate_contracts.py