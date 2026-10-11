package evalrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

type Gate struct {
	Name    string     `json:"name"`
	Outcome RunOutcome `json:"outcome"`
	Reason  string     `json:"reason,omitempty"`
}
type Interval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}
type Endpoint struct {
	Wins        int       `json:"wins"`
	Ties        int       `json:"ties"`
	Losses      int       `json:"losses"`
	Denominator int       `json:"denominator"`
	Exclusions  int       `json:"exclusions"`
	WinRate     float64   `json:"win_rate"`
	LossRate    float64   `json:"loss_rate"`
	Effect      float64   `json:"effect"`
	EffectCI    *Interval `json:"effect_ci95"`
	WinCI       *Interval `json:"win_ci95"`
	LossCI      *Interval `json:"loss_ci95"`
}
type ScoreStats struct {
	Mean     float64 `json:"mean"`
	Variance float64 `json:"variance"`
	Worst    float64 `json:"worst"`
	N        int     `json:"n"`
}
type GradedCase struct {
	CaseID    string     `json:"case_id"`
	Arm       Arm        `json:"arm"`
	Decision  ScoreStats `json:"decision"`
	Reasoning ScoreStats `json:"reasoning"`
	Quality   ScoreStats `json:"quality"`
}
type GradeReport struct {
	Thresholds            map[string]float64 `json:"thresholds"`
	AnalysisExecutableSHA string             `json:"analysis_executable_sha256"`
	SchemaVersion         string             `json:"schema_version"`
	RunID                 string             `json:"run_id"`
	BundleSHA             string             `json:"bundle_sha256"`
	Outcome               RunOutcome         `json:"outcome"`
	Benefit               RunOutcome         `json:"benefit"`
	EvidenceClass         string             `json:"evidence_class"`
	Scope                 string             `json:"scope"`
	Artifacts             string             `json:"artifacts,omitempty"`
	ReviewHashes          []string           `json:"review_sha256"`
	AdjudicationSHA       string             `json:"adjudication_sha256,omitempty"`
	Gates                 []Gate             `json:"gates"`
	Blockers              []string           `json:"blockers"`
	Decision              Endpoint           `json:"decision"`
	Reasoning             Endpoint           `json:"reasoning"`
	QualityDelta          float64            `json:"quality_delta"`
	QualityDenominator    int                `json:"quality_case_denominator"`
	QualityCI             *Interval          `json:"quality_delta_ci95"`
	HighRiskTransitions   int                `json:"high_risk_pass_to_fail"`
	HighRiskPairs         int                `json:"high_risk_pair_denominator"`
	Families              int                `json:"families"`
	BootstrapSeed         int64              `json:"bootstrap_seed"`
	BootstrapSamples      int                `json:"bootstrap_samples"`
	Cases                 []GradedCase       `json:"cases"`
}

func (r GradeReport) ExitCode() int {
	if r.Outcome == OutcomeFailed {
		return 1
	}
	if r.Outcome != OutcomePassed {
		return 3
	}
	return 0
}

// ImportGrades never edits the generation bundle. Each validated import is
// stored in a new owner-only result directory with the original review bytes.
func ImportGrades(bundle string, paths []string, adjudication, out string) (*GradeReport, error) {
	v, err := verifyBundle(bundle)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("grades required")
	}
	if len(v.Bindings) == 0 || v.Summary.DryRun {
		return nil, fmt.Errorf("no generated answers")
	}
	docs := []GradeDocument{}
	raw := [][]byte{}
	ids := map[string]bool{}
	hashes := []string{}
	for _, path := range paths {
		g, b, err := validateGrades(v, path)
		if err != nil {
			return nil, err
		}
		if g.Kind != "review" || len(g.Resolves) != 0 || ids[g.Evaluator.ID] {
			return nil, fmt.Errorf("distinct reviewers and review documents required")
		}
		ids[g.Evaluator.ID] = true
		docs = append(docs, g)
		raw = append(raw, b)
		hashes = append(hashes, sha256Hex(string(b)))
	}
	bySample := func(g GradeDocument) map[string]GradeRecord {
		m := map[string]GradeRecord{}
		for _, r := range g.Records {
			m[r.SampleID] = r
		}
		return m
	}
	final := bySample(docs[0])
	disagrees := false
	blockers := map[string]bool{}
	for _, g := range docs {
		for _, r := range g.Records {
			a := final[r.SampleID]
			if *a.Decision != *r.Decision || *a.Reasoning != *r.Reasoning || *a.Quality != *r.Quality || !reflect.DeepEqual(a.Safety, r.Safety) {
				disagrees = true
			}
			if r.Safety.Any() {
				blockers[r.SampleID] = true
			}
		}
	}
	var adjudBytes []byte
	adjudHash := ""
	if adjudication != "" {
		a, b, err := validateGrades(v, adjudication)
		if err != nil {
			return nil, err
		}
		expected := append([]string{}, hashes...)
		got := append([]string{}, a.Resolves...)
		sort.Strings(expected)
		sort.Strings(got)
		if a.Kind != "adjudication" || ids[a.Evaluator.ID] || !reflect.DeepEqual(expected, got) {
			return nil, fmt.Errorf("adjudication must independently bind every original review")
		}
		final = bySample(a)
		adjudBytes = b
		adjudHash = sha256Hex(string(b))
		disagrees = false
		for _, r := range a.Records {
			if r.Safety.Any() {
				blockers[r.SampleID] = true
			}
		} // never waive any reported blocker
	}
	r, err := compareGrades(v, final, blockers)
	if err != nil {
		return nil, err
	}
	r.ReviewHashes = hashes
	r.AdjudicationSHA = adjudHash
	if disagrees {
		r.Scope += "; PROVISIONAL first-review statistics only: unresolved disagreement is not a final grade"
		r.Gates = append(r.Gates, Gate{"adjudication", OutcomeNotRun, "independent reviewers disagree; locked adjudication required"})
		generationFailed := false
		for _, cr := range v.Summary.CaseResults {
			if cr.Outcome == OutcomeFailed {
				generationFailed = true
			}
		}
		if len(r.Blockers) == 0 && !generationFailed {
			r.Outcome = OutcomeNotRun
		}
		r.Benefit = OutcomeNotRun
	}
	if v.Summary.MechanismOnly || r.EvidenceClass == "synthetic_mechanism" {
		r.Benefit = OutcomeNotRun
	}
	if out == "" {
		return r, nil
	}
	if err := CheckOutputOutsideBundle(out, bundle); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		return nil, err
	}
	root := filepath.Join(out, opaqueID("graded"))
	if err := os.Mkdir(root, 0700); err != nil {
		return nil, err
	}
	r.Artifacts = root
	index := map[string]string{}
	for i, b := range raw {
		name := fmt.Sprintf("review-%02d.json", i+1)
		if err := writeExclusive(filepath.Join(root, name), b); err != nil {
			return nil, err
		}
		index[name] = sha256Hex(string(b))
	}
	if adjudBytes != nil {
		if err := writeExclusive(filepath.Join(root, "adjudication.json"), adjudBytes); err != nil {
			return nil, err
		}
		index["adjudication.json"] = adjudHash
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeExclusive(filepath.Join(root, "result.json"), b); err != nil {
		return nil, err
	}
	index["result.json"] = sha256Hex(string(b))
	b, err = json.MarshalIndent(index, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeExclusive(filepath.Join(root, "bundle_index.json"), b); err != nil {
		return nil, err
	}
	return r, nil
}
