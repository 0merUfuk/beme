#!/usr/bin/env python3
"""Private-data leakage scan: a hard release gate (ADR-015, FR-005).

Scans repository files for private identifiers: real names, personal paths,
credential shapes, and — via an optional local extension file — the owner's
private repository inventory (which itself must never be committed).

The public scanner carries only GENERIC patterns. Owner-specific terms
(private repo names, personal identifiers) live in
scripts/private_scan_terms.local (git-ignored) or a path given by
--extra-terms, so the public repository never embeds the private
inventory it is defending against.

Exits 1 on any hit so CI and release packaging fail closed.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# Generic patterns only. Owner identity/repo names are NEVER embedded in
# this public file — they load from the git-ignored local terms file.
PATTERNS = [
    (r"/Users/[a-z]+/(?!Library/Caches)", "personal home path (use platform dirs or <config> placeholders)"),
    (r"(?i)BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY", "private key material"),
    (r"(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*['\"][A-Za-z0-9+/]{16,}", "credential literal"),
    (r"[A-Za-z0-9._%+-]+@(?!example|noreply|users\.noreply)[A-Za-z0-9.-]+\.[A-Za-z]{2,}", "email address"),
]

# Files that legitimately contain secret-detection PATTERNS (the detector
# and its tests): the key-material pattern is skipped for these.
DETECTOR_FILES = {
    Path("scripts/scan_private_data.py"),
    Path("internal/ingestion/ingestion.go"),
    Path("internal/ingestion/ingestion_test.go"),
}

EXCLUDE_FILES = {"beme-project-blueprint-handoff.md", ".gitignore", "private_scan_terms.local"}
EXCLUDE_DIRS = {".git"}


def load_extra_terms():
    terms = []
    local = ROOT / "scripts" / "private_scan_terms.local"
    if local.exists():
        terms += [l.strip() for l in local.read_text().splitlines() if l.strip() and not l.startswith("#")]
    if len(sys.argv) > 2 and sys.argv[1] == "--extra-terms":
        p = Path(sys.argv[2])
        if p.exists():
            terms += [l.strip() for l in p.read_text().splitlines() if l.strip() and not l.startswith("#")]
    return terms


hits = []
extra_terms = load_extra_terms()

for path in sorted(ROOT.rglob("*")):
    if not path.is_file():
        continue
    rel = path.relative_to(ROOT)
    if any(part in EXCLUDE_DIRS for part in rel.parts):
        continue
    if rel.name in EXCLUDE_FILES:
        continue
    try:
        text = path.read_text(errors="ignore")
    except Exception:
        continue
    for pat, why in PATTERNS:
        if why == "private key material" and rel in DETECTOR_FILES:
            continue
        for m in re.finditer(pat, text):
            line = text[: m.start()].count("\n") + 1
            snippet = text[max(0, m.start() - 25) : m.end() + 25].replace("\n", " ")
            hits.append(f"{rel}:{line}  [{why}]  ...{snippet}...")
    for term in extra_terms:
        for m in re.finditer(rf"(?<![\w.-]){re.escape(term)}(?![\w-])", text):
            line = text[: m.start()].count("\n") + 1
            ctx = text[max(0, m.start() - 30) : m.end() + 30].replace("\n", " ")
            hits.append(f"{rel}:{line}  [private term: {term}]  ...{ctx}...")

if hits:
    print("PRIVATE-DATA LEAKAGE DETECTED:")
    for h in hits:
        print("  ", h)
    sys.exit(1)
print("private-data scan: clean")