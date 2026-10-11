package evalrunner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SampleBinding struct {
	SampleID      string `json:"sample_id"`
	CaseID        string `json:"case_id"`
	Arm           Arm    `json:"arm"`
	Repeat        int    `json:"repeat"`
	AnswerSHA     string `json:"answer_sha256"`
	ContextSHA    string `json:"context_sha256"`
	ArtifactSHA   string `json:"artifact_sha256"`
	ManifestSHA   string `json:"manifest_sha256"`
	ArtifactPath  string `json:"artifact_path"`
	ContextSalt   string `json:"context_salt"`
	RawContextSHA string `json:"raw_context_sha256"`
}

type SafetyBlockers struct {
	PrivacyLeakage      bool `json:"privacy_leakage"`
	CapabilityElevation bool `json:"capability_elevation"`
	FabricatedAuthority bool `json:"fabricated_authority"`
	HardPolicyViolation bool `json:"hard_policy_violation"`
	InventedPreference  bool `json:"invented_preference"`
}

func (s SafetyBlockers) Any() bool {
	return s.PrivacyLeakage || s.CapabilityElevation || s.FabricatedAuthority || s.HardPolicyViolation || s.InventedPreference
}

type EvaluatorIdentity struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Independent bool   `json:"independent"`
	Blinded     bool   `json:"blinded"`
	Conflicts   string `json:"conflicts"`
}
type GradeRecord struct {
	SampleID    string          `json:"sample_id"`
	AnswerSHA   string          `json:"answer_sha256"`
	ContextSHA  string          `json:"context_sha256"`
	ArtifactSHA string          `json:"artifact_sha256"`
	ManifestSHA string          `json:"manifest_sha256"`
	Decision    *float64        `json:"decision"`
	Reasoning   *float64        `json:"reasoning"`
	Quality     *float64        `json:"quality"`
	Safety      *SafetyBlockers `json:"safety"`
	Rationale   string          `json:"rationale"`
}
type GradeDocument struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	BundleSHA     string            `json:"bundle_sha256"`
	Evaluator     EvaluatorIdentity `json:"evaluator"`
	// review, or adjudication with exact original review document hashes.
	Kind     string        `json:"kind"`
	Resolves []string      `json:"resolves"`
	Records  []GradeRecord `json:"records"`
}

type verifiedBundle struct {
	Root     string
	SHA      string
	Index    map[string]string
	Summary  Summary
	Bindings []SampleBinding
	Corpus   *Corpus // reconstructed and validated, never from summary labels
}

func verifyBundle(root string) (verifiedBundle, error) {
	v := verifiedBundle{Root: root}
	b, err := readBounded(filepath.Join(root, "bundle_index.json"))
	if err != nil {
		return v, err
	}
	if err := decodeDocument(b, true, &v.Index); err != nil {
		return v, err
	}
	v.SHA = sha256Hex(string(b))
	if len(v.Index) == 0 {
		return v, fmt.Errorf("empty bundle index")
	}
	for name, digest := range v.Index {
		if !fs.ValidPath(name) || strings.Contains(name, "\\") || name == "bundle_index.json" {
			return v, fmt.Errorf("unsafe bundle path")
		}
		// Check every ancestor against symlink substitution, not just the file.
		dir := root
		for _, part := range strings.Split(name, "/") {
			dir = filepath.Join(dir, part)
			st, e := os.Lstat(dir)
			if e != nil || st.Mode()&os.ModeSymlink != 0 {
				return v, fmt.Errorf("unreadable or linked artifact")
			}
		}
		data, err := readBounded(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || sha256Hex(string(data)) != digest {
			return v, fmt.Errorf("artifact hash mismatch: %s", name)
		}
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel != "bundle_index.json" && v.Index[rel] == "" {
			return fmt.Errorf("unindexed artifact: %s", rel)
		}
		return nil
	})
	if err != nil {
		return v, err
	}
	if err := v.read("summary.json", &v.Summary); err != nil {
		return v, err
	}
	if err := v.read("grading_index.json", &v.Bindings); err != nil {
		return v, err
	}
	for _, binding := range v.Bindings {
		if v.Index[binding.ArtifactPath] != binding.ArtifactSHA || !strings.HasPrefix(binding.ArtifactPath, "blinded/") {
			return v, fmt.Errorf("grading artifact binding mismatch")
		}
		var sample map[string]any
		if err := v.read(binding.ArtifactPath, &sample); err != nil {
			return v, err
		}
		text, _ := sample["text"].(string)
		if sha256Hex(text) != binding.AnswerSHA || sample["sample_id"] != binding.SampleID || sample["case_id"] != binding.CaseID || sample["repeat"] != float64(binding.Repeat) || sample["answer_sha256"] != binding.AnswerSHA || sample["context_sha256"] != binding.ContextSHA || sha256Hex(binding.ContextSalt+binding.RawContextSHA) != binding.ContextSHA {
			return v, fmt.Errorf("answer/context binding mismatch")
		}
		manifestFound := false
		for name, digest := range v.Index {
			if strings.HasPrefix(name, "manifests/") && digest == binding.ManifestSHA {
				manifestFound = true
			}
		}
		if !manifestFound {
			return v, fmt.Errorf("manifest binding missing")
		}
	}
	if err := v.reconcileEvidence(); err != nil {
		return v, err
	}
	return v, nil
}
func (v verifiedBundle) read(name string, out any) error {
	if v.Index[name] == "" {
		return fmt.Errorf("bundle missing %s", name)
	}
	b, err := readBounded(filepath.Join(v.Root, name))
	if err != nil {
		return err
	}
	return decodeDocument(b, true, out)
}

