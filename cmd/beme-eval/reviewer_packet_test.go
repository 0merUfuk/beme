package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
)

func TestReviewerPacketHasNoConditionOrderMetadata(t *testing.T) {
	cfg, corpus := deployment(t)
	command, _ := fakeCommand(t)
	cs := evidenceJSON(t, corpus, "arms-case.json")
	for i := 0; i < 19; i++ {
		cs["id"] = fmt.Sprintf("decision.synthetic.packet-%02d.v1", i)
		evidenceWrite(t, corpus, fmt.Sprintf("packet-%02d.json", i), cs)
	}
	if err := fx.WriteSyntheticSplit(corpus); err != nil {
		t.Fatal(err)
	}
	code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0,B4", "--repeats", "1", "--out", t.TempDir())
	if code != 3 {
		t.Fatal(code, raw)
	}
	packet := filepath.Join(t.TempDir(), "review.json")
	code, _, raw = runJSON(t, "reviewer-packet", "--bundle", rep.Artifacts, "--evaluator", "synthetic-independent-reviewer", "--out", packet)
	if code != 0 {
		t.Fatalf("content-only reviewer packet unavailable: %d %s", code, raw)
	}
	p := evidenceJSON(t, filepath.Dir(packet), filepath.Base(packet))
	samples := p["samples"].([]any)
	if len(samples) != 40 {
		t.Fatal("packet population", len(samples))
	}
	allowed := map[string]bool{"sample_id": true, "case_id": true, "repeat": true, "task": true, "gold": true, "rubric_version": true, "answer_sha256": true, "context_sha256": true, "text": true}
	predictions := map[string]string{}
	counts := map[string]int{}
	// Only exposed packet order/identifiers. Do not read owner labels yet.
	for _, x := range samples {
		sample := x.(map[string]any)
		for k := range sample {
			if !allowed[k] {
				t.Fatal("unexpected per-sample metadata", k)
			}
		}
		id := sample["sample_id"].(string)
		group := fmt.Sprint(sample["case_id"], sample["repeat"])
		prediction := "B0"
		if counts[group] > 0 {
			prediction = "B4"
		}
		counts[group]++
		predictions[id] = prediction
	}
	evidenceWrite(t, filepath.Dir(packet), "metadata-only-predictions.json", predictions)
	var bindings []map[string]any
	data, err := os.ReadFile(filepath.Join(rep.Artifacts, "grading_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &bindings)
	correct := 0
	for _, b := range bindings {
		if predictions[b["sample_id"].(string)] == b["arm"] {
			correct++
		}
	}
	if correct == len(bindings) {
		t.Fatal("all conditions exposed by packet order")
	}
	t.Logf("metadata-only predictions persisted before label access: %d/%d correct", correct, len(bindings))
	template := p["template"].(map[string]any)
	template["evaluator"] = map[string]any{"id": "synthetic-independent-reviewer", "version": "1", "independent": true, "blinded": true, "conflicts": "none"}
	// Supplied scores are positive mechanism controls only, after the attack.
	arms := map[string]string{}
	for _, b := range bindings {
		arms[b["sample_id"].(string)] = b["arm"].(string)
	}
	for _, x := range template["records"].([]any) {
		r := x.(map[string]any)
		score := 0
		if arms[r["sample_id"].(string)] == "B4" {
			score = 4
		}
		r["decision"] = score
		r["reasoning"] = score
		r["quality"] = 4
		r["rationale"] = "Synthetic test input, not live benefit"
		r["safety"] = map[string]bool{"privacy_leakage": false, "capability_elevation": false, "fabricated_authority": false, "hard_policy_violation": false, "invented_preference": false}
	}
	evidenceWrite(t, filepath.Dir(packet), "grades.json", template)
	code, _, raw = runJSON(t, "grade-import", "--bundle", rep.Artifacts, "--grades", filepath.Join(filepath.Dir(packet), "grades.json"), "--out", t.TempDir())
	if code != 0 || !strings.Contains(raw, `"benefit":"not_run"`) {
		t.Fatalf("packet lost grade binding: %d %s", code, raw)
	}
}
