package main

// Harness integration tests against the INSTALLED Codex and Claude Code CLIs.
//
// Opt-in: set BEME_HARNESS_INTEGRATION=1 with the harness CLI on PATH (CI has
// neither, so these skip there). Each test points the harness at an isolated
// config home inside t.TempDir() (CLAUDE_CONFIG_DIR / CODEX_HOME): the
// operator's real harness configuration is never read or written, and no
// model call is made.
//
// Verification levels reported in docs/INTEGRATIONS.md:
//   - mcp_protocol:       TestMCPClientEndToEnd (real MCP client <-> real server)
//   - config_lifecycle:   the harness CLI adds, parses, lists, and removes beme
//   - harness_connection: the harness itself spawns beme and completes the MCP
//     handshake (Claude Code: `claude mcp get` health check)
//   - pre_decision_use:   needs live model sessions; not verifiable here

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func harnessCLI(t *testing.T, name string) string {
	t.Helper()
	if os.Getenv("BEME_HARNESS_INTEGRATION") != "1" {
		t.Skip("set BEME_HARNESS_INTEGRATION=1 to run installed-harness integration tests")
	}
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s CLI not installed", name)
	}
	return path
}

func buildBemeForHarness(t *testing.T) (bin, cfg string) {
	t.Helper()
	bin = filepath.Join(t.TempDir(), "beme-harness")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = mustRepoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s", out)
	}
	cfg = filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	return bin, cfg
}

func runHarness(t *testing.T, dir string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestClaudeCodeHarnessIntegration: config lifecycle + harness connection.
func TestClaudeCodeHarnessIntegration(t *testing.T) {
	claude := harnessCLI(t, "claude")
	bin, cfg := buildBemeForHarness(t)
	home := t.TempDir()
	work := t.TempDir()
	env := []string{"CLAUDE_CONFIG_DIR=" + home}

	out, err := runHarness(t, work, env, claude, "mcp", "add", "beme", "--",
		bin, "serve", "--config", cfg, "--projection", "work-safe", "--capability", "cap_harness_it")
	if err != nil {
		t.Fatalf("claude mcp add: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); err != nil {
		t.Fatalf("registration must land in the isolated config home: %v", err)
	}

	out, err = runHarness(t, work, env, claude, "mcp", "get", "beme")
	if err != nil {
		t.Fatalf("claude mcp get: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Connected") {
		t.Fatalf("harness_connection: Claude Code did not complete the MCP handshake with beme:\n%s", out)
	}
	t.Logf("harness_connection verified: Claude Code spawned beme and reported Connected")

	out, err = runHarness(t, work, env, claude, "mcp", "remove", "beme", "-s", "local")
	if err != nil {
		t.Fatalf("claude mcp remove: %v\n%s", err, out)
	}
	if out, err = runHarness(t, work, env, claude, "mcp", "get", "beme"); err == nil && strings.Contains(out, "Connected") {
		t.Fatalf("config_lifecycle: beme still registered after removal:\n%s", out)
	}
	t.Logf("config_lifecycle verified: add -> get -> remove in an isolated Claude Code config home")
}

// TestCodexHarnessIntegration: config lifecycle. Codex offers no MCP health
// check without a model session, so harness_connection is reported, not
// claimed.
func TestCodexHarnessIntegration(t *testing.T) {
	codex := harnessCLI(t, "codex")
	bin, cfg := buildBemeForHarness(t)
	home := t.TempDir()
	work := t.TempDir()
	env := []string{"CODEX_HOME=" + home}

	out, err := runHarness(t, work, env, codex, "mcp", "add", "beme", "--",
		bin, "serve", "--config", cfg, "--projection", "work-safe", "--capability", "cap_harness_it")
	if err != nil {
		t.Fatalf("codex mcp add: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err != nil {
		t.Fatalf("registration must land in the isolated CODEX_HOME: %v", err)
	}

	listed := func() []map[string]any {
		out, err := runHarness(t, work, env, codex, "mcp", "list", "--json")
		if err != nil {
			t.Fatalf("codex mcp list: %v\n%s", err, out)
		}
		var servers []map[string]any
		if err := json.Unmarshal([]byte(out[strings.Index(out, "["):]), &servers); err != nil {
			t.Fatalf("codex mcp list --json unparseable: %v\n%s", err, out)
		}
		return servers
	}
	found := false
	for _, s := range listed() {
		if s["name"] != "beme" {
			continue
		}
		tr, _ := s["transport"].(map[string]any)
		if tr["type"] != "stdio" || tr["command"] != bin {
			t.Fatalf("codex parsed the beme entry incorrectly: %v", s)
		}
		found = true
	}
	if !found {
		t.Fatal("config_lifecycle: codex did not list the beme server")
	}

	out, _ = runHarness(t, work, env, codex, "doctor", "--json")
	if i := strings.Index(out, "{"); i >= 0 {
		var report struct {
			Checks map[string]struct {
				Status string `json:"status"`
			} `json:"checks"`
		}
		if json.Unmarshal([]byte(out[i:]), &report) == nil {
			if st := report.Checks["mcp.config"].Status; st != "" && st != "ok" {
				t.Fatalf("codex doctor mcp.config = %s", st)
			}
		}
	}

	if out, err := runHarness(t, work, env, codex, "mcp", "remove", "beme"); err != nil {
		t.Fatalf("codex mcp remove: %v\n%s", err, out)
	}
	for _, s := range listed() {
		if s["name"] == "beme" {
			t.Fatal("config_lifecycle: beme still listed after removal")
		}
	}
	t.Logf("config_lifecycle verified: add -> list/doctor -> remove in an isolated CODEX_HOME")
	t.Logf("harness_connection NOT verified for Codex: no MCP health check exists without a model session (codex exec)")
}
