// Package contracts defines the core domain types of Be Me.
//
// These types mirror the versioned JSON schemas in /schemas exactly. The
// schemas are the contract; this package is their Go projection. Domain
// logic depends only on this package and its siblings — never on the CLI,
// MCP server, or storage drivers (NFR-010).
//
// Security invariants encoded here (ADR-006, ADR-016):
//   - Authority is assigned only by the trusted registration/approval path.
//     Source content cannot set authority; ingestion maps it to
//     informational + untrusted unless the source is registered trusted.
//   - Sensitivity is monotonic: a derived record may retain or increase
//     sensitivity, never lower it.
//   - The resolution request carries task context only. There is no field
//     through which a model can select a profile, capability, or authority.
package contracts

// SchemaVersion is the current contract version for all v1 artifacts.
const SchemaVersion = "1"

// Profile is a serving capability name. It is process-bound, never
// request-bound.
type Profile string

const (
	ProfilePersonal Profile = "personal"
	ProfileWorkSafe Profile = "work-safe"
	ProfileNone     Profile = "" // degraded serving: no personalization
)

// Sensitivity classes (blueprint §7.3). WorkRestricted carries a workspace
// namespace: "work_restricted:<workspace-id>".
const (
	SensPublicGeneral        = "public_general"
	SensPersonalPrivate      = "personal_private"
	SensSecretNeverIngest    = "secret_never_ingest"
	SensWorkRestrictedPrefix = "work_restricted:"
)

// Kind is the closed set of normalized record kinds. observation/candidate/
// inference are learned-model states, not canonical kinds.
type Kind string

const (
	KindDirective   Kind = "directive"
	KindFact        Kind = "fact"
	KindPreference  Kind = "preference"
	KindPrinciple   Kind = "principle"
	KindHeuristic   Kind = "heuristic"
	KindPattern     Kind = "pattern"
	KindWorkflow    Kind = "workflow"
	KindFailureMode Kind = "failure_mode"
	KindCapability  Kind = "capability"
	KindPrecedent   Kind = "precedent"
	KindUnmapped    Kind = "unmapped_reference"
)

func (k Kind) Valid() bool {
	switch k {
	case KindDirective, KindFact, KindPreference, KindPrinciple, KindHeuristic,
		KindPattern, KindWorkflow, KindFailureMode, KindCapability, KindPrecedent, KindUnmapped:
		return true
	}
	return false
}

// Status is the canonical lifecycle. superseded/revoked/stale_suspected are
// resolver states, not canonical statuses.
type Status string

const (
	StatusActive     Status = "active"
	StatusDeprecated Status = "deprecated"
)

// Confidence is epistemic confidence. No invented decimals.
type Confidence string

const (
	ConfidenceObserved  Confidence = "observed"
	ConfidenceValidated Confidence = "validated"
)

// Authority is normative authority assigned by the trusted path only.
type Authority string

const (
	AuthorityInformational Authority = "informational"
	AuthorityRecommended   Authority = "recommended"
	AuthorityDefault       Authority = "default"
)

// SourceRole and Trust classify provenance.
type SourceRole string

const (
	RoleCanonicalFoundation  SourceRole = "canonical_foundation"
	RoleCanonicalKnowledge   SourceRole = "canonical_reusable_knowledge"
	RoleTrustedProjectPolicy SourceRole = "trusted_project_policy"
	RoleTrustedReference     SourceRole = "trusted_reference"
	RoleEpisodicEvidence     SourceRole = "episodic_evidence"
	RoleDeclassifiedSafe     SourceRole = "declassified_safe"
	RoleLearnedObservation   SourceRole = "learned_observation"
)

type Trust string

const (
	TrustCanonical      Trust = "canonical"
	TrustTrustedProject Trust = "trusted_project"
	TrustReference      Trust = "reference"
	TrustUntrustedData  Trust = "untrusted_data"
	TrustQuarantined    Trust = "quarantined"
)

