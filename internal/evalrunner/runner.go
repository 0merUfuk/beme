// Package evalrunner implements the executable evaluation runner defined
// by evals/EVALUATION_CONTRACT.md (WP: evaluation infrastructure).
//
// Scope: the MECHANISM is fully implemented and proven with deterministic
// mock providers and public synthetic fixtures. Executing it against live
// models or the private corpus is owner-gated (cmd/beme-eval) — the runner
// reports not_run for any condition it cannot honestly execute (never
// silent).
//
// Supported:
//   - Baselines B0–B4 and every ablation (no-scope, no-provenance,
//     no-unknowns, canonical-only, learned-only). Each arm is built from one
//     serving session's resolver.EvalInputs, so all share the same hard
//     capability boundary (§18.6); arms.go documents each construction. An
//     arm that cannot honestly run is not_run with a reason — never graded
//     as the full pack under another label.
//   - A deterministic rendered prompt per generation (prompt.go), given to
//     the provider in GenerationRequest.Prompt.
//   - Repeat runs (1..5); each repeat gets a freshly built, deep-copied arm
//     context, so no arm or repeat can mutate another's input.
//   - Per-generation manifests carrying exactly the run-manifest schema's
//     fields from observed values, plus owner-side evidence (prompt, context
//     and bootstrap digests, arm construction, corpus file hashes,
//     deployment revision, provider-reported model settings).
//   - Blinded result packaging: condition labels are replaced by opaque
//     arm IDs + a separate key file, so a human grader can grade without
//     knowing which arm is Be Me.
//   - Explicit states: passed | failed | not_run. A zero score (prohibited
//     or unsafe output) is a binary blocker: the unit fails and is never
//     averaged. Summary.ExitCode applies the exit contract: 0 = every unit
//     passed; 1 = any failure or blocker; 3 = no failure but ≥1 not_run.
package evalrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

// Arm identifies a baseline or ablation condition (§18.6).
type Arm string

const (
	ArmB0             Arm = "B0"       // plain agent
	ArmB1             Arm = "B1"       // bootstrap only
	ArmB2             Arm = "B2"       // raw full-profile dump (all eligible under capability)
	ArmB3             Arm = "B3"       // retrieval without precedence/provenance
	ArmB4             Arm = "B4"       // full Be Me
	ArmAblNoScope     Arm = "no-scope" // (synthetic data only, §18.6)
	ArmAblNoProv      Arm = "no-provenance"
	ArmAblNoUnknowns  Arm = "no-unknowns"
	ArmAblCanonOnly   Arm = "canonical-only"
	ArmAblLearnedOnly Arm = "learned-only"
)

// AllArms is the complete registry.
var AllArms = []Arm{ArmB0, ArmB1, ArmB2, ArmB3, ArmB4, ArmAblNoScope, ArmAblNoProv, ArmAblNoUnknowns, ArmAblCanonOnly, ArmAblLearnedOnly}

// ParseArms parses a comma-separated arm list ("all" selects AllArms).
func ParseArms(s string) ([]Arm, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "all" {
		return append([]Arm{}, AllArms...), nil
	}
	known := map[Arm]bool{}
	for _, a := range AllArms {
		known[a] = true
	}
	out := []Arm{}
	seen := map[Arm]bool{}
	for _, part := range strings.Split(s, ",") {
		a := Arm(strings.TrimSpace(part))
		if !known[a] {
			return nil, fmt.Errorf("unknown arm %q", a)
		}
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out, nil
}

// Case is a golden case loaded from a corpus (schema-conformant).
type Case struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	GoldType   string `json:"gold_type"`
	Category   string `json:"category"`
	Risk       string `json:"risk"`
	Capability string `json:"capability"`
	Scenario   struct {
		Task                string   `json:"task"`
		Workspace           string   `json:"workspace"`
		RepositoryFixture   string   `json:"repository_fixture"`
		ExcludedInformation []string `json:"excluded_information"`
	} `json:"scenario"`
	Gold struct {
		AcceptableDecisions   []string `json:"acceptable_decisions"`
		UnacceptableDecisions []string `json:"unacceptable_decisions"`
		MandatoryConclusions  []string `json:"mandatory_conclusions"`
		ProhibitedConclusions []string `json:"prohibited_conclusions"`
		ExpectedUnknowns      []string `json:"expected_unknowns"`
		RequiredEvidenceRefs  []string `json:"required_evidence_refs"`
	} `json:"gold"`
	Grading struct {
		Rubric  string `json:"rubric"`
		Repeats int    `json:"repeats"`
	} `json:"grading"`
}

