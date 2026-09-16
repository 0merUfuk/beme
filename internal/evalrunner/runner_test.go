package evalrunner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"github.com/0merUfuk/beme/internal/resolver"
)

// fixtureDeployment builds an isolated work-safe Be Me deployment whose
// records are designed so each golden case's acceptable/mandatory text is
// retrievable. Its source is not marked synthetic-*, so it is also the
// negative control for the no-scope eligibility rule.
func fixtureDeployment(t *testing.T) (*app.Runtime, string) {
	t.Helper()
	base := t.TempDir()
	cfg := filepath.Join(base, "cfg")
	srcRoot := filepath.Join(base, "src", "entries")
	os.MkdirAll(srcRoot, 0o755)
	os.MkdirAll(filepath.Join(cfg, "sources"), 0o700)

	// records whose compact text embeds the case gold phrases (deterministic
	// retrieval for the mock pipeline proof)
	os.WriteFile(filepath.Join(srcRoot, "P-001.md"), []byte("---\nid: P-001\ntitle: \"Measured need\"\ntype: heuristic\nstatus: active\n---\n\nembedded database file for a single-writer local batch tool: storage follows the actual constraint profile\n"), 0o644)
	os.WriteFile(filepath.Join(srcRoot, "P-002.md"), []byte("---\nid: P-002\ntitle: \"Migration path\"\ntype: principle\nstatus: active\n---\n\nembedded_with_migration_path: migration path documented\n"), 0o644)

	os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte("schema_version: \"1\"\nsource_id: synth-src\ntype: directory\nroot: "+filepath.Join(base, "src")+"\npurpose: [safe_declassified]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: recommended\nsensitivity: public_general\nprofiles_allowed: [personal, work-safe]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"), 0o600)

	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BuildProfile(contracts.ProfileWorkSafe); err != nil {
		t.Fatal(err)
	}
	return rt, cfg
}

// fixtureCorpus writes two public synthetic golden cases into a temp corpus.
func fixtureCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	base := map[string]any{
		"id":             "decision.architecture.storage.embedded-vs-server.synthetic.v1",
		"schema_version": "1",
		"status":         "locked",
		"gold_type":      "historical_truth",
		"category":       "architecture",
		"risk":           "medium",
		"capability":     "work-safe",
		"harness":        nil,
		"scenario": map[string]any{
			"task":                 "Choose the storage engine for a single-writer local batch tool.",
			"workspace":            "ws-synth-alpha",
			"repository_fixture":   "fix-synth-01",
			"excluded_information": []string{"gold_answer"},
		},
		"gold": map[string]any{
			"acceptable_decisions":   []string{"embedded database file"},
			"unacceptable_decisions": []string{"client_server_db_by_convention"},
			"mandatory_conclusions":  []string{"storage follows the actual constraint profile"},
			"prohibited_conclusions": []string{"the user dislikes client-server databases"},
			"expected_unknowns":      []string{"preferred SQL dialect"},
			"required_evidence_refs": []string{"engineering.storage.single-writer-local-batch"},
		},
		"provenance": map[string]any{
			"approved_by_user": true,
			"evidence_refs":    []string{"synthetic-only: shape reference"},
			"valid_at":         "2026-09-14",
		},
		"grading": map[string]any{"rubric": "0-4 blind paired", "repeats": 3},
	}
	b, _ := json.MarshalIndent(base, "", "  ")
	os.WriteFile(filepath.Join(dir, "case1.json"), b, 0o600)
	c2 := map[string]any{}
	for k, v := range base {
		c2[k] = v
	}
	c2["id"] = "decision.architecture.storage.migration-path.synthetic.v1"
	c2["gold"] = map[string]any{
		"acceptable_decisions":   []string{"embedded_with_migration_path"},
		"unacceptable_decisions": []string{"big_table_by_default"},
		"mandatory_conclusions":  []string{"migration path documented"},
		"prohibited_conclusions": []string{"the user hates managed databases"},
		"expected_unknowns":      []string{"sharding policy"},
		"required_evidence_refs": []string{"engineering.storage.migration"},
	}
	b2, _ := json.MarshalIndent(c2, "", "  ")
	os.WriteFile(filepath.Join(dir, "case2.json"), b2, 0o600)
	return dir
}

