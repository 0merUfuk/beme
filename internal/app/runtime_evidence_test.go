package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

// The runtime regressions enter through real registered files, not seeded
// normalized records. Every deployment and source here is synthetic.
func evidenceWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func evidenceDescriptor(t *testing.T, cfg, root, id string) {
	t.Helper()
	sd := contracts.SourceDescriptor{SchemaVersion: "1", SourceID: id, Type: "directory", Root: root,
		Purpose: []string{"reusable_knowledge"}, Trust: contracts.TrustCanonical,
		InstructionSemantics: "registered_files_only", AuthorityCeiling: contracts.AuthorityRecommended,
		Sensitivity: contracts.SensPublicGeneral, ProfilesAllowed: []contracts.Profile{contracts.ProfilePersonal},
		IngestionMode: "index_content", Include: []string{"entries/**/*.md"}}
	b, err := json.Marshal(sd)
	if err != nil {
		t.Fatal(err)
	}
	evidenceWrite(t, filepath.Join(cfg, "sources", id+".json"), string(b))
}

func evidenceDeployment(t *testing.T, body string) (*app.Runtime, string) {
	t.Helper()
	base := t.TempDir()
	cfg := filepath.Join(base, "config")
	root := filepath.Join(base, "source")
	entry := filepath.Join(root, "entries", "SYN-001.md")
	evidenceWrite(t, entry, "---\nid: SYN-001\ntitle: Storage policy\ntype: heuristic\nstatus: active\n---\n"+body+"\n")
	evidenceDescriptor(t, cfg, root, "synthetic-runtime")
	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil || report.RecordsIngested != 1 {
		t.Fatalf("build: %+v %v", report, err)
	}
	return rt, entry
}

