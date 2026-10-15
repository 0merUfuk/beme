package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/google/jsonschema-go/jsonschema"
)

// Real source -> built CLI -> pack, including states absent from old fixtures.
func TestGeneratedRuntimePackSchema(t *testing.T) {
	bin := buildBemeBinary(t, "beme-runtime-schema")
	cfg, rt := diagnosticsDeployment(t)
	sd := rt.Sources[0]
	sd.Purpose = []string{"safe_declassified"}
	sd.Sensitivity = "public_general"
	sd.ProfilesAllowed = []contracts.Profile{contracts.ProfilePersonal, contracts.ProfileWorkSafe}
	b, err := json.Marshal(sd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cfg, "sources", "s.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "sources", "safe.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"personal", "work-safe"} {
		if out, se, code := runBeme(t, bin, "build", "--config", cfg, "--profile", profile); code != 0 {
			t.Fatalf("build %s: %d %s %s", profile, code, out, se)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "context-pack", "context-pack.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	validator, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"personal", "work-safe", "cache-failure"} {
		t.Run(state, func(t *testing.T) {
			profile := state
			if state == "cache-failure" {
				profile = "personal"
				if err := os.Rename(rt.Config.CacheDir, rt.Config.CacheDir+"-saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(rt.Config.CacheDir, []byte("synthetic blocker"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out, se, code := runBeme(t, bin, "preview", "--config", cfg, "--projection", profile, "--task", "Diagnostics fixture statement", "--json")
			if code != 0 {
				t.Fatalf("preview: %d %s %s", code, out, se)
			}
			var pack map[string]any
			if err := json.Unmarshal([]byte(out), &pack); err != nil {
				t.Fatal(err)
			}
			if len(pack["guidance"].([]any)) != 1 {
				t.Fatal("positive source evidence missing")
			}
			if state == "work-safe" && pack["resolution"].(map[string]any)["policy_digest"] != "withheld" {
				t.Fatal("work-safe correlatable digest exposed")
			}
			if state == "cache-failure" {
				if pack["trace_ref"] != "" {
					t.Fatal("unpersisted trace handle advertised")
				}
				reduced := false
				for _, d := range pack["degradations"].([]any) {
					if d.(map[string]any)["kind"] == "reduced_assurance" {
						reduced = true
					}
				}
				if !reduced {
					t.Fatal("empty trace must be an honestly reported persistence failure")
				}
			} else if pack["trace_ref"] == "" {
				t.Fatal("healthy trace not persisted")
			}
			if err := validator.Validate(pack); err != nil {
				t.Errorf("generated %s pack violates schema: %v", state, err)
			}
			for _, bad := range []struct {
				key   string
				value any
			}{{"trace_ref", "trace_short"}, {"policy_digest", "sha256:bad"}, {"policy_digest", "hidden"}} {
				var control map[string]any
				if err := json.Unmarshal([]byte(out), &control); err != nil {
					t.Fatal(err)
				}
				if bad.key == "policy_digest" {
					control["resolution"].(map[string]any)[bad.key] = bad.value
				} else {
					control[bad.key] = bad.value
				}
				if validator.Validate(control) == nil {
					t.Errorf("invalid %s=%v accepted", bad.key, bad.value)
				}
			}
		})
	}
}