func runtimeInputs(rt *app.Runtime, capability string) evalrunner.InputsFn {
	return func(p contracts.Profile, task, ws string) (resolver.EvalInputs, error) {
		return rt.EvalInputs(p, capability, false, task, ws)
	}
}

func syntheticCorpus(t *testing.T, d *fx.Deployment, repeats int) *evalrunner.Corpus {
	t.Helper()
	dir := t.TempDir()
	if err := fx.WriteCorpus(dir, d.AlphaPath, repeats); err != nil {
		t.Fatal(err)
	}
	corpus, err := evalrunner.LoadCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	return corpus
}

// TestRunnerFullPipelineB0ThroughB4 proves the complete mechanism with
// deterministic mocks and the public synthetic deployment (no paid calls):
// every baseline and ablation runs, repeats work, manifests exist, blinded
// packaging hides arm identity, and B4 (full pack) beats B0 (no pack) as the
// contract's primary endpoint expects.
func TestRunnerFullPipelineB0ThroughB4(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	corpus := syntheticCorpus(t, d, 3)

	outDir := filepath.Join(t.TempDir(), "eval-out")
	summary, err := evalrunner.Run(corpus, evalrunner.RunConfig{
		Arms:          evalrunner.AllArms,
		OutputDir:     outDir,
		Harness:       "mock-harness",
		NetworkPolicy: "disabled",
		SkipRetrieval: true,
	}, evalrunner.NewMockProvider(""), evalrunner.DeterministicGrader{}, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}

	if len(summary.CaseResults) != len(evalrunner.AllArms) {
		t.Fatalf("expected %d case×arm units, got %d", len(evalrunner.AllArms), len(summary.CaseResults))
	}
	for _, cr := range summary.CaseResults {
		if cr.Outcome != evalrunner.OutcomePassed {
			t.Fatalf("arm %s: outcome %s (%s) — every arm must run on the synthetic deployment", cr.Arm, cr.Outcome, cr.Reason)
		}
		if len(cr.PerRepeat) != 3 {
			t.Fatalf("arm %s: expected 3 repeats, got %d", cr.Arm, len(cr.PerRepeat))
		}
		for _, rr := range cr.PerRepeat {
			if rr.Text == "" || rr.PromptSHA256 == "" {
				t.Fatalf("arm %s repeat %d: generated text and prompt digest must be preserved", cr.Arm, rr.Repeat)
			}
		}
	}
	if code := summary.ExitCode(); code != 0 {
		t.Fatalf("an all-passed behavioral run must exit 0; got %d", code)
	}
	if summary.Generations != 3*len(evalrunner.AllArms) {
		t.Fatalf("generations: %d", summary.Generations)
	}

	b0 := meanFor(summary, evalrunner.ArmB0)
	b4 := meanFor(summary, evalrunner.ArmB4)
	if b4 <= b0 {
		t.Fatalf("fixture design violated: B4 (%.2f) must beat B0 (%.2f) when the pack holds the gold phrases", b4, b0)
	}

	manifestDir := filepath.Join(outDir, summary.RunID, "manifests")
	entries, _ := os.ReadDir(manifestDir)
	if len(entries) != len(evalrunner.AllArms)*3 {
		t.Fatalf("expected %d manifests, got %d", len(evalrunner.AllArms)*3, len(entries))
	}

	blindDir := filepath.Join(outDir, summary.RunID, "blinded")
	blFiles, _ := os.ReadDir(blindDir)
	if len(blFiles) != len(entries) {
		t.Fatalf("blinded results: %d, want %d", len(blFiles), len(entries))
	}
	for _, f := range blFiles {
		data, _ := os.ReadFile(filepath.Join(blindDir, f.Name()))
		for _, arm := range evalrunner.AllArms {
			if strings.Contains(string(data), "\""+string(arm)+"\"") {
				t.Fatalf("blinded result %s leaks arm identity %q", f.Name(), arm)
			}
		}
	}
	keyData, err := os.ReadFile(filepath.Join(outDir, summary.RunID, "arm_key.json"))
	if err != nil {
		t.Fatal("arm key file must exist separately from blinded results")
	}
	var key map[string]string
	json.Unmarshal(keyData, &key)
	if len(key) != len(evalrunner.AllArms) {
		t.Fatalf("arm key should map %d opaque ids; got %d", len(evalrunner.AllArms), len(key))
	}
}

