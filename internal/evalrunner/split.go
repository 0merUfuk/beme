package evalrunner

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FrozenSplit is owner-authored before inspecting outcomes. The executable
// checks the binding, not the truth of the approval attestation. No command
// automatically approves gold or freezes a private corpus.
type FrozenSplit struct {
	SchemaVersion     string        `json:"schema_version"`
	ApprovedByUser    bool          `json:"approved_by_user"`
	Frozen            bool          `json:"frozen"`
	ApprovalRef       string        `json:"approval_ref"`
	EvidenceClass     string        `json:"evidence_class"` // synthetic_mechanism | owner_corpus
	DatasetVersion    string        `json:"dataset_version"`
	EvaluatorProtocol string        `json:"evaluator_protocol"`
	RubricVersion     string        `json:"rubric_version"`
	BootstrapSeed     int64         `json:"bootstrap_seed"`
	BootstrapSamples  int           `json:"bootstrap_samples"`
	Members           []SplitMember `json:"members"`
}
type SplitMember struct {
	CaseID   string `json:"case_id"`
	FileHash string `json:"file_sha256"`
	Family   string `json:"family"`
	Split    string `json:"split"`
}
type SplitBinding struct {
	Manifest FrozenSplit `json:"manifest"`
	SHA256   string      `json:"sha256"`
	Bytes    []byte      `json:"bytes"`
	Selected []string    `json:"selected_case_ids"`
}

func validSplit(s string) bool {
	return s == "calibration" || s == "development" || s == "locked_holdout" || s == "privacy_red_team"
}

func (c *Corpus) bindSplit(path, split string) (SplitBinding, error) {
	var binding SplitBinding
	if err := c.ensureUnchanged(); err != nil {
		return binding, err
	}
	if path == "" {
		path = filepath.Join(c.Dir, "split-manifest.json")
	}
	data, err := readBounded(path)
	if err != nil {
		return binding, fmt.Errorf("split verification required: %w", err)
	}
	var value any
	if err := decodeDocument(data, true, &value); err != nil {
		return binding, err
	}
	if err := validateSchema("split-manifest.schema.json", value); err != nil {
		return binding, fmt.Errorf("invalid split manifest: %w", err)
	}
	var f FrozenSplit
	if err := decodeDocument(data, true, &f); err != nil {
		return binding, fmt.Errorf("invalid split manifest: %w", err)
	}
	if f.SchemaVersion != "1" || !f.ApprovedByUser || !f.Frozen || strings.TrimSpace(f.ApprovalRef) == "" {
		return binding, fmt.Errorf("split manifest must be frozen and approved")
	}
	if f.EvidenceClass != "synthetic_mechanism" && f.EvidenceClass != "owner_corpus" {
		return binding, fmt.Errorf("invalid split evidence_class")
	}
	if f.DatasetVersion != c.Version() {
		return binding, fmt.Errorf("split dataset hash mismatch")
	}
	if f.EvaluatorProtocol != "offline-blind-v1" || f.RubricVersion != RubricVersion || f.BootstrapSamples < 1000 || f.BootstrapSamples > 100000 {
		return binding, fmt.Errorf("split evaluator protocol or bootstrap configuration invalid")
	}
	if !validSplit(split) {
		return binding, fmt.Errorf("invalid split")
	}
	seen, families := map[string]bool{}, map[string]string{}
	for _, m := range f.Members {
		if seen[m.CaseID] || c.caseFile[m.CaseID] == "" || m.FileHash != c.FixtureHash(m.CaseID) || strings.TrimSpace(m.Family) == "" || !validSplit(m.Split) {
			return binding, fmt.Errorf("invalid/duplicate split member or case hash")
		}
		if prior := families[m.Family]; prior != "" && prior != m.Split {
			return binding, fmt.Errorf("decision family crosses splits")
		}
		if m.Split == "locked_holdout" {
			for _, cs := range c.Cases {
				if cs.ID == m.CaseID && cs.Status != "locked" {
					return binding, fmt.Errorf("locked_holdout requires locked case status")
				}
			}
		}
		seen[m.CaseID] = true
		families[m.Family] = m.Split
		if m.Split == split {
			binding.Selected = append(binding.Selected, m.CaseID)
		}
	}
	if len(seen) != len(c.Cases) {
		return binding, fmt.Errorf("split membership must cover every case exactly once")
	}
	if len(binding.Selected) == 0 {
		return binding, fmt.Errorf("requested split has no members")
	}
	binding.Manifest = f
	binding.Bytes = data
	binding.SHA256 = sha256Hex(string(data))
	return binding, nil
}
