// Package evalrunner implements validated, split-bound generation/retrieval
// and independent offline grading. Successful generation is not benefit.
// Deterministic graders are explicit mechanism-only test tools; real answers
// remain pending until blinded, hash-bound grades and paired gates exist.
// See docs/EVALUATION_USAGE.md for evidence custody and control limitations.
package evalrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
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
	SchemaVersion string  `json:"schema_version"`
	Harness       *string `json:"harness"`
	Provenance    struct {
		ApprovedByUser bool     `json:"approved_by_user"`
		EvidenceRefs   []string `json:"evidence_refs"`
		ValidAt        string   `json:"valid_at"`
		CandidateNote  string   `json:"candidate_note"`
	} `json:"provenance"`
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
		AcceptableDecisions     []string `json:"acceptable_decisions"`
		UnacceptableDecisions   []string `json:"unacceptable_decisions"`
		MandatoryConclusions    []string `json:"mandatory_conclusions"`
		ProhibitedConclusions   []string `json:"prohibited_conclusions"`
		ExpectedUnknowns        []string `json:"expected_unknowns"`
		RequiredEvidenceRefs    []string `json:"required_evidence_refs"`
		OptionalEvidenceRefs    []string `json:"optional_evidence_refs,omitempty"`
		ForbiddenEvidenceRefs   []string `json:"forbidden_evidence_refs,omitempty"`
		RelevanceLabelsComplete bool     `json:"relevance_labels_complete,omitempty"`
	} `json:"gold"`
	Grading struct {
		Rubric  string `json:"rubric"`
		Repeats int    `json:"repeats"`
	} `json:"grading"`
}