func meanFor(s *evalrunner.Summary, arm evalrunner.Arm) float64 {
	sum, n := 0.0, 0
	for _, cr := range s.CaseResults {
		if cr.Arm == arm {
			sum += cr.Score
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// TestRunnerNotRunStates: a resolution failure yields an explicit not_run
// case result — never a silent pass.
func TestRunnerNotRunStates(t *testing.T) {
	corpus, err := evalrunner.LoadCorpus(fixtureCorpus(t))
	if err != nil {
		t.Fatal(err)
	}
	failing := func(profile contracts.Profile, task, wsHint string) (resolver.EvalInputs, error) {
		return resolver.EvalInputs{}, os.ErrNotExist // simulate unavailable projection
	}
	summary, err := evalrunner.Run(corpus, evalrunner.RunConfig{
		Arms:    []evalrunner.Arm{evalrunner.ArmB4},
		Harness: "mock",
	}, evalrunner.NewMockProvider(""), evalrunner.DeterministicGrader{}, failing)
	if err != nil {
		t.Fatal(err)
	}
	for _, cr := range summary.CaseResults {
		if cr.Outcome != evalrunner.OutcomeNotRun {
			t.Fatalf("unavailable resolution must be not_run; got %s", cr.Outcome)
		}
		if !strings.Contains(cr.Reason, "resolution failed") {
			t.Fatalf("not_run must carry a reason; got %q", cr.Reason)
		}
	}
}

// TestRunnerRetrievalMetrics proves context precision/recall measurement
// (ACCEPTANCE §5 mechanism) on synthetic fixtures: per-case recall and
// precision against required_evidence_refs, micro-averaged aggregate,
// threshold evaluation, and explicit not_run when refs are absent.
func TestRunnerRetrievalMetrics(t *testing.T) {
	rt, _ := fixtureDeployment(t)
	dir := t.TempDir()
	write := func(name, id string, refs []string) {
		c := map[string]any{
			"id": id, "schema_version": "1", "status": "locked", "gold_type": "historical_truth",
			"category": "architecture", "risk": "medium", "capability": "work-safe",
			"scenario": map[string]any{"task": "Choose the storage engine for a single-writer local batch tool."},
			"gold":     map[string]any{"acceptable_decisions": []string{"embedded database file"}, "required_evidence_refs": refs},
			"grading":  map[string]any{"repeats": 1},
		}
		b, _ := json.Marshal(c)
		os.WriteFile(filepath.Join(dir, name), b, 0o600)
	}
	write("a.json", "synthetic.retrieval.all-found", []string{"P-001", "P-002"})
	write("b.json", "synthetic.retrieval.one-missing", []string{"P-001", "P-404"})
	write("c.json", "synthetic.retrieval.no-refs", nil)
	corpus, err := evalrunner.LoadCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	refs := func(profile contracts.Profile, recordID string) []string {
		s, err := rt.Serve(profile, "cap_eval_refs", false)
		if err != nil {
			return nil
		}
		defer s.Store.Close()
		recs, err := s.VisibleRecords()
		if err != nil {
			return nil
		}
		for _, r := range recs {
			if r.RecordID == recordID {
				return []string{r.SourceRecordID, r.Key}
			}
		}
		return nil
	}
	inputs := runtimeInputs(rt, "cap_eval_retrieval")
	sum, err := evalrunner.Run(corpus, evalrunner.RunConfig{
		Arms: []evalrunner.Arm{evalrunner.ArmB4}, OutputDir: t.TempDir(), Refs: refs,
	}, evalrunner.NewMockProvider(""), evalrunner.DeterministicGrader{}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]evalrunner.RetrievalCase{}
	for _, c := range sum.Retrieval.Cases {
		byID[c.CaseID] = c
	}
	all := byID["synthetic.retrieval.all-found"]
	if all.Recall != 1 || all.Selected == 0 || all.Precision != float64(all.Relevant)/float64(all.Selected) {
		t.Fatalf("all-found case: %+v", all)
	}
	miss := byID["synthetic.retrieval.one-missing"]
	if miss.Recall != 0.5 || miss.Retrieved != 1 || miss.Outcome != evalrunner.OutcomeFailed {
		t.Fatalf("one-missing case must score recall 0.5 and fail the threshold: %+v", miss)
	}
	if none := byID["synthetic.retrieval.no-refs"]; none.Outcome != evalrunner.OutcomeNotRun {
		t.Fatalf("a case without required refs must be not_run, got %+v", none)
	}
	if sum.Retrieval.Measured != 2 || sum.Retrieval.Recall != 0.75 || sum.Retrieval.ThresholdsMet {
		t.Fatalf("aggregate retrieval: %+v", sum.Retrieval)
	}
	if code := sum.ExitCode(); code != 1 {
		t.Fatalf("a failed retrieval measurement must force exit 1; got %d", code)
	}

	// no lookup configured → every case explicitly not_run
	sum2, err := evalrunner.Run(corpus, evalrunner.RunConfig{Arms: []evalrunner.Arm{evalrunner.ArmB0}, OutputDir: t.TempDir()},
		evalrunner.NewMockProvider(""), evalrunner.DeterministicGrader{}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if sum2.Retrieval.Measured != 0 || sum2.Retrieval.ThresholdsMet {
		t.Fatalf("retrieval without a refs lookup must not be measured: %+v", sum2.Retrieval)
	}

	// retrieval-only run: no arms, no provider
	sum3, err := evalrunner.Run(corpus, evalrunner.RunConfig{OutputDir: t.TempDir(), Refs: refs}, nil, nil, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum3.CaseResults) != 0 || sum3.Retrieval.Measured != 2 || sum3.ExitCode() != 1 {
		t.Fatalf("retrieval-only run: results=%d retrieval=%+v exit=%d", len(sum3.CaseResults), sum3.Retrieval, sum3.ExitCode())
	}
}

// prohibitedProvider always answers with case 1's prohibited conclusion.
type prohibitedProvider struct{}

const prohibitedAnswer = "embedded database file; the user dislikes client-server databases"

func (prohibitedProvider) Name() string { return "mock_prohibited" }
func (prohibitedProvider) Settings() evalrunner.ModelSettings {
	return evalrunner.ModelSettings{Provider: "mock_prohibited", ModelID: "fixed-answer", ModelVersion: "1", TopP: 1}
}
func (prohibitedProvider) Generate(req evalrunner.GenerationRequest) (evalrunner.GenerationResult, error) {
	return evalrunner.GenerationResult{Text: prohibitedAnswer + " [" + req.CaseID + "]"}, nil
}

// TestRunnerBlockerFailsUnitAndPreservesText: a zero-score (prohibited)
// generation fails its unit, is recorded as a blocker, is never averaged,
// forces a non-success exit, and the generated text survives into the result,
// raw artifacts, and blinded grading artifacts.
func TestRunnerBlockerFailsUnitAndPreservesText(t *testing.T) {
	rt, _ := fixtureDeployment(t)
	corpus, err := evalrunner.LoadCorpus(fixtureCorpus(t))
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	arms := []evalrunner.Arm{evalrunner.ArmB0, evalrunner.ArmB4}
	sum, err := evalrunner.Run(corpus, evalrunner.RunConfig{Arms: arms, OutputDir: out}, prohibitedProvider{}, evalrunner.DeterministicGrader{}, runtimeInputs(rt, "cap_eval_blocker"))
	if err != nil {
		t.Fatal(err)
	}
	blocked, passed := 0, 0
	for _, cr := range sum.CaseResults {
		if len(cr.PerRepeat) == 0 || cr.PerRepeat[0].Text == "" || !strings.Contains(cr.PerRepeat[0].Text, prohibitedAnswer) {
			t.Fatalf("%s/%s: generated text must be preserved in the result; got %+v", cr.CaseID, cr.Arm, cr.PerRepeat)
		}
		switch cr.CaseID {
		case "decision.architecture.storage.embedded-vs-server.synthetic.v1":
			if cr.Outcome != evalrunner.OutcomeFailed || !strings.HasPrefix(cr.Reason, "blocker:") || cr.Score != 0 || len(cr.PerRepeat) != 1 {
				t.Fatalf("prohibited output must fail the unit at the first blocker: %+v", cr)
			}
			blocked++
		default:
			if cr.Outcome != evalrunner.OutcomePassed || len(cr.PerRepeat) != 3 {
				t.Fatalf("non-prohibited case must still pass with all repeats: %+v", cr)
			}
			passed++
		}
	}
	if blocked != len(arms) || passed != len(arms) || len(sum.Blockers) != len(arms) {
		t.Fatalf("blocked=%d passed=%d blockers=%d", blocked, passed, len(sum.Blockers))
	}
	if code := sum.ExitCode(); code != 1 {
		t.Fatalf("a blocker must force exit 1; got %d", code)
	}

	var saved evalrunner.Summary
	data, err := os.ReadFile(filepath.Join(out, sum.RunID, "summary.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil {
		t.Fatalf("summary.json unreadable: %v", err)
	}
	if saved.ExitCode() != 1 {
		t.Fatal("the persisted summary must carry the blocker")
	}
	for _, sub := range []string{"raw", "blinded"} {
		files, err := os.ReadDir(filepath.Join(out, sum.RunID, sub))
		if err != nil || len(files) == 0 {
			t.Fatalf("%s artifacts missing: %v", sub, err)
		}
		for _, f := range files {
			b, _ := os.ReadFile(filepath.Join(out, sum.RunID, sub, f.Name()))
			if !strings.Contains(string(b), prohibitedAnswer) {
				t.Fatalf("%s/%s lacks the generated text", sub, f.Name())
			}
			if sub == "blinded" {
				for _, arm := range arms {
					if strings.Contains(string(b), "\""+string(arm)+"\"") {
						t.Fatalf("blinded artifact %s leaks arm %s", f.Name(), arm)
					}
				}
			}
		}
	}
}

// forbiddenProvider fails the test if a generation is requested.
type forbiddenProvider struct{ t *testing.T }

func (forbiddenProvider) Name() string { return "forbidden" }
func (forbiddenProvider) Settings() evalrunner.ModelSettings {
	return evalrunner.ModelSettings{Provider: "forbidden", ModelID: "none", ModelVersion: "none"}
}
func (p forbiddenProvider) Generate(req evalrunner.GenerationRequest) (evalrunner.GenerationResult, error) {
	p.t.Errorf("dry run executed a generation for %s/%s", req.CaseID, req.Arm)
	return evalrunner.GenerationResult{}, nil
}

// TestRunnerDryRunRendersWithoutGenerating: a dry run renders every prompt,
// writes manifests, evidence, and raw prompts, reports how many generations
// would run, never calls the provider, and exits not_run.
func TestRunnerDryRunRendersWithoutGenerating(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	corpus := syntheticCorpus(t, d, 2)
	out := t.TempDir()
	sum, err := evalrunner.Run(corpus, evalrunner.RunConfig{Arms: evalrunner.AllArms, OutputDir: out, DryRun: true, SkipRetrieval: true},
		forbiddenProvider{t}, nil, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}
	want := 2 * len(evalrunner.AllArms)
	if sum.Generations != 0 || sum.PlannedGenerations != want || !sum.DryRun {
		t.Fatalf("dry run: generations=%d planned=%d (want %d)", sum.Generations, sum.PlannedGenerations, want)
	}
	for _, cr := range sum.CaseResults {
		if cr.Outcome != evalrunner.OutcomeNotRun || !strings.HasPrefix(cr.Reason, "dry run:") || len(cr.PerRepeat) != 2 || cr.PerRepeat[0].PromptSHA256 == "" {
			t.Fatalf("dry-run unit must be not_run with rendered prompts: %+v", cr)
		}
	}
	if code := sum.ExitCode(); code != 3 {
		t.Fatalf("dry run must exit 3; got %d", code)
	}
	for sub, n := range map[string]int{"manifests": want, "evidence": want, "raw": want, "blinded": 0} {
		files, err := os.ReadDir(filepath.Join(out, sum.RunID, sub))
		if err != nil || len(files) != n {
			t.Fatalf("%s: %d files (want %d): %v", sub, len(files), n, err)
		}
		if sub == "raw" {
			b, _ := os.ReadFile(filepath.Join(out, sum.RunID, sub, files[0].Name()))
			if !strings.Contains(string(b), "=== TASK ===") {
				t.Fatalf("raw artifact lacks the rendered prompt: %s", b)
			}
		}
	}
}

func TestParseArms(t *testing.T) {
	all, err := evalrunner.ParseArms("all")
	if err != nil || len(all) != len(evalrunner.AllArms) {
		t.Fatalf("all: %v %v", all, err)
	}
	two, err := evalrunner.ParseArms(" B0, B4,B0")
	if err != nil || len(two) != 2 || two[0] != evalrunner.ArmB0 || two[1] != evalrunner.ArmB4 {
		t.Fatalf("list: %v %v", two, err)
	}
	if _, err := evalrunner.ParseArms("B0,B9"); err == nil {
		t.Fatal("unknown arm must be rejected")
	}
}

// TestSummaryExitCodeContract pins exit aggregation.
func TestSummaryExitCodeContract(t *testing.T) {
	pass := evalrunner.CaseResult{Outcome: evalrunner.OutcomePassed}
	fail := evalrunner.CaseResult{Outcome: evalrunner.OutcomeFailed}
	notRun := evalrunner.CaseResult{Outcome: evalrunner.OutcomeNotRun}
	cases := []struct {
		name string
		s    evalrunner.Summary
		want int
	}{
		{"all passed", evalrunner.Summary{CaseResults: []evalrunner.CaseResult{pass, pass}}, 0},
		{"not_run only", evalrunner.Summary{CaseResults: []evalrunner.CaseResult{pass, notRun}}, 3},
		{"failure beats not_run", evalrunner.Summary{CaseResults: []evalrunner.CaseResult{notRun, fail}}, 1},
		{"blocker alone", evalrunner.Summary{CaseResults: []evalrunner.CaseResult{pass}, Blockers: []string{"x"}}, 1},
		{"retrieval failed", evalrunner.Summary{CaseResults: []evalrunner.CaseResult{pass}, Retrieval: evalrunner.RetrievalSummary{Cases: []evalrunner.RetrievalCase{{Outcome: evalrunner.OutcomeFailed}}}}, 1},
		{"retrieval not_run", evalrunner.Summary{CaseResults: []evalrunner.CaseResult{pass}, Retrieval: evalrunner.RetrievalSummary{Cases: []evalrunner.RetrievalCase{{Outcome: evalrunner.OutcomeNotRun}}}}, 3},
	}
	for _, c := range cases {
		if got := c.s.ExitCode(); got != c.want {
			t.Errorf("%s: exit %d, want %d", c.name, got, c.want)
		}
	}
}
