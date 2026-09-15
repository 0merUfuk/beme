package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
)

// TestToolSurfaceContract pins ADR-009/FR-041: the shared tool list is
// exactly the four narrow tools, and mcp.go registers exactly that list.
func TestToolSurfaceContract(t *testing.T) {
	expected := map[string]bool{
		"beme.resolve_context":  true,
		"beme.get_context_item": true,
		"beme.report_feedback":  true,
		"beme.status":           true,
	}
	if len(contracts.MCPTools) != len(expected) {
		t.Fatalf("contracts.MCPTools must list exactly %d tools; got %v", len(expected), contracts.MCPTools)
	}
	for _, name := range contracts.MCPTools {
		if !expected[name] {
			t.Errorf("unexpected tool %s in contracts.MCPTools — ADR-009 violation (narrow surface)", name)
		}
	}
	data, err := os.ReadFile("mcp.go")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`Name:\s*"beme\.`).Match(data) {
		t.Error("mcp.go registers a tool by string literal; use the contracts constants")
	}
	registered := regexp.MustCompile(`Name:\s*contracts\.(Tool\w+)`).FindAllStringSubmatch(string(data), -1)
	if len(registered) != len(contracts.MCPTools) {
		t.Fatalf("mcp.go must register exactly %d tools; found %d", len(contracts.MCPTools), len(registered))
	}
	consts := map[string]string{
		"ToolResolveContext": contracts.ToolResolveContext,
		"ToolGetContextItem": contracts.ToolGetContextItem,
		"ToolReportFeedback": contracts.ToolReportFeedback,
		"ToolStatus":         contracts.ToolStatus,
	}
	seen := map[string]bool{}
	for _, m := range registered {
		name, ok := consts[m[1]]
		if !ok || seen[name] {
			t.Fatalf("mcp.go registers unknown or duplicate tool constant %s", m[1])
		}
		seen[name] = true
	}
}

// TestElevationNotRepresentable: the resolve args struct and the schema
// structurally lack profile/capability/authority fields (ADR-016).
func TestElevationNotRepresentable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas", "context-pack", "resolution-request.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"profile", "capability", "authority", "sources", "sensitivity", "expand_all"} {
		if _, ok := s.Properties[f]; ok {
			t.Fatalf("request schema must structurally lack a %q field — elevation representable!", f)
		}
	}
	for _, r := range []string{"schema_version", "task"} {
		found := false
		for _, got := range s.Required {
			if got == r {
				found = true
			}
		}
		if !found {
			t.Errorf("required field %s missing", r)
		}
	}
}

// TestStdioOnlyTransport: ADR-014 — network transports rejected in v1.
func TestStdioOnlyTransport(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "beme-bin")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir, _ = os.Getwd()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s", out)
	}
	cmd := exec.Command(bin, "serve", "--projection", "personal", "--capability", "x", "--transport", "http")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("network transport must be rejected in v1; got success: %s", out)
	}
	if !regexp.MustCompile("stdio").Match(out) {
		t.Fatalf("rejection must name stdio-only policy: %s", out)
	}
}
