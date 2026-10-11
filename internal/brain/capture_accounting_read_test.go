package brain

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Both public readers consume the error-or-unknown contract. An error's zero
// state must never become known counters or a conclusion that no capture occurred.
func assertCaptureAccountingUnknown(t *testing.T, v *Vault, root string) {
	t.Helper()
	s := v.Status()
	_, invocationsKnown := s.Invocations["demo"]
	_, sessionsKnown := s.SilentSession["demo"]
	if len(s.AccountingUnknown) != 1 || s.AccountingUnknown[0] != "demo" || invocationsKnown || sessionsKnown {
		t.Errorf("public status falsely known: %+v", s)
	}
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if !strings.Contains(text, "capture accounting degraded/unknown") || strings.Contains(text, "no accepted captures") {
		t.Errorf("public briefing falsely known: %s", text)
	}
}

type captureAccountingNode struct {
	info fs.FileInfo
	data []byte
}

func captureAccountingTree(t *testing.T, v *Vault) map[string]captureAccountingNode {
	t.Helper()
	tree := map[string]captureAccountingNode{}
	err := filepath.Walk(v.Root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		node := captureAccountingNode{info: info}
		if info.Mode().IsRegular() {
			node.data, err = os.ReadFile(path)
		} else if info.Mode()&os.ModeSymlink != 0 {
			var target string
			target, err = os.Readlink(path)
			node.data = []byte(target)
		}
		tree[path] = node
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func assertCaptureAccountingTreeUnchanged(t *testing.T, v *Vault, before map[string]captureAccountingNode) {
	t.Helper()
	after := captureAccountingTree(t, v)
	if len(after) != len(before) {
		t.Errorf("read changed vault entries: before=%d after=%d", len(before), len(after))
	}
	for path, a := range before {
		b, ok := after[path]
		if !ok || !os.SameFile(a.info, b.info) || a.info.Mode() != b.info.Mode() || a.info.Size() != b.info.Size() || !a.info.ModTime().Equal(b.info.ModTime()) || !bytes.Equal(a.data, b.data) {
			t.Errorf("read changed vault bytes or metadata: %s", path)
		}
	}
}

// Run only while writers are quiescent. All raw reads must succeed, and neither
// raw nor public reads may repair bookkeeping or mutate any vault node.
func assertCaptureAccountingReadsUnchanged(t *testing.T, v *Vault, root string) {
	t.Helper()
	before := captureAccountingTree(t, v)
	for i := 0; i < 3; i++ {
		if st, err := v.readState("demo"); err != nil || !st.AccountingUnknown {
			t.Fatalf("quiescent pending state must be readable and unknown: %+v %v", st, err)
		}
		assertCaptureAccountingUnknown(t, v, root)
	}
	assertCaptureAccountingTreeUnchanged(t, v, before)
}

func assertCaptureAccountingNotes(t *testing.T, v *Vault, notes []*Note, want int) {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(v.inboxDir(), "*.md"))
	if err != nil || len(entries) != want || len(notes) != want {
		t.Fatalf("accepted-note inventory differs: persisted=%d accepted=%d want=%d error=%v", len(entries), len(notes), want, err)
	}
	seen := map[string]bool{}
	for _, n := range notes {
		if seen[n.Path] {
			t.Errorf("accepted notes share a path: %s", n.ID)
		}
		seen[n.Path] = true
		wantData, err := readableNote(n)
		if err != nil {
			t.Fatal(err)
		}
		data, err := v.readFile(n.Path)
		if err != nil || !bytes.Equal(data, wantData) {
			t.Errorf("accepted note missing or changed: %s %v", n.ID, err)
		}
	}
}

func TestCaptureAccountingReadErrorsFailClosed(t *testing.T) {
	for _, kind := range []string{"malformed", "directory", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			v, root := newVault(t)
			for i := 0; i < nudgeAfter; i++ {
				if err := v.RecordSession("demo", t0); err != nil {
					t.Fatal(err)
				}
			}
			assertKnownInvocations(t, v, nudgeAfter)
			path := v.stateFile("demo")
			good, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "malformed":
				err = os.WriteFile(path, []byte("invocations_since_capture: [\n"), 0600)
			case "directory":
				if err = os.Remove(path); err == nil {
					err = os.Mkdir(path, 0700)
				}
			case "oversized":
				err = os.WriteFile(path, bytes.Repeat([]byte("x"), maxDocumentBytes+1), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := captureAccountingTree(t, v)
			for i := 0; i < 3; i++ {
				if _, err := v.readState("demo"); err == nil {
					t.Fatal("invalid state was not refused")
				}
				assertCaptureAccountingUnknown(t, v, root)
			}
			assertCaptureAccountingTreeUnchanged(t, v, before)
			changed, committed := false, false
			err = v.updateState("demo", func(*projState) error { changed = true; return nil }, func() error { committed = true; return nil })
			if err == nil || changed || committed {
				t.Fatalf("refused read authorized write callbacks: changed=%t committed=%t error=%v", changed, committed, err)
			}
			if err := v.RecordSessionEvent("demo", "synthetic", "refused-event", t0); err == nil {
				t.Fatal("session event reported success over unreadable state")
			}
			// Acceptance is separate from accounting. Never retry this note
			// just because its post-publication accounting read was refused.
			n := mustCapture(t, v, sqliteDecision, t0)
			if n.AccountingWarning == "" {
				t.Fatal("accepted note's accounting failure was not reported")
			}
			assertCaptureAccountingNotes(t, v, []*Note{n}, 1)
			assertCaptureAccountingUnknown(t, v, root)
			// Writes may create/remove the lock, but cannot repair the refused
			// state. Compare that exact node before explicit fixture recovery.
			after := captureAccountingTree(t, v)[path]
			prior := before[path]
			if !os.SameFile(prior.info, after.info) || prior.info.Mode() != after.info.Mode() || !prior.info.ModTime().Equal(after.info.ModTime()) || !bytes.Equal(prior.data, after.data) {
				t.Fatal("refused state was overwritten by a write caller")
			}
			if kind == "directory" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeAtomic(path, good); err != nil {
				t.Fatal(err)
			}
			assertCaptureAccountingReadsUnchanged(t, v, root)
			recovered := mustCapture(t, v, sqliteDecision, t0.Add(time.Minute))
			if recovered.AccountingWarning != "" {
				t.Fatal("quiescent recovery warned:", recovered.AccountingWarning)
			}
			assertKnownInvocations(t, v, 0)
			assertCaptureAccountingNotes(t, v, []*Note{n, recovered}, 2)
		})
	}
}