// Record is the normalized envelope (schemas/record/normalized-record).
type Record struct {
	SchemaVersion    string        `json:"schema_version"`
	RecordID         string        `json:"record_id"`
	SourceID         string        `json:"source_id"`
	SourceRecordID   string        `json:"source_record_id"`
	SourceRevision   string        `json:"source_revision,omitempty"`
	Key              string        `json:"key,omitempty"`
	DecisionKey      string        `json:"decision_key,omitempty"`
	Kind             Kind          `json:"kind"`
	Title            string        `json:"title,omitempty"`
	Statement        string        `json:"statement,omitempty"`
	CompactText      string        `json:"compact_text,omitempty"`
	Status           Status        `json:"status"`
	Confidence       Confidence    `json:"confidence"`
	Authority        Authority     `json:"authority"`
	SourceRole       SourceRole    `json:"source_role"`
	Trust            Trust         `json:"trust"`
	Sensitivity      string        `json:"sensitivity"`
	Criticality      string        `json:"criticality,omitempty"`
	Stability        string        `json:"stability,omitempty"`
	Scope            Scope         `json:"scope"`
	Validity         Validity      `json:"validity"`
	Relationships    Relationships `json:"relationships"`
	ProvenanceRefs   []string      `json:"provenance_refs"`
	DeclassifiedFrom string        `json:"declassified_from,omitempty"`
}

type Scope struct {
	Profiles        []string `json:"profiles"`
	WorkspaceIDs    []string `json:"workspace_ids"`
	PathGlobs       []string `json:"path_globs"`
	TaskKinds       []string `json:"task_kinds"`
	LifecyclePhases []string `json:"lifecycle_phases"`
	Technologies    []string `json:"technologies"`
	Harnesses       []string `json:"harnesses"`
	RiskLevels      []string `json:"risk_levels"`
	Environments    []string `json:"environments"`
	Exclude         []string `json:"exclude"`
}

type Validity struct {
	EffectiveFrom string  `json:"effective_from,omitempty"`
	ReviewAfter   *string `json:"review_after"`
	ExpiresAt     *string `json:"expires_at"`
}

type Relationships struct {
	Supersedes    []string `json:"supersedes"`
	Challenges    []string `json:"challenges"`
	ConflictsWith []string `json:"conflicts_with"`
	Refines       []string `json:"refines"`
	DerivedFrom   []string `json:"derived_from"`
}

// Provenance explains lineage; it never alone proves authority.
type Provenance struct {
	SchemaVersion       string           `json:"schema_version,omitempty"`
	ProvenanceID        string           `json:"provenance_id"`
	SourceID            string           `json:"source_id"`
	SourceRecordID      string           `json:"source_record_id"`
	Locator             string           `json:"locator"`
	SourceRevision      string           `json:"source_revision"`
	ContentHash         string           `json:"content_hash"`
	CapturedAt          string           `json:"captured_at"`
	IngestionVersion    int              `json:"ingestion_version"`
	TransformationChain []Transformation `json:"transformation_chain,omitempty"`
	ApprovalEventID     *string          `json:"approval_event_id"`
	SafeView            bool             `json:"safe_view,omitempty"`
}

type Transformation struct {
	Parser  string `json:"parser"`
	Mapping string `json:"mapping"`
}

// SourceDescriptor registers a source root (schemas/source). Registration
// sets a trust ceiling; it never approves future revisions.
type SourceDescriptor struct {
	SchemaVersion        string          `json:"schema_version" yaml:"schema_version"`
	SourceID             string          `json:"source_id" yaml:"source_id"`
	Type                 string          `json:"type" yaml:"type"`
	Root                 string          `json:"root" yaml:"root"`
	Purpose              []string        `json:"purpose" yaml:"purpose"`
	Trust                Trust           `json:"trust" yaml:"trust"`
	InstructionSemantics string          `json:"instruction_semantics" yaml:"instruction_semantics"`
	AuthorityCeiling     Authority       `json:"authority_ceiling" yaml:"authority_ceiling"`
	Sensitivity          string          `json:"sensitivity" yaml:"sensitivity"`
	ProfilesAllowed      []Profile       `json:"profiles_allowed" yaml:"profiles_allowed"`
	IngestionMode        string          `json:"ingestion_mode" yaml:"ingestion_mode"`
	Include              []string        `json:"include,omitempty" yaml:"include"`
	Exclude              []string        `json:"exclude,omitempty" yaml:"exclude"`
	Revision             *SourceRevision `json:"revision,omitempty" yaml:"revision"`
	Rifja                *RifjaContract  `json:"rifja,omitempty" yaml:"rifja"`
	// Registered marks out-of-band trusted registration (FR-016): a
	// descriptor is only active when explicitly registered by the operator.
	Registered bool `json:"-" yaml:"-"`
	Revoked    bool `json:"-" yaml:"-"`
}

