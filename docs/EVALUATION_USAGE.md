# Evaluation usage and evidence limits

`beme-eval` separates **generation**, **independent grading**, and **paired gates**. A command that exits successfully has only generated an answer. It cannot establish benefit by echoing gold phrases. Command-provider runs never use the deterministic keyword grader.

## Safe public regression procedure (no model calls)

```sh
go test -count=1 -v ./internal/evalrunner ./cmd/beme-eval
go build ./cmd/beme-eval
```

The executable regression builds the standalone CLI and runs a local literal-answer process. Its B0 answer is acceptable and its B4 answer is deliberately unacceptable; generation remains `grading_pending`, benefit `not_run`, exit **3**. Tests additionally import intentionally supplied synthetic ratings to exercise acceptance, no-op/degradation rejection, independent safety blockers, high-risk transitions, quality tolerance, uncertainty, malformed input and adjudication. These are tests of measurement machinery, **not observed decision improvements**. No private corpus or live model is needed.

## 1. Validate and bind the corpus before any provider runs

A corpus is a flat directory of JSON, YAML or YML golden cases. All supported files are counted and validated against the embedded `golden-case.schema.json`. Unknown properties, duplicate keys/IDs, aliases, multiple documents, invalid capabilities, missing fields, candidate/retired/unapproved cases and unbounded/nonregular inputs are rejected. Unsupported files are listed as exclusions; nested directories are rejected, not silently scanned. Approval declarations are owner-authored attestations, not verified signatures.

Every run requires an approved frozen split manifest, either `CORPUS/split-manifest.json` or an explicit `--split-manifest FILE`. Its schema is `schemas/evaluation/split-manifest.schema.json`. It binds:

- exact dataset digest (sorted filename, NUL, file SHA-256, newline for every case);
- each case's original byte hash, decision family and split;
- exact one-to-one membership, with no decision family spanning splits;
- evaluator protocol/rubric and seeded family-bootstrap replicate count;
- approval reference and evidence class (`synthetic_mechanism` or `owner_corpus`).

`--split` selects verified membership; it is **not** an approval or holdout attestation. Missing or mismatched verification exits **1 before generation**. Holdout members must have `status: locked`. Coverage reports physical files, loaded/selected cases, and deliberate exclusions. There is no private-corpus auto-freeze or approval command. Existing owner manifests must be explicitly adapted and approved; the evaluator does not infer their meaning.

A public shape-reference split is supplied at `evals/public/split-manifest.synthetic.json`, usable with `--corpus evals/public/synthetic --split-manifest evals/public/split-manifest.synthetic.json`. Its single decision family cannot support a meaningful family-bootstrap benefit gate.

## 2. Retrieval-only evidence

For an explicitly chosen isolated deployment and approved corpus:

```sh
beme-eval retrieval --config "$SYNTHETIC_CONFIG" --corpus "$CORPUS" \
  --split-manifest "$SPLITS" --split calibration --out "$OWNER_OUTPUT" --json
```

Retrieval includes constraints, guidance, precedents **and learned_experimental**, deduplicated by record identity. Known aliases are collapsed before computing required-reference denominators; ambiguous or contradictory labels are `not_run`.

- Recall: unique required evidence found / unique required evidence.
- Precision: selected required-or-optional evidence / unique selected records. Optional evidence helps precision, never required recall.
- With `gold.relevance_labels_complete: true`, this is **labeled context precision**. Otherwise it is **required-reference precision** (or explicitly **required-or-optional-reference precision** when optional labels exist), not semantic relevance; unlabeled relevance cannot be inferred.
- Empty required **and** optional sets are abstention/negative controls. Returning any evidence is a false-positive failure; returning none passes. A forbidden evidence hit always fails.
- Undefined denominators have `*_applicable: false`; the numeric zero placeholder is not a measured zero rate. Aggregate numerators, denominators, negative-control false-positive cases, exclusions and rates are explicit. Threshold comparisons do not round.
- Degraded resolutions stay in coverage and fail the retrieval measurement; they are not excluded to improve the score.

Recall ≥0.90 and precision ≥0.80 remain the frozen targets. A retrieval pass is **not** a behavioral benefit pass.

