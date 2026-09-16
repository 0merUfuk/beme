// Package evalfixture builds the public synthetic deployment and golden
// case the evaluation runner's tests (and cmd/beme-eval's tests) use. Every
// record is invented, public_general, and registered under synthetic-*
// sources. The deployment is designed so that every arm must differ:
//
//   - two competing decisions share one decision_key (the more task-relevant
//     one loses precedence);
//   - irrelevant records, two of them long enough that B4's token budget
//     must truncate them;
//   - canonical foundation, canonical knowledge, episodic precedent, and
//     learned-observation evidence;
//   - a workspace-scoped record out of scope for the task, a task-scoped
//     record out of scope, an in-scope workspace record, a revoked record
//     (durable ledger + tombstone), and a deprecated record;
//   - a task that triggers an unknown (SQL dialect).
//
// Build prepares the deployment; Seed writes the records into a projection
// store the caller opens from its own _test.go file (see Seed).
package evalfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/resolver"
	"github.com/0merUfuk/beme/internal/storage"
)

const (
	WorkspaceAlpha = "ws-synth-alpha"
	WorkspaceBeta  = "ws-synth-beta"
	DecisionKey    = "synthetic.storage.engine"
	Task           = "Choose the storage engine and SQL dialect for a single-writer local batch tool."
	CaseID         = "decision.architecture.storage.synthetic-arms.v1"
	CapabilityID   = "cap_eval_synthetic"
	Profile        = contracts.ProfilePersonal

	RecDecisionWinner  = "rec_syn_decision_embedded"
	RecDecisionLoser   = "rec_syn_decision_server"
	RecFoundation      = "rec_syn_foundation"
	RecPrecedent       = "rec_syn_precedent"
	RecLearned         = "rec_syn_learned"
	RecIrrelevantShort = "rec_syn_irrelevant_short"
	RecIrrelevantLong1 = "rec_syn_irrelevant_long_1"
	RecIrrelevantLong2 = "rec_syn_irrelevant_long_2"
	RecScopedAlpha     = "rec_syn_scoped_alpha"
	RecScopedBeta      = "rec_syn_scoped_beta"
	RecTaskScoped      = "rec_syn_task_scoped_deploy"
	RecRevoked         = "rec_syn_revoked"
	RecDeprecated      = "rec_syn_deprecated"
	RecPersonalPrivate = "rec_syn_personal_private"
)

// Gold phrases of the synthetic case.
const (
	AcceptableDecision  = "embedded database file"
	MandatoryConclusion = "storage follows the actual constraint profile"
	ProhibitedPhrase    = "the user dislikes client-server databases"
)

// Marker returns the unique marker token embedded in a record's text.
func Marker(recordID string) string {
	return "SYNMARK-" + strings.ToUpper(strings.TrimPrefix(recordID, "rec_syn_"))
}

// Options vary the deployment.
type Options struct {
	// PersonalPrivateRecord adds one personal_private record, which must
	// make the deployment ineligible for the no-scope ablation.
	PersonalPrivateRecord bool
}

// Deployment is a built synthetic deployment.
type Deployment struct {
	ConfigDir string
	// AlphaPath is a workspace hint that resolves to WorkspaceAlpha.
	AlphaPath string
	Runtime   *app.Runtime
	// Records are the records Seed writes into the personal projection.
	Records []contracts.Record

	provs []contracts.Provenance
}

type recSpec struct {
	id, source, text  string
	kind              contracts.Kind
	role              contracts.SourceRole
	trust             contracts.Trust
	confidence        contracts.Confidence
	status            contracts.Status
	sensitivity       string
	decisionKey       string
	workspaces, kinds []string
}

