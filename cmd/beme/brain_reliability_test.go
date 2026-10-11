package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/brain"
)

func brainReliabilityBinaryPath(original string) string {
	return filepath.Join(filepath.Dir(original), "synthetic tools' directory", "beme"+filepath.Ext(original))
}

func TestBrainReliabilityPortableFixture(t *testing.T) {
	for _, name := range []string{"beme-helper", "beme-helper.exe"} {
		original := filepath.Join(t.TempDir(), name)
		if got := brainReliabilityBinaryPath(original); filepath.Ext(got) != filepath.Ext(original) {
			t.Fatalf("relocation dropped executable extension: %s -> %s", original, got)
		}
	}
}

func brainReliabilityEnv(home string) []string {
	// Retain OS process-launch essentials, not ambient deployment/model settings.
	return []string{
		"HOME=" + home, "USERPROFILE=" + home, "PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + home, "TMP=" + home, "TEMP=" + home,
		"SystemRoot=" + os.Getenv("SystemRoot"), "COMSPEC=" + os.Getenv("COMSPEC"),
		"PATHEXT=" + os.Getenv("PATHEXT"),
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=commit.gpgsign", "GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=tag.gpgsign", "GIT_CONFIG_VALUE_1=false",
	}
}

func TestBrainReliabilityPortableEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SystemRoot", "synthetic-system-root")
	t.Setenv("BEME_CONFIG_HOME", "must-not-inherit")
	env := map[string]string{}
	for _, entry := range brainReliabilityEnv(home) {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	for key, want := range map[string]string{
		"HOME": home, "USERPROFILE": home, "TMPDIR": home, "TEMP": home, "TMP": home,
		"PATH": os.Getenv("PATH"), "SystemRoot": "synthetic-system-root",
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
	} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}
	if _, present := env["BEME_CONFIG_HOME"]; present {
		t.Fatal("test inherited a deployment override")
	}
}

