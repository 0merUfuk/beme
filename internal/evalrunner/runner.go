// Package evalrunner implements the executable evaluation runner defined
// by evals/EVALUATION_CONTRACT.md (WP: evaluation infrastructure).
//
// Scope: the MECHANISM is fully implemented and proven with deterministic
// mock providers and public synthetic fixtures. Executing it against paid
// live models or the private corpus is owner-gated — the runner reports
// not_run for any condition it cannot honestly execute (never silent).
//
// Supported:
//   - Baselines B0–B4 and the ablations whose pack transform exists
//     (no-provenance, no-unknowns, learned-only) — every baseline shares the
//     same hard capability boundary (§18.6). no-scope and canonical-only have
//     no transform yet and are reported not_run, never graded as the full
//     pack under an ablation label.
//   - Repeat runs (1..5) with per-case mean/variance/worst.
//   - Immutable manifests per §18.7 (36 required fields) written per run,
//     content-hashed.
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

// Provider is a model backend. A deterministic mock implements the same
// interface as a live provider, so the runner is proven without paid calls.
type Provider interface {
	// Name identifies the provider for manifests (mock_* for mocks).
	Name() string
	// Generate produces the agent's answer for one case under one arm.
	Generate(req GenerationRequest) (GenerationResult, error)
}

// GenerationRequest is what a provider sees for one case run.
type GenerationRequest struct {
	RunID     string
	CaseID    string
	Arm       Arm
	Repeat    int
	Pack      resolver.Pack // resolved context (empty for B0)
	Bootstrap string        // bootstrap text (B1+)
	Task      string        // case task text
}

// GenerationResult is the raw model output for grading.
type GenerationResult struct {
	Text       string            `json:"text"`
	TokensUsed int               `json:"tokens_used"`
	Meta       map[string]string `json:"meta,omitempty"`
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
}

// LoadCorpus reads every *.json case in dir.
func LoadCorpus(dir string) (*Corpus, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := &Corpus{Dir: dir}
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
		if cs.Grading.Repeats <= 0 {
			cs.Grading.Repeats = 1
		}
		c.Cases = append(c.Cases, cs)
	}
	if len(c.Cases) == 0 {
		return nil, fmt.Errorf("no cases in %s", dir)
	}
	return c, nil
}

// RunConfig controls a run set.
type RunConfig struct {
	Arms          []Arm
	Repeats       int // override per-case repeats when >0
	OutputDir     string
	Harness       string
	NetworkPolicy string // disabled | integration_only
	// Only arms listed are executed; others are reported not_run.

	// Refs maps a selected record to the identifiers gold
	// required_evidence_refs may use (key, source record ID). When nil,
	// retrieval metrics are reported not_run.
	Refs RefsFn
}

// RefsFn returns the identifiers a record answers to under a profile.
type RefsFn func(profile contracts.Profile, recordID string) []string

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
	Repeat int     `json:"repeat"`
	Score  float64 `json:"score"`
	Text   string  `json:"text,omitempty"`
}

// Summary aggregates a run set.
type Summary struct {
	RunID         string           `json:"run_id"`
	StartedAt     string           `json:"started_at"`
	EndedAt       string           `json:"ended_at"`
	Corpus        string           `json:"corpus"`
	Arms          []Arm            `json:"arms"`
	NetworkPolicy string           `json:"network_policy"`
	CaseResults   []CaseResult     `json:"case_results"`
	Blockers      []string         `json:"blockers"`
	ManifestPaths []string         `json:"manifest_paths"`
	Retrieval     RetrievalSummary `json:"retrieval"`
}

// ResolverFn resolves a pack for a case under a capability; injected so the
// runner works with any Session implementation.
type ResolverFn func(capability contracts.Profile, task, workspaceHint string) (resolver.Pack, error)

// Grader scores a generation against gold, 0–4 (§18.7.1).
type Grader interface {
	Grade(cs Case, gen GenerationResult) (float64, string, error)
}

