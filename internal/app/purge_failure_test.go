package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

// Inspection failures during purge (ADR-027): a store, trace directory, or
// canonical path that cannot be inspected aborts the purge with an error. No
// step is ever reported "done" over data that could not be read, and the
// purge completes once the storage is readable again.

func chmodSupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits")
	}
}

func stepDone(rep *app.PurgeReport, step string) bool {
	if rep == nil {
		return false
	}
	for _, s := range rep.Steps {
		if s.Step == step && s.Outcome == "done" {
			return true
		}
	}
	return false
}

func TestPurgePlanningFailsOnCorruptObservation(t *testing.T) {
	f := newPurgeFixture(t)
	sc := addDerivedCopies(t, f)
	corrupt := filepath.Join(f.rt.Config.DataDir, "observations", "obs_corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
	if err == nil || !strings.Contains(err.Error(), "inspect observations") {
		t.Fatalf("a corrupt observation must abort planning; got %v", err)
	}
	if stepDone(rep, "derived_purge_observations") {
		t.Fatal("observation step reported done over an uninspectable store")
	}
	if _, statErr := os.Stat(f.rt.LedgerPath()); !errors.Is(statErr, os.ErrNotExist) || mustPendingPurges(t, f.rt) != 0 {
		t.Fatal("planning failure must happen before any ledger or journal write")
	}
	if text, _ := resolvedText(t, f.rt); !strings.Contains(text, purgeCanary) {
		t.Fatal("planning failure must not erase anything")
	}
	if err := os.Remove(corrupt); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatalf("purge must succeed once the store is readable: %v", err)
	}
	assertFullyPurged(t, f, sc)
}

