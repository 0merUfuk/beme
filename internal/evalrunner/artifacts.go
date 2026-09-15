package evalrunner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/0merUfuk/beme/internal/resolver"
)

// ResolverVersion labels the resolver release the runner links against.
const ResolverVersion = "beme-alpha.1"

// RubricVersion is the behavioral rubric (EVALUATION_CONTRACT §5.1).
const RubricVersion = "0-4-blind-paired"

// unitRecord is the observed input of one generation (or planned generation
// in a dry run).
type unitRecord struct {
	CaseID       string
	Arm          Arm
	Repeat       int
	Prompt       string
	PromptSHA    string
	BootstrapSHA string
	ContextSHA   string
	Construction ArmConstruction
	Deployment   deploymentFacts
}

// deploymentFacts identify the projection and capability an arm was built
// from, as the serving session reported them.
type deploymentFacts struct {
	Profile              string `json:"profile"`
	CapabilityID         string `json:"capability_id"`
	WorkspaceID          string `json:"workspace_id,omitempty"`
	IndexRevision        string `json:"index_revision"`
	BuildGeneration      string `json:"build_generation"`
	PolicyDigest         string `json:"policy_digest"`
	SourceRevisionDigest string `json:"source_revision_digest"`
}

func factsFrom(in resolver.EvalInputs) deploymentFacts {
	return deploymentFacts{
		Profile:              string(in.Profile),
		CapabilityID:         in.CapabilityID,
		WorkspaceID:          in.WorkspaceID,
		IndexRevision:        in.Pack.Resolution.IndexRevision,
		BuildGeneration:      in.BuildGeneration,
		PolicyDigest:         in.Pack.Resolution.PolicyDigest,
		SourceRevisionDigest: deref(in.Pack.Resolution.SourceRevisionDigest),
	}
}

// environment is the runtime environment recorded in manifests.
type environment struct {
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	GoVersion      string `json:"go_version"`
	MCPSDKVersion  string `json:"mcp_sdk_version"`
	SQLiteVersion  string `json:"sqlite_version"`
	NetworkPolicy  string `json:"network_policy"`
	Harness        string `json:"harness"`
	HarnessVersion string `json:"harness_version"`
}

