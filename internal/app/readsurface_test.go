package app_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
	"github.com/0merUfuk/beme/internal/storage"
)

func selectedIDs(p resolver.Pack) map[string]bool {
	ids := map[string]bool{}
	for _, section := range [][]resolver.ContextItem{p.Constraints, p.Guidance, p.Precedents, p.LearnedExperimental} {
		for _, it := range section {
			ids[it.RecordID] = true
		}
	}
	return ids
}

func persistTrace(t *testing.T, rt *app.Runtime, traceRef string, steps []resolver.TraceStep) string {
	t.Helper()
	path := filepath.Join(rt.Config.CacheDir, "traces", strings.TrimPrefix(traceRef, "trace_")+".json")
	data, _ := json.Marshal(steps)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRestoredBackupHiddenOnEveryReadSurface restores a pre-purge projection
// backup (and a pre-purge trace) and attempts every read surface.
func TestRestoredBackupHiddenOnEveryReadSurface(t *testing.T) {
	f := newPurgeFixture(t)
	rt := f.rt
	storePath := rt.ProjectionPath(contracts.ProfilePersonal)

	sess, err := rt.Serve(contracts.ProfilePersonal, "cap_surface", false)
	if err != nil {
		t.Fatal(err)
	}
	pack, trace, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "owner preference for working"})
	if err != nil {
		t.Fatal(err)
	}
	if !selectedIDs(pack)["rec_can-001"] {
		t.Fatal("fixture: the canary record must be selected before purge")
	}
	if _, err := sess.ExpandItem(pack.PackID, "rec_can-001"); err != nil {
		t.Fatalf("fixture: canary must expand before purge: %v", err)
	}
	tracePath := persistTrace(t, rt, pack.TraceRef, trace)
	traceBackup, _ := os.ReadFile(tracePath)
	backup := filepath.Join(t.TempDir(), "backup")
	snapshotStore(t, storePath, backup)

	if _, err := rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.ExpandItem(pack.PackID, "rec_can-001"); err != app.ErrItemUnavailable {
		t.Fatalf("a pack issued before purge must not expand the purged record: %v", err)
	}
	sess.Store.Close()

	restoreStore(t, backup, storePath)
	if err := os.WriteFile(tracePath, traceBackup, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := storage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	rawRecs, err := raw.AllRecords()
	raw.Close()
	if err != nil || len(rawRecs) != 2 {
		t.Fatalf("positive control: restored backup must hold both records: %v %v", rawRecs, err)
	}
	if b, _ := os.ReadFile(tracePath); !strings.Contains(string(b), "rec_can-001") {
		t.Fatal("positive control: restored trace must name the purged record")
	}

	sess, err = rt.Serve(contracts.ProfilePersonal, "cap_surface", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()

	// resolve / preview
	after, _, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "owner preference for working"})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(after); strings.Contains(string(b), purgeCanary) || selectedIDs(after)["rec_can-001"] {
		t.Fatal("resolve surfaced the purged record")
	}
	// visible records and status count
	visible, err := sess.VisibleRecords()
	if err != nil {
		t.Fatal(err)
	}
	if count, err := sess.VisibleCount(); err != nil || count != 1 || len(visible) != 1 || visible[0].RecordID != "rec_keep-001" {
		t.Fatalf("status must count only visible records: count=%d visible=%v err=%v", count, visible, err)
	}
	// export (content + provenance)
	exp, err := rt.ExportProjection(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(exp); strings.Contains(string(b), purgeCanary) || strings.Contains(strings.ToLower(string(b)), "can-001") || len(exp.Records) != 1 {
		t.Fatalf("export surfaced the purged record or its provenance: %s", b)
	}
	// get_context_item
	if _, err := sess.ExpandItem(after.PackID, "rec_can-001"); err != app.ErrItemUnavailable {
		t.Fatalf("expansion of the purged record must be refused: %v", err)
	}
	// explain (trace metadata and counts)
	steps, err := rt.LoadTrace(contracts.ProfilePersonal, pack.TraceRef)
	if err != nil {
		t.Fatal(err)
	}
	sb, _ := json.Marshal(steps)
	if strings.Contains(string(sb), "rec_can-001") || strings.Contains(string(sb), "guidance=") {
		t.Fatalf("explain surfaced the purged record or pack counts: %s", sb)
	}
	// administrative inspection (doctor)
	findings, err := rt.ProjectionFindings(contracts.ProfilePersonal)
	if err != nil || len(findings) != 1 || strings.Contains(strings.ToLower(findings[0]), "can-001") {
		t.Fatalf("doctor must report a rebuild without identifying the record: %v %v", findings, err)
	}

	// Revoked (forgotten) records are hidden the same way.
	if err := rt.Forget(contracts.ProfilePersonal, "rec_keep-001", "surface test"); err != nil {
		t.Fatal(err)
	}
	if count, err := sess.VisibleCount(); err != nil || count != 0 {
		t.Fatalf("revoked record still counted: %d %v", count, err)
	}
	if exp, err := rt.ExportProjection(contracts.ProfilePersonal); err != nil || len(exp.Records) != 0 {
		t.Fatalf("revoked record still exported: %+v %v", exp, err)
	}
	if _, err := sess.ExpandItem(after.PackID, "rec_keep-001"); err != app.ErrItemUnavailable {
		t.Fatalf("revoked record still expands: %v", err)
	}
}