func TestBrainReliabilityEndToEnd(t *testing.T) {
	r := newBrainRig(t)
	// Exercise shell quoting, not just an executable path with no whitespace.
	bin := brainReliabilityBinaryPath(r.bin)
	if err := os.MkdirAll(filepath.Dir(bin), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(r.bin, bin); err != nil {
		t.Fatal(err)
	}
	r.bin = bin
	fixture := func(name string) string {
		r.cfg = filepath.Join(r.home, name, "config beme brain with spaces")
		proj := filepath.Join(r.home, name, "project")
		os.MkdirAll(proj, 0700)
		r.mustRun("", "brain", "init")
		r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
		r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "learning", "--title", "Synthetic durable lesson", "--body", "SYNTHETIC_RELIABILITY_MEMORY remains available across clean CLI processes.")
		r.mustRun("", "brain", "doctor", "--quiet")
		return proj
	}
	shell := func(command, cwd, stdin string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, "/bin/sh", "-c", command)
		c.Dir = cwd
		c.Env = brainReliabilityEnv(r.home)
		c.Stdin = strings.NewReader(stdin)
		out, err := c.CombinedOutput()
		return string(out), err
	}
	quotedCommand := func(s, contains string) string {
		for _, part := range strings.Split(s, "`") {
			if strings.Contains(part, contains) {
				return part
			}
		}
		t.Fatalf("no generated command %q in %s", contains, s)
		return ""
	}
	t.Run("manual-pull-and-executable-continuity", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("generated command execution requires a POSIX shell")
		}
		proj := fixture("manual")
		os.WriteFile(filepath.Join(r.cfg, "config.yaml"), []byte("brain_mode: manual\n"), 0600)
		text := r.sessionStart(proj)
		pull := quotedCommand(text, " brain briefing")
		out, err := shell(pull, proj, "")
		if err != nil || !strings.Contains(out, "SYNTHETIC_RELIABILITY_MEMORY") {
			t.Fatalf("exact advertised pull failed: %s %v", out, err)
		}
		capture := quotedCommand(out, " brain capture")
		capture = strings.Split(capture, " --type ")[0] + " --type learning --title 'Hook path result' --body 'The generated command reached the same synthetic deployment.'"
		if out, err := shell(capture, proj, ""); err != nil {
			t.Fatalf("advertised capture failed: %s %v", out, err)
		}
		if !strings.Contains(capture, shellQuote(r.bin)) || !strings.Contains(capture, shellQuote(r.cfg)) {
			t.Fatal("generated command lost executable/deployment identity")
		}
	})
	t.Run("hook-failure-visible-fail-open", func(t *testing.T) {
		proj := fixture("locked")
		r.mustRun("", "brain", "capture", "--cwd", proj, "--type", "open", "--title", "Pending sentinel", "--body", "SYNTHETIC_PENDING_OUTCOME awaits promotion and must not appear live.")
		os.WriteFile(filepath.Join(r.cfg, "data", "brain", ".lock"), []byte("held"), 0600)
		text := r.sessionStart(proj)
		if !strings.Contains(text, "maintenance pass skipped") || strings.Contains(text, "SYNTHETIC_PENDING_OUTCOME") || !strings.Contains(text, "SYNTHETIC_RELIABILITY_MEMORY") {
			t.Fatalf("maintenance failure hidden or proposal injected: %s", text)
		}
	})
	t.Run("identified-resumes-and-fork", func(t *testing.T) {
		proj := fixture("sessions")
		for _, e := range [][2]string{{"startup", "one"}, {"resume", "one"}, {"resume", "one"}, {"fork", "two"}, {"fork", "two"}} {
			in, _ := json.Marshal(map[string]string{"cwd": proj, "source": e[0], "session_id": e[1]})
			r.mustRun(string(in), "brain", "session-start", "--format", "claude-hook")
		}
		var st struct {
			Sessions    map[string]int `json:"sessions_since_capture"`
			Invocations map[string]int `json:"invocations_since_capture"`
		}
		json.Unmarshal([]byte(r.mustRun("", "brain", "status", "--json")), &st)
		if st.Sessions["demo"] != 2 || st.Invocations["demo"] != 5 {
			t.Fatalf("IDs/resumes/forks counted incorrectly: %+v", st)
		}
	})
	t.Run("cross-process-session-accounting", func(t *testing.T) {
		proj := fixture("concurrent-events")
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			go func(i int) {
				payload, _ := json.Marshal(map[string]string{"cwd": proj, "source": "resume", "session_id": string(rune('a' + i/2))})
				cmd := exec.Command(r.bin, "brain", "session-start", "--format", "claude-hook", "--no-doctor", "--config", r.cfg)
				cmd.Env = brainReliabilityEnv(r.home)
				cmd.Stdin = bytes.NewReader(payload)
				out, err := cmd.CombinedOutput()
				if err == nil && bytes.Contains(out, []byte("accounting not recorded")) {
					err = fmt.Errorf("event refused: %s", out)
				}
				results <- err
			}(i)
		}
		for i := 0; i < 8; i++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		var st struct {
			Sessions    map[string]int `json:"sessions_since_capture"`
			Invocations map[string]int `json:"invocations_since_capture"`
		}
		json.Unmarshal([]byte(r.mustRun("", "brain", "status", "--json")), &st)
		if st.Sessions["demo"] != 4 || st.Invocations["demo"] != 8 {
			t.Fatalf("cross-process update lost/duplicated events: %+v", st)
		}
	})
	t.Run("hookless-explicit-opt-in-only", func(t *testing.T) {
		proj := fixture("hookless")
		for i := 0; i < 3; i++ {
			r.mustRun("", "brain", "briefing", "--cwd", proj)
		}
		state := filepath.Join(r.cfg, "data", "brain", ".state", "demo.yaml")
		before, _ := os.ReadFile(state)
		r.mustRun("", "brain", "briefing", "--cwd", proj)
		after, _ := os.ReadFile(state)
		if !bytes.Equal(before, after) {
			t.Fatal("default read wrote accounting state")
		}
		for i := 0; i < 3; i++ {
			r.mustRun("", "brain", "briefing", "--cwd", proj, "--record-session", "--session-id", "hookless-one")
		}
		var st struct {
			Sessions    map[string]int `json:"sessions_since_capture"`
			Invocations map[string]int `json:"invocations_since_capture"`
		}
		json.Unmarshal([]byte(r.mustRun("", "brain", "status", "--json")), &st)
		if st.Sessions["demo"] != 1 || st.Invocations["demo"] != 3 {
			t.Fatalf("explicit hookless event missing or duplicated: %+v", st)
		}
	})
	t.Run("generated-hook-and-hookless-commands", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("generated command execution requires a POSIX shell")
		}
		proj := fixture("generated")
		var hooks struct {
			Hooks map[string][]struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal([]byte(r.mustRun("", "brain", "hooks", "--binary", r.bin)), &hooks); err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(map[string]string{"cwd": proj, "source": "fork", "session_id": "generated-fork"})
		out, err := shell(hooks.Hooks["SessionStart"][0].Hooks[0].Command, proj, string(payload))
		if err != nil {
			t.Fatalf("generated hook failed: %s %v", out, err)
		}
		var result struct {
			Hook struct {
				Context string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Hook.Context, "SYNTHETIC_RELIABILITY_MEMORY") || !strings.Contains(result.Hook.Context, shellQuote(r.bin)) {
			t.Fatalf("hook lost memory/executable: %s", out)
		}
		r.mustRun("", "brain", "adapter", "install", "codex", "--binary", r.bin)
		block, err := os.ReadFile(filepath.Join(r.home, ".codex", "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		startup := quotedCommand(string(block), " brain briefing")
		if !strings.Contains(startup, "--record-session") || !strings.Contains(startup, "--mode normal") {
			t.Fatalf("hookless opt-in/pull missing: %s", startup)
		}
		os.WriteFile(filepath.Join(r.cfg, "config.yaml"), []byte("brain_mode: manual\n"), 0600)
		out, err = shell(startup, proj, "")
		if err != nil || !strings.Contains(out, "SYNTHETIC_RELIABILITY_MEMORY") {
			t.Fatalf("generated hookless startup failed: %s %v", out, err)
		}
	})
	t.Run("permission-and-unreadable-vault-fail-open", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix permission bits do not establish Windows ACL behavior")
		}
		if os.Geteuid() == 0 {
			t.Skip("filesystem permission fixture does not bind root")
		}
		proj := fixture("permissions")
		vault := filepath.Join(r.cfg, "data", "brain")
		if err := chmodTree(vault, 0500); err != nil {
			t.Fatal(err)
		}
		defer chmodTree(vault, 0700)
		text := r.sessionStart(proj)
		if !strings.Contains(text, "maintenance pass skipped: permission denied") || !strings.Contains(text, "SYNTHETIC_RELIABILITY_MEMORY") {
			t.Fatalf("permission failure hidden: %s", text)
		}
		os.Chmod(vault, 0000)
		text = r.sessionStart(proj)
		os.Chmod(vault, 0700)
		if !strings.Contains(text, "unreadable") || strings.Contains(text, "SYNTHETIC_RELIABILITY_MEMORY") {
			t.Fatalf("unreadable vault reported as healthy: %s", text)
		}
	})
	t.Run("fifo-unrelated-and-scoped-reads", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("named-pipe fixture requires POSIX mkfifo")
		}
		proj := fixture("fifo")
		other := filepath.Join(r.home, "fifo-other")
		os.MkdirAll(other, 0700)
		r.mustRun("", "brain", "project", "add", "other", "--root", other)
		for _, slug := range []string{"other", "demo"} {
			fifo := filepath.Join(r.cfg, "data", "brain", "projects", slug, "learnings", "lrn-20260919-fifo-1111.md")
			os.MkdirAll(filepath.Dir(fifo), 0700)
			if out, err := exec.Command("mkfifo", fifo).CombinedOutput(); err != nil {
				t.Fatalf("FIFO fixture unavailable: %s %v", out, err)
			}
			out, err := shell(shellQuote(r.bin)+" brain briefing --config "+shellQuote(r.cfg)+" --cwd "+shellQuote(proj), proj, "")
			os.Remove(fifo)
			if err != nil || !strings.Contains(out, "SYNTHETIC_RELIABILITY_MEMORY") {
				t.Fatalf("nonregular file blocked read: %s %v", out, err)
			}
		}
	})
}

