package app_test

import (
	"os"
	"path/filepath"
	"runtime"
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
// defaulted DataDir to the platform data home, silently
// writing synthetic records, tombstones, and observations into the real
// operator store.
func TestExplicitConfigDirIsSelfContained(t *testing.T) {
	home := t.TempDir()
	isolateHome(t, home)

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
	_, realData, realCache, err := app.DefaultDirs()
	if err != nil {
		t.Fatal(err)
	}
	for _, real := range []string{realData, realCache} {
		if _, err := os.Stat(real); !os.IsNotExist(err) {
			t.Fatalf("BuildProfile touched the operator home %s — test isolation violated", real)
		}
	}
}

// isolateHome points every home/platform-dir variable at a temp dir.
func isolateHome(t *testing.T, home string) {
	t.Helper()
	for _, k := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(k, home)
	}
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "BEME_CONFIG_HOME", "BEME_DATA_HOME", "BEME_CACHE_HOME"} {
		t.Setenv(k, "")
	}
}

// TestDefaultDirsPerPlatform pins the platform directory contract (NFR-007):
// no platform borrows another's layout, and BEME_* overrides win.
func TestDefaultDirsPerPlatform(t *testing.T) {
	home := t.TempDir()
	isolateHome(t, home)
	cfg, data, cache, err := app.DefaultDirs()
	if err != nil {
		t.Fatal(err)
	}
	var want [3]string
	switch runtime.GOOS {
	case "darwin":
		want = [3]string{
			filepath.Join(home, "Library", "Application Support", "beme"),
			filepath.Join(home, "Library", "Application Support", "beme"),
			filepath.Join(home, "Library", "Caches", "beme"),
		}
	case "windows":
		want = [3]string{
			filepath.Join(home, "AppData", "Roaming", "beme"),
			filepath.Join(home, "AppData", "Roaming", "beme"),
			filepath.Join(home, "AppData", "Local", "beme"),
		}
	default:
		want = [3]string{
			filepath.Join(home, ".config", "beme"),
			filepath.Join(home, ".local", "share", "beme"),
			filepath.Join(home, ".cache", "beme"),
		}
	}
	if got := [3]string{cfg, data, cache}; got != want {
		t.Fatalf("%s default dirs: got %v, want %v", runtime.GOOS, got, want)
	}

	t.Setenv("BEME_DATA_HOME", filepath.Join(home, "override"))
	if _, data, _, _ := app.DefaultDirs(); data != filepath.Join(home, "override") {
		t.Fatalf("BEME_DATA_HOME must override the platform default; got %s", data)
	}
}
