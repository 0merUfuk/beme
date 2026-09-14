#!/usr/bin/env python3
"""Validate all Be Me contract fixtures against the versioned schemas.

WP3 exit gate: schemas validate all seed fixtures, and the elevation-negative
fixtures must be REJECTED by the request schema (the schema cannot represent
model-selected authority elevation).
"""
import json
import sys
from pathlib import Path

try:
    from jsonschema import Draft202012Validator
except ImportError:
    print("FATAL: jsonschema not installed", file=sys.stderr)
    sys.exit(2)

ROOT = Path(__file__).resolve().parent.parent
SCHEMAS = ROOT / "schemas"
TESTDATA = ROOT / "testdata"

passed, failed = [], []

def load(p):
    with open(p) as f:
        return json.load(f)

def check(name, ok, detail=""):
    (passed if ok else failed).append((name, detail))
    print(f"{'PASS' if ok else 'FAIL'}  {name}" + (f"  — {detail}" if detail else ""))

def validate(schema_path, doc, name):
    try:
        schema = load(schema_path)
        Draft202012Validator.check_schema(schema)
        errs = sorted(Draft202012Validator(schema).iter_errors(doc), key=lambda e: e.path)
        if errs:
            check(name, False, "; ".join(f"{'/'.join(map(str, e.absolute_path)) or '<root>'}: {e.message}" for e in errs[:3]))
        else:
            check(name, True)
    except Exception as e:
        check(name, False, f"exception: {e}")

# --- positive: record schema vs seed records
records = load(TESTDATA / "synthetic" / "records.json")
for r in records:
    validate(SCHEMAS / "record" / "normalized-record.schema.json", r, f"record {r['record_id']}")

# --- positive: provenance
provs = load(TESTDATA / "synthetic" / "provenance.json")
for p in provs:
    validate(SCHEMAS / "record" / "provenance.schema.json", p, f"provenance {p['provenance_id']}")

# --- positive: context pack
validate(SCHEMAS / "context-pack" / "context-pack.schema.json",
         load(TESTDATA / "synthetic" / "context-pack.json"), "context pack fixture")

# --- positive: source descriptor (synthetic canonical-knowledge source)
source_fixture = {
    "schema_version": "1",
    "source_id": "knowhow-synthetic",
    "type": "git_repository",
    "root": "<configured-root>/knowhow-synthetic",
    "purpose": ["canonical_foundation", "reusable_knowledge"],
    "trust": "canonical",
    "instruction_semantics": "registered_files_only",
    "authority_ceiling": "default",
    "sensitivity": "personal_private",
    "profiles_allowed": ["personal"],
    "ingestion_mode": "index_content",
    "include": ["INDEX.md", "knowledge/entries/**/*.md"],
    "exclude": ["**/.env*", "**/*.{pem,key,p12}"],
    "refresh": {"informational_content": "explicit_or_on_change", "normative_content": "approval_required"},
    "revision": {"kind": "git_commit", "approved_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000001"},
}
validate(SCHEMAS / "source" / "source-descriptor.schema.json", source_fixture, "source descriptor fixture")

# --- positive: workspace identity
ws_fixture = {
    "schema_version": "1",
    "workspace_id": "ws-synth-alpha",
    "canonical_roots": ["<configured-root>/project-synth-alpha"],
    "expected_remote_identity": None,
    "fingerprint": "fp-synth-alpha-01",
    "approved_revision_policy": {"pinned_commit": "0" * 40},
    "worktree_ids": [],
    "sensitivity_namespace": "work_restricted",
    "bound_project_policy": [
        {
            "path": "docs/DECISIONS.md",
            "approved_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000002",
            "scope": "repository",
            "authority_ceiling": "default",
        }
    ],
    "authority_ceiling": "default",
}
validate(SCHEMAS / "policy" / "workspace.schema.json", ws_fixture, "workspace fixture")

# --- negative: elevation attempts MUST be rejected by request schema
req_schema = load(SCHEMAS / "context-pack" / "resolution-request.schema.json")
Draft202012Validator.check_schema(req_schema)
validator = Draft202012Validator(req_schema)

elevation_attempts = load(TESTDATA / "injection" / "regression-cases.json")
for case in elevation_attempts:
    if case["case"].startswith("P1-"):
        errors = list(validator.iter_errors(case["input"]))
        check(f"elevation rejected: {case['case']}", len(errors) > 0,
              "schema ACCEPTED an elevation attempt" if not errors else "")

# --- negative: request with clean shape must validate
ok_request = {"schema_version": "1", "task": "Decide storage", "workspace_hint": "/x", "risk_hint": "medium", "budget_hint_tokens": 3000}
errs = list(validator.iter_errors(ok_request))
check("clean request validates", not errs, "; ".join(e.message for e in errs[:2]))

# --- WP3 gate: golden-case schema validates a private candidate shape (synthetic stand-in)
gc_synthetic = {
    "id": "decision.architecture.storage.embedded-vs-server.synthetic.v1",
    "schema_version": "1",
    "status": "candidate",
    "gold_type": "historical_truth",
    "category": "architecture",
    "risk": "medium",
    "capability": "work-safe",
    "harness": None,
    "scenario": {"task": "Choose storage for a single-writer local batch tool.", "workspace": "ws-synth-alpha", "repository_fixture": "fix-synth-01", "excluded_information": ["gold_answer"]},
    "gold": {
        "acceptable_decisions": ["embedded_db", "embedded_with_migration_path"],
        "unacceptable_decisions": ["client_server_db_by_convention"],
        "mandatory_conclusions": ["storage follows actual constraint profile"],
        "prohibited_conclusions": ["the user dislikes client-server databases"],
        "expected_unknowns": ["preferred SQL dialect"],
        "required_evidence_refs": ["engineering.storage.single-writer-local-batch"],
    },
    "provenance": {"approved_by_user": False, "evidence_refs": ["synthetic-adr-013"], "valid_at": "2026-09-14"},
    "grading": {"rubric": "0-4 blind paired", "repeats": 3},
}
validate(SCHEMAS / "evaluation" / "golden-case.schema.json", gc_synthetic, "golden-case synthetic fixture")

# --- private candidate corpus: validate every YAML case against golden-case schema (if pyyaml available)
private_dir = Path.home() / ".config" / "beme" / "evals" / "omer-regression-pack" / "candidates"
try:
    import yaml
    n = 0
    for f in sorted(private_dir.glob("*.yaml")):
        with open(f) as fh:
            doc = yaml.safe_load(fh)
        # strip comment header lines handled by yaml parser already
        validate(SCHEMAS / "evaluation" / "golden-case.schema.json", doc, f"private candidate {f.name}")
        n += 1
    if n == 0:
        check("private candidates found", False, f"none in {private_dir}")
except ImportError:
    check("private candidates validated", False, "pyyaml missing")

print(f"\n{'=' * 50}\n{len(passed)} passed, {len(failed)} failed")
sys.exit(1 if failed else 0)