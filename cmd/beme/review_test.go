package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripBrainHooksLeavesUnknownShapesAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	orig := `{"hooks":{"Custom":{"note":"not an array"},"Empty":[],"PreToolUse":[{"matcher":"Bash","hooks":"weird"}]}}`
	os.WriteFile(path, []byte(orig), 0o600)
	if _, err := installSettingsHooks(path, hooksFragment("beme", "", false), false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := installSettingsHooks(path, nil, true, false); err != nil {
		t.Fatal(err)
	}
	got := string(mustMarshal(readSettings(t, path)))
	for _, want := range []string{`"Custom":{"note":"not an array"}`, `"Empty":[]`, `"hooks":"weird"`} {
		if !strings.Contains(got, want) {
			t.Errorf("settings the feature does not own were altered; missing %s in %s", want, got)
		}
	}
}

func TestInstallHooksRefusesTemporaryBinary(t *testing.T) {
	for _, p := range []string{"/var/folders/x/T/go-build123/b001/exe/beme", "/tmp/go-build9/beme"} {
		if !looksTemporary(p) {
			t.Errorf("%s must be recognized as a throwaway build path", p)
		}
	}
	if looksTemporary("/usr/local/bin/beme") {
		t.Error("a normal install path must be accepted")
	}
}

func TestDoctorCommitIgnoresUserSigningConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	// A hostile-to-automation git config: signing required, signer fails.
	os.WriteFile(filepath.Join(r.home, ".gitconfig"), []byte("[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = /usr/bin/false\n"), 0o600)
	proj := filepath.Join(r.home, "proj")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init", "--git")
	r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
	r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "learning", "--title", "Signing config",
		"--body", "Automated vault commits must not depend on the user's signing setup.")
	if _, se, code := r.run("", "brain", "doctor", "--quiet", "--commit"); code != 0 {
		t.Fatalf("automated commit must not require signing (exit %d): %s", code, se)
	}
}
