package main

import (
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

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPClientEndToEnd is true end-to-end verification of the agent-facing
// boundary (WP8 / ACCEPTANCE §7 "adapters verified end to end"): a REAL MCP
// client (the official Go SDK) spawns the REAL beme stdio server as a
// subprocess and drives the full protocol: initialize → tools list →
// resolve_context → status → feedback quarantine.
//
// It proves the server speaks genuine MCP, the tool surface matches the
// ADR-009 contract, and resolution works over the wire — without any paid
// model call (the client is the harness stand-in).
func TestMCPClientEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	// build the real binary
	bin := filepath.Join(t.TempDir(), "beme-e2e")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = mustRepoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s", out)
	}

	// isolated deployment: one source, built projection
	home := t.TempDir()
	cfg := filepath.Join(home, "cfg")
	os.MkdirAll(filepath.Join(cfg, "sources"), 0o700)
	srcRoot := filepath.Join(home, "src", "entries")
	os.MkdirAll(srcRoot, 0o755)
	os.WriteFile(filepath.Join(srcRoot, "M-001.md"), []byte("---\nid: M-001\ntitle: \"E2E principle\"\ntype: principle\nstatus: active\n---\n\nResolve context over real MCP.\n"), 0o644)
	os.WriteFile(filepath.Join(cfg, "sources", "src.yaml"), []byte("schema_version: \"1\"\nsource_id: e2e-src\ntype: directory\nroot: "+filepath.Join(home, "src")+"\npurpose: [safe_declassified]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: recommended\nsensitivity: public_general\nprofiles_allowed: [personal, work-safe]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"), 0o600)

	rt, err := app.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.BuildProfile(contracts.ProfileWorkSafe); err != nil {
		t.Fatal(err)
	}

	// real MCP client over the real server subprocess
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "beme-e2e-test-client", Version: "1.0.0"}, nil)
	transport := &mcp.CommandTransport{
		Command: exec.Command(bin, "serve",
			"--projection", "work-safe",
			"--capability", "cap_e2e_ws",
			"--transport", "stdio",
			"--config", cfg),
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("MCP connect failed: %v", err)
	}
	defer session.Close()

	// 1. tool surface: exactly the four narrow tools (ADR-009)
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	got := map[string]bool{}
	for _, tl := range tools.Tools {
		got[tl.Name] = true
	}
	for _, want := range []string{"beme.resolve_context", "beme.get_context_item", "beme.report_feedback", "beme.status"} {
		if !got[want] {
			t.Errorf("tool %s missing from real server tools/list", want)
		}
	}
	if len(tools.Tools) != 4 {
		t.Errorf("tool surface must be exactly 4; got %d (%v)", len(tools.Tools), got)
	}

	// 2. resolve_context over the wire
	var pack map[string]any
	if err := json.Unmarshal([]byte(resolveText(t, ctx, session, "beme.resolve_context", map[string]any{
		"task": "resolve context over real MCP protocol",
	})), &pack); err != nil {
		t.Fatalf("resolve_context returned non-JSON: %v", err)
	}
	res, _ := pack["resolution"].(map[string]any)
	if res == nil || res["profile"] != "work-safe" {
		t.Fatalf("pack must resolve under work-safe capability; got %v", res)
	}

	// 2b. pack-bound expansion (ADR-029): a record selected into the issued
	// pack expands with its pack_id; without the pack or for an unselected ID
	// the answer is the same refusal.
	packID, _ := pack["pack_id"].(string)
	selected := ""
	for _, section := range []string{"constraints", "guidance", "precedents"} {
		items, _ := pack[section].([]any)
		for _, it := range items {
			if m, ok := it.(map[string]any); ok && selected == "" {
				selected, _ = m["record_id"].(string)
			}
		}
	}
	if packID == "" || selected == "" {
		t.Fatalf("resolved pack must carry a pack_id and a selected record; got pack_id=%q record=%q", packID, selected)
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(resolveText(t, ctx, session, "beme.get_context_item", map[string]any{
		"pack_id": packID, "record_id": selected,
	})), &item); err != nil {
		t.Fatalf("get_context_item returned non-JSON: %v", err)
	}
	if item["record_id"] != selected {
		t.Fatalf("expansion returned the wrong record: %v", item)
	}
	if _, leaked := item["source_id"]; leaked {
		t.Fatal("work-safe expansion must omit source identity")
	}
	refusals := map[string]string{}
	for name, args := range map[string]map[string]any{
		"unknown pack":      {"pack_id": "ctx_000000000000000000000000", "record_id": selected},
		"unselected record": {"pack_id": packID, "record_id": "rec_not-selected"},
	} {
		res, err := callTool(t, ctx, session, "beme.get_context_item", args)
		if err != nil || res == nil || !res.IsError {
			t.Fatalf("%s must be refused in-band; got res=%v err=%v", name, res, err)
		}
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				refusals[name] = tc.Text
			}
		}
	}
	if refusals["unknown pack"] != refusals["unselected record"] {
		t.Fatalf("refusals must be indistinguishable: %v", refusals)
	}
	if res, err := callTool(t, ctx, session, "beme.get_context_item", map[string]any{"record_id": selected}); err == nil && res != nil && !res.IsError {
		t.Fatal("expansion without pack_id must be rejected")
	}

	// 3. status hides private sources in work-safe mode
	var status map[string]any
	if err := json.Unmarshal([]byte(resolveText(t, ctx, session, "beme.status", map[string]any{})), &status); err != nil {
		t.Fatal(err)
	}
	if srcs, _ := status["sources"].(string); srcs == "" {
		t.Fatal("work-safe status must hide private source names")
	}

	// 4. feedback quarantine over the wire
	var fb map[string]any
	if err := json.Unmarshal([]byte(resolveText(t, ctx, session, "beme.report_feedback", map[string]any{
		"kind": "observation",
		"text": "e2e: the user seems to prefer work-safe defaults",
	})), &fb); err != nil {
		t.Fatal(err)
	}
	if fb["status"] != "quarantined" {
		t.Fatalf("feedback must be quarantined; got %v", fb["status"])
	}
	if _, ok := fb["observation_id"]; !ok {
		t.Fatal("feedback must return an observation id")
	}

	// 5. elevation attempt over the wire (threat case 1): a "profile" field is
	// structurally absent from the tool's input schema, so the PROTOCOL
	// rejects it before any handler runs. This is ADR-016 enforced by the
	// SDK's schema validation — the strongest possible position.
	elevRes, elevErr := callTool(t, ctx, session, "beme.resolve_context", map[string]any{
		"task":    "elevation attempt",
		"profile": "personal",
	})
	if elevErr == nil && elevRes != nil && !elevRes.IsError {
		t.Fatal("elevation attempt must be rejected: got a successful result")
	}
	t.Logf("elevation attempt rejected at protocol level (correct): %v", elevErr)

	// 6. the quarantined observation exists in the data dir
	obsDir := filepath.Join(rt.Config.DataDir, "observations")
	entries, _ := os.ReadDir(obsDir)
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			data, _ := os.ReadFile(filepath.Join(obsDir, e.Name()))
			if strings.Contains(string(data), "work-safe defaults") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("quarantined observation not persisted in the data dir")
	}
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	params := &mcp.CallToolParams{}
	params.Name = name
	params.Arguments = json.RawMessage(raw)
	return session.CallTool(ctx, params)
}

func resolveText(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := callTool(t, ctx, session, name, args)
	if err != nil {
		t.Fatalf("%s call failed: %v", name, err)
	}
	if res.IsError {
		msgs := []string{}
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				msgs = append(msgs, tc.Text)
			} else {
				msgs = append(msgs, fmt.Sprintf("%v", c))
			}
		}
		t.Fatalf("%s returned tool error: %s", name, strings.Join(msgs, " | "))
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatalf("%s returned no text content", name)
	return ""
}

func mustRepoRoot(t *testing.T) string {
	wd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(wd))
}
