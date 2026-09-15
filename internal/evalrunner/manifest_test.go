package evalrunner_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/bootstrap"
	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"github.com/0merUfuk/beme/internal/storage"
)

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

type evidence struct {
	Arm          string                     `json:"arm"`
	PromptSHA    string                     `json:"prompt_sha256"`
	BootstrapSHA string                     `json:"bootstrap_sha256"`
	ContextSHA   string                     `json:"context_sha256"`
	Construction evalrunner.ArmConstruction `json:"arm_construction"`
	Corpus       struct {
		DatasetVersion string            `json:"dataset_version"`
		Files          map[string]string `json:"files"`
	} `json:"corpus"`
	Deployment struct {
		BuildGeneration string `json:"build_generation"`
		IndexRevision   string `json:"index_revision"`
		PolicyDigest    string `json:"policy_digest"`
	} `json:"deployment"`
	ModelSettings evalrunner.ModelSettings `json:"model_settings"`
	ManifestSHA   string                   `json:"manifest_sha256"`
}

// TestManifestsRecordObservedValues: every manifest holds exactly the
// run-manifest schema's fields, filled from observed values — corpus file
// hashes, the deployment's policy digest, index revision and build
// generation, the real bootstrap hash, provider-reported settings — and
// none of the former placeholders.
func TestManifestsRecordObservedValues(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	corpusDir := t.TempDir()
	if err := fx.WriteCorpus(corpusDir, d.AlphaPath, 1); err != nil {
		t.Fatal(err)
	}
	corpus, err := evalrunner.LoadCorpus(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	provider := evalrunner.NewMockProvider("")
	arms := []evalrunner.Arm{evalrunner.ArmB0, evalrunner.ArmB1, evalrunner.ArmB4}
	sum, err := evalrunner.Run(corpus, evalrunner.RunConfig{Arms: arms, OutputDir: out, SkipRetrieval: true, Harness: "mock-harness", HarnessVersion: "0.0.1-test"},
		provider, evalrunner.DeterministicGrader{}, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}
	in, err := d.Inputs(true)(fx.Profile, fx.Task, d.AlphaPath)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(out, sum.RunID)

	var schema struct {
		Required []string `json:"required"`
	}
	sb, err := os.ReadFile(filepath.Join("..", "..", "schemas", "evaluation", "run-manifest.schema.json"))
	if err != nil || json.Unmarshal(sb, &schema) != nil || len(schema.Required) != 36 {
		t.Fatalf("run-manifest schema unreadable: %v (%d fields)", err, len(schema.Required))
	}
	caseBytes, err := os.ReadFile(filepath.Join(corpusDir, "arms-case.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureHash := sha(caseBytes)
	datasetVersion := sha([]byte("arms-case.json\x00" + fixtureHash + "\n"))
	st, err := storage.Open(d.Runtime.ProjectionPath(fx.Profile))
	if err != nil {
		t.Fatal(err)
	}
	generation, _ := st.GetMeta("build_generation")
	st.Close()
	if generation == "" || !strings.HasPrefix(in.Pack.Resolution.IndexRevision, "idx_") || in.Pack.Resolution.SourceRevisionDigest == nil {
		t.Fatal("control: the deployment must expose a build generation, index revision, and source revision digest")
	}
	settings := provider.Settings()
	placeholders := []string{
		sha([]byte("capability-policy-v1")), sha([]byte("index-synthetic")), sha([]byte("bootstrap-v1")),
		sha([]byte("ranking-v1")), sha([]byte("tool-policy-v1")), "sha256:" + strings.Repeat("0", 64),
		settings.Provider + "-deterministic",
	}

	entries, err := os.ReadDir(filepath.Join(root, "manifests"))
	if err != nil || len(entries) != len(arms) {
		t.Fatalf("manifests: %d (%v)", len(entries), err)
	}
	manifests, evs := map[string]map[string]any{}, map[string]evidence{}
	for _, e := range entries {
		mb, _ := os.ReadFile(filepath.Join(root, "manifests", e.Name()))
		m := map[string]any{}
		if err := json.Unmarshal(mb, &m); err != nil {
			t.Fatal(err)
		}
		if len(m) != len(schema.Required) {
			t.Fatalf("manifest has %d fields, schema requires exactly %d", len(m), len(schema.Required))
		}
		for _, f := range schema.Required {
			if _, ok := m[f]; !ok {
				t.Fatalf("manifest missing %q", f)
			}
		}
		var ev evidence
		eb, _ := os.ReadFile(filepath.Join(root, "evidence", e.Name()))
		if err := json.Unmarshal(eb, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.ManifestSHA != sha(mb) {
			t.Fatal("evidence must record the manifest's digest")
		}
		var raw struct {
			Prompt    string `json:"prompt"`
			PromptSHA string `json:"prompt_sha256"`
		}
		rb, _ := os.ReadFile(filepath.Join(root, "raw", e.Name()))
		if err := json.Unmarshal(rb, &raw); err != nil || raw.Prompt == "" || sha([]byte(raw.Prompt)) != ev.PromptSHA || raw.PromptSHA != ev.PromptSHA {
			t.Fatalf("raw prompt and its digest must match the evidence (%v)", err)
		}
		manifests[ev.Arm], evs[ev.Arm] = m, ev

		want := map[string]any{
			"fixture_hash":            fixtureHash,
			"dataset_version":         datasetVersion,
			"capability_policy_hash":  in.Pack.Resolution.PolicyDigest,
			"index_snapshot_hash":     in.Pack.Resolution.IndexRevision,
			"profile_projection_hash": sha([]byte(fmt.Sprintf("profile=%s\ncapability_id=%s\nbuild_generation=%s\nindex_revision=%s\nsource_revision_digest=%s\n", fx.Profile, fx.CapabilityID, generation, in.Pack.Resolution.IndexRevision, *in.Pack.Resolution.SourceRevisionDigest))),
			"model_provider":          settings.Provider,
			"model_id":                settings.ModelID,
			"model_version":           settings.ModelVersion,
			"seed":                    float64(settings.Seed),
			"temperature":             settings.Temperature,
			"top_p":                   settings.TopP,
			"max_output_tokens":       float64(settings.MaxOutputTokens),
			"reasoning_budget":        nil,
			"harness":                 "mock-harness",
			"harness_version":         "0.0.1-test",
			"evaluator_id":            "deterministic-keyword-grader",
			"split":                   "calibration",
			"network_policy":          "disabled",
			"run_id":                  sum.RunID,
		}
		for k, v := range want {
			if m[k] != v {
				t.Fatalf("%s manifest %s = %v, want %v", ev.Arm, k, m[k], v)
			}
		}
		if ev.Corpus.DatasetVersion != datasetVersion || ev.Corpus.Files["arms-case.json"] != fixtureHash || ev.Deployment.BuildGeneration != generation || ev.ModelSettings != settings {
			t.Fatalf("%s evidence: %+v", ev.Arm, ev)
		}
		// Go test binaries record no module dependency information, so the
		// honest value here is "unknown"; a real binary reports the linked
		// module version. Either way it is never invented.
		if v, _ := m["sqlite_version"].(string); !strings.HasPrefix(v, "v") && v != "unknown" {
			t.Fatalf("sqlite_version must be the linked module version or unknown, got %q", v)
		}
		for k, v := range m {
			if s, ok := v.(string); ok {
				for _, p := range placeholders {
					if s == p {
						t.Fatalf("%s manifest %s holds placeholder %q", ev.Arm, k, s)
					}
				}
			}
		}
	}

	if manifests["B0"]["bootstrap_prompt_hash"] != sha(nil) || evs["B0"].ContextSHA != sha(nil) {
		t.Fatal("B0 must record the empty bootstrap and empty context")
	}
	for _, arm := range []string{"B1", "B4"} {
		if manifests[arm]["bootstrap_prompt_hash"] != bootstrap.SHA256() || evs[arm].BootstrapSHA != bootstrap.SHA256() {
			t.Fatalf("%s must record the hash of the real bootstrap text", arm)
		}
	}
	if evs["B4"].ContextSHA == evs["B1"].ContextSHA || manifests["B0"]["ranking_config_hash"] == manifests["B4"]["ranking_config_hash"] {
		t.Fatal("context digest and ranking config must reflect each arm's construction")
	}
	if c := evs["B4"].Construction; c.Source != "session_resolve" || c.Eligible != 9 || c.Selected != 6 || c.Truncated != 2 || c.BudgetTokens != 3000 {
		t.Fatalf("B4 construction descriptor: %+v", c)
	}

	var index map[string]string
	ib, err := os.ReadFile(filepath.Join(root, "bundle_index.json"))
	if err != nil || json.Unmarshal(ib, &index) != nil || len(index) != 4*len(arms)+2 {
		t.Fatalf("bundle index: %d entries (%v)", len(index), err)
	}
	for rel, digest := range index {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || sha(b) != digest {
			t.Fatalf("bundle index entry %s does not match the file (%v)", rel, err)
		}
	}
	blinded, _ := os.ReadDir(filepath.Join(root, "blinded"))
	for _, f := range blinded {
		b, _ := os.ReadFile(filepath.Join(root, "blinded", f.Name()))
		for _, arm := range arms {
			if strings.Contains(string(b), "\""+string(arm)+"\"") || strings.Contains(string(b), "prompt") {
				t.Fatalf("blinded artifact %s leaks arm identity or the prompt", f.Name())
			}
		}
	}
}
