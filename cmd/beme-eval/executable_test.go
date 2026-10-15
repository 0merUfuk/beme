package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"gopkg.in/yaml.v3"
)

// This builds and executes the real standalone CLI, not run() alone. The only
// provider process is this test's literal synthetic answer helper.
func TestExecutableEvaluationEvidence(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "beme-eval")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, b)
	}
	cfg, corpus := deployment(t)
	command, marker := fakeCommand(t)
	t.Setenv("BEME_EVAL_FAKE_REGRESSION", "1")
	invoke := func(want int, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(bin, append(args, "--json")...)
		b, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				code = e.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		t.Logf("executable %s exit=%d", args[0], code)
		if code != want {
			t.Fatalf("exit=%d want=%d: %s", code, want, b)
		}
		return b
	}
	// Exact JSON/YAML/YML coverage; all enter the frozen calibration split.
	b, _ := os.ReadFile(filepath.Join(corpus, "arms-case.json"))
	var cs map[string]any
	json.Unmarshal(b, &cs)
	cs["risk"] = "high"
	for _, ext := range []string{"yaml", "yml"} {
		cs["id"] = "decision.synthetic." + ext + ".v1"
		data, _ := yaml.Marshal(cs)
		os.WriteFile(filepath.Join(corpus, "case."+ext), data, 0600)
	}
	if err := fx.WriteSyntheticSplit(corpus); err != nil {
		t.Fatal(err)
	}
	b = invoke(3, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0,B4", "--out", t.TempDir())
	var report cliReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if report.Summary.Generations != 6 || report.Summary.Coverage.Physical != 3 || report.Summary.Coverage.Selected != 3 || report.Summary.Benefit != "not_run" {
		t.Fatalf("false success or lost corpus: %+v", report.Summary)
	}
	for _, cr := range report.Summary.CaseResults {
		if cr.Outcome != "grading_pending" {
			t.Fatalf("ungraded answer passed: %+v", cr)
		}
		if cr.Arm == "B4" && !strings.Contains(cr.PerRepeat[0].Text, "client_server_db_by_convention") {
			t.Fatal("regression control not exercised")
		}
	}
	grade := filepath.Join(t.TempDir(), "grades.json")
	invoke(0, "grade-template", "--bundle", report.Artifacts, "--out", grade, "--evaluator", "synthetic-human")
	invoke(1, "grade-import", "--bundle", report.Artifacts, "--grades", grade, "--out", t.TempDir())
	// Intentionally supplied ratings are public mechanism inputs, not a
	// claim that a model or independent reviewer produced these judgments.
	b, _ = os.ReadFile(grade)
	var baseGrades map[string]any
	json.Unmarshal(b, &baseGrades)
	baseGrades["evaluator"] = map[string]any{"id": "synthetic-reviewer", "version": "1", "independent": true, "blinded": true, "conflicts": "none"}
	b, _ = os.ReadFile(filepath.Join(report.Artifacts, "grading_index.json"))
	var bindings []map[string]any
	json.Unmarshal(b, &bindings)
	arms := map[string]string{}
	for _, binding := range bindings {
		arms[binding["sample_id"].(string)] = binding["arm"].(string)
	}
	for _, control := range []string{"improvement", "no-op", "degraded", "missing", "out-of-range"} {
		g := cloneGrade(baseGrades)
		for _, value := range g["records"].([]any) {
			r := value.(map[string]any)
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
			r["rationale"] = "Supplied synthetic control, not live benefit"
			r["safety"] = map[string]any{"privacy_leakage": false, "capability_elevation": false, "fabricated_authority": false, "hard_policy_violation": false, "invented_preference": false}
		}
		if control == "missing" {
			g["records"] = g["records"].([]any)[1:]
		}
		if control == "out-of-range" {
			g["records"].([]any)[0].(map[string]any)["decision"] = 5
		}
		want := 1
		if control == "improvement" {
			want = 0
		}
		result := invoke(want, "grade-import", "--bundle", report.Artifacts, "--grades", writeGrade(t, g), "--out", t.TempDir())
		if control == "improvement" && !strings.Contains(string(result), `"benefit":"not_run"`) {
			t.Fatal("synthetic grades asserted benefit")
		}
		t.Logf("standalone supplied-control=%s expected_exit=%d", control, want)
	}
	before := calls(t, marker)
	os.Remove(filepath.Join(corpus, "split-manifest.json"))
	invoke(1, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--split", "locked_holdout", "--out", t.TempDir())
	if calls(t, marker) != before {
		t.Fatal("provider ran before split verification")
	}
	// No silent fallback for malformed/unapproved cases.
	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "invalid.json"), []byte(`{"id":"decision.invalid.v1"}`), 0600)
	invoke(1, "retrieval", "--corpus", bad, "--config", cfg, "--out", t.TempDir())
}
