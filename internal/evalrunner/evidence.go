package evalrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

func writeExclusive(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	return err
}

func hashJSON(v any) string { b, _ := json.Marshal(v); return sha256Hex(string(b)) }

func caseEvidence(cs Case, cfg RunConfig, req contracts.ResolutionRequest, in resolver.EvalInputs, inErr error, c *Corpus, split SplitBinding) any {
	measurement := measureRetrieval(cs, cfg.Refs, in, inErr)
	reason := ""
	if inErr != nil {
		reason = inErr.Error()
	}
	return map[string]any{
		"case_id": cs.ID, "fixture_sha256": c.FixtureHash(cs.ID), "split_sha256": split.SHA256,
		"request": req, "profile": cs.Capability,
		"deployment": factsFrom(in), "pack": in.Pack, "eligible_snapshot": in.Eligible,
		"arm_replay":               armReplay{in.LearnedEnabled, in.Roles, in.NoScopePack, in.NoScopeRefusal, in.NoScopeEligible},
		"eligible_snapshot_sha256": hashJSON(in.Eligible), "pack_sha256": hashJSON(in.Pack),
		"measurement": measurement, "resolution_error": reason,
		"identity_scope": "eligible snapshot and delivered pack under this capability; runtime revision fields are recorded as reported, not independently reconstructed",
	}
}

func writeRunEvidence(s *Summary, cfg RunConfig, c *Corpus, root string, index map[string]string, write func(string, any) (string, error)) error {
	for name, b := range c.Bytes {
		rel := "corpus/" + name
		path, err := bundleFilePath(root, rel)
		if err != nil {
			return err
		}
		if err := writeExclusive(path, b); err != nil {
			return err
		}
		index[rel] = sha256Hex(string(b))
	}
	b := s.SplitBinding.Bytes
	if err := writeExclusive(filepath.Join(root, "corpus", "split-manifest.json"), b); err != nil {
		return err
	}
	index["corpus/split-manifest.json"] = sha256Hex(string(b))
	if _, err := write("run_manifest.json", map[string]any{
		"schema_version": "1", "run_id": s.RunID, "dataset_version": c.Version(), "corpus_files": c.Files,
		"repeat_override": cfg.Repeats, "request_mode": s.requestMode, "skip_retrieval": cfg.SkipRetrieval,
		"split": cfg.Split, "split_sha256": s.SplitBinding.SHA256, "coverage": s.Coverage, "thresholds": gateTargets(),
		"git_commit": gitCommit(), "executable_sha256": executableHash(), "go_version": runtime.Version(), "os": runtime.GOOS, "architecture": runtime.GOARCH,
		"cases": c.Cases, "arms": cfg.Arms, "started_at": s.StartedAt, "ended_at": s.EndedAt,
		"controls":         map[string]any{"network_policy_declared": cfg.NetworkPolicy, "provider_process_isolation_observed": false, "provider_tools_observed": false, "repository_fixture_provisioning_observed": false, "model_settings_source": "provider-declared; not independently observed", "excluded_information": "omitted from rendered prompt only; external harness access not enforced"},
		"comparison_scope": "B4 vs B0 primary; B3 vs B4 bundled pipeline contrast (relevance filtering, budget, precedence, provenance differ)",
	}); err != nil {
		return err
	}
	for id, e := range s.caseEvidence {
		if _, err := write("retrieval/"+sanitize(id)+".json", e); err != nil {
			return err
		}
	}
	return nil
}
func executableHash() string {
	path, err := os.Executable()
	if err != nil {
		return "unavailable"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("unavailable: %T", err)
	}
	return sha256Hex(string(b))
}