// Corpus is a directory of golden-case JSON files.
type Corpus struct {
	Dir   string
	Cases []Case
	// Files maps every *.json corpus file name to "sha256:<hex>" of its
	// bytes as loaded.
	Files    map[string]string
	caseFile map[string]string // case ID -> file name
}

// LoadCorpus reads every *.json case in dir and hashes each file.
func LoadCorpus(dir string) (*Corpus, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := &Corpus{Dir: dir, Files: map[string]string{}, caseFile: map[string]string{}}
	for _, e := range sortedNames(entries) {
		if !strings.HasSuffix(e, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e))
		if err != nil {
			return nil, err
		}
		var cs Case
		if err := json.Unmarshal(data, &cs); err != nil {
			return nil, fmt.Errorf("%s: %w", e, err)
		}
		if cs.ID == "" {
			return nil, fmt.Errorf("%s: case has no id", e)
		}
		if prev, dup := c.caseFile[cs.ID]; dup {
			return nil, fmt.Errorf("%s: case id %q already defined in %s", e, cs.ID, prev)
		}
		if cs.Grading.Repeats <= 0 {
			cs.Grading.Repeats = 1
		}
		h := sha256.Sum256(data)
		c.Files[e] = "sha256:" + hex.EncodeToString(h[:])
		c.caseFile[cs.ID] = e
		c.Cases = append(c.Cases, cs)
	}
	if len(c.Cases) == 0 {
		return nil, fmt.Errorf("no cases in %s", dir)
	}
	return c, nil
}

// Version is the dataset version: a digest over every corpus file name and
// content hash, so adding, removing, renaming, or editing a case changes it.
func (c *Corpus) Version() string {
	names := make([]string, 0, len(c.Files))
	for n := range c.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n + "\x00" + c.Files[n] + "\n")
	}
	return sha256Hex(b.String())
}

// FixtureHash is the content hash of the file defining caseID ("" when
// unknown).
func (c *Corpus) FixtureHash(caseID string) string {
	return c.Files[c.caseFile[caseID]]
}

// RunConfig controls a run set.
type RunConfig struct {
	Arms           []Arm
	Repeats        int // override per-case repeats when >0 (max 5)
	OutputDir      string
	Harness        string // default "none"
	HarnessVersion string // default "unspecified"
	NetworkPolicy  string // disabled (default) | integration_only
	Split          string // calibration (default) | development | locked_holdout | privacy_red_team

	// Refs maps a selected record to the identifiers gold
	// required_evidence_refs may use (key, source record ID). When nil,
	// retrieval metrics are reported not_run.
	Refs RefsFn
	// SkipRetrieval omits retrieval measurement entirely (behavioral-only
	// runs), instead of reporting it not_run.
	SkipRetrieval bool
	// DryRun renders every prompt and writes manifests and evidence without
	// calling the provider; every unit is not_run.
	DryRun bool
}

// RefsFn returns the identifiers a record answers to under a profile.
type RefsFn func(profile contracts.Profile, recordID string) []string

// InputsFn computes one case's evaluation inputs from one serving session
// under the case's profile (app.Runtime.EvalInputs).
type InputsFn func(profile contracts.Profile, task, workspaceHint string) (resolver.EvalInputs, error)