// Build creates the deployment under base (which must be empty or absent).
func Build(base string, opts Options) (*Deployment, error) {
	cfg := filepath.Join(base, "cfg")
	alpha := filepath.Join(base, "repo-alpha")
	beta := filepath.Join(base, "repo-beta")
	sources := []struct{ id, purpose, trust string }{
		{"synthetic-knowledge", "reusable_knowledge", "canonical"},
		{"synthetic-foundation", "canonical_foundation", "canonical"},
		{"synthetic-episodes", "episodic_evidence", "reference"},
		{"synthetic-observations", "evidence", "reference"},
	}
	for _, d := range []string{filepath.Join(cfg, "sources"), filepath.Join(cfg, "policies"), alpha, beta} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	for _, s := range sources {
		root := filepath.Join(base, "src", s.id)
		if err := os.MkdirAll(filepath.Join(root, "entries"), 0o700); err != nil {
			return nil, err
		}
		desc := fmt.Sprintf("schema_version: \"1\"\nsource_id: %s\ntype: directory\nroot: %q\npurpose: [%s]\ntrust: %s\ninstruction_semantics: registered_files_only\nauthority_ceiling: recommended\nsensitivity: public_general\nprofiles_allowed: [personal, work-safe]\ningestion_mode: index_content\ninclude: [\"entries/**/*.md\"]\n", s.id, filepath.ToSlash(root), s.purpose, s.trust)
		if err := os.WriteFile(filepath.Join(cfg, "sources", s.id+".yaml"), []byte(desc), 0o600); err != nil {
			return nil, err
		}
	}
	for _, ws := range []struct{ id, root string }{{WorkspaceAlpha, alpha}, {WorkspaceBeta, beta}} {
		reg := fmt.Sprintf("schema_version: \"1\"\nworkspace_id: %s\ncanonical_roots: [%q]\nfingerprint: synthetic\nsensitivity_namespace: %s\nauthority_ceiling: recommended\n", ws.id, filepath.ToSlash(ws.root), ws.id)
		if err := os.WriteFile(filepath.Join(cfg, "policies", ws.id+".yaml"), []byte(reg), 0o600); err != nil {
			return nil, err
		}
	}

	rt, err := app.Load(cfg)
	if err != nil {
		return nil, err
	}
	if _, err := rt.BuildProfile(Profile); err != nil {
		return nil, err
	}

	long := func(id string) string {
		return Marker(id) + " " + strings.Repeat("knitting yarn gauge swatch notes. ", 420)
	}
	canon := func(id, text string) recSpec {
		return recSpec{id: id, source: "synthetic-knowledge", text: text, kind: contracts.KindHeuristic, role: contracts.RoleCanonicalKnowledge, trust: contracts.TrustCanonical, confidence: contracts.ConfidenceValidated}
	}
	specs := []recSpec{
		{id: RecDecisionWinner, source: "synthetic-knowledge", kind: contracts.KindPreference, role: contracts.RoleCanonicalKnowledge, trust: contracts.TrustCanonical, confidence: contracts.ConfidenceValidated, decisionKey: DecisionKey,
			text: Marker(RecDecisionWinner) + " " + AcceptableDecision + " kept beside the batch tool"},
		{id: RecDecisionLoser, source: "synthetic-knowledge", kind: contracts.KindPreference, role: contracts.RoleCanonicalKnowledge, trust: contracts.TrustCanonical, confidence: contracts.ConfidenceObserved, decisionKey: DecisionKey,
			text: Marker(RecDecisionLoser) + " choose a client-server storage engine for every single-writer local batch tool"},
		{id: RecFoundation, source: "synthetic-foundation", kind: contracts.KindPrinciple, role: contracts.RoleCanonicalFoundation, trust: contracts.TrustCanonical, confidence: contracts.ConfidenceValidated,
			text: Marker(RecFoundation) + " " + MandatoryConclusion},
		{id: RecPrecedent, source: "synthetic-episodes", kind: contracts.KindPrecedent, role: contracts.RoleEpisodicEvidence, trust: contracts.TrustReference, confidence: contracts.ConfidenceObserved,
			text: Marker(RecPrecedent) + " an earlier batch job shipped with a file-backed store"},
		{id: RecLearned, source: "synthetic-observations", kind: contracts.KindPattern, role: contracts.RoleLearnedObservation, trust: contracts.TrustQuarantined, confidence: contracts.ConfidenceObserved,
			text: Marker(RecLearned) + " the owner usually documents a migration path"},
		canon(RecIrrelevantShort, Marker(RecIrrelevantShort)+" water tomato seedlings at dawn"),
		canon(RecIrrelevantLong1, long(RecIrrelevantLong1)),
		canon(RecIrrelevantLong2, long(RecIrrelevantLong2)),
		func() recSpec {
			r := canon(RecScopedAlpha, Marker(RecScopedAlpha)+" alpha keeps its data in one file")
			r.workspaces = []string{WorkspaceAlpha}
			return r
		}(),
		func() recSpec {
			r := canon(RecScopedBeta, Marker(RecScopedBeta)+" beta runs a managed server")
			r.workspaces = []string{WorkspaceBeta}
			return r
		}(),
		func() recSpec {
			r := canon(RecTaskScoped, Marker(RecTaskScoped)+" container rollout checklist")
			r.kinds = []string{"deployment"}
			return r
		}(),
		canon(RecRevoked, Marker(RecRevoked)+" forgotten guidance"),
		func() recSpec {
			r := canon(RecDeprecated, Marker(RecDeprecated)+" retired guidance")
			r.status = contracts.StatusDeprecated
			return r
		}(),
	}
	if opts.PersonalPrivateRecord {
		r := canon(RecPersonalPrivate, Marker(RecPersonalPrivate)+" private note")
		r.sensitivity = contracts.SensPersonalPrivate
		specs = append(specs, r)
	}

	recs := []contracts.Record{}
	provs := []contracts.Provenance{}
	for _, s := range specs {
		srcRecID := strings.ToUpper(strings.ReplaceAll(strings.TrimPrefix(s.id, "rec_"), "_", "-"))
		provID := "prov_" + strings.TrimPrefix(s.id, "rec_")
		status, sens := s.status, s.sensitivity
		if status == "" {
			status = contracts.StatusActive
		}
		if sens == "" {
			sens = contracts.SensPublicGeneral
		}
		h := sha256.Sum256([]byte(s.text))
		recs = append(recs, contracts.Record{
			SchemaVersion: contracts.SchemaVersion, RecordID: s.id, SourceID: s.source, SourceRecordID: srcRecID,
			SourceRevision: "synthetic", DecisionKey: s.decisionKey, Kind: s.kind, Title: Marker(s.id),
			Statement: s.text, CompactText: s.text, Status: status, Confidence: s.confidence,
			Authority: contracts.AuthorityRecommended, SourceRole: s.role, Trust: s.trust, Sensitivity: sens,
			Scope:          contracts.Scope{WorkspaceIDs: s.workspaces, TaskKinds: s.kinds},
			ProvenanceRefs: []string{provID},
		})
		provs = append(provs, contracts.Provenance{
			SchemaVersion: contracts.SchemaVersion, ProvenanceID: provID, SourceID: s.source, SourceRecordID: srcRecID,
			Locator: "entries/" + srcRecID + ".md", SourceRevision: "synthetic", ContentHash: "sha256:" + hex.EncodeToString(h[:]),
			CapturedAt: "2026-09-14T00:00:00Z", IngestionVersion: 1,
		})
	}
	return &Deployment{ConfigDir: cfg, AlphaPath: alpha, Runtime: rt, Records: recs, provs: provs}, nil
}