// GradeTemplate contains no prefilled judgments or independence attestations.
// Owner-side API; share ExportReviewerPacket, never the blinded/ directory.
func GradeTemplate(bundle, evaluator string) (GradeDocument, error) {
	var g GradeDocument
	v, err := verifyBundle(bundle)
	if err != nil {
		return g, err
	}
	if v.Summary.DryRun || len(v.Bindings) == 0 {
		return g, fmt.Errorf("no generated answers to grade")
	}
	return templateFrom(v, evaluator), nil
}

func templateFrom(v verifiedBundle, evaluator string) GradeDocument {
	g := GradeDocument{SchemaVersion: "1", RunID: v.Summary.RunID, BundleSHA: v.SHA, Kind: "review", Resolves: []string{}, Evaluator: EvaluatorIdentity{ID: evaluator}}
	for _, b := range v.Bindings {
		g.Records = append(g.Records, GradeRecord{SampleID: b.SampleID, AnswerSHA: b.AnswerSHA, ContextSHA: b.ContextSHA, ArtifactSHA: b.ArtifactSHA, ManifestSHA: b.ManifestSHA})
	}
	sort.Slice(g.Records, func(i, j int) bool { return g.Records[i].SampleID < g.Records[j].SampleID })
	return g
}

func validateGrades(v verifiedBundle, path string) (GradeDocument, []byte, error) {
	var g GradeDocument
	b, err := readBounded(path)
	if err != nil {
		return g, nil, err
	}
	var value any
	if err := decodeDocument(b, true, &value); err != nil {
		return g, nil, err
	}
	if err := validateSchema("grades.schema.json", value); err != nil {
		return g, nil, fmt.Errorf("invalid grades: %w", err)
	}
	if err := decodeDocument(b, true, &g); err != nil {
		return g, nil, err
	}
	if g.BundleSHA != v.SHA || g.RunID != v.Summary.RunID {
		return g, nil, fmt.Errorf("grades bind another bundle")
	}
	if !g.Evaluator.Independent || !g.Evaluator.Blinded || g.Evaluator.Conflicts != "none" || strings.TrimSpace(g.Evaluator.ID) == "" || strings.TrimSpace(g.Evaluator.Version) == "" {
		return g, nil, fmt.Errorf("independent blinded evaluator declaration required")
	}
	known := map[string]SampleBinding{}
	for _, binding := range v.Bindings {
		if _, ok := known[binding.SampleID]; ok {
			return g, nil, fmt.Errorf("duplicate bound sample")
		}
		known[binding.SampleID] = binding
	}
	seen := map[string]bool{}
	for _, r := range g.Records {
		binding, ok := known[r.SampleID]
		if !ok || seen[r.SampleID] {
			return g, nil, fmt.Errorf("unknown or duplicate grade")
		}
		seen[r.SampleID] = true
		if r.AnswerSHA != binding.AnswerSHA || r.ContextSHA != binding.ContextSHA || r.ArtifactSHA != binding.ArtifactSHA || r.ManifestSHA != binding.ManifestSHA {
			return g, nil, fmt.Errorf("grade answer/context/artifact binding mismatch")
		}
		if strings.TrimSpace(r.Rationale) == "" {
			return g, nil, fmt.Errorf("grade rationale required")
		}
	}
	if len(seen) != len(known) {
		return g, nil, fmt.Errorf("incomplete grades: need every generated answer exactly once")
	}
	return g, b, nil
}
