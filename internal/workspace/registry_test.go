package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/workspace"
)

func writeRegistry(t *testing.T, dir string, wss ...contracts.Workspace) *workspace.Registry {
	t.Helper()
	os.MkdirAll(dir, 0o700)
	for i, ws := range wss {
		data := []byte("workspace_id: " + ws.WorkspaceID + "\n" +
			"canonical_roots:\n" +
			"  - " + ws.CanonicalRoots[0] + "\n" +
			"sensitivity_namespace: " + ws.SensitivityNamespace + "\n" +
			"authority_ceiling: " + string(ws.AuthorityCeiling) + "\n")
		os.WriteFile(filepath.Join(dir, "ws"+string(rune('a'+i))+".yaml"), data, 0o600)
	}
	reg, err := workspace.LoadRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestRegisteredRootMatches(t *testing.T) {
	root := t.TempDir()
	reg := writeRegistry(t, t.TempDir(), contracts.Workspace{
		WorkspaceID: "ws-alpha", CanonicalRoots: []string{root},
		SensitivityNamespace: "work_restricted", AuthorityCeiling: contracts.AuthorityDefault,
	})
	m := reg.Resolve(root)
	if m.WorkspaceID != "ws-alpha" || m.Ambiguous {
		t.Fatalf("registered root must match exactly: %+v", m)
	}
	// subdirectory of a registered root matches too (must exist for real-path resolution)
	child := filepath.Join(root, "src", "pkg")
	os.MkdirAll(child, 0o755)
	m = reg.Resolve(child)
	if m.WorkspaceID != "ws-alpha" {
		t.Fatalf("child path of registered root must match: %+v", m)
	}
}

func TestUnregisteredPathNoPersonalization(t *testing.T) {
	reg := writeRegistry(t, t.TempDir(), contracts.Workspace{
		WorkspaceID: "ws-alpha", CanonicalRoots: []string{t.TempDir()},
		SensitivityNamespace: "work_restricted", AuthorityCeiling: contracts.AuthorityDefault,
	})
	m := reg.Resolve(t.TempDir())
	if m.WorkspaceID != "" || m.Ambiguous {
		t.Fatalf("unregistered path must yield no workspace personalization: %+v", m)
	}
}

func TestAmbiguousPathsFailClosed(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	reg := writeRegistry(t, t.TempDir(),
		contracts.Workspace{WorkspaceID: "ws-a", CanonicalRoots: []string{a}, SensitivityNamespace: "work_restricted", AuthorityCeiling: contracts.AuthorityDefault},
		contracts.Workspace{WorkspaceID: "ws-b", CanonicalRoots: []string{b}, SensitivityNamespace: "work_restricted", AuthorityCeiling: contracts.AuthorityDefault},
	)
	// Build a path that lives under both roots: impossible for distinct tmp
	// dirs, so simulate ambiguity via overlapping registration instead.
	reg.Workspaces["ws-c"] = contracts.Workspace{WorkspaceID: "ws-c", CanonicalRoots: []string{a}, SensitivityNamespace: "work_restricted", AuthorityCeiling: contracts.AuthorityDefault}
	m := reg.Resolve(a)
	if !m.Ambiguous {
		t.Fatalf("two registered workspaces claiming the same root must fail closed; got %+v", m)
	}
}

func TestSymlinkedHintResolvesRealPath(t *testing.T) {
	root := t.TempDir()
	reg := writeRegistry(t, t.TempDir(), contracts.Workspace{
		WorkspaceID: "ws-alpha", CanonicalRoots: []string{root},
		SensitivityNamespace: "work_restricted", AuthorityCeiling: contracts.AuthorityDefault,
	})
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(root, link)
	m := reg.Resolve(link)
	if m.WorkspaceID != "ws-alpha" {
		t.Fatalf("symlinked path must resolve through real path to registered root: %+v", m)
	}
}

// TestCloneDoesNotInheritTrust: a same-name directory elsewhere never
// matches the registered root (threat case 27).
func TestCloneDoesNotInheritTrust(t *testing.T) {
	root := t.TempDir()
	clone := t.TempDir()
	reg := writeRegistry(t, t.TempDir(), contracts.Workspace{
		WorkspaceID: "ws-alpha", CanonicalRoots: []string{root},
		SensitivityNamespace: "work_restricted", AuthorityCeiling: contracts.AuthorityDefault,
	})
	m := reg.Resolve(clone)
	if m.WorkspaceID != "" {
		t.Fatalf("a clone path must not inherit workspace trust: %+v", m)
	}
}
