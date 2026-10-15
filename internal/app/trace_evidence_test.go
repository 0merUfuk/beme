package app_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
)

func TestLegacyTraceReappliesAvailablePolicyAndHidesExclusions(t *testing.T) {
	rt, entry := evidenceDeployment(t, "Prefer embedded storage.")
	ref := "trace_legacy0001"
	steps := []resolver.TraceStep{{Step: "capability", Outcome: "bound profile=personal capability=synthetic"}, {Step: "stage_a", RecordID: "rec_syn-001", Outcome: "eligible"}, {Step: "pack", Outcome: "guidance=1"}}
	persistTrace(t, rt, ref, steps)
	loaded, err := rt.LoadTrace(contracts.ProfilePersonal, ref)
	if err != nil || len(loaded) != 3 {
		t.Fatalf("legacy positive control: %+v %v", loaded, err)
	}
	steps[1].Outcome = "excluded"
	steps[1].Reasons = []string{"private scope reason"}
	persistTrace(t, rt, ref, steps)
	loaded, err = rt.LoadTrace(contracts.ProfilePersonal, ref)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(loaded)
	if strings.Contains(string(b), "rec_syn-001") || strings.Contains(string(b), "private scope") || strings.Contains(string(b), "guidance=") {
		t.Errorf("legacy denial leaked through explain: %s", b)
	}
	steps[1].Outcome = "eligible"
	steps[1].Reasons = nil
	persistTrace(t, rt, ref, steps)
	evidenceWrite(t, entry, "---\nid: SYN-001\ntype: heuristic\nscope:\n  workspace_ids: [synthetic-restricted]\n---\nPrefer embedded storage.\n")
	if _, err = rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	loaded, err = rt.LoadTrace(contracts.ProfilePersonal, ref)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(loaded)
	if strings.Contains(string(b), "rec_syn-001") || strings.Contains(string(b), "guidance=") {
		t.Errorf("old unbound trace bypassed current scope: %s", b)
	}
}

func TestTraceRetentionKeepsNewestNotLargestRandomID(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "traces")
	for i := 0; i < 200; i++ {
		path := filepath.Join(dir, fmt.Sprintf("zzzz%08d.json", i))
		evidenceWrite(t, path, "[]")
		at := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.PersistTrace(cache, "trace_00000000", []resolver.TraceStep{{Step: "capability", Outcome: "bound profile=personal capability=synthetic"}}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 200 {
		t.Fatalf("retention: %d %v", len(entries), err)
	}
	if _, err = os.Stat(filepath.Join(dir, "00000000.json")); err != nil {
		t.Fatal("new random ID pruned instead of oldest trace")
	}
	if _, err = os.Stat(filepath.Join(dir, "zzzz00000000.json")); !os.IsNotExist(err) {
		t.Fatal("oldest trace not pruned")
	}
	for _, ref := range []string{"", "../outside", "trace_../../outside", "trace_bad.json"} {
		if err := app.PersistTrace(cache, ref, nil); err == nil {
			t.Errorf("invalid handle accepted: %q", ref)
		}
	}
}

func TestTraceFailureIsHonestAndReadStillServes(t *testing.T) {
	rt, _ := evidenceDeployment(t, "Prefer embedded storage.")
	rt.Config.CacheDir = filepath.Join(t.TempDir(), "not-a-directory")
	evidenceWrite(t, rt.Config.CacheDir, "synthetic cache failure")
	p := evidencePack(t, rt, "embedded storage")
	if len(p.Guidance) != 1 {
		t.Fatal("optional cache failure blocked evidence reads")
	}
	if p.TraceRef != "" || len(p.Degradations) == 0 {
		t.Fatalf("unusable explain handle advertised: trace=%q degradations=%v", p.TraceRef, p.Degradations)
	}
	b, _ := json.Marshal(p.Degradations)
	if strings.Contains(string(b), rt.Config.CacheDir) {
		t.Fatal("cache path leaked in degradation")
	}
	b, _ = json.Marshal(p)
	if p.Budget.EstimatedTokens != (len(b)+3)/4 {
		t.Fatal("trace degradation not counted in output budget")
	}
}

func TestPersistedTraceOmitsDeniedDataAndFiltersRevocations(t *testing.T) {
	rt, entry := evidenceDeployment(t, "Prefer embedded storage.")
	evidenceWrite(t, filepath.Join(filepath.Dir(entry), "DENIED.md"), "---\nid: SYN-DENIED-CANARY\ntype: heuristic\nscope:\n  workspace_ids: [synthetic-other]\n---\nPrefer storage for a denied workspace.\n")
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	s, err := rt.Serve(contracts.ProfilePersonal, "synthetic-runtime", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	p, trace, err := s.Resolve(contracts.ResolutionRequest{SchemaVersion: "1", Task: "storage"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(trace)
	if strings.Contains(strings.ToLower(string(b)), "denied-canary") {
		t.Error("returned trace disclosed denied record identity")
	}
	path := filepath.Join(rt.Config.CacheDir, "traces", strings.TrimPrefix(p.TraceRef, "trace_")+".json")
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "denied-canary") || strings.Contains(string(b), "synthetic-other") {
		t.Fatal("cache persisted denied identity/scope")
	}
	if _, err = rt.LoadTrace(contracts.ProfileWorkSafe, p.TraceRef); err == nil {
		t.Fatal("personal trace accepted under work-safe profile")
	}
	if err = rt.Forget(contracts.ProfilePersonal, "rec_syn-001", "synthetic revocation"); err != nil {
		t.Fatal(err)
	}
	trace, err = rt.LoadTrace(contracts.ProfilePersonal, p.TraceRef)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(trace)
	if strings.Contains(string(b), "rec_syn-001") || strings.Contains(string(b), "guidance=") {
		t.Fatal("revoked ID/count survived trace read filtering")
	}
}
