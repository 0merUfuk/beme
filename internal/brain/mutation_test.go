package brain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIndexFailureKeepsCommittedActionReport(t *testing.T) {
	v, _ := newVault(t)
	n := mustCapture(t, v, sqliteDecision, t0)
	os.Mkdir(filepath.Join(v.Root, "index.md"), 0700)
	r, err := v.Doctor(Options{Now: t0})
	if err == nil || r == nil || !has(r, "promoted", n.ID) {
		t.Fatalf("index failure erased completed mutation report: %+v %v", r, err)
	}
	if _, err := v.Find(n.ID); err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(filepath.Join(v.Root, "log.md"))
	if !strings.Contains(string(log), "promoted") {
		t.Fatal("index failure erased successful action from audit trail")
	}
}

func TestMergeArchiveFailurePreservesCountAndRetry(t *testing.T) {
	v, _ := newVault(t)
	a := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	b := mustCapture(t, v, sqliteDecision, t0.Add(time.Hour))
	if err := os.MkdirAll(v.archivePath(b), 0700); err != nil {
		t.Fatal(err)
	}
	r := mustDoctor(t, v, t0.Add(time.Hour))
	n, _ := v.Find(a.ID)
	if n.Confirmed != 0 || has(r, "merged", b.ID) || !hasFlag(r, "mutation_failed") {
		t.Errorf("failed merge falsely committed: %+v %+v", n, r)
	}
	if !fileExists(b.Path) {
		t.Fatal("retry proposal lost")
	}
	os.Remove(v.archivePath(b))
	r = mustDoctor(t, v, t0.Add(2*time.Hour))
	n, _ = v.Find(a.ID)
	if n.Confirmed != 1 || !has(r, "merged", b.ID) {
		t.Fatalf("retry must apply once: %+v %+v", n, r)
	}
	mustDoctor(t, v, t0.Add(3*time.Hour))
	n, _ = v.Find(a.ID)
	if n.Confirmed != 1 {
		t.Fatal("retry inflated recapture count")
	}
}

func TestRejectionAndRetentionFailureAreNotSuccessActions(t *testing.T) {
	v, _ := newVault(t)
	p := filepath.Join(v.inboxDir(), "invalid.md")
	os.WriteFile(p, []byte("malformed proposal"), 0600)
	os.MkdirAll(filepath.Join(v.rejectedDir(), "invalid.md"), 0700)
	r := mustDoctor(t, v, t0)
	if has(r, "rejected", "invalid") || !hasFlag(r, "mutation_failed") || !fileExists(p) {
		t.Fatalf("rejection falsely reported: %+v", r)
	}
	var first *Note
	for i := 0; i < 4; i++ {
		n := mustCapture(t, v, CaptureInput{Type: TypeSummary, Title: "Checkpoint", Statement: "The synthetic checkpoint records the current service state."}, t0.Add(time.Duration(i)*time.Hour))
		if first == nil {
			first = n
		}
	}
	os.MkdirAll(v.archivePath(first), 0700)
	r = mustDoctor(t, v, t0.Add(5*time.Hour))
	live, _ := v.List("demo", TypeSummary, false)
	if has(r, "retired", first.ID) || len(live) != 4 || r.Live["demo"] != 4 {
		t.Fatalf("retention falsely reported: live=%d %+v", len(live), r)
	}
	os.Remove(v.archivePath(first))
	r = mustDoctor(t, v, t0.Add(6*time.Hour))
	live, _ = v.List("demo", TypeSummary, false)
	if len(live) != 3 || !has(r, "retired", first.ID) {
		t.Fatalf("retention not retryable: %+v", r)
	}
}
