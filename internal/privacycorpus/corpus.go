// Package privacycorpus implements the deterministically testable threat
// cases from the blueprint's §19 ship-blocking list and the P1–P18 invariant
// groups of evals/EVALUATION_CONTRACT.md §7.
//
// Each case is executable WITHOUT live models: it drives the real engine
// (policy, resolver, ingestion, workspace, learning, MCP args surface) and
// asserts the invariant. Acceptance rate must be 100% — one failure blocks
// release (binary blockers, never averaged).
//
// The registry (NewSuite) is shared by the Go test and the executable runner
// cmd/beme-threat-corpus. Results are passed | failed | not_run per case with
// reasons; not_run is reserved for cases whose environment is unavailable
// (only case 19 without a repository checkout).
package privacycorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
)

// Result is one threat-case execution.
type Result struct {
	Case   string `json:"case"`
	Group  string `json:"group"`
	State  string `json:"state"` // passed | failed | not_run
	Reason string `json:"reason,omitempty"`
}

// CaseFn executes one threat case; a returned error records a failure (or a
// not_run, via NotRun) instead of aborting the suite.
type CaseFn func() error

// Options configures the registry.
type Options struct {
	// RepoRoot is a repository checkout for process-gate cases (case 19
	// audits the CI workflow). Empty → those cases report not_run.
	RepoRoot string
}

// Suite is the shared registry: case IDs → executable case and invariant
// group. Built by NewSuite (cases.go).
type Suite struct {
	Cases  map[string]CaseFn
	Groups map[string]string
}

// SupplementaryCases are the purge-reliability cases added beyond the §19
// list (ADR-027).
var SupplementaryCases = []string{"S1", "S2", "S3"}

// Run executes every case in ID order.
func (s *Suite) Run() []Result {
	return Run(s.Cases, func(id string) string { return s.Groups[id] })
}

// Run executes the provided cases and returns results in ID order.
func Run(suite map[string]CaseFn, groupOf func(caseID string) string) []Result {
	ids := make([]string, 0, len(suite))
	for id := range suite {
		ids = append(ids, id)
	}
	sortStrings(ids)
	out := []Result{}
	for _, id := range ids {
		r := Result{Case: id, Group: groupOf(id)}
		err := suite[id]()
		switch {
		case err == nil:
			r.State = "passed"
		case strings.HasPrefix(err.Error(), "NOT_RUN:"):
			r.State = "not_run"
			r.Reason = strings.TrimPrefix(err.Error(), "NOT_RUN: ")
		default:
			r.State = "failed"
			r.Reason = err.Error()
		}
		out = append(out, r)
	}
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// --- shared fixture builders for threat-case suites ---

// ThreatDeployment is an isolated deployment with one personal + one safe
// source and a registered workspace, for boundary/escape cases.
type ThreatDeployment struct {
	Home      string
	Runtime   *app.Runtime
	Workspace string // registered workspace path
}

// NewThreatDeployment builds the standard adversarial fixture: a personal
// source with secret-shaped content, a safe declassified source, and a
// registered workspace root.
func NewThreatDeployment(dir string) (*ThreatDeployment, error) {
	if dir == "" {
		d, err := os.MkdirTemp("", "beme-threat-")
		if err != nil {
			return nil, err
		}
		dir = d
	}
	cfg := filepath.Join(dir, "cfg")
	os.MkdirAll(filepath.Join(cfg, "sources"), 0o700)
	os.MkdirAll(filepath.Join(cfg, "policies"), 0o700)

	// personal source (private preference + a secrets file that must be excluded)
	personalRoot := filepath.Join(dir, "personal", "entries")
	os.MkdirAll(personalRoot, 0o755)
	os.WriteFile(filepath.Join(personalRoot, "PRIV-001.md"), []byte("---\nid: PRIV-001\ntitle: \"Private preference\"\ntype: preference\nstatus: active\n---\n\nThe owner prefers private working sessions late at night.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "personal", "secrets", "prod.env"), []byte("AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMIsecret"), 0o600)

	// safe declassified source
	safeRoot := filepath.Join(dir, "safe", "entries")
	os.MkdirAll(safeRoot, 0o755)
	os.WriteFile(filepath.Join(safeRoot, "SAFE-001.md"), []byte("---\nid: SAFE-001\ntitle: \"Measured need\"\ntype: heuristic\nstatus: active\n---\n\nIntroduce complexity only for a demonstrated measured need.\n"), 0o644)

	// registered workspace
	wsDir := filepath.Join(dir, "workspace-repo")
	os.MkdirAll(wsDir, 0o755)

	os.WriteFile(filepath.Join(cfg, "sources", "personal.yaml"), []byte("schema_version: \"1\"\nsource_id: personal-th\ntype: directory\nroot: "+filepath.Join(dir, "personal")+"\npurpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: default\nsensitivity: personal_private\nprofiles_allowed: [personal]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\nexclude: [\"secrets/**\"]\n"), 0o600)
	os.WriteFile(filepath.Join(cfg, "sources", "safe.yaml"), []byte("schema_version: \"1\"\nsource_id: safe-th\ntype: directory\nroot: "+filepath.Join(dir, "safe")+"\npurpose: [safe_declassified]\ntrust: canonical\ninstruction_semantics: registered_files_only\nauthority_ceiling: recommended\nsensitivity: public_general\nprofiles_allowed: [personal, work-safe]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n"), 0o600)
	os.WriteFile(filepath.Join(cfg, "policies", "ws.yaml"), []byte("schema_version: \"1\"\nworkspace_id: ws-threat\ncanonical_roots:\n  - "+wsDir+"\nsensitivity_namespace: work_restricted\nauthority_ceiling: default\n"), 0o600)

	rt, err := app.Load(cfg)
	if err != nil {
		return nil, err
	}
	return &ThreatDeployment{Home: dir, Runtime: rt, Workspace: wsDir}, nil
}

// Build builds both projections.
func (d *ThreatDeployment) Build() error {
	if _, err := d.Runtime.BuildProfile(contracts.ProfilePersonal); err != nil {
		return err
	}
	_, err := d.Runtime.BuildProfile(contracts.ProfileWorkSafe)
	return err
}

// PersonalText is the canary string that must NEVER reach work-safe.
const PersonalText = "private working sessions late at night"

// Serve opens a session under a profile.
func (d *ThreatDeployment) Serve(profile contracts.Profile) (*app.Session, error) {
	return d.Runtime.Serve(profile, "cap_threat_"+string(profile), false)
}

// NotRun marks a case as explicitly not executable in this environment.
func NotRun(reason string) error { return fmt.Errorf("NOT_RUN: %s", reason) }

// LoadRuntime loads a runtime from a config dir (test/suite helper).
func LoadRuntime(cfg string) (*app.Runtime, error) {
	return app.Load(cfg)
}
