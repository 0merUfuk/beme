package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

// TestExplicitConfigDirIsSelfContained (ADR-026) pins the test-isolation
// invariant that failed on 2026-09-15: an explicit config dir is a
// self-contained deployment root. Derived data must live INSIDE it —
// never in the operator's real data home.
//
// Failure mode this pins: Load(tmpdir) with no config.yaml previously
// defaulted DataDir to ~/Library/Application Support/beme, silently
// writing synthetic records, tombstones, and observations into the real
// operator store.
func TestExplicitConfigDirIsSelfContained(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BEME_DATA_HOME", "")
	os.Unsetenv("BEME_DATA_HOME")

	cfg := filepath.Join(home, "cfg-self")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}

	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if rt.Config.DataDir != filepath.Join(cfg, "data") {
		t.Fatalf("explicit config dir must self-contain data; got DataDir=%q", rt.Config.DataDir)
	}
	if rt.Config.CacheDir != filepath.Join(cfg, "cache") {
		t.Fatalf("explicit config dir must self-contain cache; got CacheDir=%q", rt.Config.CacheDir)
	}

	// The real default home (under our fake HOME) must not exist after a
	// projection build using this runtime.
	if _, err := rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Logf("build with no sources: %v (fine — no sources registered)", err)
	}
	realHome := filepath.Join(home, "Library", "Application Support", "beme")
	if _, err := os.Stat(realHome); !os.IsNotExist(err) {
		t.Fatalf("BuildProfile touched the operator data home %s — test isolation violated", realHome)
	}
}