// TestUnusableLedgerFailsClosedOnEveryReadSurface: with the purge key missing
// or the ledger corrupt, every surface that can reveal projection information
// refuses with ErrLedgerUnusable.
func TestUnusableLedgerFailsClosedOnEveryReadSurface(t *testing.T) {
	for _, c := range []struct {
		name   string
		break_ func(rt *app.Runtime) error
	}{
		{"missing purge key", func(rt *app.Runtime) error { return os.Remove(rt.PurgeKeyPath()) }},
		{"corrupt ledger", func(rt *app.Runtime) error { return os.WriteFile(rt.LedgerPath(), []byte("{"), 0o600) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newPurgeFixture(t)
			rt := f.rt
			sess, err := rt.Serve(contracts.ProfilePersonal, "cap_closed", false)
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Store.Close()
			pack, trace, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "owner preference"})
			if err != nil {
				t.Fatal(err)
			}
			persistTrace(t, rt, pack.TraceRef, trace)
			if _, err := rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
				t.Fatal(err)
			}
			if err := c.break_(rt); err != nil {
				t.Fatal(err)
			}
			checks := map[string]error{}
			_, _, checks["resolve"] = sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "owner preference"})
			_, checks["visible records"] = sess.VisibleRecords()
			_, checks["status count"] = sess.VisibleCount()
			_, checks["export"] = rt.ExportProjection(contracts.ProfilePersonal)
			_, checks["expand"] = sess.ExpandItem(pack.PackID, "rec_keep-001")
			_, checks["explain"] = rt.LoadTrace(contracts.ProfilePersonal, pack.TraceRef)
			_, checks["doctor findings"] = rt.ProjectionFindings(contracts.ProfilePersonal)
			_, checks["rebuild"] = rt.BuildProfile(contracts.ProfilePersonal)
			_, checks["purge"] = rt.PhysicalPurge(purgeReq("rec_keep-001"))
			for surface, err := range checks {
				if !errors.Is(err, app.ErrLedgerUnusable) {
					t.Errorf("%s did not fail closed: %v", surface, err)
				}
			}
		})
	}
}

