package brain

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestIdentifiedSessionAccountingIsIdempotent(t *testing.T) {
	v, _ := newVault(t)
	recorder, ok := any(v).(interface {
		RecordSessionEvent(string, string, string, time.Time) error
	})
	if !ok {
		t.Fatal("explicit identified-session accounting is missing")
	}
	for _, id := range []string{"one", "one", "one", "fork-two"} {
		if err := recorder.RecordSessionEvent("demo", "claude-code", id, t0); err != nil {
			t.Fatal(err)
		}
	}
	if got := v.Status().SilentSession["demo"]; got != 2 {
		t.Fatalf("same session resumes are idempotent; new fork counts: %d", got)
	}
	mustCapture(t, v, sqliteDecision, t0)
	recorder.RecordSessionEvent("demo", "claude-code", "one", t0)
	if got := v.Status().SilentSession["demo"]; got != 0 {
		t.Fatalf("recapture reset must not forget IDs: %d", got)
	}
}

func TestSchemaGateCoversRegistrationAndBookkeeping(t *testing.T) {
	v, _ := newVault(t)
	os.WriteFile(filepath.Join(v.Root, "brain.yaml"), []byte("schema_version: 99\n"), 0600)
	if _, err := v.AddProject("other", "Other", t.TempDir()); err == nil {
		t.Error("new schema must refuse registration")
	}
	v.RecordSession("demo", t0)
	if fileExists(v.stateFile("demo")) {
		t.Error("new schema must refuse bookkeeping")
	}
}

func TestStaleAccountingLockRecovers(t *testing.T) {
	v, _ := newVault(t)
	lock := filepath.Join(v.Root, ".state", "demo.lock")
	os.MkdirAll(filepath.Dir(lock), 0700)
	os.WriteFile(lock, []byte("crashed"), 0600)
	os.Chtimes(lock, t0, t0)
	if err := v.RecordSession("demo", time.Now()); err != nil {
		t.Fatalf("stale accounting lock must be retryable: %v", err)
	}
	if v.Status().Invocations["demo"] != 1 {
		t.Fatal("recovered event was not recorded")
	}
}

func TestConcurrentSessionInvocationsDoNotLoseUpdates(t *testing.T) {
	v, _ := newVault(t)
	const invocations = 12
	results := make(chan error, invocations)
	var wg sync.WaitGroup
	for i := 0; i < invocations; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- v.RecordSession("demo", t0) }()
	}
	wg.Wait()
	close(results)
	refused := 0
	for err := range results {
		if err == nil {
			continue
		}
		// A bounded lock refusal is explicitly before any write. It is not a
		// lost successful event. Any other error needs investigation, not retry.
		if err.Error() != "session accounting locked; event not recorded" {
			t.Fatalf("unexpected accounting error: %v", err)
		}
		refused++
		t.Logf("explicit pre-write refusal: %v", err)
	}
	assertKnownInvocations(t, v, invocations-refused)
	// Retry only explicitly refused events, sequentially after contention has
	// ended. Preserve the original requirement of all twelve persisted events;
	// never retry a successful/ambiguous write or lower the final expected count.
	for i := 0; i < refused; i++ {
		if err := v.RecordSession("demo", t0); err != nil {
			t.Fatal("quiescent retry failed:", err)
		}
	}
	assertKnownInvocations(t, v, invocations)
}

func TestFrozenPartialVaultCannotBeReinitialized(t *testing.T) {
	v, _ := newVault(t)
	if err := v.Freeze(true, t0); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(v.projectsDir()); err != nil {
		t.Fatal(err)
	} // synthetic damaged skeleton
	if err := v.Init(); err == nil {
		t.Fatal("missing skeleton must not bypass incident freeze")
	}
	if fileExists(v.projectsDir()) {
		t.Fatal("frozen damaged vault was written")
	}
}

func TestFrozenInitDoesNotRepairOrWriteFiles(t *testing.T) {
	v, _ := newVault(t)
	v.Freeze(true, t0)
	os.Remove(filepath.Join(v.Root, "README.md"))
	if err := v.Init(); err == nil {
		t.Fatal("init must not bypass freeze")
	}
	if fileExists(filepath.Join(v.Root, "README.md")) {
		t.Fatal("frozen init wrote a file")
	}
	// Dry-run remains a read, including when maintenance is locked.
	os.WriteFile(filepath.Join(v.Root, ".lock"), []byte("held"), 0600)
	started := time.Now()
	if _, err := v.Doctor(Options{DryRun: true, Now: t0}); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("dry-run waited for a write lock")
	}
}
