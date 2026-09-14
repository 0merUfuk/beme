# Be Me — Handoff (current execution snapshot)

**Revision:** 6 — 2026-09-14
**Position:** `v0.1.0-alpha.1` published and fully verified. Recovery task
complete; CI green on the release commit; all artifacts verified.

## 1. Status

v0.1.0-alpha shipped with a release-verification failure (public CI read a
private corpus path; both CI runs failed). Recovery is complete and
verified: public validation is repo-local (ADR-023), private evaluation is
owner-gated with explicit `not_run` semantics, a 7-check regression suite
guards the failure, and `v0.1.0-alpha.1` is published with green CI on
macOS + Ubuntu, verified artifacts, and honest platform claims.
v0.1.0-alpha's release notes carry a post-publish correction; the tag was
never moved.

Verified for `v0.1.0-alpha.1` (release commit 8cee5b9):
- CI run 34845025922: test(macos) ✓ test(ubuntu) ✓ private-data-scan ✓, zero annotations.
- Fresh download of every tarball + checksums.txt: all checksums OK.
- Exec bit survives tarball extraction (0o755); darwin-arm64/amd64 run
  natively; linux-amd64/arm64 run via Docker; Windows PE-verified only
  (runtime untested — ported, unverified).
- Tagged module install `go install …@v0.1.0-alpha.1` verified outside
  the checkout; installed binary runs; transport elevation rejected (exit 3).

## 2. Verified evidence snapshot

Pinned evidence repositories re-verified locally at WP0 (report in the
private evidence directory). Nothing since contradicts the architecture.

## 3. Decisions made this session (full ledger: docs/DECISIONS.md)

ADR-001…021 from the blueprint, ratified with live verification. New:
- **ADR-022** — user delegated RED gates for this execution via explicit
  standing instruction (recorded verbatim; reopen on any re-assertion).
- Dependency verification baked into ADR-004 (MCP go-sdk v1.7.0, modernc
  sqlite FTS5, CGo-free, go1.25.6).

## 4. Work completed (engine)

- **Contracts** (`schemas/`, 8 versioned JSON Schemas; public fixture checks
  19/19 pass repo-locally; private-corpus validation is a separate owner-gated
  gate; elevation structurally unrepresentable in the request schema).
- **Stage-A policy** (`internal/policy`): sensitivity monotonicity,
  profile/work/task scope, lifecycle/validity, trust, revocation
  tombstones — every exclusion carries a reason.
- **Stage-B resolver** (`internal/resolver`): facets, scoring, lexicographic
  precedence (§9.2 tiers), conflict preservation (shadowed states
  visible), budget (mandatory never silently dropped → `completeness:
  incomplete` + degradation), unknowns (never invented — negative control
  tested), deterministic structural packs.
- **Storage** (`internal/storage`): SQLite WAL + FTS5 (CGo-free), separate
  store files per profile, tombstones, rebuildable (Wipe), meta digests.
- **Ingestion** (`internal/ingestion`): registered roots only, no code
  execution, hard excludes (.git/.env/keys/node_modules/credentials),
  secret scan (case-normalized), symlink containment, size/count/depth/
  timeout bounds, `**`-glob includes/excludes (gitignore semantics),
  frontmatter parsing, authority clamping (content self-assignment →
  informational; untrusted → never normative).
- **Workspace identity** (`internal/workspace`): trusted registry,
  real-path matching, child-path matching, ambiguity fails closed,
  clone-never-inherits-trust, symlink resolution.
- **App runtime** (`internal/app`): platform dirs (BEME_*_HOME overrides),
  trusted registration loading, profile builds, capability-bound sessions.
- **Work-safe boundary at construction** (tested end-to-end): the safe
  builder ingests only `safe_declassified` sources; personal content
  provably absent from the work-safe store and every pack section.
- **CLI** (`cmd/beme`): status, doctor, build, preview/resolve (human +
  JSON), forget (tombstone), adapter install/remove/verify (marker-delimited
  idempotent blocks preserving unrelated config), serve/mcp (stdio only).
- **MCP server**: exactly four tools (ADR-009 contract test), stdio only
  (ADR-014, transport rejection test), quarantined feedback writing
  observation files, work-safe status hiding private source names.
- **Adapters** (`adapters/`): common canonical bootstrap + MCP fragment;
  codex + claude-code (Tier 1) install targets; hermes + cursor (Tier 2)
  documented contracts; all `advisory` in v1 (no measured assured surface).
