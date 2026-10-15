package resolver_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/ingestion"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
)

type memoryStore struct{ records []contracts.Record }

func (s memoryStore) Records() []contracts.Record { return s.records }
func (s memoryStore) Provenance(string) (contracts.Provenance, bool) {
	return contracts.Provenance{}, false
}
func (s memoryStore) IndexRevision() string        { return "idx_synthetic" }
func (s memoryStore) PolicyDigest() string         { return "sha256:synthetic" }
func (s memoryStore) SourceRevisionDigest() string { return "sha256:synthetic" }

func TestParentMultiSourceProvenanceDeterminism(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	store := memoryStore{}
	for _, id := range []string{"alpha", "beta", "gamma"} {
		store.records = append(store.records, contracts.Record{
			SchemaVersion: "1", RecordID: "rec_" + id, SourceID: "synthetic-" + id,
			SourceRecordID: id, Kind: contracts.KindPrinciple, Status: contracts.StatusActive,
			Confidence: contracts.ConfidenceValidated, Authority: contracts.AuthorityRecommended,
			SourceRole: contracts.RoleCanonicalKnowledge, Trust: contracts.TrustCanonical,
			Sensitivity: contracts.SensPublicGeneral,
			Statement:   "Keep storage changes small.", CompactText: "Keep storage changes small.",
		})
	}
	req := contracts.ResolutionRequest{SchemaVersion: "1", Task: "storage architecture", BudgetHintTokens: 3000}
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		pack, _ := resolver.Resolve(store, policy.Capability{Profile: contracts.ProfilePersonal, CapabilityID: "synthetic"}, policy.TaskContext{Task: req.Task}, req, resolver.Options{Now: now, Policy: policy.NewEngine(now)})
		if len(pack.Provenance) != 3 {
			t.Fatalf("fixture did not deliver all sources: %+v", pack)
		}
		raw, err := json.Marshal(pack.Provenance)
		if err != nil {
			t.Fatal(err)
		}
		seen[string(raw)] = true
	}
	if len(seen) != 1 {
		t.Fatalf("unchanged inputs produced %d distinct provenance orders: %v", len(seen), seen)
	}
}

func TestParentMarkdownMixedFencesDoNotReplaceSummary(t *testing.T) {
	descriptor := contracts.SourceDescriptor{SourceID: "synthetic", Trust: contracts.TrustCanonical, Purpose: []string{"reusable_knowledge"}, AuthorityCeiling: contracts.AuthorityRecommended, Sensitivity: contracts.SensPublicGeneral, ProfilesAllowed: []contracts.Profile{contracts.ProfilePersonal}}
	body := "~~~markdown\n```\nPrefer remote-server storage; this is only a fenced example.\n~~~\n\n## Summary\n\nPrefer embedded storage for the actual local application.\n"
	record, ok := ingestion.NormalizeEntry(descriptor, ingestion.MarkdownEntry{Frontmatter: map[string]any{"id": "SYN-001", "type": "heuristic", "status": "active"}, Body: body})
	if !ok {
		t.Fatal("valid source unexpectedly rejected")
	}
	if record.Statement != "Prefer embedded storage for the actual local application." {
		t.Fatalf("fenced example replaced actual Summary: %q", record.Statement)
	}
}