// TestExpandItemIsPackBound pins ADR-029: expansion requires a pack this
// session issued with the record selected into it; every other combination
// gets the single indistinguishable refusal.
func TestExpandItemIsPackBound(t *testing.T) {
	f := newPurgeFixture(t)
	rt := f.rt
	st, err := storage.Open(rt.ProjectionPath(contracts.ProfilePersonal))
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id, text string, validated bool) contracts.Record {
		r := contracts.Record{SchemaVersion: contracts.SchemaVersion, RecordID: id, SourceID: "purge-src", SourceRecordID: strings.ToUpper(id),
			Kind: contracts.KindPreference, Title: text, Statement: text, CompactText: text, Status: contracts.StatusActive,
			Authority: contracts.AuthorityDefault, SourceRole: contracts.RoleCanonicalKnowledge, Trust: contracts.TrustCanonical,
			Sensitivity: "personal_private", DecisionKey: "expand.decision", ProvenanceRefs: []string{}}
		if validated {
			r.Confidence = contracts.ConfidenceValidated
		}
		return r
	}
	err = st.PutRecords([]contracts.Record{mk("rec_winner", "winning expansion choice", true), mk("rec_loser", "losing expansion choice", false)}, nil)
	st.Close()
	if err != nil {
		t.Fatal(err)
	}

	sess, err := rt.Serve(contracts.ProfilePersonal, "cap_expand", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	pack, trace, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "expansion choice"})
	if err != nil {
		t.Fatal(err)
	}
	sel := selectedIDs(pack)
	eligible := map[string]bool{}
	for _, s := range trace {
		if s.Step == "stage_a" && s.Outcome == "eligible" {
			eligible[s.RecordID] = true
		}
	}
	if !sel["rec_winner"] || sel["rec_loser"] || !eligible["rec_loser"] {
		t.Fatalf("fixture: need selected winner and eligible-but-unselected loser; selected=%v eligible=%v", sel, eligible)
	}

	item, err := sess.ExpandItem(pack.PackID, "rec_winner")
	if err != nil || item.Statement != "winning expansion choice" || item.SourceID != "purge-src" {
		t.Fatalf("selected record must expand with personal source identity: %+v %v", item, err)
	}

	refusals := map[string]error{}
	_, refusals["eligible but unselected"] = sess.ExpandItem(pack.PackID, "rec_loser")
	_, refusals["nonexistent record"] = sess.ExpandItem(pack.PackID, "rec_nope")
	_, refusals["unknown pack"] = sess.ExpandItem("ctx_000000000000000000000000", "rec_winner")
	_, refusals["empty pack id"] = sess.ExpandItem("", "rec_winner")
	_, refusals["empty record id"] = sess.ExpandItem(pack.PackID, "")
	other, err := rt.Serve(contracts.ProfilePersonal, "cap_expand", false)
	if err != nil {
		t.Fatal(err)
	}
	_, refusals["replayed in another session"] = other.ExpandItem(pack.PackID, "rec_winner")
	other.Store.Close()

	revPack, _, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "owner preference for working"})
	if err != nil {
		t.Fatal(err)
	}
	if !selectedIDs(revPack)["rec_can-001"] {
		t.Fatal("fixture: canary must be selected")
	}
	if err := rt.Forget(contracts.ProfilePersonal, "rec_can-001", "expand test"); err != nil {
		t.Fatal(err)
	}
	_, refusals["revoked after issuance"] = sess.ExpandItem(revPack.PackID, "rec_can-001")

	rebuildPack, _, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "small reviewable changes"})
	if err != nil || !selectedIDs(rebuildPack)["rec_keep-001"] {
		t.Fatalf("fixture: keep record must be selected: %v", err)
	}
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	_, refusals["issued before a rebuild"] = sess.ExpandItem(rebuildPack.PackID, "rec_keep-001")

	expiring, _, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "small reviewable changes"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.ExpandItem(expiring.PackID, "rec_keep-001"); err != nil {
		t.Fatalf("a pack issued after the rebuild must expand: %v", err)
	}
	sess.Now = func() time.Time { return time.Now().Add(app.DefaultPackTTL + time.Minute) }
	_, refusals["expired pack"] = sess.ExpandItem(expiring.PackID, "rec_keep-001")
	sess.Now = nil

	for name, err := range refusals {
		if err != app.ErrItemUnavailable {
			t.Errorf("%s: got %v, want the single ErrItemUnavailable", name, err)
		}
	}
}

func TestWorkSafeExpansionOmitsSourceIdentity(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "cfg")
	src := filepath.Join(base, "src", "entries", "WS-001.md")
	for _, d := range []string{filepath.Join(cfg, "sources"), filepath.Dir(src)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(src, []byte("---\nid: WS-001\ntitle: \"Safe\"\ntype: heuristic\nstatus: active\n---\n\nWork-safe expansion content.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	desc := "schema_version: \"1\"\nsource_id: ws-src\ntype: directory\nroot: " + filepath.ToSlash(filepath.Join(base, "src")) +
		"\npurpose: [safe_declassified]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: recommended\nsensitivity: public_general\nprofiles_allowed: [personal, work-safe]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"
	if err := os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte(desc), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BuildProfile(contracts.ProfileWorkSafe); err != nil {
		t.Fatal(err)
	}
	sess, err := rt.Serve(contracts.ProfileWorkSafe, "cap_ws", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	pack, err := sess.ResolveOnly("work-safe expansion", "")
	if err != nil || !selectedIDs(pack)["rec_ws-001"] {
		t.Fatalf("fixture: record must be selected: %v", err)
	}
	item, err := sess.ExpandItem(pack.PackID, "rec_ws-001")
	if err != nil {
		t.Fatal(err)
	}
	if item.SourceID != "" || item.SourceRecordID != "" || len(item.ProvenanceRefs) != 0 {
		t.Fatalf("work-safe expansion leaked source identity: %+v", item)
	}
}
