package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRestoredBackupCannotResurrectOnAnySurface drives the real binary: after
// a physical purge, a pre-purge projection backup and trace cache are
// restored, then every CLI and MCP read surface is attempted. With the purge
// key removed, every surface must fail closed.
func TestRestoredBackupCannotResurrectOnAnySurface(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	bin := filepath.Join(t.TempDir(), "beme-surfaces")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = mustRepoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s", out)
	}

	home := t.TempDir()
	cfg := filepath.Join(home, "cfg")
	entries := filepath.Join(home, "src", "entries")
	for _, d := range []string{filepath.Join(cfg, "sources"), entries} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	const canary = "purged surface canary statement"
	write := func(id, body string) {
		if err := os.WriteFile(filepath.Join(entries, id+".md"), []byte("---\nid: "+id+"\ntitle: \""+id+"\"\ntype: preference\nstatus: active\n---\n\n"+body+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("CAN-001", "The owner has a "+canary+".")
	write("KEEP-001", "Keep changes small and reviewable.")
	if err := os.WriteFile(filepath.Join(cfg, "sources", "s.yaml"), []byte("schema_version: \"1\"\nsource_id: surf-src\ntype: directory\nroot: "+filepath.ToSlash(filepath.Join(home, "src"))+"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: personal_private\nprofiles_allowed: [personal]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runCLI := func(args ...string) (string, int) {
		cmd := exec.Command(bin, append(args, "--config", cfg)...)
		out, err := cmd.CombinedOutput()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return string(out), code
	}
	// flags must precede positional args for purge; build the argv by hand
	cli := func(args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return string(out), ee.ExitCode()
		} else if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return string(out), 0
	}

	if out, code := runCLI("build", "--profile", "personal"); code != 0 {
		t.Fatalf("build: %d %s", code, out)
	}
	out, code := runCLI("preview", "--task", "owner surface canary preference", "--json")
	if code != 0 || !strings.Contains(out, canary) {
		t.Fatalf("positive control: preview must show the canary before purge: %d %s", code, out)
	}
	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	storePath := rt.ProjectionPath(contracts.ProfilePersonal)
	traceDir := filepath.Join(rt.Config.CacheDir, "traces")
	traces, _ := os.ReadDir(traceDir)
	if len(traces) != 1 {
		t.Fatalf("positive control: preview must persist one trace, got %d", len(traces))
	}
	traceID := "trace_" + strings.TrimSuffix(traces[0].Name(), ".json")
	traceBytes, _ := os.ReadFile(filepath.Join(traceDir, traces[0].Name()))
	storeBytes, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(traceBytes), "rec_can-001") {
		t.Fatal("positive control: trace must name the canary record")
	}

	if out, code := cli("purge", "--config", cfg, "--confirm", "rec_can-001", "rec_can-001"); code != 0 {
		t.Fatalf("purge: %d %s", code, out)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		os.Remove(storePath + suffix)
	}
	if err := os.WriteFile(storePath, storeBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(traceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(traceDir, traces[0].Name()), traceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(storeBytes), canary) {
		t.Fatal("positive control: restored store must contain the canary")
	}

	surfaces := map[string][]string{
		"export":  {"export", "--projection", "personal"},
		"preview": {"preview", "--task", "owner surface canary preference", "--json"},
		"explain": {"explain", "--trace", traceID, "--json"},
		"doctor":  {"doctor", "--json"},
	}
	for name, args := range surfaces {
		out, code := runCLI(args...)
		if code != 0 {
			t.Fatalf("%s after restore: exit %d %s", name, code, out)
		}
		if strings.Contains(out, canary) || strings.Contains(strings.ToLower(out), "can-001") {
			t.Fatalf("%s surfaced the purged record after a backup restore:\n%s", name, out)
		}
	}
	if out, _ := runCLI("doctor", "--json"); !strings.Contains(out, "rebuild required") {
		t.Fatalf("doctor must report that a rebuild is required: %s", out)
	}

	// MCP surfaces over the real protocol.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "surfaces", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(bin, "serve", "--projection", "personal", "--capability", "cap_surfaces", "--config", cfg)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(resolveText(t, ctx, session, contracts.ToolStatus, map[string]any{})), &status); err != nil {
		t.Fatal(err)
	}
	if status["record_count"] != float64(1) {
		t.Fatalf("MCP status must count only the visible record: %v", status)
	}
	packText := resolveText(t, ctx, session, contracts.ToolResolveContext, map[string]any{"task": "owner surface canary preference"})
	if strings.Contains(packText, canary) {
		t.Fatal("MCP resolve surfaced the purged record")
	}
	var pack struct {
		PackID string `json:"pack_id"`
	}
	json.Unmarshal([]byte(packText), &pack)
	res, err := callTool(t, ctx, session, contracts.ToolGetContextItem, map[string]any{"pack_id": pack.PackID, "record_id": "rec_can-001"})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("MCP get_context_item must refuse the purged record: %v %v", res, err)
	}
	session.Close()

	// Remove the purge key: every surface fails closed.
	if err := os.Remove(rt.PurgeKeyPath()); err != nil {
		t.Fatal(err)
	}
	for name, args := range surfaces {
		out, code := runCLI(args...)
		if name == "doctor" {
			if !strings.Contains(out, "policy_blocked") || strings.Contains(out, canary) {
				t.Fatalf("doctor must report policy_blocked without content: %s", out)
			}
			continue
		}
		if code != 3 || strings.Contains(out, canary) {
			t.Fatalf("%s without the purge key must exit 3 without content; got %d %s", name, code, out)
		}
	}
	session, err = client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(bin, "serve", "--projection", "personal", "--capability", "cap_surfaces", "--config", cfg)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for tool, args := range map[string]map[string]any{
		contracts.ToolStatus:         {},
		contracts.ToolResolveContext: {"task": "owner surface canary preference"},
		contracts.ToolGetContextItem: {"pack_id": pack.PackID, "record_id": "rec_keep-001"},
	} {
		res, err := callTool(t, ctx, session, tool, args)
		if err != nil || res == nil || !res.IsError {
			t.Fatalf("MCP %s without the purge key must fail closed: %v %v", tool, res, err)
		}
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok && strings.Contains(tc.Text, canary) {
				t.Fatalf("MCP %s leaked content while failing closed", tool)
			}
		}
	}
}
