package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// TestToolSurfaceContract pins ADR-009: exactly the four narrow tools.
func TestToolSurfaceContract(t *testing.T) {
	expected := map[string]bool{
		"beme.resolve_context":  true,
		"beme.get_context_item": true,
		"beme.report_feedback":  true,
		"beme.status":           true,
	}
	data, err := os.ReadFile("mcp.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`Name:\s*"(beme\.[a-z_]+)"`)
	got := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		got[m[1]] = true
	}
	for name := range expected {
		if !got[name] {
			t.Errorf("expected tool %s missing from server registration", name)
		}
	}
	for name := range got {
		if !expected[name] {
			t.Errorf("unexpected tool %s registered — ADR-009 violation (narrow surface)", name)
		}
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
