package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A real source file -> real build executable -> real MCP server -> explain
// round trip. No normalized records are seeded around the ingestion boundary.
func TestMCPSourceEvidenceTraceAndHealth(t *testing.T) {
	bin := buildBemeBinary(t, "beme-runtime-evidence")
	cfg, rt := diagnosticsDeployment(t)
	entry := filepath.Join(rt.Sources[0].Root, "entries", "DIAG-001.md")
	body := "---\nid: SYN-DEPLOY\ntype: heuristic\nscope:\n  task_kinds: [deployment]\n---\n# Rollout\n\n## Summary\n\nCheck container health before deployment.\n"
	if err := os.WriteFile(entry, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code != 0 {
		t.Fatalf("build %d: %s", code, stderr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-runtime-test", Version: "1"}, nil)
	s, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(bin, "serve", "--config", cfg, "--projection", "personal", "--capability", "synthetic-runtime", "--transport", "stdio")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var p resolver.Pack
	getPack := func() {
		t.Helper()
		text := resolveText(t, ctx, s, contracts.ToolResolveContext, map[string]any{"task": "Deploy the container rollout checklist"})
		if err := json.Unmarshal([]byte(text), &p); err != nil {
			t.Fatal(err)
		}
	}
	getPack()
	if len(p.Guidance) != 1 || p.Guidance[0].Text != "Check container health before deployment." {
		t.Fatalf("source decision missing over MCP: %+v", p)
	}
	if p.TraceRef == "" {
		t.Fatal("trace not issued")
	}
	if out, stderr, code := runBeme(t, bin, "explain", "--config", cfg, "--projection", "personal", "--trace", p.TraceRef, "--json"); code != 0 || !strings.Contains(out, "rec_syn-deploy") {
		t.Errorf("MCP trace cannot be explained: exit=%d %s %s", code, out, stderr)
	}
	// The CLI natural-language entrypoint must deliver the same scoped decision.
	out, stderr, code := runBeme(t, bin, "preview", "--config", cfg, "--projection", "personal", "--task", "Deploy the container rollout checklist", "--json")
	if code != 0 || !strings.Contains(out, "Check container health") {
		t.Fatalf("CLI parity: %d %s %s", code, out, stderr)
	}
	// Class-only health must change in an existing MCP session after partial
	// build, and recover after rebuilding the now-available source.
	missing := filepath.Join(t.TempDir(), "missing")
	sd := rt.Sources[0]
	sd.SourceID = "synthetic-unavailable-label"
	sd.Root = missing
	b, _ := json.Marshal(sd)
	if err := os.WriteFile(filepath.Join(cfg, "sources", "missing.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code == 0 {
		t.Fatal("partial-build operator control")
	}
	status := resolveText(t, ctx, s, contracts.ToolStatus, map[string]any{})
	var h struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(status), &h); err != nil {
		t.Fatal(err)
	}
	if h.Status != "degraded" {
		t.Errorf("MCP concealed partial build: %s", status)
	}
	getPack()
	if p.Resolution.Completeness == "complete" || len(p.Degradations) == 0 {
		t.Error("MCP concealed missing evidence")
	}
	b, _ = json.Marshal(p.Degradations)
	if strings.Contains(string(b), missing) || strings.Contains(string(b), sd.SourceID) {
		t.Error("source failure identity leaked")
	}
	if err := os.MkdirAll(filepath.Join(missing, "entries"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(missing, "entries", "RECOVERED.md"), []byte(strings.ReplaceAll(body, "SYN-DEPLOY", "SYN-RECOVERED")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code != 0 {
		t.Fatalf("recovery build: %d %s", code, stderr)
	}
	status = resolveText(t, ctx, s, contracts.ToolStatus, map[string]any{})
	if err := json.Unmarshal([]byte(status), &h); err != nil {
		t.Fatal(err)
	}
	getPack()
	if h.Status != "ok" || len(p.Guidance) != 2 || len(p.Degradations) != 0 {
		t.Fatalf("MCP did not recover: %s %+v", status, p)
	}
	// The optional trace cache may fail while both CLI and MCP keep serving.
	if err := os.Rename(rt.Config.CacheDir, rt.Config.CacheDir+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rt.Config.CacheDir, []byte("synthetic cache blocker"), 0600); err != nil {
		t.Fatal(err)
	}
	getPack()
	if len(p.Guidance) != 2 || p.TraceRef != "" || len(p.Degradations) == 0 {
		t.Fatalf("MCP cache failure dishonest: %+v", p)
	}
	out, stderr, code = runBeme(t, bin, "preview", "--config", cfg, "--projection", "personal", "--task", "Deploy the container rollout checklist", "--json")
	var cli resolver.Pack
	if code != 0 || json.Unmarshal([]byte(out), &cli) != nil || len(cli.Guidance) != 2 || cli.TraceRef != "" {
		t.Fatalf("CLI cache failure blocked read/advertised handle: %d %s %s", code, out, stderr)
	}
	if err := os.Remove(rt.Config.CacheDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(rt.Config.CacheDir+"-saved", rt.Config.CacheDir); err != nil {
		t.Fatal(err)
	}
	// A fully unavailable build is different from a healthy negative control.
	for _, source := range []string{rt.Sources[0].Root, missing} {
		if err := os.Rename(source, source+"-saved"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, code := runBeme(t, bin, "build", "--profile", "personal", "--config", cfg); code == 0 {
		t.Fatal("empty failed build must fail operator check")
	}
	status = resolveText(t, ctx, s, contracts.ToolStatus, map[string]any{})
	if err := json.Unmarshal([]byte(status), &h); err != nil {
		t.Fatal(err)
	}
	getPack()
	if h.Status != "unavailable" || len(p.DeliveredItems()) != 0 || p.Resolution.Completeness != "incomplete" {
		t.Fatalf("unavailable evidence called complete: %s %+v", status, p)
	}
}