type SourceRevision struct {
	Kind           string `json:"kind" yaml:"kind"`
	ApprovedDigest string `json:"approved_digest,omitempty" yaml:"approved_digest"`
}

type RifjaContract struct {
	Surface      string   `json:"surface" yaml:"surface"`
	ToolsAllowed []string `json:"tools_allowed" yaml:"tools_allowed"`
	Bounded      bool     `json:"bounded" yaml:"bounded"`
}

// Workspace is a trusted registry object, never inferred from requests.
type Workspace struct {
	SchemaVersion          string                  `json:"schema_version" yaml:"schema_version"`
	WorkspaceID            string                  `json:"workspace_id" yaml:"workspace_id"`
	CanonicalRoots         []string                `json:"canonical_roots" yaml:"canonical_roots"`
	ExpectedRemoteIdentity *string                 `json:"expected_remote_identity" yaml:"expected_remote_identity"`
	Fingerprint            string                  `json:"fingerprint" yaml:"fingerprint"`
	ApprovedRevisionPolicy *ApprovedRevisionPolicy `json:"approved_revision_policy,omitempty" yaml:"approved_revision_policy"`
	WorktreeIDs            []string                `json:"worktree_ids" yaml:"worktree_ids"`
	SensitivityNamespace   string                  `json:"sensitivity_namespace" yaml:"sensitivity_namespace"`
	BoundProjectPolicy     []BoundPolicy           `json:"bound_project_policy" yaml:"bound_project_policy"`
	AuthorityCeiling       Authority               `json:"authority_ceiling" yaml:"authority_ceiling"`
}

type ApprovedRevisionPolicy struct {
	PinnedCommit  string `json:"pinned_commit,omitempty" yaml:"pinned_commit"`
	ContentDigest string `json:"content_digest,omitempty" yaml:"content_digest"`
}

type BoundPolicy struct {
	Path             string    `json:"path" yaml:"path"`
	ApprovedDigest   string    `json:"approved_digest" yaml:"approved_digest"`
	Scope            string    `json:"scope" yaml:"scope"`
	AuthorityCeiling Authority `json:"authority_ceiling" yaml:"authority_ceiling"`
}

// ResolutionRequest is untrusted agent task context. There is deliberately
// no Profile/Capability/Authority field (ADR-016; verified by schema
// negative fixtures).
type ResolutionRequest struct {
	SchemaVersion    string      `json:"schema_version"`
	Task             string      `json:"task"`
	WorkspaceHint    string      `json:"workspace_hint,omitempty"`
	TaskKindHints    []string    `json:"task_kind_hints,omitempty"`
	RiskHint         string      `json:"risk_hint,omitempty"`
	BudgetHintTokens int         `json:"budget_hint_tokens,omitempty"`
	Harness          *string     `json:"harness,omitempty"`
	TaskOrigin       *TaskOrigin `json:"task_origin,omitempty"`
}

// TaskOrigin is present only when the trusted harness boundary captured the
// user-authored prompt and integrity-bound it (ADR-016 Tier-2 authority).
type TaskOrigin struct {
	Integrity  IntegrityBinding `json:"integrity"`
	CapturedBy string           `json:"captured_by"`
}

type IntegrityBinding struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

// ConflictState enumerates resolver conflict outcomes.
type ConflictState string

const (
	ConflictResolved       ConflictState = "resolved"
	ConflictShadowed       ConflictState = "shadowed"
	ConflictUnresolved     ConflictState = "unresolved"
	ConflictStaleSuspected ConflictState = "stale_suspected"
	ConflictScopeSeparated ConflictState = "scope_separated"
)

// DegradationKind enumerates degradation states.
type DegradationKind string

const (
	DegradeSourceUnavailable DegradationKind = "source_unavailable"
	DegradeStaleProjection   DegradationKind = "stale_projection"
	DegradeReducedAssurance  DegradationKind = "reduced_assurance"
	DegradeUnavailable       DegradationKind = "unavailable"
	DegradePolicyBlocked     DegradationKind = "policy_blocked"
	DegradeRebuildRequired   DegradationKind = "rebuild_required"
)