// writeArtifacts implements §18.7 (manifests) and §18.7.1 (blinded result
// packaging). Directory layout under <OutputDir>/<run_id>/:
//
//   - manifests/  one file per generation holding exactly the fields of
//     schemas/evaluation/run-manifest.schema.json, from observed values;
//   - evidence/   owner-side: arm label and construction, prompt/context/
//     bootstrap digests, corpus file hashes, deployment revision, model
//     settings, and the manifest's own digest;
//   - raw/        owner-side: score, generated text, and the rendered prompt;
//   - blinded/    grader-visible: opaque arm IDs only (never condition
//     labels), score and generated text; not written for dry runs;
//   - arm_key.json (opaque→condition, withheld from graders), summary.json,
//     and bundle_index.json (sha256 of every other artifact).
func writeArtifacts(s *Summary, cfg RunConfig, provider Provider, grader Grader, corpus *Corpus) error {
	if cfg.OutputDir == "" {
		return nil // caller opted out of artifacts (unit tests may)
	}
	root := filepath.Join(cfg.OutputDir, s.RunID)
	for _, d := range []string{"raw", "blinded", "manifests", "evidence"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			return err
		}
	}

	// opaque arm IDs for blinding
	armKeys := map[Arm]string{}
	blindArms := map[string]string{} // opaque -> real, for the key file
	for _, a := range cfg.Arms {
		op := opaqueID("arm")
		armKeys[a] = op
		blindArms[op] = string(a)
	}

	settings := ModelSettings{Provider: "none", ModelID: "none", ModelVersion: "none"}
	if provider != nil {
		settings = provider.Settings()
	}
	evaluatorID, evaluatorVersion := fmt.Sprintf("%T", grader), "unversioned"
	if id, ok := grader.(Identified); ok {
		evaluatorID, evaluatorVersion = id.ID(), id.Version()
	}
	env := environment{
		OS: runtime.GOOS, Architecture: runtime.GOARCH, GoVersion: runtime.Version(),
		MCPSDKVersion: moduleVersion("github.com/modelcontextprotocol/go-sdk"),
		SQLiteVersion: moduleVersion("modernc.org/sqlite"),
		NetworkPolicy: s.NetworkPolicy, Harness: cfg.Harness, HarnessVersion: cfg.HarnessVersion,
	}
	envJSON, _ := json.Marshal(env)
	// Generation requests carry no tools; the policy is the network policy.
	toolPolicy, _ := json.Marshal(struct {
		Tools         []string `json:"tools"`
		NetworkPolicy string   `json:"network_policy"`
	}{[]string{}, s.NetworkPolicy})

	units := map[string]unitRecord{}
	for _, u := range s.units {
		units[unitKey(u.CaseID, u.Arm, u.Repeat)] = u
	}
	index := map[string]string{}
	write := func(rel string, v any) (string, error) {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(root, rel), b, 0o600); err != nil {
			return "", err
		}
		digest := sha256Hex(string(b))
		index[filepath.ToSlash(rel)] = digest
		return digest, nil
	}

	for _, cr := range s.CaseResults {
		for _, rep := range cr.PerRepeat {
			u, ok := units[unitKey(cr.CaseID, cr.Arm, rep.Repeat)]
			if !ok {
				return fmt.Errorf("no recorded inputs for %s/%s repeat %d", cr.CaseID, cr.Arm, rep.Repeat)
			}
			ranking, _ := json.Marshal(struct {
				Source       string `json:"source"`
				Ordering     string `json:"ordering"`
				Transform    string `json:"transform"`
				BudgetTokens int    `json:"budget_tokens"`
			}{u.Construction.Source, u.Construction.Ordering, u.Construction.Transform, u.Construction.BudgetTokens})
			d := u.Deployment
			m := map[string]any{
				"run_id":                  s.RunID,
				"git_commit":              gitCommit(),
				"dataset_version":         corpus.Version(),
				"split":                   cfg.Split,
				"case_id":                 cr.CaseID,
				"repeat_index":            rep.Repeat,
				"fixture_hash":            corpus.FixtureHash(cr.CaseID),
				"capability_policy_hash":  d.PolicyDigest,
				"profile_projection_hash": sha256Hex(fmt.Sprintf("profile=%s\ncapability_id=%s\nbuild_generation=%s\nindex_revision=%s\nsource_revision_digest=%s\n", d.Profile, d.CapabilityID, d.BuildGeneration, d.IndexRevision, d.SourceRevisionDigest)),
				"index_snapshot_hash":     d.IndexRevision,
				"resolver_version":        ResolverVersion,
				"ranking_config_hash":     sha256Hex(string(ranking)),
				"bootstrap_prompt_hash":   u.BootstrapSHA,
				"model_provider":          settings.Provider,
				"model_id":                settings.ModelID,
				"model_version":           settings.ModelVersion,
				"harness":                 cfg.Harness,
				"harness_version":         cfg.HarnessVersion,
				"seed":                    settings.Seed,
				"temperature":             settings.Temperature,
				"top_p":                   settings.TopP,
				"max_output_tokens":       settings.MaxOutputTokens,
				"reasoning_budget":        settings.ReasoningBudget,
				"evaluator_id":            evaluatorID,
				"evaluator_version":       evaluatorVersion,
				"rubric_version":          RubricVersion,
				"tool_policy_hash":        sha256Hex(string(toolPolicy)),
				"os":                      env.OS,
				"architecture":            env.Architecture,
				"go_version":              env.GoVersion,
				"mcp_sdk_version":         env.MCPSDKVersion,
				"sqlite_version":          env.SQLiteVersion,
				"environment_digest":      sha256Hex(string(envJSON)),
				"network_policy":          s.NetworkPolicy,
				"started_at":              s.StartedAt,
				"ended_at":                s.EndedAt,
			}
			name := fmt.Sprintf("%s_%s_rep%02d.json", sanitize(cr.CaseID), armKeys[cr.Arm], rep.Repeat)
			manifestDigest, err := write(filepath.Join("manifests", name), m)
			if err != nil {
				return err
			}
			s.ManifestPaths = append(s.ManifestPaths, filepath.Join("manifests", name))

			if _, err := write(filepath.Join("evidence", name), map[string]any{
				"case_id":          cr.CaseID,
				"arm":              string(cr.Arm),
				"opaque_arm":       armKeys[cr.Arm],
				"repeat":           rep.Repeat,
				"prompt_sha256":    u.PromptSHA,
				"bootstrap_sha256": u.BootstrapSHA,
				"context_sha256":   u.ContextSHA,
				"arm_construction": u.Construction,
				"corpus": map[string]any{
					"dataset_version": corpus.Version(),
					"files":           corpus.Files,
				},
				"deployment":      d,
				"model_settings":  settings,
				"environment":     env,
				"manifest_sha256": manifestDigest,
			}); err != nil {
				return err
			}

			// raw result (owner-visible)
			if _, err := write(filepath.Join("raw", name), map[string]any{
				"case_id":       cr.CaseID,
				"repeat":        rep.Repeat,
				"score":         rep.Score,
				"text":          rep.Text,
				"prompt_sha256": u.PromptSHA,
				"prompt":        u.Prompt,
			}); err != nil {
				return err
			}
			if s.DryRun {
				continue // nothing generated, nothing to grade
			}
			// blinded result (grader-visible: opaque arm)
			if _, err := write(filepath.Join("blinded", name), map[string]any{
				"case_id": cr.CaseID,
				"arm":     armKeys[cr.Arm],
				"repeat":  rep.Repeat,
				"score":   rep.Score,
				"text":    rep.Text,
			}); err != nil {
				return err
			}
		}
	}

	// the arm key: withheld from the grader (blinding contract, §18.7.1)
	if _, err := write("arm_key.json", blindArms); err != nil {
		return err
	}
	if _, err := write("summary.json", s); err != nil {
		return err
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "bundle_index.json"), b, 0o600)
}

func unitKey(caseID string, arm Arm, repeat int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", caseID, arm, repeat)
}

func opaqueID(prefix string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return prefix + "_fallback00"
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func sanitize(s string) string {
	out := strings.Builder{}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// gitCommit: BEME_EVAL_GIT_COMMIT when set, else the VCS revision stamped
// into the binary by the Go toolchain, else "unknown-checkout".
func gitCommit() string {
	if v := os.Getenv("BEME_EVAL_GIT_COMMIT"); v != "" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", false
		for _, st := range bi.Settings {
			switch st.Key {
			case "vcs.revision":
				rev = st.Value
			case "vcs.modified":
				dirty = st.Value == "true"
			}
		}
		if rev != "" {
			if dirty {
				rev += "-dirty"
			}
			return rev
		}
	}
	return "unknown-checkout"
}

// moduleVersion reports the linked version of a module dependency:
// "not-linked" when this binary records dependencies but not this one, and
// "unknown" when the binary records no dependency information at all (Go
// test binaries do not). It never guesses a version.
func moduleVersion(path string) string {
	bi, ok := debug.ReadBuildInfo()
	if !ok || len(bi.Deps) == 0 {
		return "unknown"
	}
	for _, dep := range bi.Deps {
		if dep.Path == path {
			if dep.Replace != nil {
				return dep.Replace.Version
			}
			return dep.Version
		}
	}
	return "not-linked"
}
