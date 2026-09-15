package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/durable"
	"github.com/0merUfuk/beme/internal/learning"
)

// TestLegacyLedgerMigratesAndRollbackIsDetected: a pre-v3 deployment (bare hex
// key + schema-2 ledger) keeps enforcing, migrates on its next ledger write
// with the same key bytes, and a restore of the pre-migration ledger over the
// migrated key fails closed.
func TestLegacyLedgerMigratesAndRollbackIsDetected(t *testing.T) {
	rt, _ := internalPurgeFixture(t)
	if _, err := rt.PhysicalPurge(PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001"}); err != nil {
		t.Fatal(err)
	}
	var kf purgeKeyFile
	data, _ := os.ReadFile(rt.PurgeKeyPath())
	if err := json.Unmarshal(data, &kf); err != nil {
		t.Fatal(err)
	}
	var cur Ledger
	data, _ = os.ReadFile(rt.LedgerPath())
	if err := json.Unmarshal(data, &cur); err != nil {
		t.Fatal(err)
	}
	legacyLedger, _ := json.Marshal(map[string]any{"schema_version": "2", "revocations": []any{}, "purges": cur.Purges})
	if err := os.WriteFile(rt.PurgeKeyPath(), []byte(kf.Key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rt.LedgerPath(), legacyLedger, 0o600); err != nil {
		t.Fatal(err)
	}

	l, err := rt.LoadLedger()
	if err != nil || !l.PurgedKey("rec_can-001") {
		t.Fatalf("a legacy ledger must keep enforcing: purged=%v err=%v", l != nil && l.PurgedKey("rec_can-001"), err)
	}
	if err := os.Rename(rt.LedgerPath(), rt.LedgerPath()+".bak"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.LoadLedger(); !errors.Is(err, ErrLedgerUnusable) {
		t.Fatalf("a legacy key without its ledger must fail closed; got %v", err)
	}
	if err := os.Rename(rt.LedgerPath()+".bak", rt.LedgerPath()); err != nil {
		t.Fatal(err)
	}

	if err := rt.Forget(contracts.ProfilePersonal, "rec_other", "migrates"); err != nil {
		t.Fatal(err)
	}
	var migrated purgeKeyFile
	data, _ = os.ReadFile(rt.PurgeKeyPath())
	if err := json.Unmarshal(data, &migrated); err != nil || migrated.Key != kf.Key || !migrated.Committed || migrated.SchemaVersion != keySchemaVersion {
		t.Fatalf("migration must keep the key bytes and commit the v3 key: %s %v", data, err)
	}
	if l, err := rt.LoadLedger(); err != nil || !l.PurgedKey("rec_can-001") || l.SchemaVersion != ledgerSchemaVersion {
		t.Fatalf("migrated ledger must keep enforcing: %v", err)
	}
	if err := os.WriteFile(rt.LedgerPath(), legacyLedger, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.LoadLedger(); !errors.Is(err, ErrLedgerUnusable) {
		t.Fatalf("a pre-migration ledger restored over a committed key must fail closed; got %v", err)
	}
}

// TestMaintenanceLockTimesOut: a held lock makes every maintenance operation
// fail with ErrLocked rather than interleave.
func TestMaintenanceLockTimesOut(t *testing.T) {
	rt, _ := internalPurgeFixture(t)
	held, err := rt.lock()
	if err != nil {
		t.Fatal(err)
	}
	orig := maintenanceLockTimeout
	maintenanceLockTimeout = 50 * time.Millisecond
	t.Cleanup(func() { maintenanceLockTimeout = orig })
	store, err := rt.OpenLearning()
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]error{}
	checks["forget"] = rt.Forget(contracts.ProfilePersonal, "rec_can-001", "locked")
	_, checks["purge"] = rt.PhysicalPurge(PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001"})
	_, checks["build"] = rt.BuildProfile(contracts.ProfilePersonal)
	_, checks["feedback"] = store.Observe("observation", "locked observation", "fam", "personal", "personal_private", "")
	for op, err := range checks {
		if !errors.Is(err, durable.ErrLocked) {
			t.Errorf("%s must wait for the maintenance lock and time out; got %v", op, err)
		}
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Forget(contracts.ProfilePersonal, "rec_can-001", "unlocked"); err != nil {
		t.Fatalf("released lock must admit the next operation: %v", err)
	}
}

type flushLog struct {
	dirs  []string
	files []string
}

// resolveDir follows symlinks so a flushed path compares equal to the fixture
// path (macOS resolves /var to /private/var, and canonical locators are
// resolved before erasure).
func resolveDir(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

func (f *flushLog) hasDir(dir string) bool {
	for _, d := range f.dirs {
		if resolveDir(d) == resolveDir(dir) {
			return true
		}
	}
	return false
}

func (f *flushLog) hasFile(path string) bool {
	for _, p := range f.files {
		if resolveDir(p) == resolveDir(path) {
			return true
		}
	}
	return false
}

// durabilityScene adds a trace and an observation naming the canary record.
func durabilityScene(t *testing.T, rt *Runtime, src string) map[string]string {
	t.Helper()
	traceDir := filepath.Join(rt.Config.CacheDir, "traces")
	if err := os.MkdirAll(traceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(traceDir, "t1.json"), []byte(`[{"step":"select","record_id":"rec_can-001"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	obs, err := learning.Open(rt.Config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obs.Observe("observation", "Durability boundary canary statement", "fam", "personal", "personal_private", ""); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"trace":       traceDir,
		"observation": filepath.Join(rt.Config.DataDir, "observations"),
		"canonical":   filepath.Dir(src),
		"projection":  filepath.Dir(rt.ProjectionPath(contracts.ProfilePersonal)),
	}
}

// TestPurgeFlushesEveryDeletionBeforeFinalize: when the journal is finalized,
// the directory of every erased trace, observation, and canonical file, and
// the compacted projection's file and directory, have been flushed.
func TestPurgeFlushesEveryDeletionBeforeFinalize(t *testing.T) {
	rt, src := internalPurgeFixture(t)
	dirs := durabilityScene(t, rt, src)
	log := &flushLog{}
	restore := durable.SetFlushHooks(func(dir string) error {
		log.dirs = append(log.dirs, dir)
		return durable.PlatformDirFlush(dir)
	}, func(f *os.File) error {
		log.files = append(log.files, f.Name())
		return f.Sync()
	})
	defer restore()
	var atFinalize *flushLog
	req := PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001", RemoveCanonical: true, FailAt: func(stage string) error {
		if stage == StageFinalize {
			atFinalize = &flushLog{dirs: append([]string{}, log.dirs...), files: append([]string{}, log.files...)}
		}
		return nil
	}}
	if _, err := rt.PhysicalPurge(req); err != nil {
		t.Fatal(err)
	}
	if atFinalize == nil {
		t.Fatal("finalize stage not reached")
	}
	for class, dir := range dirs {
		if !atFinalize.hasDir(dir) {
			t.Errorf("%s directory %s not flushed before the journal was finalized", class, dir)
		}
	}
	if !atFinalize.hasFile(rt.ProjectionPath(contracts.ProfilePersonal)) {
		t.Error("compacted projection file not flushed before the journal was finalized")
	}
}

// TestPurgeRetryCompletesOutstandingFlushes: when a directory flush fails
// after a file was already unlinked, the retry — finding the file gone —
// still flushes that directory before finalizing.
func TestPurgeRetryCompletesOutstandingFlushes(t *testing.T) {
	const needle = "Durability boundary canary statement"
	for _, class := range []string{"trace", "observation", "canonical", "projection"} {
		t.Run(class, func(t *testing.T) {
			rt, src := internalPurgeFixture(t)
			dirs := durabilityScene(t, rt, src)
			target := dirs[class]
			injected := errors.New("injected directory flush failure")
			restore := durable.SetFlushHooks(func(dir string) error {
				if resolveDir(dir) == resolveDir(target) {
					return injected
				}
				return durable.PlatformDirFlush(dir)
			}, nil)
			req := PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001", RemoveCanonical: true}
			_, err := rt.PhysicalPurge(req)
			restore()
			if !errors.Is(err, injected) {
				t.Fatalf("purge must fail on the %s flush; got %v", class, err)
			}
			if rt.PendingPurges() != 1 {
				t.Fatal("a purge whose flush failed must stay pending")
			}
			flushed := false
			restore = durable.SetFlushHooks(func(dir string) error {
				if resolveDir(dir) == resolveDir(target) {
					flushed = true
				}
				return durable.PlatformDirFlush(dir)
			}, nil)
			defer restore()
			req.FailAt = func(stage string) error {
				if stage == StageFinalize && !flushed {
					t.Errorf("retry reached finalize without flushing the %s directory", class)
				}
				return nil
			}
			if _, err := rt.PhysicalPurge(req); err != nil {
				t.Fatalf("retry must complete: %v", err)
			}
			if storeHolds(t, rt, needle) || rt.PendingPurges() != 0 {
				t.Fatal("retry did not complete the purge")
			}
		})
	}
}

// TestPurgeRefusesToEraseHardLinkedCanonicalFile: zeroizing a file with other
// hard links would destroy content under names the purge was not asked to
// touch, so it is reported as a residual instead.
func TestPurgeRefusesToEraseHardLinkedCanonicalFile(t *testing.T) {
	rt, src := internalPurgeFixture(t)
	other := filepath.Join(t.TempDir(), "hardlink.md")
	if err := os.Link(src, other); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}
	rep, err := rt.PhysicalPurge(PurgeRequest{Key: "rec_can-001", Confirm: "rec_can-001", RemoveCanonical: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rep.Residuals {
		found = found || strings.Contains(r, "hard links")
	}
	if !found {
		t.Fatalf("hard-linked canonical file must be reported as a residual: %v", rep.Residuals)
	}
	for _, p := range []string{src, other} {
		if data, err := os.ReadFile(p); err != nil || len(data) == 0 {
			t.Fatalf("hard-linked content must be left intact at %s: %v", p, err)
		}
	}
	if rt.PendingPurges() != 0 || storeHolds(t, rt, "Durability boundary canary statement") {
		t.Fatal("derived erasure must still complete")
	}
}