func TestBrainHooksIncludeForkSource(t *testing.T) {
	b, _ := json.Marshal(hooksFragment("/synthetic/bin/beme", "", false))
	if !strings.Contains(string(b), "startup|resume|clear|compact|fork") {
		t.Fatalf("forked session omitted: %s", b)
	}
}

// Real git-generated reciprocal worktree metadata, without commits or hooks.
func TestBrainRealWorktreeIdentity(t *testing.T) {
	root := t.TempDir()
	wt := filepath.Join(t.TempDir(), "linked")
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		c.Env = brainReliabilityEnv(root)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %s %v", out, err)
		}
	}
	git("init", "-q")
	git("worktree", "add", "--orphan", "-b", "synthetic-branch", wt)
	v := brain.Open(filepath.Join(t.TempDir(), "brain"))
	if err := v.Init(); err != nil {
		t.Fatal(err)
	}
	realRoot, _ := filepath.EvalSymlinks(root)
	realWT, _ := filepath.EvalSymlinks(wt)
	v.AddProject("demo", "Demo", realRoot)
	if m := v.ResolveProject(realWT); m.Slug != "demo" {
		t.Fatalf("real worktree not recognized: %+v", m)
	}
	marker := filepath.Join(realWT, ".git")
	data, _ := os.ReadFile(marker)
	gitdir := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir:"))
	relative, _ := filepath.Rel(realWT, gitdir)
	os.WriteFile(marker, []byte("gitdir: "+relative+"\n"), 0600)
	if m := v.ResolveProject(realWT); m.Slug != "demo" {
		t.Fatalf("valid relative marker refused: %+v", m)
	}
	os.WriteFile(filepath.Join(gitdir, "gitdir"), []byte(filepath.Join(t.TempDir(), ".git")), 0600)
	if m := v.ResolveProject(realWT); m.Slug != "" {
		t.Fatal("mismatched backlink granted project scope")
	}
	os.WriteFile(filepath.Join(gitdir, "gitdir"), []byte(marker+"\n"), 0600)
	v.AddProject("explicit", "Explicit", realWT)
	if m := v.ResolveProject(realWT); m.Slug != "explicit" {
		t.Fatalf("explicit root must win: %+v", m)
	}
}
