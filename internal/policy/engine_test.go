package policy_test

import (
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
)

func mkRec(sensitivity string, profile contracts.Profile, role contracts.SourceRole, kind contracts.Kind) contracts.Record {
	return contracts.Record{
		RecordID:    "rec_test00001",
		SourceID:    "canonical-knowledge",
		Kind:        kind,
		Status:      contracts.StatusActive,
		Confidence:  contracts.ConfidenceValidated,
		Authority:   contracts.AuthorityDefault,
		SourceRole:  role,
		Trust:       contracts.TrustCanonical,
		Sensitivity: sensitivity,
		Scope:       contracts.Scope{Profiles: []string{string(profile)}},
	}
}

func TestSecretNeverIngestEvenUnderPersonal(t *testing.T) {
	e := policy.NewEngine(time.Now())
	cap := policy.Capability{CapabilityID: "cap_p", Profile: contracts.ProfilePersonal}
	rec := mkRec(contracts.SensSecretNeverIngest, contracts.ProfilePersonal, contracts.RoleCanonicalKnowledge, contracts.KindPrinciple)
	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("secret_never_ingest must never resolve")
	}
}

func TestPersonalPrivateDeniedUnderWorkSafe(t *testing.T) {
	e := policy.NewEngine(time.Now())
	rec := mkRec(contracts.SensPersonalPrivate, contracts.ProfilePersonal, contracts.RoleCanonicalKnowledge, contracts.KindPrinciple)
	cap := policy.Capability{CapabilityID: "cap_w", Profile: contracts.ProfileWorkSafe}
	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("personal_private must be denied under work-safe capability")
	}
	// and allowed under personal
	capP := policy.Capability{CapabilityID: "cap_p", Profile: contracts.ProfilePersonal}
	if !e.Evaluate(rec, capP, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("personal_private should resolve under personal capability")
	}
}

func TestWorkRestrictedNamespaceMatch(t *testing.T) {
	e := policy.NewEngine(time.Now())
	rec := mkRec("work_restricted:ws-alpha", contracts.ProfileWorkSafe, contracts.RoleTrustedProjectPolicy, contracts.KindPrecedent)
	cap := policy.Capability{CapabilityID: "cap_w", Profile: contracts.ProfileWorkSafe}

	// matching workspace -> allowed
	if !e.Evaluate(rec, cap, policy.TaskContext{Task: "x", WorkspaceID: "ws-alpha"}, nil, false).Eligible {
		t.Fatal("work_restricted should resolve in its registered workspace")
	}
	// wrong workspace -> denied (cross-namespace join denied)
	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x", WorkspaceID: "ws-beta"}, nil, false).Eligible {
		t.Fatal("work_restricted must not cross namespaces")
	}
	// no workspace -> denied
	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("work_restricted without workspace must be denied")
	}
}

func TestQuarantinedObservationRequiresExplicitEnable(t *testing.T) {
	e := policy.NewEngine(time.Now())
	rec := mkRec("public_general", contracts.ProfilePersonal, contracts.RoleLearnedObservation, contracts.KindPreference)
	rec.Trust = contracts.TrustQuarantined
	cap := policy.Capability{CapabilityID: "cap_p", Profile: contracts.ProfilePersonal}

	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("learned observation must be excluded by default")
	}
	cap.ExperimentalLearnedGuidance = true
	if !e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, true).Eligible {
		t.Fatal("learned observation should be eligible when explicitly enabled")
	}
}

func TestRevocationTombstone(t *testing.T) {
	e := policy.NewEngine(time.Now())
	rec := mkRec("public_general", contracts.ProfilePersonal, contracts.RoleCanonicalKnowledge, contracts.KindPrinciple)
	cap := policy.Capability{CapabilityID: "cap_p", Profile: contracts.ProfilePersonal}
	tc := policy.TaskContext{Task: "x"}

	if !e.Evaluate(rec, cap, tc, nil, false).Eligible {
		t.Fatal("baseline eligibility failed")
	}
	if e.Evaluate(rec, cap, tc, map[string]bool{rec.RecordID: true}, false).Eligible {
		t.Fatal("revoked record must not resolve")
	}
	if e.Evaluate(rec, cap, tc, map[string]bool{"source:canonical-knowledge": true}, false).Eligible {
		t.Fatal("source-level revocation must block all its records")
	}
}

func TestExpiredRecordDenied(t *testing.T) {
	e := policy.NewEngine(time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	rec := mkRec("public_general", contracts.ProfilePersonal, contracts.RoleCanonicalKnowledge, contracts.KindFact)
	exp := "2026-01-01"
	rec.Validity = contracts.Validity{ExpiresAt: &exp}
	cap := policy.Capability{CapabilityID: "cap_p", Profile: contracts.ProfilePersonal}
	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("expired record must not resolve")
	}
}

func TestUntrustedDataNeverNormative(t *testing.T) {
	e := policy.NewEngine(time.Now())
	rec := mkRec("public_general", contracts.ProfilePersonal, contracts.RoleTrustedReference, contracts.KindPrinciple)
	rec.Trust = contracts.TrustUntrustedData
	cap := policy.Capability{CapabilityID: "cap_p", Profile: contracts.ProfilePersonal}
	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("untrusted data must not be normative guidance")
	}
}

func TestProfileScopeMismatch(t *testing.T) {
	e := policy.NewEngine(time.Now())
	rec := mkRec("public_general", contracts.ProfileWorkSafe, contracts.RoleCanonicalKnowledge, contracts.KindPrinciple)
	cap := policy.Capability{CapabilityID: "cap_p", Profile: contracts.ProfilePersonal}
	if e.Evaluate(rec, cap, policy.TaskContext{Task: "x"}, nil, false).Eligible {
		t.Fatal("record scoped to work-safe must not resolve under personal-only scope list mismatch... unless unconstrained")
	}
	// Note: empty scope.profiles means unconstrained; a work-safe-listed record
	// under a personal capability is a scope mismatch -> excluded.
}
