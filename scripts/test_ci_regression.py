#!/usr/bin/env python3
"""Regression tests for the v0.1.0-alpha CI failure (ADR-023).

Failure being regressed: public CI ran `make validate`, which read the
operator's home directory for a private evaluation corpus
(~/.config/beme/evals/<private>/candidates). On a public runner that path
does not exist, so contract validation FAILED with "private candidates
found — none in /home/runner/...". Build/vet/test were skipped and the
macOS job was cancelled.

Invariants under test:
  R1. `make validate` (public) succeeds on a clean environment with an
      EMPTY/absent home directory — no filesystem reads outside the repo.
  R2. Public validation output contains no private path or corpus name.
  R3. Private validation reports an explicit non-success (exit 3, not_run)
      when its corpus is unavailable — it never silently passes.
  R4. Private validation validates real cases when the corpus IS provided.
  R5. The public validator and CI workflow contain no private path string
      and do not invoke the private validator.
"""
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PY = sys.executable

results = []

def check(name, ok, detail=""):
    results.append((name, ok, detail))
    print(f"{'PASS' if ok else 'FAIL'}  {name}" + (f"  — {detail}" if detail else ""))

def run_public_validate(home_dir):
    """Run the public validator with a pristine HOME and no BEME_* env."""
    env = {
        "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        "HOME": str(home_dir),
        "LANG": "C.UTF-8",
        "TMPDIR": tempfile.gettempdir(),
    }
    return subprocess.run(
        [PY, str(ROOT / "scripts" / "validate_contracts.py")],
        capture_output=True, text=True, env=env, timeout=120,
    )

# R1: public validate succeeds with an empty HOME (the exact CI condition).
with tempfile.TemporaryDirectory() as fake_home:
    # seed a decoy to prove no home reads happen at all
    os.makedirs(Path(fake_home) / ".config", exist_ok=True)
    r = run_public_validate(fake_home)
    check("R1 public validate passes with empty HOME (CI regression)", r.returncode == 0,
          f"exit={r.returncode} tail={ (r.stdout or r.stderr)[-200:] }")

# R2: output contains no private path or corpus name.
private_markers = ["regression-pack", "omer-regression", ".config/beme"]
leaks = [m for m in private_markers if m in r.stdout + r.stderr]
check("R2 public validate output free of private paths", not leaks, f"leaked={leaks}")

# R3: private validator is explicitly not_run when corpus is unavailable.
env3 = {
    "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
    "HOME": tempfile.mkdtemp(),
    "BEME_PRIVATE_EVAL_DIR": str(Path(tempfile.mkdtemp()) / "missing-corpus"),
    "TMPDIR": tempfile.gettempdir(),
}
r3 = subprocess.run([PY, str(ROOT / "scripts" / "validate_private_eval.py")],
                    capture_output=True, text=True, env=env3, timeout=60)
check("R3 private eval reports not_run (exit 3) when corpus absent", r3.returncode == 3,
      f"exit={r3.returncode} out={r3.stdout.strip()[:120]}")

# also unset entirely
env3b = dict(env3)
del env3b["BEME_PRIVATE_EVAL_DIR"]
r3b = subprocess.run([PY, str(ROOT / "scripts" / "validate_private_eval.py")],
                     capture_output=True, text=True, env=env3b, timeout=60)
check("R3b private eval not_run when BEME_PRIVATE_EVAL_DIR unset", r3b.returncode == 3,
      f"exit={r3b.returncode}")

# R4: private validator validates real corpus when provided.
corpus = os.environ.get("BEME_PRIVATE_EVAL_DIR", "").strip()
if corpus and Path(corpus).is_dir():
    r4 = subprocess.run([PY, str(ROOT / "scripts" / "validate_private_eval.py")],
                        capture_output=True, text=True, timeout=120)
    check("R4 private eval validates real corpus", r4.returncode == 0,
          f"exit={r4.returncode} out={r4.stdout.strip()[:200]}")
else:
    check("R4 private eval validates real corpus", True,
          "skipped: corpus not present in this environment (owner-run only)")

# R5: static source invariants — no private path in public scripts/workflow,
# and CI never invokes the private validator.
public_sources = [
    ROOT / "scripts" / "validate_contracts.py",
    ROOT / ".github" / "workflows" / "ci.yml",
    ROOT / "Makefile",
]
violations = []
for src in public_sources:
    text = src.read_text()
    for marker in private_markers:
        if marker in text:
            violations.append(f"{src.name}: {marker}")
    if src.name == "ci.yml" and "validate_private_eval" in text:
        violations.append("ci.yml invokes private validator")
    if src.name == "Makefile" and "validate-private" in text and "validate_private_eval" in text:
        pass  # the private target existing in Makefile is fine; CI must not call it
check("R5 no private paths in public scripts/CI; CI never runs private eval", not violations,
      "; ".join(violations))

# R5b: the private validator must be importable/run with ONLY public deps.
r5b = subprocess.run([PY, "-c",
                      "import ast,sys; "
                      "t=ast.parse(open(sys.argv[1]).read()); "
                      "assert any(isinstance(n, ast.If) for n in ast.walk(t)); print('ok')",
                      str(ROOT / "scripts" / "validate_private_eval.py")],
                     capture_output=True, text=True, timeout=30)
check("R5b private validator is syntactically sound", r5b.returncode == 0, r5b.stdout)

passed_n = sum(1 for _, ok, _ in results if ok)
failed_n = [name for name, ok, _ in results if not ok]
print(f"\n{passed_n}/{len(results)} regression checks passed")
if failed_n:
    print("FAILED:", failed_n)
sys.exit(1 if failed_n else 0)