// DeterministicGrader implements the 0–4 rubric mechanically for mock
// providers: acceptable-decision keyword presence → ≥3; mandatory
// conclusions covered add; prohibited conclusion present → 0 (blocker).
// Live grading is human/blind per the contract; this grader exists so the
// runner is fully testable end-to-end without paid models.
type DeterministicGrader struct{}

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
func Run(corpus *Corpus, cfg RunConfig, provider Provider, grader Grader, resolve ResolverFn) (*Summary, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider is required (use MockProvider for deterministic runs)")
	}
	runID := "run_" + hexTimeID()
	started := time.Now().UTC()

	if cfg.NetworkPolicy == "" {
		cfg.NetworkPolicy = "disabled"
	}
	requested := map[Arm]bool{}
	for _, a := range cfg.Arms {
		requested[a] = true
	}

	summary := &Summary{
		RunID:         runID,
		StartedAt:     started.Format(time.RFC3339),
		Corpus:        corpus.Dir,
		Arms:          cfg.Arms,
		NetworkPolicy: cfg.NetworkPolicy,
	}

	summary.Retrieval = RetrievalSummary{RecallThreshold: RecallThreshold, PrecisionThreshold: PrecisionThreshold}
	for _, cs := range corpus.Cases {
		summary.Retrieval.Cases = append(summary.Retrieval.Cases, measureRetrieval(cs, cfg.Refs, resolve))
		for _, arm := range cfg.Arms {
			// §18.6: the no-scope ablation runs ONLY on synthetic data in an
			// isolated, no-network environment. Corpus dirs are synthetic in
			// the public path; the private corpus is owner-run.
			repeats := cs.Grading.Repeats
			if cfg.Repeats > 0 {
				repeats = cfg.Repeats
			}
			if repeats > 5 {
				repeats = 5
			}
			res := CaseResult{CaseID: cs.ID, Arm: arm}
			if reason := unimplementedArm(arm); reason != "" {
				res.Outcome = OutcomeNotRun
				res.Reason = reason
				summary.CaseResults = append(summary.CaseResults, res)
				continue
			}
			// resolve the pack per arm semantics
			pack, packErr := resolve(armProfile(cs), cs.Scenario.Task, cs.Scenario.Workspace)
			if packErr != nil {
				res.Outcome = OutcomeNotRun
				res.Reason = "resolution failed: " + packErr.Error()
				summary.CaseResults = append(summary.CaseResults, res)
				continue
			}
			scores := []float64{}
			worst := math.Inf(1)
			for rep := 1; rep <= repeats; rep++ {
				req := GenerationRequest{
					RunID:     runID,
					CaseID:    cs.ID,
					Arm:       arm,
					Repeat:    rep,
					Pack:      packForArm(pack, arm),
					Bootstrap: bootstrapForArm(arm),
					Task:      cs.Scenario.Task,
				}
				gen, err := provider.Generate(req)
				if err != nil {
					res.Outcome = OutcomeFailed
					res.Reason = "generation error: " + err.Error()
					break
				}
				score, why, gerr := grader.Grade(cs, gen)
				if gerr != nil {
					res.Outcome = OutcomeFailed
					res.Reason = "grading error: " + gerr.Error()
					break
				}
				res.PerRepeat = append(res.PerRepeat, RepeatResult{Repeat: rep, Score: score, Text: gen.Text})
				if score == 0 {
					// prohibited/unsafe → binary blocker: the unit fails and
					// is never averaged (§18.7.1)
					summary.Blockers = append(summary.Blockers, fmt.Sprintf("%s/%s rep%d: %s", cs.ID, arm, rep, why))
					res.Outcome = OutcomeFailed
					res.Reason = "blocker: " + why
					res.Score = 0
					break
				}
				scores = append(scores, score)
				worst = math.Min(worst, score)
			}
			if res.Outcome == "" && len(scores) > 0 {
				mean := 0.0
				for _, s := range scores {
					mean += s
				}
				mean /= float64(len(scores))
				res.Score = math.Round(mean*100) / 100
				res.Outcome = OutcomePassed
			}
			summary.CaseResults = append(summary.CaseResults, res)
		}
	}
	summary.Retrieval.aggregate()
	summary.EndedAt = time.Now().UTC().Format(time.RFC3339)

	// manifests + blinded packaging
	if err := writeArtifacts(summary, cfg, provider, corpus); err != nil {
		return nil, err
	}
	return summary, nil
}

