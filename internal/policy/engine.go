// Package policy implements the non-bypassable Stage-A eligibility gate
// (blueprint §9.1, ADR-007). Policy runs before retrieval — always.
package policy

import (
	"fmt"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
)

// Capability is the immutable serving ceiling bound to one process
// (FR-010/011). Requests may narrow, never widen.
type Capability struct {
	CapabilityID string
	Profile      contracts.Profile
	// ExperimentalLearnedGuidance explicitly enables the non-normative
	// learned section (personal mode only, ADR-010).
	ExperimentalLearnedGuidance bool
}

// Engine evaluates policy eligibility deterministically.
type Engine struct {
	now time.Time
}

func NewEngine(now time.Time) *Engine {
	return &Engine{now: now}
}

// SensitivityClass is the coarse class of a sensitivity string.
func SensitivityClass(s string) string {
	switch {
	case strings.HasPrefix(s, contracts.SensWorkRestrictedPrefix):
		return "work_restricted"
	case s == contracts.SensPublicGeneral:
		return "public_general"
	case s == contracts.SensPersonalPrivate:
		return "personal_private"
	case s == contracts.SensSecretNeverIngest:
		return "secret_never_ingest"
	default:
		return "unknown"
	}
}

// CheckSensitivity denies: secret content always; personal_private unless the
// profile is personal; work_restricted unless namespace matches the
// registered workspace. Cross-namespace joins are denied by default (§7.3).
func (e *Engine) CheckSensitivity(rec contracts.Record, cap Capability, workspaceID string) error {
	class := SensitivityClass(rec.Sensitivity)
	switch class {
	case "secret_never_ingest", "unknown":
		return fmt.Errorf("sensitivity denied: %s", rec.Sensitivity)
	case "personal_private":
		if cap.Profile != contracts.ProfilePersonal {
			return fmt.Errorf("personal_private record denied under %s capability", cap.Profile)
		}
	case "work_restricted":
		ns := strings.TrimPrefix(rec.Sensitivity, contracts.SensWorkRestrictedPrefix)
		if cap.Profile != contracts.ProfilePersonal && cap.Profile != contracts.ProfileWorkSafe {
			return fmt.Errorf("no profile bound")
		}
		if workspaceID == "" || ns != workspaceID {
			return fmt.Errorf("work_restricted namespace %q does not match registered workspace %q", ns, workspaceID)
		}
	case "public_general":
		// allowed under any bound profile
	default:
		return fmt.Errorf("unreachable sensitivity class")
	}
	return nil
}

// CheckProfileScope applies record scope-profile eligibility.
func (e *Engine) CheckProfileScope(rec contracts.Record, cap Capability) error {
	if len(rec.Scope.Profiles) == 0 {
		return nil // unconstrained for this dimension
	}
	for _, p := range rec.Scope.Profiles {
		if p == string(cap.Profile) {
			return nil
		}
	}
	return fmt.Errorf("record not eligible for profile %s", cap.Profile)
}

// CheckStatusValid applies lifecycle and validity filters.
func (e *Engine) CheckStatusValid(rec contracts.Record) error {
	if rec.Status != contracts.StatusActive {
		return fmt.Errorf("record status %s not active", rec.Status)
	}
	// validity windows
	if rec.Validity.EffectiveFrom != "" {
		t, err := time.Parse("2006-01-02", rec.Validity.EffectiveFrom)
		if err == nil && e.now.Before(t) {
			return fmt.Errorf("record not yet effective")
		}
	}
	if rec.Validity.ExpiresAt != nil && *rec.Validity.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", *rec.Validity.ExpiresAt)
		if err == nil && e.now.After(t) {
			return fmt.Errorf("record expired")
		}
	}
	return nil
}

// CheckWorkspaceScope: workspace-scoped records only resolve inside their
// registered workspace.
func (e *Engine) CheckWorkspaceScope(rec contracts.Record, workspaceID string) error {
	if len(rec.Scope.WorkspaceIDs) == 0 {
		return nil
	}
	if workspaceID == "" {
		return fmt.Errorf("workspace-scoped record requires a registered workspace")
	}
	for _, w := range rec.Scope.WorkspaceIDs {
		if w == workspaceID {
			return nil
		}
	}
	return fmt.Errorf("record not in workspace %s", workspaceID)
}