func TestPurgePlanningFailsWhenObservationStoreIsNotADirectory(t *testing.T) {
	f := newPurgeFixture(t)
	sc := addDerivedCopies(t, f)
	obsDir := filepath.Join(f.rt.Config.DataDir, "observations")
	moved := obsDir + ".moved"
	if err := os.Rename(obsDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(obsDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err == nil {
		t.Fatal("an observation store that is not a directory must abort the purge")
	}
	if err := os.Remove(obsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, obsDir); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	assertFullyPurged(t, f, sc)
}

// TestPurgePlanningFailsOnUnreadableObservationStore is the original defect:
// with the observation directory unreadable, purge used to report
// derived_purge_observations done (count 0) while matching observations
// stayed on disk.
func TestPurgePlanningFailsOnUnreadableObservationStore(t *testing.T) {
	chmodSupported(t)
	f := newPurgeFixture(t)
	sc := addDerivedCopies(t, f)
	obsDir := filepath.Join(f.rt.Config.DataDir, "observations")
	if err := os.Chmod(obsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	rep, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
	os.Chmod(obsDir, 0o700)
	if err == nil {
		t.Fatal("an unreadable observation store must abort the purge")
	}
	if stepDone(rep, "derived_purge_observations") {
		t.Fatal("observation step reported done over an unreadable store")
	}
	if n := sc.remainingObservations(); n != 2 {
		t.Fatalf("fixture: both matching observations must still exist, got %d", n)
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	assertFullyPurged(t, f, sc)
}

// TestPurgeResumeWithObservationStorageFailures: after an interruption at the
// observation stage, a resume over an unusable observation store fails (never
// "done"); a planned observation whose file became corrupt is still removed by
// ID; and the resume completes once the store is usable.
func TestPurgeResumeWithObservationStorageFailures(t *testing.T) {
	f := newPurgeFixture(t)
	sc := addDerivedCopies(t, f)
	injected := errors.New("injected")
	req := purgeReq("rec_can-001")
	req.FailAt = func(stage string) error {
		if stage == app.StageObservation {
			return injected
		}
		return nil
	}
	if _, err := f.rt.PhysicalPurge(req); !errors.Is(err, injected) {
		t.Fatalf("want injected failure, got %v", err)
	}
	req.FailAt = nil

	obsDir := filepath.Join(f.rt.Config.DataDir, "observations")
	moved := obsDir + ".moved"
	if err := os.Rename(obsDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(obsDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := f.rt.PhysicalPurge(req)
	if err == nil || !strings.Contains(err.Error(), "observation") {
		t.Fatalf("resume over an unusable observation store must fail; got %v", err)
	}
	if stepDone(rep, "derived_purge_observations") {
		t.Fatal("resume reported the observation step done over an unusable store")
	}
	if mustPendingPurges(t, f.rt) != 1 {
		t.Fatal("the journal must stay pending after a failed resume")
	}
	if err := os.Remove(obsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, obsDir); err != nil {
		t.Fatal(err)
	}

	// A planned observation corrupted between runs is still removed by ID.
	corrupted := filepath.Join(obsDir, sc.obsIDs[0]+".json")
	if err := os.WriteFile(corrupted, []byte("{garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err = f.rt.PhysicalPurge(req)
	if err != nil || !rep.Resumed {
		t.Fatalf("resume must complete once the store is usable: rep=%+v err=%v", rep, err)
	}
	assertFullyPurged(t, f, sc)
}

func TestPurgeResumeFailsOnUnreadableObservationStore(t *testing.T) {
	chmodSupported(t)
	f := newPurgeFixture(t)
	sc := addDerivedCopies(t, f)
	injected := errors.New("injected")
	req := purgeReq("rec_can-001")
	req.FailAt = func(stage string) error {
		if stage == app.StageObservation {
			return injected
		}
		return nil
	}
	if _, err := f.rt.PhysicalPurge(req); !errors.Is(err, injected) {
		t.Fatalf("want injected failure, got %v", err)
	}
	req.FailAt = nil
	obsDir := filepath.Join(f.rt.Config.DataDir, "observations")
	if err := os.Chmod(obsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	rep, err := f.rt.PhysicalPurge(req)
	os.Chmod(obsDir, 0o700)
	if err == nil || stepDone(rep, "derived_purge_observations") {
		t.Fatalf("resume over an unreadable store must fail without reporting done; rep=%+v err=%v", rep, err)
	}
	if _, err := f.rt.PhysicalPurge(req); err != nil {
		t.Fatal(err)
	}
	assertFullyPurged(t, f, sc)
}

func TestPurgeFailsOnUnreadableTracesAndCanonicalPaths(t *testing.T) {
	chmodSupported(t)
	t.Run("trace directory", func(t *testing.T) {
		f := newPurgeFixture(t)
		sc := addDerivedCopies(t, f)
		if err := os.Chmod(sc.traceDir, 0o000); err != nil {
			t.Fatal(err)
		}
		rep, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
		os.Chmod(sc.traceDir, 0o700)
		if err == nil || stepDone(rep, "derived_purge_traces") {
			t.Fatalf("an unreadable trace directory must abort the purge; rep=%+v err=%v", rep, err)
		}
		if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
			t.Fatal(err)
		}
		assertFullyPurged(t, f, sc)
	})
	t.Run("canonical parent", func(t *testing.T) {
		f := newPurgeFixture(t)
		sc := addDerivedCopies(t, f)
		entries := filepath.Dir(f.sourceFile)
		if err := os.Chmod(entries, 0o000); err != nil {
			t.Fatal(err)
		}
		rep, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
		os.Chmod(entries, 0o700)
		if err == nil || stepDone(rep, "canonical_source_removed") {
			t.Fatalf("an uninspectable canonical path must abort the purge; rep=%+v err=%v", rep, err)
		}
		if mustPendingPurges(t, f.rt) != 1 {
			t.Fatal("the failed canonical stage must leave a resumable journal")
		}
		rep, err = f.rt.PhysicalPurge(purgeReq("rec_can-001"))
		if err != nil || !rep.Resumed {
			t.Fatalf("resume: rep=%+v err=%v", rep, err)
		}
		assertFullyPurged(t, f, sc)
	})
}

// TestPhysicalPurgeCoversBothProjectionStores: a record present in the
// personal and work-safe stores is erased from both.
func TestPhysicalPurgeCoversBothProjectionStores(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "cfg")
	root := filepath.Join(base, "src")
	if err := os.MkdirAll(filepath.Join(cfg, "sources"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCanaryAt := filepath.Join(root, "entries", "DUAL-001.md")
	if err := os.MkdirAll(filepath.Dir(writeCanaryAt), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCanary(t, writeCanaryAt, "DUAL-001")
	desc := "schema_version: \"1\"\nsource_id: dual-src\ntype: directory\nroot: " + filepath.ToSlash(root) +
		"\npurpose: [safe_declassified]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: recommended\nsensitivity: public_general\nprofiles_allowed: [personal, work-safe]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"
	if err := os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte(desc), 0o600); err != nil {
		t.Fatal(err)
	}
	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []contracts.Profile{contracts.ProfilePersonal, contracts.ProfileWorkSafe} {
		if _, err := rt.BuildProfile(p); err != nil {
			t.Fatal(err)
		}
		if hits := bytesUnder(t, filepath.Dir(rt.ProjectionPath(p)), purgeCanary); len(hits) == 0 {
			t.Fatalf("fixture: %s store must hold the record before purge", p)
		}
	}
	rep, err := rt.PhysicalPurge(purgeReq("rec_dual-001"))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, s := range rep.Steps {
		counts[s.Step] = s.Count
	}
	if counts["derived_purge_projection_personal"] != 1 || counts["derived_purge_projection_work-safe"] != 1 {
		t.Fatalf("both stores must report the erased record: %v", counts)
	}
	if hits := bytesUnder(t, rt.Config.DataDir, purgeCanary); len(hits) > 0 {
		t.Fatalf("purged content survives in a projection store: %v", hits)
	}
	for _, p := range []contracts.Profile{contracts.ProfilePersonal, contracts.ProfileWorkSafe} {
		sess, err := rt.Serve(p, "cap_dual", false)
		if err != nil {
			t.Fatal(err)
		}
		visible, err := sess.VisibleRecords()
		sess.Store.Close()
		if err != nil || len(visible) != 0 {
			t.Fatalf("%s store still exposes records after purge: %v %v", p, visible, err)
		}
	}
}