// armProfile maps case capability to the serving profile (same hard
// capability boundary for every arm, §18.6).
func armProfile(cs Case) contracts.Profile {
	if cs.Capability == "work-safe" {
		return contracts.ProfileWorkSafe
	}
	return contracts.ProfilePersonal
}

// packForArm applies arm semantics to the resolved pack.
func packForArm(p resolver.Pack, arm Arm) resolver.Pack {
	out := p
	switch arm {
	case ArmB0:
		// plain agent: NO pack content at all
		out.Constraints, out.Guidance, out.Precedents, out.Knowledge, out.Unknowns, out.Conflicts = nil, nil, nil, nil, nil, nil
		out.LearnedExperimental = nil
	case ArmB1:
		// bootstrap only: pack text present but no guidance content? B1 is
		// the managed bootstrap text with an EMPTY pack (§18.6: bootstrap
		// only). Keep sections empty.
		out.Constraints, out.Guidance, out.Precedents, out.Knowledge, out.Unknowns, out.Conflicts = nil, nil, nil, nil, nil, nil
		out.LearnedExperimental = nil
	case ArmB2:
		// raw full-profile dump: everything eligible under the capability is
		// already IN the pack (resolver returns eligible content); keep all
		// sections as-is.
	case ArmB3:
		// retrieval without precedence: flatten all sections into guidance,
		// losing precedence structure + provenance visibility.
		all := append([]resolver.ContextItem{}, out.Constraints...)
		all = append(all, out.Guidance...)
		all = append(all, out.Precedents...)
		out.Guidance = all
		out.Constraints, out.Precedents = nil, nil
		for i := range out.Guidance {
			out.Guidance[i].ProvenanceRefs = nil // no provenance in B3
		}
	case ArmAblNoProv:
		for i := range out.Guidance {
			out.Guidance[i].ProvenanceRefs = nil
		}
		for i := range out.Constraints {
			out.Constraints[i].ProvenanceRefs = nil
		}
		for i := range out.Precedents {
			out.Precedents[i].ProvenanceRefs = nil
		}
	case ArmAblNoUnknowns:
		out.Unknowns = nil
	case ArmAblLearnedOnly:
		out.Constraints, out.Guidance, out.Precedents = nil, nil, nil
	}
	return out
}

func bootstrapForArm(arm Arm) string {
	if arm == ArmB0 {
		return ""
	}
	return "beme-bootstrap-v1" // placeholder; real bootstrap text injected by caller config
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

// measureRetrieval resolves the full (B4) pack once per case and scores it
// against required_evidence_refs.
func measureRetrieval(cs Case, refs RefsFn, resolve ResolverFn) RetrievalCase {
	rc := RetrievalCase{CaseID: cs.ID, Required: len(cs.Gold.RequiredEvidenceRefs)}
	if refs == nil {
		rc.Outcome, rc.Reason = OutcomeNotRun, "no record reference lookup configured"
		return rc
	}
	if rc.Required == 0 {
		rc.Outcome, rc.Reason = OutcomeNotRun, "case declares no required_evidence_refs"
		return rc
	}
	profile := armProfile(cs)
	pack, err := resolve(profile, cs.Scenario.Task, cs.Scenario.Workspace)
	if err != nil {
		rc.Outcome, rc.Reason = OutcomeNotRun, "resolution failed: "+err.Error()
		return rc
	}
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

// unimplementedArm names arms whose pack transform does not exist. Grading
// them would silently re-grade the full pack under an ablation label.
func unimplementedArm(arm Arm) string {
	switch arm {
	case ArmAblNoScope:
		return "ablation not implemented: no-scope needs a scope-disabled resolver run on synthetic data (§18.6)"
	case ArmAblCanonOnly:
		return "ablation not implemented: canonical-only needs source-role data that ContextPack items do not carry"
	}
	return ""
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
