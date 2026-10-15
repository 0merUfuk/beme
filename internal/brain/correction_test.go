package brain

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCorrectionRecaptureStopsPendingRetirement(t *testing.T) {
	v, _ := newVault(t)
	a := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	first, _ := v.Find(a.ID)
	in := sqliteDecision
	in.Statement = "Use a separate server database for this synthetic workload."
	b := mustCapture(t, v, in, t0.Add(time.Hour))
	os.MkdirAll(v.archivePath(first), 0700)
	mustDoctor(t, v, t0.Add(time.Hour))
	// Archive fault leaves a persisted supersession intent. Apply a later exact
	// recapture before repairing that fault, just as a partial maintenance page can.
	later := mustCapture(t, v, sqliteDecision, t0.Add(2*time.Hour))
	d := &doctor{v: v, now: t0.Add(2 * time.Hour), r: &Report{Live: map[string]int{}}, live: []*Note{first}}
	d.merge(later.Path, later, first)
	os.Remove(v.archivePath(first))
	r := mustDoctor(t, v, t0.Add(3*time.Hour))
	if _, err := v.Find(a.ID); err != nil || has(r, "superseded", a.ID) {
		t.Fatalf("retry retired later recapture: %v %+v", err, r)
	}
	if _, err := v.Find(b.ID); err != nil {
		t.Fatal("ambiguous older claim should remain reviewable", err)
	}
}

func TestCorrectionAllGlobalWarningPathsWithholdUnsupportedIDs(t *testing.T) {
	v, root := newVault(t)
	n := &Note{ID: "opn-synthetic-private-project", Type: TypeOpen, Project: GlobalProject, Title: "Legacy project state", Statement: "The synthetic project needs another review.", Status: StatusOpen, Created: t0.Format(time.RFC3339Nano)}
	putLive(t, v, n)
	for _, kind := range []string{"conflict", "overlap", "stale_open", "cap_exceeded", "unsupported_scope", "mutation_failed", "backup_failed", "maintenance_failed", "maintenance_budget"} {
		for _, mode := range []Mode{ModeNormal, ModeEconomy, ModeManual} {
			r := &Report{Flags: []Flag{{Kind: kind, Project: GlobalProject, IDs: []string{n.ID}, Detail: "failed operation for " + n.ID}}}
			text, _ := v.Briefing(BriefingOptions{CWD: root, Mode: mode, Now: t0, Report: r})
			if strings.Contains(text, n.ID) {
				t.Fatalf("%s %s leaked ID", kind, mode)
			}
		}
	}
}

func TestCorrectionSchemaAbsenceIsNotInvalidity(t *testing.T) {
	for _, body := range []string{"", "{}", "schema_version: 0\n", "schema_version: -1\n", "schema_version: 1\n---\nschema_version: 99\n", "schema_version: 1\nunknown: true\n", "schema_version: 1\nschema_version: 99\n"} {
		t.Run(fmt.Sprintf("%q", body), func(t *testing.T) {
			v, _ := newVault(t)
			os.WriteFile(filepath.Join(v.Root, "brain.yaml"), []byte(body), 0600)
			for _, op := range []func() error{v.Init, func() error { return v.RecordSession("demo", t0) }, func() error { return v.Freeze(true, t0) }} {
				if err := op(); err == nil {
					t.Fatal("invalid marker authorized writes")
				}
			}
		})
	}
	v, _ := newVault(t)
	os.Remove(filepath.Join(v.Root, "brain.yaml"))
	if err := v.RecordSession("demo", t0); err != nil {
		t.Fatal("actual legacy absence refused", err)
	}
	if err := v.Init(); err != nil {
		t.Fatal(err)
	}
}