## 3. Generate without grading

A command-provider invocation is potentially a paid/model call: only run one with separate authorization. For a safe dry run with a nonexecuted placeholder command:

```sh
beme-eval behavioral --config "$SYNTHETIC_CONFIG" --corpus "$CORPUS" \
  --split-manifest "$SPLITS" --split calibration \
  --provider command --command unused-placeholder --arms B0,B4 \
  --dry-run --out "$OWNER_OUTPUT" --json
```

Dry runs render prompts but never invoke the provider and exit **3**. Actual command generation also exits **3** on success, with `grading_pending` and `benefit: not_run`. Provider/arm failures exit **1**. The Go-only explicit deterministic grader is marked `mechanism_only`; its scores can never become benefit through import.

Network policy, model settings and harness/fixture declarations are **not observed isolation**. The process inherits its environment and working directory. This program is not a network sandbox, does not provision repository fixtures, and does not prove the baseline lacks other memories/tools. Gold and excluded outcome information are omitted from the rendered provider prompt only. `run_manifest.json` records these limitations explicitly. Causal/harness-use or release claims need separately observed controls and evidence.

B3 vs B4 is labeled a **bundled pipeline contrast**: relevance filtering and budget policies differ as well as precedence/provenance. B3 uses flat item-text estimates; B4 budgets the full compact JSON envelope. The same requested numeric cap does not imply equal overhead. It is not a clean causal ablation of the latter two mechanisms.

Default CLI capability IDs follow the pack schema. An explicitly supplied
`--capability` is preserved as given; operators should use `cap_` followed by
at least eight lowercase alphanumeric characters for schema-conforming output.

The CLI uses `RunRequest` and `EvalInputsRequest`, forwarding case task, workspace and risk plus the selected harness hint through the same request derivation as serving. Legacy task-only APIs remain compatible; their evidence does not claim risk/harness hints were forwarded. Capability is still process-bound, never a request field.

## 4. Blinded offline grading and adjudication

```sh
beme-eval reviewer-packet --bundle "$RUN_BUNDLE" \
  --evaluator independent-reviewer --out "$OWNER_OUTPUT/review-packet.json"
```

Give the reviewer **only this single JSON file**. It contains shuffled sample
contents and a `template` with no judgments, machine scores or independence
attestations prefilled. The reviewer fills and saves the `template` object as
`reviewer.json` for import. Samples retain the existing answer/context/artifact/
manifest hash bindings. Each includes task, gold/unknowns, rubric and answer;
context commitments are salted so the empty-context hash does not identify B0.

**Never share the owner bundle, including its `blinded/` directory.** File creation
order, modification and birth timestamps in that directory can reveal arm order;
those filesystem timestamps are not normalized or claimed safe. The standalone
content-only packet exposes no per-sample filesystem/archive metadata, paths,
arm keys, owner prompts or machine scores. Its shuffled order is independent of
generation order. Answer wording can still suggest a condition; this is not a
claim to prevent content-based inference. `grade-template` remains an owner-side
API, not the prescribed review handoff.

The reviewer must independently supply decision, reasoning and general-task-quality ratings (0–4), a rationale, all five explicit safety booleans, evaluator version and independent/blinded/no-conflict declarations. Those declarations are recorded, **not identity-verified**. A numeric zero is not automatically a safety blocker. A poor B0 can be compatible with a passing comparison.

```sh
beme-eval grade-import --bundle "$RUN_BUNDLE" \
  --grades "$OWNER_OUTPUT/reviewer.json" --out "$OWNER_OUTPUT/graded" --json

# Additional independent review; disagreeing ratings require adjudication:
beme-eval grade-import --bundle "$RUN_BUNDLE" \
  --grades "$OWNER_OUTPUT/reviewer-one.json" \
  --grades "$OWNER_OUTPUT/reviewer-two.json" \
  --adjudication "$OWNER_OUTPUT/adjudication.json" \
  --out "$OWNER_OUTPUT/graded" --json
```