// CheckTrust denies quarantined records from normative output and untrusted
// data from anything but evidence roles.
func (e *Engine) CheckTrust(rec contracts.Record, includeLearned bool) error {
	if rec.Trust == contracts.TrustQuarantined {
		if !includeLearned {
			return fmt.Errorf("quarantined observation excluded (learned guidance not enabled)")
		}
	}
	switch rec.SourceRole {
	case contracts.RoleLearnedObservation:
		if !includeLearned {
			return fmt.Errorf("learned observation excluded (learned guidance not enabled)")
		}
	case contracts.RoleEpisodicEvidence:
		// allowed as precedent/reference evidence
	default:
		if rec.Trust == contracts.TrustUntrustedData {
			return fmt.Errorf("untrusted data cannot be normative guidance")
		}
	}
	return nil
}

// CheckRevocation applies tombstones (FR-026). revoked set is keyed by
// record ID; source-level revocation keys use "source:" prefix.
func (e *Engine) CheckRevocation(rec contracts.Record, revoked map[string]bool) error {
	if revoked[rec.RecordID] {
		return fmt.Errorf("record revoked")
	}
	if revoked["source:"+rec.SourceID] {
		return fmt.Errorf("source revoked")
	}
	return nil
}

// TaskContext is the trusted resolution context derived from the request
// after clamping hints to the capability.
type TaskContext struct {
	Task        string
	TaskKinds   []string
	Risk        string
	WorkspaceID string // resolved (trusted), or "" if none/ambiguous
}

// CheckTaskScope: task-kind and risk scope dimensions (OR within, AND across).
func (e *Engine) CheckTaskScope(rec contracts.Record, tc TaskContext) error {
	if len(rec.Scope.TaskKinds) > 0 {
		match := false
		for _, k := range rec.Scope.TaskKinds {
			for _, t := range tc.TaskKinds {
				if k == t {
					match = true
				}
			}
		}
		if !match {
			return fmt.Errorf("task kind not in scope")
		}
	}
	if len(rec.Scope.RiskLevels) > 0 && tc.Risk != "" {
		match := false
		for _, r := range rec.Scope.RiskLevels {
			if r == tc.Risk {
				match = true
			}
		}
		if !match {
			return fmt.Errorf("risk level not in scope")
		}
	}
	// excludes override includes (§9.3)
	for _, ex := range rec.Scope.Exclude {
		if ex == tc.WorkspaceID || ex == tc.Task {
			return fmt.Errorf("explicit exclude matched")
		}
	}
	return nil
}

// EligibilityResult reports Stage-A outcome with a reason for every
// exclusion class (NFR-005).
type EligibilityResult struct {
	Eligible bool
	Reasons  []string // why excluded (or eligibility notes)
}

// Evaluate runs the full Stage-A gate for one record.
func (e *Engine) Evaluate(rec contracts.Record, cap Capability, tc TaskContext, revoked map[string]bool, includeLearned bool) EligibilityResult {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"sensitivity", func() error { return e.CheckSensitivity(rec, cap, tc.WorkspaceID) }},
		{"profile_scope", func() error { return e.CheckProfileScope(rec, cap) }},
		{"status_validity", func() error { return e.CheckStatusValid(rec) }},
		{"workspace_scope", func() error { return e.CheckWorkspaceScope(rec, tc.WorkspaceID) }},
		{"trust", func() error { return e.CheckTrust(rec, includeLearned) }},
		{"revocation", func() error { return e.CheckRevocation(rec, revoked) }},
		{"task_scope", func() error { return e.CheckTaskScope(rec, tc) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			return EligibilityResult{Eligible: false, Reasons: []string{fmt.Sprintf("%s: %s", s.name, err.Error())}}
		}
	}
	return EligibilityResult{Eligible: true}
}