func TestCorrectionCommittedMergeReceiptsSurviveManyRecaptures(t *testing.T) {
	v, _ := newVault(t)
	a := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	b := mustCapture(t, v, sqliteDecision, t0.Add(time.Minute))
	proposal, _ := os.ReadFile(b.Path)
	mustDoctor(t, v, t0.Add(time.Minute))
	for i := 2; i <= 140; i++ {
		mustCapture(t, v, sqliteDecision, t0.Add(time.Duration(i)*time.Minute))
		mustDoctor(t, v, t0.Add(time.Duration(i)*time.Minute))
	}
	before, _ := v.Find(a.ID)
	if len(before.RecapturedBy) > 128 || before.Confirmed != 140 {
		t.Fatalf("bad bounded metadata %+v", before)
	}
	// Replay an old cleanup after its ID was pruned from the target. Its durable
	// archive receipt still proves this capture already applied.
	os.WriteFile(b.Path, proposal, 0600)
	mustDoctor(t, v, t0.Add(3*time.Hour))
	after, _ := v.Find(a.ID)
	if after.Confirmed != before.Confirmed || fileExists(b.Path) {
		t.Fatal("old cleanup double counted or remained pending")
	}
}

func TestCorrectionMergeRetryCapacityDoesNotEvictUnresolvedIDs(t *testing.T) {
	v, _ := newVault(t)
	a := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	into, _ := v.Find(a.ID)
	d := &doctor{v: v, now: t0, r: &Report{Live: map[string]int{}}, live: []*Note{into}}
	var first *Note
	for i := 0; i < 128; i++ {
		n := mustCapture(t, v, sqliteDecision, t0.Add(time.Duration(i+1)*time.Minute))
		if first == nil {
			first = n
		}
		// Preserve ordinary completed proposals to exercise the bound without a
		// platform-specific permission fault. The receipt is the cleanup proof.
		p, _ := os.ReadFile(n.Path)
		d.merge(n.Path, n, into)
		os.WriteFile(n.Path, p, 0600)
	}
	n := mustCapture(t, v, sqliteDecision, t0.Add(3*time.Hour))
	d.merge(n.Path, n, into)
	if into.Confirmed != 128 || len(into.RecapturedBy) != 128 || !fileExists(n.Path) || !hasFlag(d.r, "mutation_failed") {
		t.Fatal("retry bound evicted unresolved state")
	}
	d.handle(first.Path, first)
	d.merge(n.Path, n, into)
	if into.Confirmed != 129 || fileExists(n.Path) {
		t.Fatal("cleanup did not release retry capacity")
	}
}

func TestCorrectionWriterRefusesUnreadableOutputWithoutReplacingLive(t *testing.T) {
	v, _ := newVault(t)
	a := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	n, _ := v.Find(a.ID)
	before, _ := os.ReadFile(n.Path)
	n.Extra = strings.Repeat("x", maxDocumentBytes)
	if err := v.writeNote(n.Path, n); err == nil {
		t.Fatal("oversized output published")
	}
	after, _ := os.ReadFile(n.Path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed writer replaced readable live note")
	}
}

func TestCorrectionAcceptedCaptureAccountingUnknownUntilReset(t *testing.T) {
	v, root := newVault(t)
	v.RecordSession("demo", t0)
	v.RecordSession("demo", t0)
	lock := filepath.Join(v.Root, ".state", "demo.lock")
	os.WriteFile(lock, []byte("held"), 0600)
	in := sqliteDecision
	in.Project = "demo"
	n, err := v.Capture(in, t0)
	if err != nil || n.AccountingWarning == "" || !fileExists(n.Path) {
		t.Fatalf("lost acceptance/degradation: %+v %v", n, err)
	}
	os.Remove(lock)
	mustDoctor(t, v, t0)
	text, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if strings.Contains(text, "no accepted captures") || !strings.Contains(text, "accounting degraded/unknown") {
		t.Fatal(text)
	}
	if len(v.Status().AccountingUnknown) != 1 {
		t.Fatal("status counter uncertainty hidden")
	}
	v.RecordSession("demo", t0.Add(time.Minute))
	if !v.loadState("demo").AccountingUnknown {
		t.Fatal("new session pretended to repair unknown accounting")
	}
	mustCapture(t, v, sqliteDecision, t0.Add(2*time.Minute))
	if v.loadState("demo").AccountingUnknown || v.loadState("demo").InvocationsSinceCapture != 0 {
		t.Fatal("successful capture failed to reset accounting")
	}
}