// Seed writes the synthetic records into an open projection store and
// applies the fixture's durable revocation (through the app API, so the
// tombstone ledger is written too).
//
// The caller opens the store from its own _test.go file: ADR-027's
// read-surface guard (TestReadSurfacesUseLedgerFilter) keeps storage.Open
// out of non-test code outside internal/app, storage, projection, resolver,
// and privacycorpus, and this fixture is not one of those.
func (d *Deployment) Seed(st *storage.Store) error {
	if err := st.PutRecords(d.Records, d.provs); err != nil {
		return err
	}
	return d.Runtime.Forget(Profile, RecRevoked, "synthetic revocation")
}

// Inputs returns an evaluation inputs function over this deployment.
func (d *Deployment) Inputs(learned bool) func(contracts.Profile, string, string) (resolver.EvalInputs, error) {
	return func(p contracts.Profile, task, ws string) (resolver.EvalInputs, error) {
		return d.Runtime.EvalInputs(p, CapabilityID, learned, task, ws)
	}
}

// Refs maps visible record IDs to their source record IDs.
func (d *Deployment) Refs() func(contracts.Profile, string) []string {
	return func(p contracts.Profile, recordID string) []string {
		sess, err := d.Runtime.Serve(p, CapabilityID, false)
		if err != nil {
			return nil
		}
		defer sess.Store.Close()
		recs, err := sess.VisibleRecords()
		if err != nil {
			return nil
		}
		for _, r := range recs {
			if r.RecordID == recordID {
				return []string{r.SourceRecordID}
			}
		}
		return nil
	}
}

// WriteCorpus writes the synthetic golden case into dir, with workspaceHint
// as the scenario workspace (use Deployment.AlphaPath).
func WriteCorpus(dir, workspaceHint string, repeats int) error {
	c := map[string]any{
		"id": CaseID, "schema_version": "1", "status": "locked", "gold_type": "historical_truth",
		"category": "architecture", "risk": "medium", "capability": "personal", "harness": nil,
		"scenario": map[string]any{
			"task": Task, "workspace": workspaceHint, "repository_fixture": "fix-synth-arms",
			"excluded_information": []string{"gold_answer"},
		},
		"gold": map[string]any{
			"acceptable_decisions":   []string{AcceptableDecision},
			"unacceptable_decisions": []string{"client_server_db_by_convention"},
			"mandatory_conclusions":  []string{MandatoryConclusion},
			"prohibited_conclusions": []string{ProhibitedPhrase},
			"expected_unknowns":      []string{"preferred SQL dialect"},
			"required_evidence_refs": []string{"SYN-DECISION-EMBEDDED", "SYN-FOUNDATION"},
		},
		"provenance": map[string]any{"approved_by_user": true, "evidence_refs": []string{"synthetic-only"}, "valid_at": "2026-09-14"},
		"grading":    map[string]any{"rubric": "0-4 blind paired", "repeats": repeats},
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "arms-case.json"), b, 0o600)
}