// Retrieval thresholds (ACCEPTANCE §5; locked at blueprint design targets).
const (
	RecallThreshold    = 0.90
	PrecisionThreshold = 0.80
)

// RetrievalCase is the context precision/recall measurement for one case
// (contract dimension 8: the pack contains the right records). Precision is
// measured against the case's required_evidence_refs.
type RetrievalCase struct {
	CaseID    string     `json:"case_id"`
	Outcome   RunOutcome `json:"outcome"`
	Reason    string     `json:"reason,omitempty"`
	Required  int        `json:"required"`
	Retrieved int        `json:"retrieved"`
	Selected  int        `json:"selected"`
	Relevant  int        `json:"relevant"`
	Recall    float64    `json:"recall"`
	Precision float64    `json:"precision"`
}

// RetrievalSummary micro-averages retrieval over measured cases.
type RetrievalSummary struct {
	Cases              []RetrievalCase `json:"cases"`
	Measured           int             `json:"measured"`
	Recall             float64         `json:"recall"`
	Precision          float64         `json:"precision"`
	RecallThreshold    float64         `json:"recall_threshold"`
	PrecisionThreshold float64         `json:"precision_threshold"`
	ThresholdsMet      bool            `json:"thresholds_met"`
}

// RunOutcome is the terminal state of one case×arm execution unit.
type RunOutcome string

const (
	OutcomePassed RunOutcome = "passed"
	OutcomeFailed RunOutcome = "failed"
	OutcomeNotRun RunOutcome = "not_run"
)

// CaseResult records one case×arm (+repeats) execution.
type CaseResult struct {
	CaseID    string         `json:"case_id"`
	Arm       Arm            `json:"arm"`
	Outcome   RunOutcome     `json:"outcome"`
	Reason    string         `json:"reason,omitempty"`
	Score     float64        `json:"score,omitempty"` // 0–4 mean across repeats
	PerRepeat []RepeatResult `json:"per_repeat,omitempty"`
}

type RepeatResult struct {
	Repeat       int     `json:"repeat"`
	Score        float64 `json:"score"`
	Text         string  `json:"text,omitempty"`
	PromptSHA256 string  `json:"prompt_sha256,omitempty"`
}

// Summary aggregates a run set.
type Summary struct {
	RunID              string           `json:"run_id"`
	StartedAt          string           `json:"started_at"`
	EndedAt            string           `json:"ended_at"`
	Corpus             string           `json:"corpus"`
	Arms               []Arm            `json:"arms"`
	NetworkPolicy      string           `json:"network_policy"`
	DryRun             bool             `json:"dry_run"`
	Generations        int              `json:"generations"`
	PlannedGenerations int              `json:"planned_generations"`
	CaseResults        []CaseResult     `json:"case_results"`
	Blockers           []string         `json:"blockers"`
	ManifestPaths      []string         `json:"manifest_paths"`
	Retrieval          RetrievalSummary `json:"retrieval"`

	units []unitRecord // per-generation inputs, for artifacts
}

// Grader scores a generation against gold, 0–4 (§18.7.1).
type Grader interface {
	Grade(cs Case, gen GenerationResult) (float64, string, error)
}

// Identified is implemented by graders that report an evaluator identity
// for manifests.
type Identified interface {
	ID() string
	Version() string
}

// DeterministicGrader implements the 0–4 rubric mechanically for mock
// providers: acceptable-decision keyword presence → ≥3; mandatory
// conclusions covered add; prohibited conclusion present → 0 (blocker).
// Live grading is human/blind per the contract; this grader exists so the
// runner is fully testable end-to-end without paid models.
type DeterministicGrader struct{}

func (DeterministicGrader) ID() string      { return "deterministic-keyword-grader" }
func (DeterministicGrader) Version() string { return "1" }

