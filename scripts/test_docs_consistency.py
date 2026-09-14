#!/usr/bin/env python3
"""Documentation-consistency regression suite.

Guards against the failure classes found by the first fresh-agent
documentation test (2026-09-14), which FAILED the documentation gate:

  D1. Lifecycle/phase contradictions — documents claiming "pre-implementation",
      "gated behind WP2B", "Blocked on user", "Not authorized" while the
      engine has shipped. Canonical status lives in docs/HANDOFF.md §1;
      every other doc must derive from it.
  D2. Stale hard-coded evidence counts — prose claims about fixture-check or
      test counts drift as code changes. Evidence must be command-derived.
  D3. Public/private validation semantics — no document may reintroduce
      private-corpus validation into the public `make validate` path
      (ADR-023; the original v0.1.0-alpha CI failure).
  D4. Validation bootstrap must be documented — DEVELOPMENT.md must contain
      a venv-based bootstrap (python3 -m venv; jsonschema pyyaml) so a clean
      checkout works exactly as documented.
  D5. Link integrity — relative markdown links inside docs/ and README.md
      must resolve to existing files.
  D6. The canonical status anchor — HANDOFF.md §1 must exist and be
      referenced by the other lifecycle docs.

Exits 1 on any violation (CI gate).
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

results = []

def check(name, ok, detail=""):
    results.append((name, ok, detail))
    print(f"{'PASS' if ok else 'FAIL'}  {name}" + (f"  — {detail}" if detail else ""))

# --- D1: phase contradictions (docs + README only; ADR evidence quotes exempt)
PHASE_LIES = [
    r"Contracts phase", r"pre-implementation",
    r"gated behind WP2B", r"Blocked on user", r"Not authorized until WP2B",
]
scan_files = [ROOT / "README.md"] + sorted((ROOT / "docs").glob("*.md"))
violations = []
for f in scan_files:
    text = f.read_text()
    for pat in PHASE_LIES:
        for m in re.finditer(pat, text, re.IGNORECASE):
            # Exemption: DECISIONS.md/ADR-023 records the historical CI
            # failure and ADR texts verbatim; historical evidence quotes are
            # allowed only there.
            if f.name == "DECISIONS.md":
                continue
            line = text[: m.start()].count("\n") + 1
            violations.append(f"{f.name}:{line}: '{m.group(0)}'")
check("D1 no phase contradictions outside ADR evidence", not violations, "; ".join(violations[:4]))

# --- D2: stale hard-coded counts (prose promises)
# Forbidden: specific test/fixture counts in prose. Allowed: command-derived
# evidence or version references.
COUNT_PATTERNS = [
    (r"\b\d{2,} tests? PASS across \d+ packages", "hard-coded test-count claim"),
    (r"\bfixtures? \d{2}/\d{2}\b", "hard-coded fixture-count claim"),
    (r"\b\d{2}/\d{2} (pass|PASS)\b", "hard-coded pass-count claim"),
]
violations = []
for f in scan_files:
    text = f.read_text()
    for pat, why in COUNT_PATTERNS:
        for m in re.finditer(pat, text):
            line = text[: m.start()].count("\n") + 1
            # ADR-023's consequence note cites the public fixture count once,
            # as of that ADR, in evidence context. Allow DECISIONS.md.
            if f.name == "DECISIONS.md":
                continue
            violations.append(f"{f.name}:{line}: {why}: '{m.group(0)}'")
check("D2 no stale hard-coded counts in prose", not violations, "; ".join(violations[:4]))

# --- D3: public validate must stay repo-local (no private-corpus coupling)
d3_bad = []
vc = (ROOT / "scripts" / "validate_contracts.py").read_text()
for pat in [r"Path\.home", r"regression-pack", r"BEME_PRIVATE_EVAL_DIR"]:
    if pat == r"Path\.home" or pat == r"regression-pack":
        if re.search(pat, vc):
            d3_bad.append(f"validate_contracts.py contains {pat}")
# DEVELOPMENT/README must describe validate as repo-local
dev = (ROOT / "docs" / "DEVELOPMENT.md").read_text()
if "private candidate corpus if present locally" in dev:
    d3_bad.append("DEVELOPMENT.md still documents private-corpus coupling")
# make validate must not invoke the private validator
mk = (ROOT / "Makefile").read_text()
validate_body = re.search(r"^validate:\n((?:\t.*\n)+)", mk, re.M)
if validate_body and "private" in validate_body.group(1):
    d3_bad.append("Makefile validate target touches the private validator")
# and the private validator must not_run when unset (behavioral, also in
# test_ci_regression.py R3; here check the contract statically)
pv = (ROOT / "scripts" / "validate_private_eval.py").read_text()
if "not_run" not in pv or "BEME_PRIVATE_EVAL_DIR" not in pv:
    d3_bad.append("private validator lost not_run/env-gate semantics")
check("D3 public validation repo-local; private eval owner-gated + not_run", not d3_bad, "; ".join(d3_bad))

# --- D4: bootstrap documented
dev = (ROOT / "docs" / "DEVELOPMENT.md").read_text()
need = ["python3 -m venv", "jsonschema", "pyyaml", "3.11", "make validate"]
missing = [n for n in need if n not in dev]
readme = (ROOT / "README.md").read_text()
if "make validate" not in readme:
    missing.append("README quick start")
check("D4 validation bootstrap documented (venv, deps, version, commands)", not missing, f"missing: {missing}")

# --- D5: relative link integrity
broken = []
doc_files = [ROOT / "README.md", ROOT / "AGENTS.md"] + sorted((ROOT / "docs").glob("*.md"))
for f in doc_files:
    base = f.parent
    text = f.read_text()
    for m in re.finditer(r"\[[^\]]*\]\((?!http|#|mailto:)([^)#\s]+)", text):
        target = m.group(1)
        if not (base / target).exists() and not (ROOT / target).exists():
            broken.append(f"{f.name}: {target}")
check("D5 relative links resolve", not broken, "; ".join(broken[:5]))

# --- D6: canonical status anchor
handoff = (ROOT / "docs" / "HANDOFF.md").read_text()
anchor_ok = re.search(r"^## 1\. Status", handoff, re.M) is not None
refs = [f.name for f in [ROOT / "README.md", ROOT / "docs" / "PROJECT_CONTEXT.md", ROOT / "docs" / "ROADMAP.md"]
        if "HANDOFF.md" not in f.read_text()]
check("D6 HANDOFF §1 status anchor exists and is referenced", anchor_ok and not refs,
      f"anchor={anchor_ok}; docs without HANDOFF reference: {refs}")

passed = sum(1 for _, ok, _ in results if ok)
failed = [n for n, ok, _ in results if not ok]
print(f"\n{passed}/{len(results)} documentation-consistency checks passed")
if failed:
    print("FAILED:", failed)
sys.exit(1 if failed else 0)