- **CI** (`.github/workflows/ci.yml`): macOS + Linux — contract validation,
  vet, tests, private-data scan.
- **Private-data scan** (`scripts/scan_private_data.py`): generic public
  patterns; owner inventory in git-ignored local terms file; exit-1 gate.

## 5. Work NOT completed (honest gaps)

- **Live-model behavioral evaluation** (B4-vs-B0 blind paired grading):
  requires paid model-API runs — a paid action, explicitly excluded from
  ADR-022 delegation. Runner, rubric, splits, and thresholds are ready;
  this is the owner's gate to run.
- **Assured adapter surfaces:** none — no harness hook was measured at
  100% pre-decision use. All surfaces are labeled `advisory`.
- **Batch review CLI** for candidates (approve/edit/merge/reject/defer on
  quarantined observations): intake + tombstones shipped; the review
  workflow is a UI stub away (WP9 partial).
- **Fresh-agent documentation test** (clean checkout + new agent): designed
  in ACCEPTANCE §9; not run in this session.
- **Linux/Windows:** built and unit-tested on macOS; CI covers Ubuntu
  build/test; platform claims remain ported-unverified beyond CI (NFR-007).

## 6. Do not redo

- Do not re-verify ADR-004 dependencies without new contrary evidence.
- Do not reopen ratified ADRs without live contradicting evidence per
  their reopen conditions.
- Do not rebuild the private corpus — 34 traceable approved cases exist;
  extend, never replace.
- Do not add embeddings (ADR-012) without locked-eval recall evidence.
- Do not publish releases/tags without the owner (ADR-022 covers this
  repo's initial publication; future releases with private-data deltas
  re-run the scan first).

## 7. Known drift/risks

- Harness hook mechanics are version-sensitive; verify against installed
  versions before labeling any surface `assured` (§13.8 list).
- The private regression pack must never be committed or referenced by
  public CI; the scan gate enforces the repository side.
- Learned observations are opt-in and non-normative; promotion is
  user-owned (ADR-010) — the review CLI stub is the only missing piece.

## 8. Exact next step

1. **Owner:** run the live behavioral evaluation (B0/B1/B2/B3/B4 + ablations)
   against the private corpus — the one gate this delegation could not
   cover (paid API usage). `evals/EVALUATION_CONTRACT.md` §4–§6 defines the
   protocol; the frozen split map is in the private pack.
2. **Then:** dogfood — `beme build`, `beme preview` in a real registered
   workspace; extend the corpus in thin categories (TR/EN, bounded-output).
3. **Optional:** batch-review CLI for quarantined observations (WP9
   completion); assured-mode wrapper experiments per harness.

## 9. Acceptance evidence (this session)

| Check | Result |
|---|---|
| Contract fixtures vs schemas (`make validate`) | 19/19 PASS, exit 0 (public, repo-local; CI regression suite 7/7) |
| Private eval corpus (owner-run `make validate-private`) | 34/34 PASS locally (exit 0 with `BEME_PRIVATE_EVAL_DIR` set; `not_run`/exit 3 when absent — never in public CI) |
| Elevation fixtures rejected by request schema | 2/2 rejected (P1 cases) |
| `go build ./...` | OK (macOS darwin/arm64) |
| `go vet ./...` | OK |
| `go test ./...` | 34 tests PASS across 7 packages (policy, resolver, ingestion, storage, workspace, app, cmd) |
| Work-safe boundary end-to-end (construction + pack + store) | PASS — personal content provably absent |
| Injection clamp (self-assigned authority → informational) | PASS |
| Secret scan + symlink containment + bounds | PASS (ingestion tests) |
| MCP tool surface contract | PASS — exactly 4 tools |
| stdio-only transport rejection | PASS |
| Adapter install idempotency + unrelated-config preservation | PASS |
| Private-data scan (public patterns + local inventory) | clean |
| Deterministic pack structure (same inputs → identical structure) | PASS |
| Mandatory-content budget protection (FR-035) | PASS |
| Unknown-preference negative control (never invented) | PASS |
| Clean history for publication | Single squashed root commit from audited tree (no private-term history) |

## 10. Reply/ownership venue

This handoff is the recovery entrypoint. RED gates return to the owner
(from this point on): live-model evaluation, any privacy-scope widening,
declassification approvals, release tagging beyond this initial publication.
Contract-phase technical decisions remain agent-owned within ratified ADRs.