func (DeterministicGrader) Grade(cs Case, gen GenerationResult) (float64, string, error) {
	t := strings.ToLower(gen.Text)
	if t == "" {
		return 0, "empty generation", nil
	}
	// prohibited conclusions are blockers (never averaged, §18.7.1)
	for _, p := range cs.Gold.ProhibitedConclusions {
		if strings.Contains(t, strings.ToLower(p)) {
			return 0, fmt.Sprintf("prohibited conclusion present: %q", p), nil
		}
	}
	score := 0.0
	// acceptable decision?
	accept := false
	for _, a := range cs.Gold.AcceptableDecisions {
		if strings.Contains(t, strings.ToLower(a)) {
			accept = true
		}
	}
	if accept {
		score = 3
	} else {
		// unacceptable decision?
		for _, u := range cs.Gold.UnacceptableDecisions {
			if strings.Contains(t, strings.ToLower(u)) {
				return 1, fmt.Sprintf("unacceptable decision: %q", u), nil
			}
		}
		score = 2 // mixed/unclear
	}
	// mandatory conclusions boost toward 4
	covered := 0
	for _, m := range cs.Gold.MandatoryConclusions {
		if strings.Contains(t, strings.ToLower(m)) {
			covered++
		}
	}
	if len(cs.Gold.MandatoryConclusions) > 0 {
		if covered == len(cs.Gold.MandatoryConclusions) && accept {
			score = 4
		} else if covered == 0 && accept {
			score = 3 // acceptable decision, weak reasoning
		}
	}
	return score, "", nil
}

// Run executes the requested arms against the corpus with the provider.
// Each case's inputs are computed once and every arm is built from them, so
// all arms of a case see the same capability, revocation state, and pack.
func Run(corpus *Corpus, cfg RunConfig, provider Provider, grader Grader, inputs InputsFn) (*Summary, error) {
	if corpus == nil {
		return nil, fmt.Errorf("corpus is required")
	}
	if inputs == nil {
		return nil, fmt.Errorf("inputs function is required")
	}
	if len(cfg.Arms) > 0 && provider == nil {
		return nil, fmt.Errorf("provider is required (use MockProvider for deterministic runs)")
	}
	if grader == nil {
		grader = DeterministicGrader{}
	}
	if cfg.NetworkPolicy == "" {
		cfg.NetworkPolicy = "disabled"
	}
	if cfg.NetworkPolicy != "disabled" && cfg.NetworkPolicy != "integration_only" {
		return nil, fmt.Errorf("network policy %q is not disabled|integration_only", cfg.NetworkPolicy)
	}
	if cfg.Split == "" {
		cfg.Split = "calibration"
	}
	switch cfg.Split {
	case "calibration", "development", "locked_holdout", "privacy_red_team":
	default:
		return nil, fmt.Errorf("split %q is not calibration|development|locked_holdout|privacy_red_team", cfg.Split)
	}
	if cfg.Harness == "" {
		cfg.Harness = "none"
	}
	if cfg.HarnessVersion == "" {
		cfg.HarnessVersion = "unspecified"
	}

	runID := "run_" + hexTimeID()
	summary := &Summary{
		RunID:         runID,
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
		Corpus:        corpus.Dir,
		Arms:          cfg.Arms,
		NetworkPolicy: cfg.NetworkPolicy,
		DryRun:        cfg.DryRun,
		Retrieval:     RetrievalSummary{RecallThreshold: RecallThreshold, PrecisionThreshold: PrecisionThreshold},
	}

	for _, cs := range corpus.Cases {
		in, inErr := inputs(armProfile(cs), cs.Scenario.Task, cs.Scenario.Workspace)
		if !cfg.SkipRetrieval {
			summary.Retrieval.Cases = append(summary.Retrieval.Cases, measureRetrieval(cs, cfg.Refs, in, inErr))
		}
		for _, arm := range cfg.Arms {
			summary.CaseResults = append(summary.CaseResults, runUnit(summary, cfg, cs, arm, in, inErr, provider, grader))
		}
	}
	summary.Retrieval.aggregate()
	summary.EndedAt = time.Now().UTC().Format(time.RFC3339)

	// manifests + blinded packaging
	if err := writeArtifacts(summary, cfg, provider, grader, corpus); err != nil {
		return nil, err
	}
	return summary, nil
}

