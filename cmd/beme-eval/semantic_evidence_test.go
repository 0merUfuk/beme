package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evidenceJSON(t *testing.T, root, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func evidenceWrite(t *testing.T, root, name string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, name), b, 0600); err != nil {
		t.Fatal(err)
	}
}
func evidenceHash(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}
func evidenceClone(t *testing.T, root string) string {
	t.Helper()
	dest := t.TempDir()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		q := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(q, 0700)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(q, b, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dest
}
func evidenceReindex(t *testing.T, root string) {
	t.Helper()
	idx := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "bundle_index.json" {
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		idx[filepath.ToSlash(rel)] = evidenceHash(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceWrite(t, root, "bundle_index.json", idx)
}
func evidenceFirst(t *testing.T, root, dir string) string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(root, dir, "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatal("missing", dir, err)
	}
	p, _ := filepath.Rel(root, names[0])
	return p
}

func TestReindexedContradictoryEvidenceRejectedBeforeTemplate(t *testing.T) {
	cfg, corpus := deployment(t)
	command, _ := fakeCommand(t)
	code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0,B4", "--out", t.TempDir())
	if code != 3 {
		t.Fatalf("generation %d %s", code, raw)
	}
	for _, name := range []string{"selected-population", "arm-label", "manifest-substitution", "raw-prompt", "raw-answer", "remove-owner", "blinded-gold", "blinded-task", "zero-bootstrap", "unfrozen", "cross-split", "coverage", "generations", "extra-artifact", "missing-binding", "duplicate-binding", "extra-repeat", "missing-case-result", "request-task", "request-risk", "context-pack", "eligible-hash", "arm-key", "construction"} {
		t.Run(name, func(t *testing.T) {
			b := evidenceClone(t, rep.Artifacts)
			var bindings []map[string]any
			data, _ := os.ReadFile(filepath.Join(b, "grading_index.json"))
			json.Unmarshal(data, &bindings)
			modify := func(path string, fn func(map[string]any)) {
				v := evidenceJSON(t, b, path)
				fn(v)
				evidenceWrite(t, b, path, v)
			}
			switch name {
			case "selected-population":
				modify("summary.json", func(v map[string]any) { v["split_binding"].(map[string]any)["selected_case_ids"] = []any{} })
			case "arm-label":
				for _, v := range bindings {
					if v["arm"] == "B0" {
						v["arm"] = "B4"
					} else {
						v["arm"] = "B0"
					}
				}
			case "manifest-substitution":
				for _, v := range bindings {
					v["manifest_sha256"] = bindings[0]["manifest_sha256"]
				}
			case "raw-prompt":
				modify(evidenceFirst(t, b, "raw"), func(v map[string]any) { v["prompt"] = "" })
			case "raw-answer":
				modify(evidenceFirst(t, b, "raw"), func(v map[string]any) { v["text"] = "DIFFERENT ANSWER" })
			case "remove-owner":
				for _, dir := range []string{"raw", "evidence", "retrieval"} {
					os.RemoveAll(filepath.Join(b, dir))
				}
			case "blinded-gold", "blinded-task":
				p := bindings[0]["artifact_path"].(string)
				modify(p, func(v map[string]any) {
					if name == "blinded-task" {
						v["task"] = "Different task"
					} else {
						v["gold"].(map[string]any)["acceptable_decisions"] = []string{"different approved decision"}
					}
				})
				data, _ := os.ReadFile(filepath.Join(b, p))
				bindings[0]["artifact_sha256"] = evidenceHash(data)
			case "zero-bootstrap", "unfrozen", "cross-split":
				modify("corpus/split-manifest.json", func(v map[string]any) {
					if name == "zero-bootstrap" {
						v["bootstrap_samples"] = 0
					} else if name == "unfrozen" {
						v["frozen"] = false
					} else {
						v["members"].([]any)[0].(map[string]any)["split"] = "development"
					}
				})
			case "coverage":
				modify("summary.json", func(v map[string]any) { v["coverage"].(map[string]any)["selected"] = 0 })
			case "generations":
				modify("summary.json", func(v map[string]any) { v["generations"] = 0 })
			case "extra-artifact":
				evidenceWrite(t, b, "raw/extra.json", map[string]any{"extra": true})
			case "missing-binding":
				bindings = bindings[1:]
			case "duplicate-binding":
				bindings = append(bindings, bindings[0])
			case "extra-repeat":
				bindings[0]["repeat"] = 99
			case "missing-case-result":
				modify("summary.json", func(v map[string]any) { v["case_results"] = v["case_results"].([]any)[1:] })
			case "request-task", "request-risk", "context-pack", "eligible-hash":
				modify(evidenceFirst(t, b, "retrieval"), func(v map[string]any) {
					switch name {
					case "request-task":
						v["request"].(map[string]any)["task"] = "unrelated request"
					case "request-risk":
						v["request"].(map[string]any)["risk_hint"] = "low"
					case "context-pack":
						v["pack"].(map[string]any)["unknowns"] = []any{}
					case "eligible-hash":
						v["eligible_snapshot_sha256"] = "sha256:bad"
					}
				})
			case "arm-key":
				modify("arm_key.json", func(v map[string]any) {
					for k, a := range v {
						if a == "B0" {
							v[k] = "B4"
						} else {
							v[k] = "B0"
						}
					}
				})
			case "construction":
				modify(evidenceFirst(t, b, "evidence"), func(v map[string]any) { v["arm_construction"].(map[string]any)["source"] = "invented" })
			}
			evidenceWrite(t, b, "grading_index.json", bindings)
			evidenceReindex(t, b)
			target := filepath.Join(t.TempDir(), "template.json")
			code, _, raw := runJSON(t, "grade-template", "--bundle", b, "--out", target, "--evaluator", "synthetic")
			if code != 1 || strings.Contains(raw, "panic:") {
				t.Fatalf("contradictory evidence accepted or panicked: %d %s", code, raw)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("invalid evidence emitted a template")
			}
		})
	}
}

func TestRecordedRepeatOverrideRemainsGradeable(t *testing.T) {
	for _, repeats := range []string{"1", "3"} {
		t.Run(repeats, func(t *testing.T) {
			cfg, corpus := deployment(t)
			command, _ := fakeCommand(t)
			code, rep, raw := runJSON(t, "behavioral", "--corpus", corpus, "--config", cfg, "--provider", "command", "--command", command, "--arms", "B0,B4", "--repeats", repeats, "--out", t.TempDir())
			if code != 3 {
				t.Fatal(code, raw)
			}
			code, _, raw = runJSON(t, "grade-template", "--bundle", rep.Artifacts, "--out", filepath.Join(t.TempDir(), "template.json"), "--evaluator", "synthetic")
			if code != 0 {
				t.Fatal(code, raw)
			}
		})
	}
}
