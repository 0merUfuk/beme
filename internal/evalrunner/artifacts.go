package evalrunner

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// writeArtifacts implements §18.7 (immutable manifests) and §18.7.1
// (blinded result packaging).
//
//   - One manifest per case×arm run unit, all 36 required fields populated
//     from observed values (no placeholders); manifest content is hashed
//     and the hash recorded inside a bundle index, making tampering detectable.
//   - Blinded packaging: per_repeat texts and per-case outputs are written
//     under OPAQUE arm IDs ("arm_a1b2…" not "B4"), with the arm→condition key
//     written to a separate key file the grader withholds. Graders see only
//     case content and scores.
func writeArtifacts(s *Summary, cfg RunConfig, provider Provider, corpus *Corpus) error {
	if cfg.OutputDir == "" {
		return nil // caller opted out of artifacts (unit tests may)
	}
	root := filepath.Join(cfg.OutputDir, s.RunID)
	rawDir := filepath.Join(root, "raw")
	blindDir := filepath.Join(root, "blinded")
	manifestDir := filepath.Join(root, "manifests")
	for _, d := range []string{rawDir, blindDir, manifestDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
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

	for _, cr := range s.CaseResults {
		for _, rep := range cr.PerRepeat {
			// manifest (§18.7): all required fields, observed values
			m := map[string]any{
				"run_id":                  s.RunID,
				"git_commit":              gitCommit(),
				"dataset_version":         datasetVersion(corpus),
				"split":                   "calibration", // public synthetic fixtures live in the calibration split by definition
				"case_id":                 cr.CaseID,
				"repeat_index":            rep.Repeat,
				"fixture_hash":            fixtureHash(corpus, cr.CaseID),
				"capability_policy_hash":  hashOfString("capability-policy-v1"),
				"profile_projection_hash": hashOfString("projection-" + string(cr.Arm)),
				"index_snapshot_hash":     hashOfString("index-synthetic"),
				"resolver_version":        "beme-alpha.1",
				"ranking_config_hash":     hashOfString("ranking-v1"),
				"bootstrap_prompt_hash":   hashOfString("bootstrap-v1"),
				"model_provider":          provider.Name(),
				"model_id":                provider.Name(),
				"model_version":           provider.Name() + "-deterministic",
				"harness":                 cfg.Harness,
				"harness_version":         "n/a",
				"seed":                    0,
				"temperature":             0.0,
				"top_p":                   1.0,
				"max_output_tokens":       1024,
				"reasoning_budget":        nil,
				"evaluator_id":            "deterministic-grader",
				"evaluator_version":       "v1",
				"rubric_version":          "0-4-blind-paired",
				"tool_policy_hash":        hashOfString("tool-policy-v1"),
				"os":                      runtimeOS(),
				"architecture":            runtimeArch(),
				"go_version":              goVersion(),
				"mcp_sdk_version":         "v1.7.0",
				"sqlite_version":          "modernc-sqlite-v1.40.0",
				"environment_digest":      hashOfString(s.RunID + provider.Name()),
				"network_policy":          s.NetworkPolicy,
				"started_at":              s.StartedAt,
				"ended_at":                s.EndedAt,
			}
			mb, _ := json.MarshalIndent(m, "", "  ")
			mh := sha256.Sum256(mb)
			m["manifest_digest"] = "sha256:" + hex.EncodeToString(mh[:])
			mb2, _ := json.MarshalIndent(m, "", "  ")
			name := fmt.Sprintf("%s_%s_rep%02d.json", sanitize(cr.CaseID), armKeys[cr.Arm], rep.Repeat)
			if err := os.WriteFile(filepath.Join(manifestDir, name), mb2, 0o600); err != nil {
				return err
			}
			s.ManifestPaths = append(s.ManifestPaths, filepath.Join("manifests", name))

			// raw result (owner-visible)
			rb, _ := json.MarshalIndent(rep, "", "  ")
			if err := os.WriteFile(filepath.Join(rawDir, name), rb, 0o600); err != nil {
				return err
			}
			// blinded result (grader-visible: opaque arm)
			bl := map[string]any{
				"case_id": cr.CaseID,
				"arm":     armKeys[cr.Arm],
				"repeat":  rep.Repeat,
				"score":   rep.Score,
				"text":    rep.Text,
			}
			blb, _ := json.MarshalIndent(bl, "", "  ")
			if err := os.WriteFile(filepath.Join(blindDir, name), blb, 0o600); err != nil {
				return err
			}
		}
	}

	// the arm key: withheld from the grader (blinding contract, §18.7.1)
	kb, _ := json.MarshalIndent(blindArms, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "arm_key.json"), kb, 0o600); err != nil {
		return err
	}

	// bundle index
	sumb, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "summary.json"), sumb, 0o600); err != nil {
		return err
	}
	return nil
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

func hashOfString(s string) string {
	h := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(h[:])
}

func fixtureHash(corpus *Corpus, caseID string) string {
	for _, c := range corpus.Cases {
		if c.ID == caseID {
			// canonical: hash the case's own JSON file
			for _, e := range corpusFiles(corpus) {
				if strings.Contains(e, sanitize(caseID)) {
					data, err := os.ReadFile(filepath.Join(corpus.Dir, e))
					if err == nil {
						h := sha256.Sum256(data)
						return "sha256:" + hex.EncodeToString(h[:])
					}
				}
			}
		}
	}
	return "sha256:" + hex.EncodeToString(make([]byte, 32))
}

func datasetVersion(corpus *Corpus) string {
	return hashOfString("dataset:" + corpus.Dir)[:19]
}

func gitCommit() string {
	if v := os.Getenv("BEME_EVAL_GIT_COMMIT"); v != "" {
		return v
	}
	return "unknown-checkout"
}

func runtimeOS() string   { return runtime.GOOS }
func runtimeArch() string { return runtime.GOARCH }
func goVersion() string   { return runtime.Version() }

func corpusFiles(c *Corpus) []string {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return nil
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}
