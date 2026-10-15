package evalfixture

import (
	"encoding/json"
	"github.com/0merUfuk/beme/internal/evalrunner"
	"os"
	"path/filepath"
)

// WriteSyntheticSplit is test fixture provisioning, never owner approval.
func WriteSyntheticSplit(dir string) error {
	c, err := evalrunner.LoadCorpus(dir)
	if err != nil {
		return err
	}
	f := evalrunner.FrozenSplit{SchemaVersion: "1", ApprovedByUser: true, Frozen: true, ApprovalRef: "public-synthetic-test-only", EvidenceClass: "synthetic_mechanism", DatasetVersion: c.Version(), EvaluatorProtocol: "offline-blind-v1", RubricVersion: evalrunner.RubricVersion, BootstrapSeed: 42, BootstrapSamples: 2000}
	for _, cs := range c.Cases {
		f.Members = append(f.Members, evalrunner.SplitMember{CaseID: cs.ID, Family: cs.ID, Split: "calibration", FileHash: c.FixtureHash(cs.ID)})
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "split-manifest.json"), b, 0600)
}