// Corpus is a validated flat directory of JSON/YAML/YML golden cases.
type Corpus struct {
	Dir   string
	Cases []Case
	// Files maps every supported corpus case file name to "sha256:<hex>" of its
	// bytes as loaded.
	Files    map[string]string
	caseFile map[string]string // case ID -> file name
	Bytes    map[string][]byte `json:"-"`
	Coverage Coverage          `json:"coverage"`
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
	SplitManifest  string
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

// RequestInputsFn preserves the complete narrowed serving request.
// It never sets the process-bound capability; profile remains a separate input.
type RequestInputsFn func(profile contracts.Profile, req contracts.ResolutionRequest) (resolver.EvalInputs, error)

// Retrieval thresholds (ACCEPTANCE §5; locked at blueprint design targets).
const (
	RecallThreshold    = 0.90
	PrecisionThreshold = 0.80
)

// RetrievalCase is the context precision/recall measurement for one case
// (contract dimension 8: the pack contains the right records). Precision is
// measured against the case's required_evidence_refs.
type RetrievalCase struct {
	FalsePositives      int                 `json:"false_positives"`
	Forbidden           int                 `json:"forbidden"`
	NegativeControl     bool                `json:"negative_control"`
	RecallApplicable    bool                `json:"recall_applicable"`
	PrecisionApplicable bool                `json:"precision_applicable"`
	PrecisionMeaning    string              `json:"precision_meaning"`
	SelectedIDs         []string            `json:"selected_ids"`
	RefMappings         map[string][]string `json:"ref_mappings"`
	CaseID              string              `json:"case_id"`
	Outcome             RunOutcome          `json:"outcome"`
	Reason              string              `json:"reason,omitempty"`
	Required            int                 `json:"required"`
	Retrieved           int                 `json:"retrieved"`
	Selected            int                 `json:"selected"`
	Relevant            int                 `json:"relevant"`
	Recall              float64             `json:"recall"`
	Precision           float64             `json:"precision"`
}

// RetrievalSummary micro-averages retrieval over measured cases.
type RetrievalSummary struct {
	Cases              []RetrievalCase `json:"cases"`
	Measured           int             `json:"measured"`
	Excluded           int             `json:"excluded"`
	Required           int             `json:"required"`
	Retrieved          int             `json:"retrieved"`
	Selected           int             `json:"selected"`
	Relevant           int             `json:"relevant"`
	NegativeControls   int             `json:"negative_controls"`
	FalsePositiveCases int             `json:"false_positive_cases"`
	FalsePositiveRate  float64         `json:"false_positive_rate"`
	Recall             float64         `json:"recall"`
	Precision          float64         `json:"precision"`
	RecallThreshold    float64         `json:"recall_threshold"`
	PrecisionThreshold float64         `json:"precision_threshold"`
	ThresholdsMet      bool            `json:"thresholds_met"`
}

// RunOutcome is the terminal state of one case×arm execution unit.
type RunOutcome string

const (
	OutcomeGradingPending RunOutcome = "grading_pending"
	OutcomePassed         RunOutcome = "passed"
	OutcomeFailed         RunOutcome = "failed"
	OutcomeNotRun         RunOutcome = "not_run"
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
	Repeat       int      `json:"repeat"`
	Score        *float64 `json:"mechanism_score,omitempty"`
	Text         string   `json:"text,omitempty"`
	PromptSHA256 string   `json:"prompt_sha256,omitempty"`
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

	Coverage       Coverage     `json:"coverage"`
	SplitBinding   SplitBinding `json:"split_binding"`
	Benefit        RunOutcome   `json:"benefit"`
	MechanismOnly  bool         `json:"mechanism_only"`
	RepeatOverride int          `json:"repeat_override,omitempty"`
	requestMode    string
	units          []unitRecord // per-generation inputs, for artifacts
	caseEvidence   map[string]any
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
// Each selected case's inputs are computed once and every arm is built from them, so
// all arms of a case see the same capability, revocation state, and pack.
func Run(corpus *Corpus, cfg RunConfig, provider Provider, grader Grader, inputs InputsFn) (*Summary, error) {
	if inputs == nil {
		return nil, fmt.Errorf("inputs function is required")
	}
	return run(corpus, cfg, provider, grader, func(p contracts.Profile, req contracts.ResolutionRequest) (resolver.EvalInputs, error) {
		return inputs(p, req.Task, req.WorkspaceHint)
	}, false)
}

// RunRequest uses the full production request path. Legacy Run is task-only
// and its evidence deliberately does not claim risk/harness were forwarded.
func RunRequest(corpus *Corpus, cfg RunConfig, provider Provider, grader Grader, inputs RequestInputsFn) (*Summary, error) {
	return run(corpus, cfg, provider, grader, inputs, true)
}

func run(corpus *Corpus, cfg RunConfig, provider Provider, grader Grader, inputs RequestInputsFn, completeRequest bool) (*Summary, error) {
	if corpus == nil {
		return nil, fmt.Errorf("corpus is required")
	}
	if inputs == nil {
		return nil, fmt.Errorf("inputs function is required")
	}
	if cfg.Repeats < 0 || cfg.Repeats > 5 {
		return nil, fmt.Errorf("repeat override must be between 0 and 5")
	}
	if len(cfg.Arms) > 0 && provider == nil {
		return nil, fmt.Errorf("provider is required (use MockProvider for deterministic runs)")
	}
	// Command/live output is never graded by the mechanism-only keyword grader.
	if _, ok := provider.(*CommandProvider); ok {
		grader = nil
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

	binding, err := corpus.bindSplit(cfg.SplitManifest, cfg.Split)
	if err != nil {
		return nil, err
	}
	runID := "run_" + hexTimeID()
	summary := &Summary{
		RunID:          runID,
		Benefit:        OutcomeNotRun,
		Coverage:       corpus.Coverage,
		SplitBinding:   binding,
		MechanismOnly:  grader != nil,
		RepeatOverride: cfg.Repeats,
		StartedAt:      time.Now().UTC().Format(time.RFC3339),
		Corpus:         corpus.Dir,
		Arms:           cfg.Arms,
		NetworkPolicy:  cfg.NetworkPolicy,
		DryRun:         cfg.DryRun,
		Retrieval:      RetrievalSummary{RecallThreshold: RecallThreshold, PrecisionThreshold: PrecisionThreshold},
	}

	selected := map[string]bool{}
	summary.requestMode = "legacy-task-only"
	if completeRequest {
		summary.requestMode = "full"
	}
	for _, id := range binding.Selected {
		selected[id] = true
	}
	summary.Coverage.Excluded = map[string]string{}
	for name, reason := range corpus.Coverage.Excluded {
		summary.Coverage.Excluded[name] = reason
	}
	for _, cs := range corpus.Cases {
		if !selected[cs.ID] {
			summary.Coverage.Excluded[corpus.caseFile[cs.ID]] = "member of another frozen split"
			continue
		}
		summary.Coverage.Selected++
		req := contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: cs.Scenario.Task, WorkspaceHint: cs.Scenario.Workspace}
		if completeRequest {
			req.RiskHint = cs.Risk
			if cfg.Harness != "none" {
				harness := cfg.Harness
				req.Harness = &harness
			}
		}
		in, inErr := inputs(armProfile(cs), req)
		if summary.caseEvidence == nil {
			summary.caseEvidence = map[string]any{}
		}
		summary.caseEvidence[cs.ID] = caseEvidence(cs, cfg, req, in, inErr, corpus, binding)
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
		rr.Text = gen.Text
		if grader == nil {
			res.PerRepeat = append(res.PerRepeat, rr)
			continue
		}
		score, why, gerr := grader.Grade(cs, gen)
		if gerr != nil {
			res.Outcome, res.Reason = OutcomeFailed, "grading error: "+gerr.Error()
			break
		}
		rr.Score, rr.Text = &score, gen.Text
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
	case res.Outcome == "" && grader == nil:
		res.Outcome, res.Reason = OutcomeGradingPending, "generation succeeded; independent blinded grades required"
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

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// ExitCode applies the runner's exit contract: 1 when any evaluation unit or
// retrieval measurement failed or any blocker was recorded; 3 when nothing
// failed but at least one was not_run; 0 only when everything passed.
func (s *Summary) ExitCode() int {
	if len(s.CaseResults) == 0 && len(s.Retrieval.Cases) == 0 && len(s.Blockers) == 0 {
		return 3
	}
	failed := len(s.Blockers) > 0
	notRun := false
	for _, cr := range s.CaseResults {
		switch cr.Outcome {
		case OutcomeFailed:
			failed = true
		case OutcomeNotRun, OutcomeGradingPending:
			notRun = true
		}
	}
	for _, rc := range s.Retrieval.Cases {
		switch rc.Outcome {
		case OutcomeFailed:
			failed = true
		case OutcomeNotRun, OutcomeGradingPending:
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