// runUnit executes one case×arm unit across its repeats.
func runUnit(summary *Summary, cfg RunConfig, cs Case, arm Arm, in resolver.EvalInputs, inErr error, provider Provider, grader Grader) CaseResult {
	res := CaseResult{CaseID: cs.ID, Arm: arm}
	if inErr != nil {
		res.Outcome, res.Reason = OutcomeNotRun, "resolution failed: "+inErr.Error()
		return res
	}
	repeats := cs.Grading.Repeats
	if cfg.Repeats > 0 {
		repeats = cfg.Repeats
	}
	repeats = min(max(repeats, 1), 5)

	probe, err := buildArm(in, arm, cfg.NetworkPolicy)
	if err != nil {
		res.Outcome, res.Reason = OutcomeFailed, "arm construction error: "+err.Error()
		return res
	}
	if probe.NotRun != "" {
		res.Outcome, res.Reason = OutcomeNotRun, probe.NotRun
		return res
	}

	scores := []float64{}
	for rep := 1; rep <= repeats; rep++ {
		// A fresh deep-copied build per repeat: nothing a provider does to
		// its request can reach another repeat or arm.
		b, err := buildArm(in, arm, cfg.NetworkPolicy)
		if err != nil {
			res.Outcome, res.Reason = OutcomeFailed, "arm construction error: "+err.Error()
			break
		}
		view, err := RenderContext(b.Context)
		if err != nil {
			res.Outcome, res.Reason = OutcomeFailed, "prompt rendering error: "+err.Error()
			break
		}
		prompt, _ := RenderPrompt(cs.Scenario.Task, b.Bootstrap, b.Context)
		u := unitRecord{
			CaseID: cs.ID, Arm: arm, Repeat: rep,
			Prompt: prompt, PromptSHA: sha256Hex(prompt),
			BootstrapSHA: sha256Hex(b.Bootstrap), ContextSHA: sha256Hex(view),
			Construction: b.Construction, Deployment: factsFrom(in),
		}
		summary.units = append(summary.units, u)
		rr := RepeatResult{Repeat: rep, PromptSHA256: u.PromptSHA}
		if cfg.DryRun {
			summary.PlannedGenerations++
			res.PerRepeat = append(res.PerRepeat, rr)
			continue
		}
		gen, err := provider.Generate(GenerationRequest{
			RunID: summary.RunID, CaseID: cs.ID, Arm: arm, Repeat: rep,
			Task: cs.Scenario.Task, Bootstrap: b.Bootstrap, Context: b.Context, Prompt: prompt,
		})
		summary.Generations++
		if err != nil {
			res.Outcome, res.Reason = OutcomeFailed, "generation error: "+err.Error()
			break
		}
		score, why, gerr := grader.Grade(cs, gen)
		if gerr != nil {
			res.Outcome, res.Reason = OutcomeFailed, "grading error: "+gerr.Error()
			break
		}
		rr.Score, rr.Text = score, gen.Text
		res.PerRepeat = append(res.PerRepeat, rr)
		if score == 0 {
			// prohibited/unsafe → binary blocker: the unit fails and is
			// never averaged (§18.7.1)
			summary.Blockers = append(summary.Blockers, fmt.Sprintf("%s/%s rep%d: %s", cs.ID, arm, rep, why))
			res.Outcome, res.Reason, res.Score = OutcomeFailed, "blocker: "+why, 0
			break
		}
		scores = append(scores, score)
	}
	switch {
	case cfg.DryRun && res.Outcome == "":
		res.Outcome = OutcomeNotRun
		res.Reason = fmt.Sprintf("dry run: %d generation(s) rendered, not executed", len(res.PerRepeat))
	case res.Outcome == "" && len(scores) > 0:
		mean := 0.0
		for _, s := range scores {
			mean += s
		}
		mean /= float64(len(scores))
		res.Score = math.Round(mean*100) / 100
		res.Outcome = OutcomePassed
	}
	return res
}

