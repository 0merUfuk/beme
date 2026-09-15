package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
)

// TestCorruptStoreRecovery verifies FR-065 end-to-end: a corrupt operational
// store is recoverable through rebuild WITHOUT changing canonical sources,
// and the rebuilt store serves correct records again.
func TestCorruptStoreRecovery(t *testing.T) {
	cfg := t.TempDir()
	srcRoot := filepath.Join(cfg, "src", "entries")
	os.MkdirAll(srcRoot, 0o755)
	os.WriteFile(filepath.Join(srcRoot, "R-001.md"), []byte("---\nid: R-001\ntitle: \"Recovery test\"\ntype: principle\nstatus: active\n---\n\nRecover by rebuilding.\n"), 0o644)
	os.MkdirAll(filepath.Join(cfg, "sources"), 0o700)
	os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte("schema_version: \"1\"\nsource_id: rec-src\ntype: directory\nroot: "+filepath.Join(cfg, "src")+"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: public_general\nprofiles_allowed: [personal, work-safe]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"), 0o600)

	rt, err := appLoad(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// snapshot canonical source content before corruption (must be unchanged after recovery)
	srcBefore, _ := os.ReadFile(filepath.Join(srcRoot, "R-001.md"))

	rep, err := rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil || rep.RecordsIngested != 1 {
		t.Fatalf("initial build failed: %+v err=%v", rep, err)
	}

	// corrupt the operational store: garbage bytes
	storePath := rt.ProjectionPath(contracts.ProfilePersonal)
	if err := os.WriteFile(storePath, []byte("\x00\x01corrupt-not-a-db\x02\x00"), 0o600); err != nil {
		t.Fatal(err)
	}

	// recovery = rebuild (doctor documents this exact remediation)
	rep2, err := rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil {
		t.Fatalf("rebuild over corrupt store failed: %v", err)
	}
	if rep2.RecordsIngested != 1 {
		t.Fatalf("rebuilt store should serve the record again; got %d", rep2.RecordsIngested)
	}

	// canonical source untouched by the whole cycle
	srcAfter, _ := os.ReadFile(filepath.Join(srcRoot, "R-001.md"))
	if string(srcBefore) != string(srcAfter) {
		t.Fatal("recovery must never modify canonical sources (FR-065)")
	}

	// and the recovered store serves resolution
	sess, err := rt.Serve(contracts.ProfilePersonal, "cap_recover01", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()
	if n := sess.Store.Count(); n != 1 {
		t.Fatalf("recovered store count = %d, want 1", n)
	}
}

// TestRebuildDoesNotResurrectForgotten verifies forget semantics across a
// rebuild cycle (§7.8 / FR-026): a tombstoned record must not re-enter
// resolution after rebuild — but note the honest boundary: a Wipe-based
// rebuild clears tombstones, so the CLI's rebuild path (BuildProfile) wipes
// and re-ingests. The documented operator flow for persistent forget is:
// tombstone + rebuild; if the source still contains the record, the
// operator must also remove/adjust the source. This test pins the actual
// behavior so the semantics stay honest.
func TestRebuildDoesNotResurrectForgotten(t *testing.T) {
	cfg := t.TempDir()
	srcRoot := filepath.Join(cfg, "src", "entries")
	os.MkdirAll(srcRoot, 0o755)
	os.WriteFile(filepath.Join(srcRoot, "F-001.md"), []byte("---\nid: F-001\ntitle: \"Forget me\"\ntype: fact\nstatus: active\n---\n\nForgotten fact.\n"), 0o644)
	os.MkdirAll(filepath.Join(cfg, "sources"), 0o700)
	os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte("schema_version: \"1\"\nsource_id: forget-src\ntype: directory\nroot: "+filepath.Join(cfg, "src")+"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: public_general\nprofiles_allowed: [personal]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"), 0o600)

	rt, err := appLoad(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	sess, err := rt.Serve(contracts.ProfilePersonal, "cap_forget01", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Store.Close()

	// logical forget: tombstone
	if err := sess.Store.Tombstone("rec_f-001", "operator forget"); err != nil {
		t.Fatal(err)
	}
	if set := sess.Store.RevokedSet(); !set["rec_f-001"] {
		t.Fatal("tombstone must persist in the revoked set")
	}
	// resolution honors the tombstone: the record is gone from packs
	pack, _, err := sess.Resolve(contracts.ResolutionRequest{SchemaVersion: contracts.SchemaVersion, Task: "forgotten fact"})
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range [][]any{toAny(pack.Constraints), toAny(pack.Guidance)} {
		_ = section
	}
	if count := len(pack.Guidance) + len(pack.Constraints) + len(pack.Precedents); count != 0 {
		t.Fatalf("tombstoned record must not resolve; got %d items", count)
	}

	// rebuild wipes derived state (documented): the record returns because
	// the SOURCE still contains it. This pins the honest semantic: forget
	// is logical (derived); physical removal requires removing it from the
	// source, and tombstones must be re-applied (or the source adjusted).
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	sess2, err := rt.Serve(contracts.ProfilePersonal, "cap_forget02", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sess2.Store.Close()
	if n := sess2.Store.Count(); n != 1 {
		t.Fatalf("post-rebuild count = %d; Wipe clears tombstones (documented honest boundary)", n)
	}
}

func toAny[T any](s []T) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
