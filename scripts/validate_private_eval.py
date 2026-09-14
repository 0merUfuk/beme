#!/usr/bin/env python3
"""Validate the private evaluation corpus against the golden-case schema.

Owner-gated (ADR-023): never runs in public CI; public CI has no dependency
on this script or its data. The corpus location is NOT hard-coded — the
operator supplies it via BEME_PRIVATE_EVAL_DIR.

Exit codes:
  0 — corpus found and every case validates
  1 — corpus found but one or more cases fail validation
  3 — corpus unavailable (explicit not_run; never silently passes)
"""
import os
import sys
from pathlib import Path

try:
    import yaml
    from jsonschema import Draft202012Validator
except ImportError:
    print("FATAL: pyyaml/jsonschema not installed", file=sys.stderr)
    sys.exit(2)

ROOT = Path(__file__).resolve().parent.parent
SCHEMA = ROOT / "schemas" / "evaluation" / "golden-case.schema.json"

def main() -> int:
    corpus_dir = os.environ.get("BEME_PRIVATE_EVAL_DIR", "").strip()
    if not corpus_dir:
        print("private eval: not_run — BEME_PRIVATE_EVAL_DIR not set "
              "(owner-gated; public CI must not depend on this check)")
        return 3
    p = Path(corpus_dir).expanduser()
    if not p.is_dir():
        print(f"private eval: not_run — corpus directory not found: {p}")
        return 3

    cases = sorted(p.glob("*.yaml")) + sorted(p.glob("*.yml")) + sorted(p.glob("*.json"))
    if not cases:
        print(f"private eval: not_run — no case files in {p}")
        return 3

    with open(SCHEMA) as f:
        schema = __import__("json").load(f)
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema)

    passed, failed = 0, []
    for f in cases:
        try:
            if f.suffix == ".json":
                import json
                doc = json.loads(f.read_text())
            else:
                doc = yaml.safe_load(f.read_text())
        except Exception as e:
            failed.append(f"{f.name}: parse error: {e}")
            continue
        errs = list(validator.iter_errors(doc))
        if errs:
            failed.append(f"{f.name}: " + "; ".join(e.message for e in errs[:2]))
        else:
            passed += 1

    print(f"private eval: {passed} passed, {len(failed)} failed, {len(cases)} total ({p})")
    for msg in failed:
        print("  FAIL", msg)
    return 1 if failed else 0

if __name__ == "__main__":
    sys.exit(main())