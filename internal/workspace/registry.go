// Package workspace implements trusted workspace identity resolution
// (blueprint §8.7, FR-014). The registry is the only authority; request
// signals are matching hints only. Ambiguity fails closed.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	"gopkg.in/yaml.v3"
)

// Registry holds registered workspaces (out-of-band, operator-owned).
type Registry struct {
	Workspaces map[string]contracts.Workspace
}

// MatchResult reports identity resolution.
type MatchResult struct {
	WorkspaceID string
	Ambiguous   bool
	Reason      string
}

// Resolve maps an untrusted path hint to a registered workspace.
// Rules:
//   - canonical root exact/real-path match wins;
//   - multiple matches = ambiguous -> fail closed;
//   - zero matches = no workspace personalization;
//   - a symlinked or non-real path never matches (FR-022).
func (r *Registry) Resolve(hint string) MatchResult {
	if hint == "" {
		return MatchResult{Reason: "no hint"}
	}
	real, err := filepath.EvalSymlinks(hint)
	if err != nil || real == "" {
		return MatchResult{Reason: "hint path not resolvable; failing closed"}
	}
	matches := []string{}
	for id, ws := range r.Workspaces {
		for _, root := range ws.CanonicalRoots {
			rootReal := root
			if resolved, err := filepath.EvalSymlinks(root); err == nil {
				rootReal = resolved
			}
			if real == rootReal || strings.HasPrefix(real, rootReal+string(filepath.Separator)) {
				matches = append(matches, id)
				break
			}
		}
	}
	if len(matches) == 0 {
		return MatchResult{Reason: "unregistered path; no project personalization"}
	}
	if len(matches) > 1 {
		return MatchResult{Ambiguous: true, Reason: "multiple registered roots match; failing closed"}
	}
	return MatchResult{WorkspaceID: matches[0]}
}

// LoadRegistry reads registered workspaces from a trusted config dir.
// Only files explicitly listed are trusted; the directory itself is
// operator-controlled (FR-016).
func LoadRegistry(dir string) (*Registry, error) {
	reg := &Registry{Workspaces: map[string]contracts.Workspace{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		ws, err := loadOne(path)
		if err != nil {
			return nil, fmt.Errorf("workspace registry %s: %w", path, err)
		}
		if ws.WorkspaceID == "" {
			return nil, fmt.Errorf("workspace registry %s: empty workspace_id", path)
		}
		reg.Workspaces[ws.WorkspaceID] = ws
	}
	return reg, nil
}

func loadOne(path string) (contracts.Workspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return contracts.Workspace{}, err
	}
	var ws contracts.Workspace
	if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal(data, &ws); err != nil {
			return contracts.Workspace{}, err
		}
	} else {
		if err := yaml.Unmarshal(data, &ws); err != nil {
			return contracts.Workspace{}, err
		}
	}
	return ws, nil
}