func evidencePack(t *testing.T, rt *app.Runtime, task string) resolver.Pack {
	t.Helper()
	s, err := rt.Serve(contracts.ProfilePersonal, "synthetic-runtime", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	p, _, err := s.Resolve(contracts.ResolutionRequest{SchemaVersion: "1", Task: task})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSnapshotDigestsBindContentAndRegisteredPolicy(t *testing.T) {
	rt, entry := evidenceDeployment(t, "Prefer an embedded database for storage.")
	a := evidencePack(t, rt, "database storage")
	s, err := rt.Serve(contracts.ProfilePersonal, "another-label", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "database storage"}
	same, _, err := s.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if same.Resolution.PolicyDigest != a.Resolution.PolicyDigest {
		t.Error("capability label, not effective policy, controls digest")
	}
	if _, err = rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	b := evidencePack(t, rt, req.Task)
	if b.Resolution.IndexRevision != a.Resolution.IndexRevision || *b.Resolution.SourceRevisionDigest != *a.Resolution.SourceRevisionDigest || b.Resolution.PolicyDigest != a.Resolution.PolicyDigest {
		t.Error("identical rebuild changed content identity")
	}
	evidenceWrite(t, entry, "---\nid: SYN-001\ntitle: Storage policy\ntype: heuristic\nstatus: active\n---\nPrefer a remote database for storage.\n")
	if _, err = rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	c := evidencePack(t, rt, req.Task)
	if c.Resolution.IndexRevision == a.Resolution.IndexRevision || *c.Resolution.SourceRevisionDigest == *a.Resolution.SourceRevisionDigest {
		t.Error("changed source bytes/delivered record did not change snapshot identity")
	}
	live, _, err := s.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if live.Resolution.IndexRevision != c.Resolution.IndexRevision {
		t.Error("long-running session cached obsolete snapshot digest")
	}
	rt.Sources[0].AuthorityCeiling = contracts.AuthorityInformational
	if _, err = rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	d := evidencePack(t, rt, req.Task)
	if d.Resolution.PolicyDigest == c.Resolution.PolicyDigest {
		t.Error("registration policy content not bound to digest")
	}
	// Source bytes outside the compact summary still belong to the source snapshot.
	evidenceWrite(t, entry, "---\nid: SYN-001\ntitle: Storage policy\ntype: heuristic\nstatus: active\n---\nPrefer a remote database for storage.\n\n## Appendix\n\nChanged source revision context.\n")
	if _, err = rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	e := evidencePack(t, rt, req.Task)
	if *e.Resolution.SourceRevisionDigest == *d.Resolution.SourceRevisionDigest {
		t.Error("digest only binds normalized text, not source bytes")
	}
}

func TestWorkSafeRejectsUnapprovedSourcesBeforeFilesystemRead(t *testing.T) {
	rt, _ := evidenceDeployment(t, "Prefer embedded database storage.")
	rt.Sources[0].Purpose = []string{"safe_declassified"}
	rt.Sources[0].ProfilesAllowed = []contracts.Profile{contracts.ProfileWorkSafe}
	denied := rt.Sources[0]
	denied.SourceID = "synthetic-private-unapproved"
	denied.Purpose = []string{"reusable_knowledge"}
	denied.Root = filepath.Join(t.TempDir(), "must-not-be-opened")
	rt.Sources = append(rt.Sources, denied)
	report, err := rt.BuildProfile(contracts.ProfileWorkSafe)
	if err != nil || len(report.Failed) != 1 || !strings.Contains(report.Failed[0].Reason, "approved safe manifest") {
		t.Fatalf("unapproved root reached filesystem instead of construction gate: %+v %v", report, err)
	}
}

func TestWorkSafeWithholdsCorrelatableSnapshotDigests(t *testing.T) {
	rt, _ := evidenceDeployment(t, "Prefer an embedded database for storage.")
	rt.Sources[0].Purpose = []string{"safe_declassified"}
	rt.Sources[0].ProfilesAllowed = []contracts.Profile{contracts.ProfileWorkSafe}
	if _, err := rt.BuildProfile(contracts.ProfileWorkSafe); err != nil {
		t.Fatal(err)
	}
	s, err := rt.Serve(contracts.ProfileWorkSafe, "synthetic-safe", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	p, _, err := s.Resolve(contracts.ResolutionRequest{SchemaVersion: "1", Task: "database storage"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Guidance) != 1 {
		t.Fatal("safe source stopped serving")
	}
	if p.Resolution.SourceRevisionDigest != nil {
		t.Error("safe pack publishes correlatable source digest")
	}
	for _, key := range []string{"index_revision", "policy_digest"} {
		raw, ok := s.Store.GetMeta(key)
		b, _ := json.Marshal(p.Resolution)
		if ok && raw != "" && strings.Contains(string(b), raw) {
			t.Errorf("safe output includes owner-local %s", key)
		}
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), rt.Sources[0].SourceID) || strings.Contains(string(b), rt.Sources[0].Root) {
		t.Error("safe pack leaked source identity/path")
	}
}

func TestPartialBuildDegradationSurvivesServingAndRecovery(t *testing.T) {
	rt, _ := evidenceDeployment(t, "Prefer embedded storage for local batch jobs.")
	missing := filepath.Join(t.TempDir(), "missing")
	evidenceDescriptor(t, rt.Config.CanonicalRoot, missing, "synthetic-unavailable-private-label")
	rt, err := app.Load(rt.Config.CanonicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	report, err := rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil || len(report.Failed) != 1 {
		t.Fatalf("failed-source positive control: %+v %v", report, err)
	}
	s, err := rt.Serve(contracts.ProfilePersonal, "synthetic-runtime", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "local batch storage"}
	p, _, err := s.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Guidance) != 1 {
		t.Fatal("healthy source stopped serving")
	}
	if p.Resolution.Completeness == "complete" || len(p.Degradations) == 0 {
		t.Errorf("agent sees false completeness: %+v", p)
	}
	b, _ := json.Marshal(p.Degradations)
	if strings.Contains(string(b), "private-label") || strings.Contains(string(b), missing) {
		t.Error("failure notice leaks source identity/path")
	}
	in, err := rt.EvalInputs(contracts.ProfilePersonal, "synthetic-runtime", false, req.Task, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Pack.Degradations) == 0 {
		t.Error("eval concealed partial build")
	}
	evidenceWrite(t, filepath.Join(missing, "entries", "SYN-002.md"), "---\nid: SYN-002\ntype: heuristic\n---\nLocal batch storage uses an embedded database.\n")
	if _, err = rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	p, _, err = s.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Guidance) != 2 || len(p.Degradations) != 0 || p.Resolution.Completeness != "complete" {
		t.Fatalf("same-session rebuild did not recover: %+v", p)
	}
	// An unreadable health record must never mean healthy.
	if err = s.Store.SetMeta("build_source_failures", "{"); err != nil {
		t.Fatal(err)
	}
	p, _, err = s.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if p.Resolution.Completeness == "complete" || len(p.Degradations) == 0 {
		t.Fatal("malformed build health silently treated as complete")
	}
}

func TestUnclassifiedTaskDoesNotRetrieveImplementationScope(t *testing.T) {
	rt, entry := evidenceDeployment(t, "Use an embedded store.")
	evidenceWrite(t, entry, "---\nid: SYN-IMPL\ntype: heuristic\nscope:\n  task_kinds: [implementation]\n---\nKeep implementation changes small.\n")
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	if p := evidencePack(t, rt, "Implement the storage schema"); len(p.Guidance) != 1 {
		t.Fatal("positive inferred implementation control")
	}
	if p := evidencePack(t, rt, "Bake a tomato tart"); len(p.Guidance) != 0 {
		t.Fatalf("a fallback facet became evidence of task applicability: %+v", p.Guidance)
	}
}

func TestNaturalTaskFacetsReachScopedSource(t *testing.T) {
	rt, entry := evidenceDeployment(t, "Deploy an embedded database.")
	evidenceWrite(t, entry, "---\nid: SYN-DEPLOY\ntype: heuristic\nscope:\n  task_kinds: [deployment]\n---\n# Rollout\n\n## Summary\n\nCheck the container health before deployment.\n")
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	sess, err := rt.Serve(contracts.ProfilePersonal, "synthetic-runtime", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "Deploy the container rollout checklist"}
	p, _, err := sess.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Guidance) != 1 {
		t.Errorf("natural-language task loses eligible scoped entry: %+v", p)
	}
	in, err := sess.EvalInputs(req.Task, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Pack.Guidance) != 1 || len(in.Eligible.Candidates) != 1 {
		t.Errorf("eval and serving disagree: %+v", in)
	}
	req.TaskKindHints = []string{"testing"}
	requestInputs, err := sess.EvalInputsRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(requestInputs.Pack.Guidance) != 0 || len(requestInputs.Eligible.Candidates) != 0 {
		t.Fatal("eval request fixture lost explicit task narrowing")
	}
	p, _, err = sess.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Guidance) != 0 {
		t.Fatal("explicit mismatched task narrowing bypassed by lexical facet")
	}
	p = evidencePack(t, rt, "Bake a tomato tart")
	if len(p.Guidance) != 0 {
		t.Fatal("unrelated task retrieved deployment-scoped source")
	}
}

func TestMarkdownSummarySurvivesSourceToPack(t *testing.T) {
	rt, _ := evidenceDeployment(t, "# Storage policy\n\n[Contents](#summary)\n\n## Context\n\nBackground without the recommendation.\n\n## Summary\n\nPrefer an embedded database for a local batch job.\n\n## References\n\nUnrelated appendix.")
	p := evidencePack(t, rt, "local batch database storage")
	if len(p.Guidance) != 1 || p.Guidance[0].Text != "Prefer an embedded database for a local batch job." {
		t.Fatalf("decision lost at source boundary: %+v", p.Guidance)
	}
	if strings.Contains(p.Guidance[0].Text, "References") {
		t.Fatal("navigation became evidence")
	}
}
