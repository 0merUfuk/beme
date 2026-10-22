package evalrunner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"github.com/0merUfuk/beme/internal/resolver"
)

func TestRetrievalEvidenceBindsRiskAndHarness(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	c := syntheticCorpus(t, d, 1)
	out := t.TempDir()
	s, err := evalrunner.RunRequest(c, evalrunner.RunConfig{OutputDir: out, Harness: "synthetic-harness", Refs: d.Refs()}, nil, nil, func(profile contracts.Profile, req contracts.ResolutionRequest) (resolver.EvalInputs, error) {
		if req.RiskHint != c.Cases[0].Risk || req.Harness == nil || *req.Harness != "synthetic-harness" {
			t.Fatalf("runtime request lost risk/harness: %+v", req)
		}
		return d.Runtime.EvalInputsRequest(profile, fx.CapabilityID, true, req)
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, s.RunID, "retrieval", c.Cases[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(b, &evidence); err != nil {
		t.Fatal(err)
	}
	request := evidence["request"].(map[string]any)
	if request["risk_hint"] != c.Cases[0].Risk || request["harness"] != "synthetic-harness" {
		t.Fatalf("case request lost narrowed risk/harness: %+v", request)
	}
}
