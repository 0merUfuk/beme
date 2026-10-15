package brain

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreInstallFailureRollsBackLiveTree(t *testing.T) {
	v, _ := newVault(t)
	n := mustCapture(t, v, sqliteDecision, t0)
	mustDoctor(t, v, t0)
	stage := filepath.Join(t.TempDir(), "stage")
	if err := Open(stage).Init(); err != nil {
		t.Fatal(err)
	}
	failed := false
	err := v.installRestore(stage, func(from, to string) error {
		if strings.HasPrefix(from, stage+string(filepath.Separator)) && filepath.Base(from) == "projects" && !failed {
			failed = true
			return errors.New("synthetic installation failure")
		}
		return os.Rename(from, to)
	})
	if err == nil || !failed {
		t.Fatal("failure seam not exercised")
	}
	if _, err := v.Find(n.ID); err != nil {
		t.Fatalf("prior live tree was not restored: %v", err)
	}
}

func TestBackupRefusesOversizedFileBeforeReading(t *testing.T) {
	v, _ := newVault(t)
	p := filepath.Join(v.Root, "oversized.md")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(restoreMaxFile + 1)
	f.Close()
	started := time.Now()
	if _, err := v.Backup("bounds", t0); err == nil {
		t.Fatal("snapshot must honor its own restore entry bound")
	}
	if time.Since(started) > time.Second {
		t.Fatal("oversized file was read instead of refused before open")
	}
	if snaps, _ := v.ListBackups(); len(snaps) != 0 {
		t.Fatal("failed snapshot was published")
	}
}

func TestExclusiveCapturePublicationNeverReplacesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "proposal.md")
	if err := writeExclusive(p, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(p, []byte("second")); !os.IsExist(err) {
		t.Fatalf("collision must be reported: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "first" {
		t.Fatal("existing proposal changed")
	}
}
