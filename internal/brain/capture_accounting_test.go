package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Stage the exact production intent -> publication -> reset boundaries, with
// no clock sleeps or mutable product hooks. The external public-CLI regression
// selects the same boundary by stopping an unmodified synthetic child process.
func stagedCapture(t *testing.T, v *Vault, title string, now time.Time) (*Note, captureObligation, func()) {
	t.Helper()
	in := sqliteDecision
	n := &Note{Type: in.Type, Project: "demo", Title: title, Status: StatusProposed, Confidence: ConfObserved, Topic: in.Topic, Source: "capture", Created: now.Format(time.RFC3339Nano), Statement: in.Statement, Why: in.Why, Next: in.Next}
	if err := v.writable(); err != nil {
		t.Fatal(err)
	}
	if vs := Check(n); len(vs) > 0 {
		t.Fatal(vs)
	}
	n.ID = newID(n, now)
	n.Path = filepath.Join(v.inboxDir(), n.ID+".md")
	data, err := readableNote(n)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := v.markCapture(n.Project, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	return n, intent, func() {
		t.Helper()
		if err := writeExclusive(n.Path, data); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCaptureOverlapLatePublicationRemainsUnknown(t *testing.T) {
	v, root := newVault(t)
	a, intent, publish := stagedCapture(t, v, "Earlier delayed synthetic capture", t0)
	b := mustCapture(t, v, sqliteDecision, t0.Add(time.Minute))
	for i := 0; i < 2; i++ {
		if err := v.RecordSession("demo", t0.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	publish()
	lock := filepath.Join(v.Root, ".state", "demo.lock")
	if err := os.WriteFile(lock, []byte("held synthetic lock"), 0600); err != nil {
		t.Fatal(err)
	}
	err := v.recordCapture(intent, t0)
	if removeErr := os.Remove(lock); removeErr != nil {
		t.Fatal(removeErr)
	}
	if err == nil {
		t.Fatal("schedule did not force the accepted publication's reset failure")
	}
	if !fileExists(a.Path) || !fileExists(b.Path) {
		t.Fatal("accepted notes lost")
	}
	st, stateErr := v.readState("demo")
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0.Add(3 * time.Minute)})
	if stateErr != nil || !st.AccountingUnknown || strings.Contains(text, "no accepted captures") || len(v.Status().AccountingUnknown) != 1 {
		t.Fatalf("overlapping capture falsely known: state=%+v error=%v briefing=%s", st, stateErr, text)
	}
	// A later published capture can reconcile completed obligations, not an
	// unrelated capture which is still paused before publication.
	_, intentC, publishC := stagedCapture(t, v, "Third still in flight", t0.Add(4*time.Minute))
	mustCapture(t, v, sqliteDecision, t0.Add(5*time.Minute))
	if !v.loadState("demo").AccountingUnknown {
		t.Fatal("reset acknowledged another in-flight publication")
	}
	publishC()
	if err := v.recordCapture(intentC, t0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st := v.loadState("demo"); st.AccountingUnknown || st.InvocationsSinceCapture != 0 {
		t.Fatalf("safe recovery failed: %+v", st)
	}
}

func TestCaptureOverlapThreeCompletionOrders(t *testing.T) {
	for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			v, _ := newVault(t)
			var intents [3]captureObligation
			var publish [3]func()
			var notes [3]*Note
			for i := range intents {
				notes[i], intents[i], publish[i] = stagedCapture(t, v, fmt.Sprintf("Overlapping synthetic capture %d", i), t0.Add(time.Duration(i)*time.Minute))
			}
			for step, i := range order {
				publish[i]()
				if err := v.recordCapture(intents[i], t0.Add(time.Duration(i)*time.Minute)); err != nil {
					t.Fatal(err)
				}
				if st := v.loadState("demo"); st.AccountingUnknown != (step < 2) || st.InvocationsSinceCapture != 0 {
					t.Fatalf("step %d: %+v", step, st)
				}
				if err := v.RecordSession("demo", t0.Add(3*time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			for _, n := range notes {
				if !fileExists(n.Path) {
					t.Fatal("accepted note lost")
				}
			}
			if st := v.loadState("demo"); st.LastCapture != t0.Add(2*time.Minute).Format(time.RFC3339Nano) || st.InvocationsSinceCapture != 1 {
				t.Fatalf("reversed completion rewound state: %+v", st)
			}
		})
	}
}

func TestCaptureOverlapDelayedResetCannotEraseNewEvents(t *testing.T) {
	v, _ := newVault(t)
	_, a, publish := stagedCapture(t, v, "Delayed successful reset", t0)
	publish()
	lock := filepath.Join(v.Root, ".state", "demo.lock")
	if err := os.WriteFile(lock, []byte("held"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := v.recordCapture(a, t0); err == nil {
		t.Fatal("expected bounded lock failure")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	// Its completion receipt is now durable, but its own reset has not run.
	mustCapture(t, v, sqliteDecision, t0.Add(time.Minute))
	for i := 0; i < 2; i++ {
		if err := v.RecordSession("demo", t0.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	_, c, publishC := stagedCapture(t, v, "Reused slot is still in flight", t0.Add(3*time.Minute))
	if a.path != c.path {
		t.Fatal("fixture did not reuse the acknowledged slot")
	}
	if err := v.resetCaptures(a, t0); err != nil {
		t.Fatal(err)
	}
	if st := v.loadState("demo"); !st.AccountingUnknown || st.InvocationsSinceCapture != 2 {
		t.Fatalf("old reset touched reused intent/new events: %+v", st)
	}
	publishC()
	if err := v.recordCapture(c, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := v.RecordSession("demo", t0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := v.resetCaptures(a, t0); err != nil {
		t.Fatal(err)
	}
	if st := v.loadState("demo"); st.AccountingUnknown || st.InvocationsSinceCapture != 1 {
		t.Fatalf("already acknowledged reset erased a later event: %+v", st)
	}
}

func TestCaptureAccountingCollisionCancellationIsPerAttempt(t *testing.T) {
	v, _ := newVault(t)
	n, a, publish := stagedCapture(t, v, "Synthetic colliding note identity", t0)
	b, err := v.markCapture("demo", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.path == b.path {
		t.Fatal("distinct attempts share an intent")
	}
	publish()
	if err := writeExclusive(n.Path, []byte("must not replace accepted note")); !os.IsExist(err) {
		t.Fatal("collision not exercised", err)
	}
	v.cancelCapture(b)
	if !fileExists(a.path) || fileExists(b.path) || !v.loadState("demo").AccountingUnknown {
		t.Fatal("collision cancellation erased another obligation")
	}
	if err := v.recordCapture(a, t0); err != nil {
		t.Fatal(err)
	}
	mustCapture(t, v, sqliteDecision, t0.Add(time.Minute))
	if v.loadState("demo").AccountingUnknown {
		t.Fatal("retry failed to recover")
	}
	if data, err := v.readFile(n.Path); err != nil || !strings.Contains(string(data), n.Statement) {
		t.Fatal("collision changed accepted content", err)
	}
}

func TestCaptureAccountingPublicationFailureCancelsOnlyItsIntent(t *testing.T) {
	v, _ := newVault(t)
	if err := os.Rename(v.inboxDir(), v.inboxDir()+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v.inboxDir(), []byte("synthetic blocked inbox"), 0600); err != nil {
		t.Fatal(err)
	}
	in := sqliteDecision
	in.Project = "demo"
	if n, err := v.Capture(in, t0); err == nil || n != nil {
		t.Fatal("publication failure reported acceptance")
	}
	if v.loadState("demo").AccountingUnknown {
		t.Fatal("known non-publication leaked an intent")
	}
	if err := os.Remove(v.inboxDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(v.inboxDir()+"-saved", v.inboxDir()); err != nil {
		t.Fatal(err)
	}
	// Doctor's lock is unrelated to this protocol, including successful reset.
	if err := os.WriteFile(filepath.Join(v.Root, ".lock"), []byte("held maintenance"), 0600); err != nil {
		t.Fatal(err)
	}
	n, err := v.Capture(in, t0)
	if err != nil || n.AccountingWarning != "" {
		t.Fatalf("capture waited for maintenance: %+v %v", n, err)
	}
}

func TestCaptureAccountingBoundedInterruptedIntents(t *testing.T) {
	v, _ := newVault(t)
	var intents []captureObligation
	for i := 0; i < captureIntentLimit; i++ {
		intent, err := v.markCapture("demo", fmt.Sprintf("interrupted-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		intents = append(intents, intent)
	}
	in := sqliteDecision
	in.Project = "demo"
	if n, err := v.Capture(in, t0); err == nil || n != nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatal("full accounting capacity did not refuse before publication", n, err)
	}
	if entries, err := filepath.Glob(filepath.Join(v.inboxDir(), "*.md")); err != nil || len(entries) != 0 {
		t.Fatal("capacity refusal published a note", err)
	}
	if !v.loadState("demo").AccountingUnknown {
		t.Fatal("interrupted intents silently forgotten")
	}
	// Only the test owners know these attempts cannot publish. Age and another
	// capture alone are deliberately insufficient evidence to cancel them.
	for _, intent := range intents {
		v.cancelCapture(intent)
	}
	for i := 0; i < captureIntentLimit+2; i++ {
		mustCapture(t, v, sqliteDecision, t0.Add(time.Duration(i)*time.Minute))
	}
	if v.loadState("demo").AccountingUnknown {
		t.Fatal("settled obligations not reclaimed")
	}
	entries, err := os.ReadDir(filepath.Join(v.Root, ".state"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "demo.yaml" {
		t.Fatalf("lifetime accounting metadata retained: %v", entries)
	}
}

func TestCaptureAccountingRefusedAndPartialIntentsStayUnknown(t *testing.T) {
	for _, kind := range []string{"empty", "partial", "oversized", "directory", "nonempty-directory", "symlink", "file-symlink", "directory-symlink", "readonly", "legacy-mismatch"} {
		t.Run(kind, func(t *testing.T) {
			v, _ := newVault(t)
			path := v.captureIntentPath("demo", 0)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "empty":
				err = os.WriteFile(path, nil, 0600)
			case "partial":
				err = os.WriteFile(path, []byte("unfinished\npubl"), 0600)
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat("x", maxDocumentBytes+1)), 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "nonempty-directory":
				if err = os.Mkdir(path, 0700); err == nil {
					err = os.WriteFile(filepath.Join(path, "retained"), []byte("do not remove"), 0600)
				}
			case "symlink":
				err = os.Symlink(filepath.Join(t.TempDir(), "absent"), path)
			case "file-symlink":
				target := filepath.Join(t.TempDir(), "retained")
				if err = os.WriteFile(target, []byte("do not replace"), 0600); err == nil {
					err = os.Symlink(target, path)
				}
				t.Cleanup(func() {
					if data, err := os.ReadFile(target); err != nil || string(data) != "do not replace" {
						t.Error("symlink target changed", err)
					}
				})
			case "directory-symlink":
				err = os.Symlink(t.TempDir(), path)
			case "readonly":
				err = os.WriteFile(path, []byte("unfinished\n"), 0400)
				t.Cleanup(func() { os.Chmod(path, 0600) })
			case "legacy-mismatch":
				path = v.captureMarker("demo")
				err = os.WriteFile(path, []byte("legacy-unacknowledged\n"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			assertRefusedIntentPreserved(t, v, path)
		})
	}
}

func assertRefusedIntentPreserved(t *testing.T, v *Vault, path string) {
	t.Helper()
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	var contents []byte
	if before.Mode().IsRegular() {
		contents, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !v.loadState("demo").AccountingUnknown {
		t.Fatal("refused intent claimed known")
	}
	// Occupancy consumes this slot, not the other free slots. Allocation alone
	// must neither read/follow the refused node nor authorize its reclamation.
	intent, err := v.markCapture("demo", "free-slot-control")
	if err != nil {
		t.Fatal("occupied intent prevented allocation of free capacity:", err)
	}
	if intent.path == path {
		t.Fatal("allocated an occupied intent")
	}
	v.cancelCapture(intent)
	n := mustCapture(t, v, sqliteDecision, t0)
	if !fileExists(n.Path) {
		t.Fatal("free capacity did not publish an accepted proposal")
	}
	if !v.loadState("demo").AccountingUnknown || len(v.Status().AccountingUnknown) != 1 {
		t.Fatal("another reset acknowledged an interrupted/refused intent")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("refused intent changed or was removed", err)
	}
	if before.Mode().IsRegular() {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(contents) {
			t.Fatal("refused intent contents changed", err)
		}
	}
}

func TestCaptureAccountingOccupiedDirectoriesExhaustOnlyTheirSlots(t *testing.T) {
	v, _ := newVault(t)
	for slot := 0; slot < captureIntentLimit-1; slot++ {
		if err := os.MkdirAll(v.captureIntentPath("demo", slot), 0700); err != nil {
			t.Fatal(err)
		}
	}
	last, err := v.markCapture("demo", "last-free-slot")
	if err != nil || last.path != v.captureIntentPath("demo", captureIntentLimit-1) {
		t.Fatalf("last free slot unavailable: %+v %v", last, err)
	}
	in := sqliteDecision
	in.Project = "demo"
	if n, err := v.Capture(in, t0); err == nil || n != nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("occupied capacity must refuse before publication: %+v %v", n, err)
	}
	if notes, err := filepath.Glob(filepath.Join(v.inboxDir(), "*.md")); err != nil || len(notes) != 0 {
		t.Fatal("capacity refusal published a proposal", notes, err)
	}
	if !v.loadState("demo").AccountingUnknown {
		t.Fatal("occupied capacity became known")
	}
}

func TestCaptureAccountingRefusedParentCannotAuthorizePublication(t *testing.T) {
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			v, _ := newVault(t)
			stateDir := filepath.Join(v.Root, ".state")
			outside := t.TempDir()
			var err error
			if kind == "file" {
				err = os.WriteFile(stateDir, []byte("not a directory"), 0600)
			} else {
				err = os.Symlink(outside, stateDir)
			}
			if err != nil {
				t.Fatal(err)
			}
			in := sqliteDecision
			in.Project = "demo"
			if n, err := v.Capture(in, t0); err == nil || n != nil {
				t.Fatalf("refused accounting parent authorized publication: %+v %v", n, err)
			}
			if notes, err := filepath.Glob(filepath.Join(v.inboxDir(), "*.md")); err != nil || len(notes) != 0 {
				t.Fatal("accounting refusal published a proposal", notes, err)
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatal("accounting wrote through a refused parent", entries, err)
			}
		})
	}
}

func TestCaptureAccountingConcurrentStressPreservesPending(t *testing.T) {
	v, _ := newVault(t)
	_, pending, publish := stagedCapture(t, v, "Paused publisher during stress", t0)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				in := sqliteDecision
				in.Project = "demo"
				in.Title = fmt.Sprintf("Concurrent synthetic outcome %d %d", worker, i)
				if _, err := v.Capture(in, t0); err != nil {
					t.Error(err)
					return
				}
				if st, err := v.readState("demo"); err != nil || !st.AccountingUnknown {
					t.Errorf("pending publication lost during concurrent reset: %+v %v", st, err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	publish()
	if err := v.recordCapture(pending, t0); err != nil {
		t.Fatal(err)
	}
	if v.loadState("demo").AccountingUnknown {
		t.Fatal("stress failed to recover")
	}
	entries, err := filepath.Glob(filepath.Join(v.inboxDir(), "*.md"))
	if err != nil || len(entries) != 65 {
		t.Fatalf("accepted notes lost: %d %v", len(entries), err)
	}
}
