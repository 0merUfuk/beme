package evalrunner_test

import (
	"encoding/json"
	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHoldoutRequiresLockedCases(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	c := syntheticCorpus(t, d, 1)
	p := filepath.Join(c.Dir, "arms-case.json")
	b, _ := os.ReadFile(p)
	var cs map[string]any
	json.Unmarshal(b, &cs)
	cs["status"] = "approved"
	b, _ = json.Marshal(cs)
	os.WriteFile(p, b, 0600)
	if err := fx.WriteSyntheticSplit(c.Dir); err != nil {
		t.Fatal(err)
	}
	p = filepath.Join(c.Dir, "split-manifest.json")
	b, _ = os.ReadFile(p)
	var f evalrunner.FrozenSplit
	json.Unmarshal(b, &f)
	f.Members[0].Split = "locked_holdout"
	b, _ = json.Marshal(f)
	os.WriteFile(p, b, 0600)
	c, err := evalrunner.LoadCorpus(c.Dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = evalrunner.Run(c, evalrunner.RunConfig{Split: "locked_holdout", SkipRetrieval: true, Arms: []evalrunner.Arm{evalrunner.ArmB0}}, evalrunner.NewMockProvider(""), nil, d.Inputs(false))
	if err == nil {
		t.Fatal("approved but unsealed case accepted as locked holdout")
	}
}

func TestFrozenMembershipAndExactSplitCoverage(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	c := syntheticCorpus(t, d, 1)
	b, _ := os.ReadFile(filepath.Join(c.Dir, "arms-case.json"))
	var cs map[string]any
	json.Unmarshal(b, &cs)
	cs["id"] = "decision.synthetic.other.v1"
	b, _ = json.Marshal(cs)
	os.WriteFile(filepath.Join(c.Dir, "other.json"), b, 0600)
	if err := fx.WriteSyntheticSplit(c.Dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.Dir, "split-manifest.json")
	original, _ := os.ReadFile(path)
	for _, mutation := range []string{"missing", "duplicate", "family", "case-hash", "dataset", "approval", "seed", "rubric"} {
		t.Run(mutation, func(t *testing.T) {
			var f map[string]any
			json.Unmarshal(original, &f)
			members := f["members"].([]any)
			switch mutation {
			case "missing":
				f["members"] = members[:1]
			case "duplicate":
				f["members"] = append(members, members[0])
			case "family":
				members[1].(map[string]any)["family"] = members[0].(map[string]any)["family"]
				members[1].(map[string]any)["split"] = "development"
			case "case-hash":
				members[0].(map[string]any)["file_sha256"] = f["dataset_version"]
			case "dataset":
				f["dataset_version"] = members[0].(map[string]any)["file_sha256"]
			case "approval":
				f["approved_by_user"] = false
			case "seed":
				delete(f, "bootstrap_seed")
			case "rubric":
				f["rubric_version"] = "permissive"
			}
			b, _ := json.Marshal(f)
			os.WriteFile(path, b, 0600)
			loaded, err := evalrunner.LoadCorpus(c.Dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := evalrunner.Run(loaded, evalrunner.RunConfig{Arms: []evalrunner.Arm{evalrunner.ArmB0}, SkipRetrieval: true}, forbiddenProvider{t}, nil, d.Inputs(false)); err == nil {
				t.Fatal("invalid frozen membership accepted")
			}
		})
	}
	var f evalrunner.FrozenSplit
	json.Unmarshal(original, &f)
	f.Members[1].Split = "development"
	b, _ = json.Marshal(f)
	os.WriteFile(path, b, 0600)
	loaded, err := evalrunner.LoadCorpus(c.Dir)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := evalrunner.Run(loaded, evalrunner.RunConfig{Arms: []evalrunner.Arm{evalrunner.ArmB0}, SkipRetrieval: true, DryRun: true}, forbiddenProvider{t}, nil, d.Inputs(false))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Coverage.Physical != 2 || sum.Coverage.Loaded != 2 || sum.Coverage.Selected != 1 || len(sum.Coverage.Excluded) != 1 || sum.PlannedGenerations != 1 {
		t.Fatalf("incorrect split selection/coverage: %+v", sum)
	}
}

func TestMissingSplitNeverBecomesHoldout(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	c := syntheticCorpus(t, d, 1)
	// Even a valid approved corpus is not evidence of frozen split membership.
	os.Remove(filepath.Join(c.Dir, "split-manifest.json"))
	_, err := evalrunner.Run(c, evalrunner.RunConfig{Split: "locked_holdout", SkipRetrieval: true, Arms: []evalrunner.Arm{evalrunner.ArmB0}}, evalrunner.NewMockProvider(""), nil, d.Inputs(false))
	if err == nil || !strings.Contains(err.Error(), "split") {
		t.Fatalf("unverified holdout accepted: %v", err)
	}
}
