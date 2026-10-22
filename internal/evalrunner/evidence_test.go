package evalrunner_test

import (
	"encoding/json"
	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestRetrievalOnlyEvidenceBindsInputs(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	c := syntheticCorpus(t, d, 1)
	out := t.TempDir()
	s, err := evalrunner.Run(c, evalrunner.RunConfig{OutputDir: out, Refs: d.Refs()}, nil, nil, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(out, s.RunID)
	for _, name := range []string{"run_manifest.json", "corpus/arms-case.json", "corpus/split-manifest.json", "retrieval/decision.architecture.storage.synthetic-arms.v1.json"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || len(b) == 0 {
			t.Fatalf("missing immutable evidence %s: %v", name, err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(root, "run_manifest.json"))
	var run map[string]any
	json.Unmarshal(b, &run)
	if _, ok := run["thresholds"]; !ok {
		t.Fatal("run does not bind the frozen gate thresholds")
	}
	b, _ = os.ReadFile(filepath.Join(root, "retrieval/decision.architecture.storage.synthetic-arms.v1.json"))
	var v map[string]any
	json.Unmarshal(b, &v)
	for _, key := range []string{"request", "deployment", "measurement", "pack", "eligible_snapshot_sha256", "fixture_sha256", "split_sha256"} {
		if _, ok := v[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
}
