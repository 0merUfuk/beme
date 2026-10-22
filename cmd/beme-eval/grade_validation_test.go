package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/evalrunner"
)

func syntheticGrades(t *testing.T) (string, map[string]any) {
	t.Helper()
	cfg, corpus := deployment(t)
	command, _ := fakeCommand(t)
	code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0,B4", "--out", t.TempDir())
	if code != 3 {
		t.Fatal(raw)
	}
	g, err := evalrunner.GradeTemplate(rep.Artifacts, "synthetic-reviewer")
	if err != nil {
		t.Fatal(err)
	}
	g.Evaluator = evalrunner.EvaluatorIdentity{ID: "synthetic-reviewer", Version: "1", Independent: true, Blinded: true, Conflicts: "none"}
	for i := range g.Records {
		a, b, c := 3.0, 3.0, 3.0
		g.Records[i].Decision = &a
		g.Records[i].Reasoning = &b
		g.Records[i].Quality = &c
		g.Records[i].Safety = &evalrunner.SafetyBlockers{}
		g.Records[i].Rationale = "Synthetic test judgment only"
	}
	b, _ := json.Marshal(g)
	var m map[string]any
	json.Unmarshal(b, &m)
	return rep.Artifacts, m
}
func writeGrade(t *testing.T, g map[string]any) string {
	t.Helper()
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "grades.json")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func cloneGrade(g map[string]any) map[string]any {
	b, _ := json.Marshal(g)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}
func digestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

func TestGradeValidationAndImmutableBindings(t *testing.T) {
	bundle, g := syntheticGrades(t)
	for _, mutation := range []string{"missing", "duplicate", "range", "null", "unknown", "safety", "unblinded", "identity", "answer", "context", "artifact", "bundle"} {
		t.Run(mutation, func(t *testing.T) {
			x := cloneGrade(g)
			rs := x["records"].([]any)
			r := rs[0].(map[string]any)
			switch mutation {
			case "missing":
				x["records"] = rs[1:]
			case "duplicate":
				x["records"] = append(rs, rs[0])
			case "range":
				r["decision"] = 4.1
			case "null":
				r["quality"] = nil
			case "unknown":
				r["waive_blocker"] = true
			case "safety":
				delete(r["safety"].(map[string]any), "fabricated_authority")
			case "unblinded":
				x["evaluator"].(map[string]any)["blinded"] = false
			case "identity":
				x["evaluator"].(map[string]any)["id"] = " "
			case "answer":
				r["answer_sha256"] = r["artifact_sha256"]
			case "context":
				r["context_sha256"] = r["artifact_sha256"]
			case "artifact":
				r["artifact_sha256"] = r["context_sha256"]
			case "bundle":
				x["bundle_sha256"] = r["context_sha256"]
			}
			code, _, raw := runJSON(t, "grade-import", "--bundle", bundle, "--grades", writeGrade(t, x), "--out", t.TempDir())
			if code != 1 {
				t.Fatalf("invalid %s accepted: %d %s", mutation, code, raw)
			}
		})
	}
	// Correctly hashed old grades cannot follow an edited answer/artifact.
	path := writeGrade(t, g)
	files, _ := os.ReadDir(filepath.Join(bundle, "blinded"))
	os.WriteFile(filepath.Join(bundle, "blinded", files[0].Name()), []byte(`{"text":"changed"}`), 0600)
	code, _, raw := runJSON(t, "grade-import", "--bundle", bundle, "--grades", path, "--out", t.TempDir())
	if code != 1 {
		t.Fatalf("edited bundle accepted: %d %s", code, raw)
	}
}

func TestIndependentAdjudicationCannotWaiveSafety(t *testing.T) {
	bundle, g := syntheticGrades(t)
	p1 := writeGrade(t, g)
	second := cloneGrade(g)
	second["evaluator"].(map[string]any)["id"] = "reviewer-two"
	second["records"].([]any)[0].(map[string]any)["decision"] = 2.0
	p2 := writeGrade(t, second)
	code, _, raw := runJSON(t, "grade-import", "--bundle", bundle, "--grades", p1, "--grades", p2, "--out", t.TempDir())
	if code != 3 {
		t.Fatalf("unadjudicated disagreement must be pending: %d %s", code, raw)
	}
	second["records"].([]any)[0].(map[string]any)["safety"].(map[string]any)["invented_preference"] = true
	p2 = writeGrade(t, second)
	adj := cloneGrade(g)
	adj["kind"] = "adjudication"
	adj["evaluator"].(map[string]any)["id"] = "independent-adjudicator"
	adj["resolves"] = []string{digestFile(t, p1), digestFile(t, p2)}
	code, _, raw = runJSON(t, "grade-import", "--bundle", bundle, "--grades", p1, "--grades", p2, "--adjudication", writeGrade(t, adj), "--out", t.TempDir())
	if code != 1 {
		t.Fatalf("adjudicator waived blocker: %d %s", code, raw)
	}
	adj["resolves"] = []string{digestFile(t, p1)}
	code, _, raw = runJSON(t, "grade-import", "--bundle", bundle, "--grades", p1, "--grades", p2, "--adjudication", writeGrade(t, adj), "--out", t.TempDir())
	if code != 1 {
		t.Fatalf("incomplete adjudication accepted: %d %s", code, raw)
	}
}
