package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/evalrunner"
)

// Rehashing a bundle must not make path aliases, duplicate identifiers or
// malformed IDs gradeable. Exercise every semantic-evidence entry point.
func TestReindexedArtifactPathsAndIDsAreNotNormalized(t *testing.T) {
	original, grades := syntheticGrades(t)
	t.Run("canonical-positive", func(t *testing.T) {
		g, err := evalrunner.ImportGrades(original, []string{writeGrade(t, grades)}, "", t.TempDir())
		if err != nil || g == nil || g.Decision.Denominator != 1 || g.Benefit != evalrunner.OutcomeNotRun {
			t.Fatalf("canonical import must keep the synthetic population: %+v %v", g, err)
		}
	})
	for _, mutation := range []string{
		"summary-backslash", "summary-dot", "summary-traversal", "summary-double-slash", "summary-case", "summary-duplicate", "summary-missing",
		"binding-backslash", "binding-dot", "binding-traversal", "binding-duplicate", "sample-id-case", "arm-id-case",
		"index-backslash", "index-dot", "index-traversal", "index-drive", "index-duplicate-key",
	} {
		t.Run(mutation, func(t *testing.T) {
			bundle := evidenceClone(t, original)
			var bindings []map[string]any
			b, err := os.ReadFile(filepath.Join(bundle, "grading_index.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &bindings); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(mutation, "summary-") {
				s := evidenceJSON(t, bundle, "summary.json")
				paths := s["manifest_paths"].([]any)
				name := paths[0].(string)
				switch mutation {
				case "summary-backslash":
					paths[0] = strings.ReplaceAll(name, "/", `\`)
				case "summary-dot":
					paths[0] = "./" + name
				case "summary-traversal":
					paths[0] = "raw/../" + name
				case "summary-double-slash":
					paths[0] = strings.ReplaceAll(name, "/", "//")
				case "summary-case":
					paths[0] = strings.ToUpper(name)
				case "summary-duplicate":
					paths = append(paths, paths[0])
				case "summary-missing":
					paths = paths[1:]
				}
				s["manifest_paths"] = paths
				evidenceWrite(t, bundle, "summary.json", s)
			}
			name := bindings[0]["artifact_path"].(string)
			switch mutation {
			case "binding-backslash":
				bindings[0]["artifact_path"] = strings.ReplaceAll(name, "/", `\`)
			case "binding-dot":
				bindings[0]["artifact_path"] = "blinded/./" + strings.TrimPrefix(name, "blinded/")
			case "binding-traversal":
				bindings[0]["artifact_path"] = "blinded/../" + name
			case "binding-duplicate":
				bindings = append(bindings, bindings[0])
			case "sample-id-case":
				id := strings.ToUpper(bindings[0]["sample_id"].(string))
				bindings[0]["sample_id"] = id
				sample := evidenceJSON(t, bundle, name)
				sample["sample_id"] = id
				evidenceWrite(t, bundle, name, sample)
				bindings[0]["artifact_sha256"] = digestFile(t, filepath.Join(bundle, filepath.FromSlash(name)))
			case "arm-id-case":
				key := evidenceJSON(t, bundle, "arm_key.json")
				for id, arm := range key {
					delete(key, id)
					key[strings.ToUpper(id)] = arm
					break
				}
				evidenceWrite(t, bundle, "arm_key.json", key)
			}
			evidenceWrite(t, bundle, "grading_index.json", bindings)
			evidenceReindex(t, bundle)
			if strings.HasPrefix(mutation, "index-") {
				index := evidenceJSON(t, bundle, "bundle_index.json")
				var old string
				for key := range index {
					if strings.HasPrefix(key, "manifests/") {
						old = key
						break
					}
				}
				if old == "" {
					t.Fatal("positive fixture has no manifest index key")
				}
				key := old
				switch mutation {
				case "index-backslash":
					key = strings.ReplaceAll(old, "/", `\`)
				case "index-dot":
					key = "./" + old
				case "index-traversal":
					key = "raw/../" + old
				case "index-drive":
					key = "C:/" + old
				}
				index[key] = index[old]
				if key != old {
					delete(index, old)
				}
				evidenceWrite(t, bundle, "bundle_index.json", index)
				if mutation == "index-duplicate-key" {
					b, err := os.ReadFile(filepath.Join(bundle, "bundle_index.json"))
					if err != nil {
						t.Fatal(err)
					}
					entry, err := json.Marshal(map[string]any{old: index[old]})
					if err != nil {
						t.Fatal(err)
					}
					duplicate := append(append(entry[:len(entry)-1], ','), b[1:]...)
					if err := os.WriteFile(filepath.Join(bundle, "bundle_index.json"), duplicate, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			// Bind to the new index so import cannot pass this test merely by
			// rejecting an old anchor before semantic/path reconciliation.
			anchored := cloneGrade(grades)
			anchored["bundle_sha256"] = digestFile(t, filepath.Join(bundle, "bundle_index.json"))
			gradeFile := writeGrade(t, anchored)
			for _, command := range []string{"grade-template", "reviewer-packet", "grade-import"} {
				t.Run(command, func(t *testing.T) {
					out := t.TempDir()
					target := filepath.Join(out, "must-not-exist")
					args := []string{command, "--bundle", bundle, "--out", target}
					if command == "grade-import" {
						args = append(args, "--grades", gradeFile)
					} else {
						args = append(args, "--evaluator", "synthetic-reviewer")
					}
					code, _, raw := runJSON(t, args...)
					if code != 1 || !strings.Contains(raw, "error: grades:") || strings.Contains(raw, "panic:") {
						t.Fatalf("malformed evidence accepted or panicked: %d %s", code, raw)
					}
					if command == "grade-import" {
						entries, err := os.ReadDir(target)
						if err != nil || len(entries) != 0 {
							t.Fatalf("invalid evidence emitted a result: %v %v", entries, err)
						}
					} else if _, err := os.Lstat(target); !os.IsNotExist(err) {
						t.Fatalf("invalid evidence emitted an export: %v", err)
					}
				})
			}
		})
	}
}
