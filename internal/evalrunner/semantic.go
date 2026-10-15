package evalrunner

// Checksums are custody commitments, not semantic joins. Reconcile the frozen
// population and every generation before issuing a grading anchor. This does
// not authenticate an owner who fabricates a wholly consistent run.
import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

// Older task-only bundles stored profile alongside the two forwarded fields.
// Admit that exact historical shape, not invented risk/harness forwarding.
type storedRequest struct {
	contracts.ResolutionRequest
	LegacyProfile string `json:"profile,omitempty"`
}

type retrievalEvidence struct {
	CaseID          string               `json:"case_id"`
	FixtureSHA      string               `json:"fixture_sha256"`
	SplitSHA        string               `json:"split_sha256"`
	Request         storedRequest        `json:"request"`
	Profile         string               `json:"profile"`
	Deployment      deploymentFacts      `json:"deployment"`
	Pack            resolver.Pack        `json:"pack"`
	Eligible        resolver.EligibleSet `json:"eligible_snapshot"`
	EligibleSHA     string               `json:"eligible_snapshot_sha256"`
	PackSHA         string               `json:"pack_sha256"`
	Measurement     RetrievalCase        `json:"measurement"`
	ResolutionError string               `json:"resolution_error"`
	IdentityScope   string               `json:"identity_scope"`
	Replay          *armReplay           `json:"arm_replay,omitempty"`
}

type armReplay struct {
	LearnedEnabled  bool                            `json:"learned_enabled"`
	Roles           map[string]contracts.SourceRole `json:"roles"`
	NoScopePack     *resolver.Pack                  `json:"no_scope_pack"`
	NoScopeRefusal  string                          `json:"no_scope_refusal"`
	NoScopeEligible int                             `json:"no_scope_eligible"`
}

func valueInto(v any, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return decodeDocument(b, true, out)
}
func fieldsMatch(v map[string]any, expected map[string]any) bool {
	for k, want := range expected {
		got, ok := v[k]
		if !ok {
			return false
		}
		// Compare JSON values, not struct field order against map key order.
		b, err := json.Marshal(want)
		if err != nil {
			return false
		}
		var normalized any
		if json.Unmarshal(b, &normalized) != nil || !reflect.DeepEqual(got, normalized) {
			return false
		}
	}
	return true
}