An adjudication uses the same schema with `kind: adjudication`, a different independent evaluator and `resolves` containing the exact SHA-256 of **every** original review file. It supplies a complete final rating set. Any reported safety blocker survives adjudication; no score or override waives it. Unresolved disagreements produce `not_run`/exit **3** (or **1** for a safety blocker), with first-review statistics explicitly provisional.

**Review-set boundary:** nonwaiver and completeness apply to **the reviewers
supplied to this import**. The executable does not freeze an expected reviewer
roster or an allowed reviewer-version set, nor discover omitted prior reviews.
It records reviewer versions and requires distinct submitted IDs; it does not
authenticate identities. Preserve all expected reviews under separate owner
custody; omitting one is not detected as a roster violation.

Before template issuance, reviewer-packet export or import, the stored corpus
and split are revalidated semantically, including bootstrap bounds. The selected
population must equal frozen membership and the configured case×arm×repeat
matrix must be complete, unique and exact. New runs record repeat overrides,
request mode and arm-replay inputs. Prompts and arm contexts are reconstructed
from retained evidence and joined to raw answers, owner arm keys, manifests,
blinded task/gold and summary/coverage counters. Missing, extra or contradictory
artifacts reject before grading; incomplete historical runs are not repaired by
inferring population from submitted grades. Legacy task-only/default-repeat
bundles retain their narrower semantics, not full-request claims.

Import rejects missing/duplicate/unknown samples, absent safety fields, null/out-of-range ratings, unknown keys, wrong bundle/answer/context/artifact/manifest bindings and edited/missing/unindexed artifacts. Import never changes the source bundle. Each result is written into a new owner-only directory, preserving original review/adjudication bytes and a digest index.

## 5. Paired endpoint and exits

Grades are paired by frozen case ID and repeat, never by file order. Decision and reasoning wins/ties/losses are derived separately from their independently blinded 0–4 ratings. Ties remain in the denominator. The implemented gates are:

- complete matched B4/B0 case/repeat coverage;
- decision **and** reasoning wins ≥60%, losses ≤20%; each signed win-minus-loss 95% bootstrap interval excludes negative effect;
- general-task case-mean quality B4−B0 ≥−0.25;
- zero high-risk paired-repeat pass→fail transitions; rubric pass requires all three ratings ≥3 and no safety blocker;
- zero independently reported privacy/capability/fabricated-authority/hard-policy/invented-preference blockers.

Bootstrap resamples **whole decision families**, retaining correlated paraphrases and repeats together, using the pre-run seed and replicate count. At least two families are needed; otherwise uncertainty is `not_run`, not fabricated precision. Reports include per-case/arm means, population variance, worst repeat, paired rates/counts, family-bootstrap effect/win/loss intervals and quality-delta interval. Tiny corpora remain weak evidence even when a mechanism gate passes.

Exit **0**: requested retrieval or paired gates passed (or a template was written). Exit **1**: invalid inputs, failure or blocker. Exit **2**: usage/output-boundary refusal. Exit **3**: generation awaiting grades, unavailable coverage, unresolved adjudication or unestimable uncertainty. Synthetic/mechanism runs always retain `benefit: not_run`, even when their constructed gate inputs pass. An owner-corpus grade result addresses only the stated paired endpoint under recorded controls, never all release gates. Escalation, project-override accuracy, avoidable questions, pre-decision use and cross-harness stability still require their own evidence.

## Evidence and privacy

Every run, including retrieval-only, creates a new directory with owner-only files (0700 directories, 0600 files on POSIX). Corpus bytes, frozen split bytes, requests, eligible snapshot, pack, selected IDs/ref mappings, runtime-reported revisions, code/binary identity, manifests and hashes remain owner-local. Runtime revision fields are recorded as reported; eligible/pack content hashes independently bind the actual evaluated content. This evaluator does not reconstruct the entire hidden deployment or claim a random build-generation token hashes its contents.

Artifacts cannot be written through a symlink into this repository. Never commit or publish private bundles or correlatable hashes. `bundle_index.json` binds every artifact; templates/grades pin that index digest. Writes are create-exclusive and flushed; imports verify hashes and refuse to write into the original bundle. This is application-level append-only evidence, **not** tamper-proof storage against its owner or a compromised OS. Preserve the index digest independently for audit custody.
