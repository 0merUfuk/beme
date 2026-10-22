package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedReadWithholdsSymlinkedDirectoryAndOversizeIdentity(t *testing.T) {
	v, root := newVault(t)
	outside := t.TempDir()
	n := &Note{ID: "lrn-20260919-outside-1111", Type: TypeLearning, Project: "demo", Title: "Outside sentinel", Statement: "SYNTHETIC_OUTSIDE_DIRECTORY must not be read through a linked type directory.", Status: StatusActive, Created: t0.Format("2006-01-02T15:04:05Z")}
	data, _ := n.Marshal()
	os.WriteFile(filepath.Join(outside, n.ID+".md"), data, 0600)
	os.Symlink(outside, v.liveDir("demo", TypeLearning))
	s, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if strings.Contains(s, "SYNTHETIC_OUTSIDE_DIRECTORY") {
		t.Error("directory symlink disclosed outside content")
	}
	os.Remove(v.liveDir("demo", TypeLearning))
	os.WriteFile(filepath.Join(v.Root, "identity.md"), []byte(strings.Repeat("safe prose ", 100000)), 0600)
	s, _ = v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if !strings.Contains(s, "identity.md was withheld") {
		t.Error("oversized identity must be refused, not loaded and sliced")
	}
}

func TestScopedBriefingReportsBoundedScanWithoutOpeningOtherProjects(t *testing.T) {
	v, root := newVault(t)
	n := &Note{ID: "lrn-20260919-large-1111", Type: TypeLearning, Project: "demo", Title: "Oversized note", Statement: strings.Repeat("safe prose ", 100000), Status: StatusActive, Created: t0.Format("2006-01-02T15:04:05Z")}
	putLive(t, v, n)
	s, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if !strings.Contains(s, "read incomplete") {
		t.Fatal("refused files need a truthful bounded-read notice")
	}
}

func TestCaptureSameInstantSameTitleDoesNotClobber(t *testing.T) {
	v, _ := newVault(t)
	for i := 0; i < 4; i++ {
		mustCapture(t, v, CaptureInput{Type: TypeSummary, Title: "Repeated checkpoint", Statement: fmt.Sprintf("Checkpoint %d records a distinct synthetic durable result.", i)}, t0)
	}
	if n := countMD(v.inboxDir()); n != 4 {
		t.Fatalf("four accepted proposals require four files, got %d", n)
	}
}

func TestGlobalProjectStateIsRefusedAndExistingNotesPreserved(t *testing.T) {
	v, root := newVault(t)
	for _, typ := range []Type{TypeOpen, TypeSummary} {
		_, err := v.Capture(CaptureInput{Type: typ, Project: GlobalProject, Title: "Global checkpoint", Statement: "Project state must have an explicitly registered project scope."}, t0)
		if err == nil || !strings.Contains(err.Error(), "scope") {
			t.Errorf("%s global capture needs actionable scope error: %v", typ, err)
		}
		n := &Note{ID: typ.prefix() + "-20260919-legacy-1111", Type: typ, Project: GlobalProject, Title: "Legacy global project state", Statement: "SYNTHETIC_GLOBAL_PROJECT_STATE must not be broadcast to other projects.", Status: typ.liveStatus(), Created: t0.Format("2006-01-02T15:04:05Z")}
		putLive(t, v, n)
	}
	r := mustDoctor(t, v, t0)
	if !hasFlag(r, "unsupported_scope") {
		t.Error("existing unsupported global state must be flagged")
	}
	ns, _ := v.List(GlobalProject, "", false)
	if len(ns) != 2 {
		t.Fatal("legacy state must be preserved")
	}
	s, _ := v.Briefing(BriefingOptions{CWD: root, Now: t0})
	if strings.Contains(s, "SYNTHETIC_GLOBAL_PROJECT_STATE") || strings.Contains(s, "20260919-legacy-1111") {
		t.Fatal("legacy project state or its identifiers were broadcast")
	}
	s, _ = v.Briefing(BriefingOptions{CWD: t.TempDir(), Now: t0})
	if strings.Contains(s, "Notes can only be captured to `global`") {
		t.Fatal("unregistered projects must not get a generic global fallback")
	}
}
