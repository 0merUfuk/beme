package main

import (
	"encoding/json"
	"fmt"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Scores below are intentionally supplied test inputs, NOT observed model
// judgments. They exercise gate acceptance/rejection without model calls.
func TestOfflineGradePairedControls(t *testing.T) {
	for _, control := range []string{"improvement", "no-op", "degraded", "high-risk", "quality-only", "blocker"} {
		t.Run(control, func(t *testing.T) {
			cfg, corpus := deployment(t)
			command, _ := fakeCommand(t)
			// Two independent synthetic families make cluster intervals estimable.
			b, _ := os.ReadFile(filepath.Join(corpus, "arms-case.json"))
			var cs map[string]any
			json.Unmarshal(b, &cs)
			cs["risk"] = "high"
			b, _ = json.Marshal(cs)
			os.WriteFile(filepath.Join(corpus, "arms-case.json"), b, 0600)
			cs["id"] = "decision.synthetic.second-family.v1"
			b, _ = json.Marshal(cs)
			os.WriteFile(filepath.Join(corpus, "second.json"), b, 0600)
			if control == "high-risk" {
				for i := 0; i < 8; i++ {
					cs["id"] = fmt.Sprintf("decision.synthetic.extra-%d.v1", i)
					data, _ := json.Marshal(cs)
					os.WriteFile(filepath.Join(corpus, fmt.Sprintf("extra-%d.json", i)), data, 0600)
				}
			}
			if err := fx.WriteSyntheticSplit(corpus); err != nil {
				t.Fatal(err)
			}
			code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0,B4", "--out", t.TempDir())
			if code != 3 {
				t.Fatalf("generation %d %s", code, raw)
			}
			path := filepath.Join(t.TempDir(), "grades.json")
			code, _, raw = runJSON(t, "grade-template", "--bundle", rep.Artifacts, "--out", path, "--evaluator", "synthetic-reviewer")
			if code != 0 {
				t.Fatal(raw)
			}
			b, _ = os.ReadFile(path)
			var g map[string]any
			json.Unmarshal(b, &g)
			g["evaluator"] = map[string]any{"id": "synthetic-reviewer", "version": "1", "independent": true, "blinded": true, "conflicts": "none"}
			b, _ = os.ReadFile(filepath.Join(rep.Artifacts, "grading_index.json"))
			var bindings []map[string]any
			json.Unmarshal(b, &bindings)
			arms := map[string]string{}
			caseIDs := map[string]string{}
			for _, x := range bindings {
				arms[x["sample_id"].(string)] = x["arm"].(string)
				caseIDs[x["sample_id"].(string)] = x["case_id"].(string)
			}
			for _, v := range g["records"].([]any) {
				r := v.(map[string]any)
				score := 0.0
				if arms[r["sample_id"].(string)] == "B4" {
					score = 4
				}
				if control == "no-op" {
					score = 2
				}
				if control == "degraded" {
					score = 4 - score
				}
				r["decision"] = score
				r["reasoning"] = score
				r["quality"] = score
				if control == "quality-only" {
					r["quality"] = 4.0
					if arms[r["sample_id"].(string)] == "B4" {
						r["quality"] = 3.74
					}
				}
				if control == "high-risk" {
					r["quality"] = 4.0
					if caseIDs[r["sample_id"].(string)] == fx.CaseID {
						r["decision"] = 3.0
						r["reasoning"] = 4.0
						if arms[r["sample_id"].(string)] == "B4" {
							r["decision"] = 2.0
						}
					}
				}
				r["rationale"] = "Synthetic test judgment; not a live evaluation"
				r["safety"] = map[string]any{"privacy_leakage": false, "capability_elevation": false, "fabricated_authority": control == "blocker" && score == 4, "hard_policy_violation": false, "invented_preference": false}
			}
			b, _ = json.Marshal(g)
			os.WriteFile(path, b, 0600)
			code, _, raw = runJSON(t, "grade-import", "--bundle", rep.Artifacts, "--grades", path, "--out", t.TempDir())
			want := 1
			if control == "improvement" {
				want = 0
			}
			if code != want {
				t.Fatalf("%s: exit %d want %d: %s", control, code, want, raw)
			}
			if !strings.Contains(raw, `"benefit":"not_run"`) {
				t.Fatalf("synthetic mechanism must not establish live benefit: %s", raw)
			}
			if control == "high-risk" || control == "quality-only" {
				var report struct {
					Result struct {
						Gates []struct {
							Name    string `json:"name"`
							Outcome string `json:"outcome"`
						} `json:"gates"`
					} `json:"result"`
				}
				if err := json.Unmarshal([]byte(raw), &report); err != nil {
					t.Fatal(err)
				}
				wanted := "high_risk_nonregression"
				if control == "quality-only" {
					wanted = "general_task_nonregression"
				}
				for _, g := range report.Result.Gates {
					if (g.Name == wanted) != (g.Outcome == "failed") {
						t.Fatalf("expected ONLY %s to fail: %s", wanted, raw)
					}
				}
			}
		})
	}
}

func TestOfflineGradeImportRejectsIncomplete(t *testing.T) {
	cfg, corpus := deployment(t)
	command, _ := fakeCommand(t)
	code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0,B4", "--out", t.TempDir())
	if code != 3 {
		t.Fatalf("generation must stay pending: %d %s", code, raw)
	}
	template := filepath.Join(t.TempDir(), "grades.json")
	code, _, raw = runJSON(t, "grade-template", "--bundle", rep.Artifacts, "--out", template, "--evaluator", "synthetic-reviewer")
	if code != 0 {
		t.Fatalf("template: %d %s", code, raw)
	}
	b, err := os.ReadFile(template)
	if err != nil {
		t.Fatal(err)
	}
	var grades map[string]any
	if err := json.Unmarshal(b, &grades); err != nil {
		t.Fatal(err)
	}
	code, _, raw = runJSON(t, "grade-import", "--bundle", rep.Artifacts, "--grades", template, "--out", t.TempDir())
	if code != 1 || !strings.Contains(raw, "grade") {
		t.Fatalf("incomplete grades must fail: %d %s", code, raw)
	}
}