func (v *verifiedBundle) reconcileEvidence() error {
	bad := func(reason string) error { return fmt.Errorf("invalid evidence: %s", reason) }
	var run map[string]any
	if err := v.read("run_manifest.json", &run); err != nil {
		return err
	}
	s := &v.Summary
	splitName, _ := run["split"].(string)
	c, err := LoadCorpus(filepath.Join(v.Root, "corpus"))
	if err != nil {
		return err
	}
	split, err := c.bindSplit("", splitName)
	if err != nil {
		return fmt.Errorf("invalid evidence: stored split: %w", err)
	}
	if !reflect.DeepEqual(split, s.SplitBinding) {
		return bad("split binding/selected population disagrees with frozen membership")
	}
	if !fieldsMatch(run, map[string]any{"schema_version": "1", "run_id": s.RunID, "dataset_version": c.Version(), "corpus_files": c.Files, "cases": c.Cases, "split_sha256": split.SHA256, "coverage": s.Coverage, "arms": s.Arms, "thresholds": gateTargets(), "started_at": s.StartedAt, "ended_at": s.EndedAt}) {
		return bad("run manifest disagrees with summary/corpus")
	}
	if s.Benefit != OutcomeNotRun {
		return bad("generation bundle cannot assert measured benefit")
	}
	override := 0
	if x, ok := run["repeat_override"]; ok {
		if err := valueInto(x, &override); err != nil || override < 0 || override > 5 {
			return bad("invalid repeat override")
		}
	}
	if override != s.RepeatOverride {
		return bad("repeat override binding mismatch")
	}
	if len(s.Arms) == 0 || s.DryRun {
		return bad("no executed generation population")
	}
	arms := map[Arm]bool{}
	for _, a := range s.Arms {
		known := false
		for _, k := range AllArms {
			known = known || a == k
		}
		if !known || arms[a] {
			return bad("unknown/duplicate configured arm")
		}
		arms[a] = true
	}
	var key map[string]Arm
	if err := v.read("arm_key.json", &key); err != nil {
		return err
	}
	opaque := map[Arm]string{}
	for id, a := range key {
		if !arms[a] || opaque[a] != "" || id != sanitize(id) || !strings.HasPrefix(id, "arm_") {
			return bad("invalid owner arm key")
		}
		opaque[a] = id
	}
	if len(key) != len(arms) {
		return bad("incomplete owner arm key")
	}
	cases := map[string]Case{}
	selected := map[string]bool{}
	for _, id := range split.Selected {
		selected[id] = true
	}
	expectedFiles := map[string]bool{"summary.json": true, "run_manifest.json": true, "grading_index.json": true, "arm_key.json": true, "corpus/split-manifest.json": true}
	excluded := map[string]string{}
	for _, cs := range c.Cases {
		cases[cs.ID] = cs
		expectedFiles["corpus/"+c.caseFile[cs.ID]] = true
		if !selected[cs.ID] {
			excluded[c.caseFile[cs.ID]] = "member of another frozen split"
		}
	}
	// Unsupported original directory entries are not copied into the corpus.
	// They may be declared exclusions, never substituted for selected case files.
	for name, reason := range s.Coverage.Excluded {
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" && filepath.Base(name) == name && reason == "not a supported case format" {
			excluded[name] = reason
		}
	}
	if s.Coverage.Physical != len(c.Files) || s.Coverage.Loaded != len(c.Cases) || s.Coverage.Selected != len(selected) || !reflect.DeepEqual(s.Coverage.Excluded, excluded) {
		return bad("coverage counters/exclusions disagree with corpus and split")
	}
	expected := map[string]bool{}
	for _, id := range split.Selected {
		n := cases[id].Grading.Repeats
		if override > 0 {
			n = override
		}
		for a := range arms {
			for rep := 1; rep <= n; rep++ {
				expected[unitKey(id, a, rep)] = true
			}
		}
	}
	if len(v.Bindings) != len(expected) || s.Generations != len(expected) || s.PlannedGenerations != 0 {
		return bad("incomplete or extra case/arm/repeat population")
	}
	repeats := map[string]RepeatResult{}
	caseArms := map[string]bool{}
	for _, cr := range s.CaseResults {
		pair := unitKey(cr.CaseID, cr.Arm, 0)
		if !selected[cr.CaseID] || !arms[cr.Arm] || caseArms[pair] {
			return bad("extra/duplicate case result")
		}
		caseArms[pair] = true
		if cr.Outcome != OutcomeGradingPending && cr.Outcome != OutcomePassed && cr.Outcome != OutcomeFailed {
			return bad("unexecuted case result")
		}
		if !s.MechanismOnly && cr.Outcome != OutcomeGradingPending {
			return bad("non-mechanism generation outcome contradicted")
		}
		for _, r := range cr.PerRepeat {
			k := unitKey(cr.CaseID, cr.Arm, r.Repeat)
			if _, exists := repeats[k]; exists || !expected[k] {
				return bad("extra/duplicate summary repeat")
			}
			if !s.MechanismOnly && r.Score != nil {
				return bad("live generation contains machine judgment")
			}
			repeats[k] = r
		}
	}
	if len(caseArms) != len(selected)*len(arms) || len(repeats) != len(expected) {
		return bad("missing summary population")
	}
	retrieved := map[string]retrievalEvidence{}
	for _, id := range split.Selected {
		name := "retrieval/" + sanitize(id) + ".json"
		expectedFiles[name] = true
		var e retrievalEvidence
		if err := v.read(name, &e); err != nil {
			return err
		}
		cs := cases[id]
		if e.Request.LegacyProfile != "" {
			if e.Profile != "" || run["request_mode"] != nil {
				return bad("ambiguous legacy request shape")
			}
			e.Profile = e.Request.LegacyProfile
		}
		if e.CaseID != id || e.FixtureSHA != c.FixtureHash(id) || e.SplitSHA != split.SHA256 || e.Profile != cs.Capability || e.PackSHA != hashJSON(e.Pack) || e.EligibleSHA != hashJSON(e.Eligible) || e.ResolutionError != "" {
			return bad("retrieval/corpus/snapshot binding mismatch")
		}
		if e.Measurement.CaseID != id {
			return bad("retrieval measurement bound to wrong case")
		}
		d := e.Deployment
		r := e.Pack.Resolution
		if d.Profile != e.Profile || d.CapabilityID != r.CapabilityID || d.PolicyDigest != r.PolicyDigest || d.IndexRevision != r.IndexRevision || d.SourceRevisionDigest != deref(r.SourceRevisionDigest) {
			return bad("deployment contradicts retained pack")
		}
		var refs RefsFn
		if e.Measurement.Reason != "no record reference lookup configured" {
			for id, keys := range e.Measurement.RefMappings {
				if len(keys) == 0 || keys[0] != id {
					return bad("invalid reference mapping")
				}
			}
			refs = func(_ contracts.Profile, id string) []string {
				keys := e.Measurement.RefMappings[id]
				if len(keys) == 0 {
					return nil
				}
				return keys[1:]
			}
		}
		measured := measureRetrieval(cs, refs, resolver.EvalInputs{Profile: contracts.Profile(e.Profile), Pack: e.Pack, Eligible: e.Eligible}, nil)
		if !reflect.DeepEqual(measured, e.Measurement) {
			return bad("retrieval measurement contradicts gold and retained evidence")
		}
		retrieved[id] = e
	}
	skip := len(s.Retrieval.Cases) == 0
	if value, ok := run["skip_retrieval"]; ok {
		if err := valueInto(value, &skip); err != nil {
			return bad("invalid retrieval configuration")
		}
	}
	aggregate := RetrievalSummary{RecallThreshold: RecallThreshold, PrecisionThreshold: PrecisionThreshold}
	measuredIDs := map[string]bool{}
	for _, r := range s.Retrieval.Cases {
		if skip || !selected[r.CaseID] || measuredIDs[r.CaseID] || !reflect.DeepEqual(r, retrieved[r.CaseID].Measurement) {
			return bad("summary retrieval population mismatch")
		}
		measuredIDs[r.CaseID] = true
		aggregate.Cases = append(aggregate.Cases, r)
	}
	if !skip && len(measuredIDs) != len(selected) {
		return bad("incomplete retrieval population")
	}
	aggregate.aggregate()
	if !reflect.DeepEqual(aggregate, s.Retrieval) {
		return bad("retrieval summary counters mismatch")
	}
	seen := map[string]bool{}
	sampleIDs := map[string]bool{}
	manifestPaths := map[string]bool{}
	for _, b := range v.Bindings {
		k := unitKey(b.CaseID, b.Arm, b.Repeat)
		if !expected[k] || seen[k] || sampleIDs[b.SampleID] || b.SampleID != sanitize(b.SampleID) || !strings.HasPrefix(b.SampleID, "sample_") {
			return bad("extra/duplicate/invalid sample binding")
		}
		seen[k] = true
		sampleIDs[b.SampleID] = true
		if b.ArtifactPath != "blinded/"+b.SampleID+".json" {
			return bad("noncanonical sample path")
		}
		expectedFiles[b.ArtifactPath] = true
		cs := cases[b.CaseID]
		e := retrieved[b.CaseID]
		rr := repeats[k]
		name := fmt.Sprintf("%s_%s_rep%02d.json", sanitize(b.CaseID), opaque[b.Arm], b.Repeat)
		manifestPaths["manifests/"+name] = true
		var m, owner, raw, sample map[string]any
		for _, x := range []struct {
			dir string
			out *map[string]any
		}{{"manifests", &m}, {"evidence", &owner}, {"raw", &raw}} {
			p := x.dir + "/" + name
			expectedFiles[p] = true
			if err := v.read(p, x.out); err != nil {
				return err
			}
		}
		if err := v.read(b.ArtifactPath, &sample); err != nil {
			return err
		}
		if len(sample) != 9 || b.ManifestSHA != v.Index["manifests/"+name] || !fieldsMatch(sample, map[string]any{"task": cs.Scenario.Task, "gold": cs.Gold, "rubric_version": RubricVersion, "text": rr.Text}) {
			return bad("sample gold/task/answer/manifest mismatch")
		}
		if !fieldsMatch(raw, map[string]any{"case_id": b.CaseID, "repeat": b.Repeat, "mechanism_score": rr.Score, "text": rr.Text, "prompt_sha256": rr.PromptSHA256}) {
			return bad("raw/summary mismatch")
		}
		d := e.Deployment
		in := resolver.EvalInputs{Profile: contracts.Profile(e.Profile), CapabilityID: d.CapabilityID, WorkspaceID: d.WorkspaceID, BuildGeneration: d.BuildGeneration, Pack: e.Pack, Eligible: e.Eligible, Roles: map[string]contracts.SourceRole{}}
		for _, cand := range e.Eligible.Candidates {
			in.Roles[cand.Item.RecordID] = cand.SourceRole
		}
		if e.Replay != nil {
			in.Roles = e.Replay.Roles
			in.LearnedEnabled = e.Replay.LearnedEnabled
			in.NoScopePack = e.Replay.NoScopePack
			in.NoScopeRefusal = e.Replay.NoScopeRefusal
			in.NoScopeEligible = e.Replay.NoScopeEligible
		}
		built, err := buildArm(in, b.Arm, s.NetworkPolicy)
		if err != nil || built.NotRun != "" {
			return bad("arm cannot be reconstructed from retained inputs")
		}
		view, err := RenderContext(built.Context)
		if err != nil {
			return err
		}
		prompt, err := RenderPrompt(cs.Scenario.Task, built.Bootstrap, built.Context)
		if err != nil {
			return err
		}
		if b.RawContextSHA != sha256Hex(view) || raw["prompt"] != prompt || rr.PromptSHA256 != sha256Hex(prompt) {
			return bad("raw prompt/context contradicts configured arm and retrieval evidence")
		}
		if !fieldsMatch(owner, map[string]any{"case_id": b.CaseID, "arm": b.Arm, "opaque_arm": opaque[b.Arm], "repeat": b.Repeat, "prompt_sha256": sha256Hex(prompt), "bootstrap_sha256": sha256Hex(built.Bootstrap), "context_sha256": sha256Hex(view), "arm_construction": built.Construction, "deployment": d, "manifest_sha256": b.ManifestSHA, "corpus": map[string]any{"dataset_version": c.Version(), "files": c.Files}}) {
			return bad("owner arm/construction/digest evidence mismatch")
		}
		var env environment
		var settings ModelSettings
		if err := valueInto(owner["environment"], &env); err != nil {
			return err
		}
		if err := valueInto(owner["model_settings"], &settings); err != nil {
			return err
		}
		if env.NetworkPolicy != s.NetworkPolicy || !fieldsMatch(run, map[string]any{"os": env.OS, "architecture": env.Architecture, "go_version": env.GoVersion}) || m["git_commit"] != run["git_commit"] {
			return bad("environment/source identity mismatch")
		}
		req := contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: cs.Scenario.Task, WorkspaceHint: cs.Scenario.Workspace}
		mode, _ := run["request_mode"].(string)
		if mode != "" && mode != "full" && mode != "legacy-task-only" {
			return bad("invalid request mode")
		}
		if mode == "full" || (mode == "" && e.Request.RiskHint != "") {
			req.RiskHint = cs.Risk
			if env.Harness != "none" {
				h := env.Harness
				req.Harness = &h
			}
		}
		expectedRequest := storedRequest{ResolutionRequest: req}
		if e.Request.LegacyProfile != "" {
			expectedRequest.LegacyProfile = cs.Capability
			expectedRequest.SchemaVersion = ""
		}
		if !reflect.DeepEqual(e.Request, expectedRequest) {
			return bad("request disagrees with frozen task/workspace/risk/harness")
		}
		ranking := struct {
			Source       string `json:"source"`
			Ordering     string `json:"ordering"`
			Transform    string `json:"transform"`
			BudgetTokens int    `json:"budget_tokens"`
		}{built.Construction.Source, built.Construction.Ordering, built.Construction.Transform, built.Construction.BudgetTokens}
		tools := struct {
			Tools         []string `json:"tools"`
			NetworkPolicy string   `json:"network_policy"`
		}{[]string{}, s.NetworkPolicy}
		if !fieldsMatch(m, map[string]any{"run_id": s.RunID, "case_id": b.CaseID, "repeat_index": b.Repeat, "fixture_hash": c.FixtureHash(b.CaseID), "dataset_version": c.Version(), "split": splitName, "rubric_version": RubricVersion, "bootstrap_prompt_hash": sha256Hex(built.Bootstrap), "ranking_config_hash": hashJSON(ranking), "capability_policy_hash": d.PolicyDigest, "index_snapshot_hash": d.IndexRevision, "profile_projection_hash": sha256Hex(fmt.Sprintf("profile=%s\ncapability_id=%s\nbuild_generation=%s\nindex_revision=%s\nsource_revision_digest=%s\n", d.Profile, d.CapabilityID, d.BuildGeneration, d.IndexRevision, d.SourceRevisionDigest)), "environment_digest": hashJSON(env), "tool_policy_hash": hashJSON(tools), "network_policy": s.NetworkPolicy, "harness": env.Harness, "harness_version": env.HarnessVersion, "os": env.OS, "architecture": env.Architecture, "go_version": env.GoVersion, "mcp_sdk_version": env.MCPSDKVersion, "sqlite_version": env.SQLiteVersion, "started_at": s.StartedAt, "ended_at": s.EndedAt}) {
			return bad("generation manifest contradicts unit evidence")
		}
		var settingsMap map[string]any
		if err := valueInto(settings, &settingsMap); err != nil {
			return err
		}
		if !fieldsMatch(m, settingsMap) {
			return bad("manifest model settings mismatch")
		}
		if err := validateSchema("run-manifest.schema.json", m); err != nil {
			return bad("invalid generation manifest schema")
		}
	}
	paths := map[string]bool{}
	for _, p := range s.ManifestPaths {
		if paths[p] || !manifestPaths[p] {
			return bad("extra/duplicate summary manifest")
		}
		paths[p] = true
	}
	if len(paths) != len(manifestPaths) {
		return bad("missing summary manifest")
	}
	for name := range v.Index {
		if !expectedFiles[name] {
			return bad("unexpected artifact: " + name)
		}
	}
	if len(v.Index) != len(expectedFiles) {
		return bad("missing required artifacts")
	}
	v.Corpus = c
	return nil
}