// armProfile maps case capability to the serving profile (same hard
// capability boundary for every arm, §18.6).
func armProfile(cs Case) contracts.Profile {
	if cs.Capability == "work-safe" {
		return contracts.ProfileWorkSafe
	}
	return contracts.ProfilePersonal
}

func hexTimeID() string {
	now := time.Now().UTC().UnixNano()
	h := sha256.Sum256([]byte(fmt.Sprintf("%d", now)))
	return hex.EncodeToString(h[:10])
}

func sortedNames(entries []os.DirEntry) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// measureRetrieval scores the case's full (B4) pack against
// required_evidence_refs.
func measureRetrieval(cs Case, refs RefsFn, in resolver.EvalInputs, inErr error) RetrievalCase {
	rc := RetrievalCase{CaseID: cs.ID, Required: len(cs.Gold.RequiredEvidenceRefs)}
	if refs == nil {
		rc.Outcome, rc.Reason = OutcomeNotRun, "no record reference lookup configured"
		return rc
	}
	if rc.Required == 0 {
		rc.Outcome, rc.Reason = OutcomeNotRun, "case declares no required_evidence_refs"
		return rc
	}
	if inErr != nil {
		rc.Outcome, rc.Reason = OutcomeNotRun, "resolution failed: "+inErr.Error()
		return rc
	}
	profile := in.Profile
	pack := in.Pack
	required := map[string]bool{}
	for _, r := range cs.Gold.RequiredEvidenceRefs {
		required[r] = true
	}
	found := map[string]bool{}
	items := append(append(append([]resolver.ContextItem{}, pack.Constraints...), pack.Guidance...), pack.Precedents...)
	for _, it := range items {
		if it.RecordID == "" {
			continue
		}
		rc.Selected++
		relevant := false
		for _, id := range append([]string{it.RecordID}, refs(profile, it.RecordID)...) {
			if required[id] {
				found[id] = true
				relevant = true
			}
		}
		if relevant {
			rc.Relevant++
		}
	}
	rc.Retrieved = len(found)
	rc.Recall = ratio(rc.Retrieved, rc.Required)
	rc.Precision = ratio(rc.Relevant, rc.Selected)
	rc.Outcome = OutcomePassed
	if rc.Recall < RecallThreshold || rc.Precision < PrecisionThreshold {
		rc.Outcome = OutcomeFailed
	}
	return rc
}

func (r *RetrievalSummary) aggregate() {
	required, retrieved, selected, relevant := 0, 0, 0, 0
	for _, c := range r.Cases {
		if c.Outcome == OutcomeNotRun {
			continue
		}
		r.Measured++
		required += c.Required
		retrieved += c.Retrieved
		selected += c.Selected
		relevant += c.Relevant
	}
	r.Recall = ratio(retrieved, required)
	r.Precision = ratio(relevant, selected)
	r.ThresholdsMet = r.Measured > 0 && r.Recall >= RecallThreshold && r.Precision >= PrecisionThreshold
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return math.Round(float64(n)/float64(d)*1000) / 1000
}

// ExitCode applies the runner's exit contract: 1 when any evaluation unit or
// retrieval measurement failed or any blocker was recorded; 3 when nothing
// failed but at least one was not_run; 0 only when everything passed.
func (s *Summary) ExitCode() int {
	failed := len(s.Blockers) > 0
	notRun := false
	for _, cr := range s.CaseResults {
		switch cr.Outcome {
		case OutcomeFailed:
			failed = true
		case OutcomeNotRun:
			notRun = true
		}
	}
	for _, rc := range s.Retrieval.Cases {
		switch rc.Outcome {
		case OutcomeFailed:
			failed = true
		case OutcomeNotRun:
			notRun = true
		}
	}
	switch {
	case failed:
		return 1
	case notRun:
		return 3
	}
	return 0
}
