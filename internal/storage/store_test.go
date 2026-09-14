package storage_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/storage"
)

func open(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutAndFetchRecords(t *testing.T) {
	s := open(t)
	rec := contracts.Record{
		RecordID: "rec_t0000001", SourceID: "canonical-knowledge", SourceRecordID: "KP-001",
		Kind: contracts.KindPrinciple, Status: contracts.StatusActive,
		Confidence: contracts.ConfidenceValidated, Authority: contracts.AuthorityDefault,
		SourceRole: contracts.RoleCanonicalKnowledge, Trust: contracts.TrustCanonical,
		Sensitivity: "public_general", CompactText: "x",
	}
	prov := contracts.Provenance{ProvenanceID: "prov_t0000001", SourceID: "canonical-knowledge", SourceRecordID: "KP-001", Locator: "a.md", ContentHash: "sha256:" + h64("1"), CapturedAt: "2026-09-14T00:00:00Z", IngestionVersion: 1}
	if err := s.PutRecords([]contracts.Record{rec}, []contracts.Provenance{prov}); err != nil {
		t.Fatal(err)
	}
	got := s.Records()
	if len(got) != 1 || got[0].RecordID != "rec_t0000001" {
		t.Fatalf("roundtrip failed: %+v", got)
	}
	if _, ok := s.Provenance("prov_t0000001"); !ok {
		t.Fatal("provenance missing")
	}
	if s.Count() != 1 {
		t.Fatal("count mismatch")
	}
}

func h64(seed string) string {
	out := []byte{}
	for i := 0; i < 64; i++ {
		out = append(out, seed[0])
	}
	return string(out)
}

func TestFTSRoundtrip(t *testing.T) {
	s := open(t)
	recs := []contracts.Record{
		{RecordID: "rec_f0000001", SourceID: "kh", Kind: contracts.KindPrinciple, Status: contracts.StatusActive, Title: "Evidence beats prose", Statement: "verify before claiming completion", Sensitivity: "public_general"},
		{RecordID: "rec_f0000002", SourceID: "kh", Kind: contracts.KindHeuristic, Status: contracts.StatusActive, Title: "Boring components", Statement: "prefer demonstrated boring components", Sensitivity: "public_general"},
	}
	for i := range recs {
		recs[i].Confidence = contracts.ConfidenceValidated
		recs[i].Authority = contracts.AuthorityRecommended
		recs[i].SourceRole = contracts.RoleCanonicalKnowledge
		recs[i].Trust = contracts.TrustCanonical
	}
	if err := s.PutRecords(recs, nil); err != nil {
		t.Fatal(err)
	}
	hits := s.SearchFTS("evidence completion verify", 10)
	if len(hits) == 0 {
		t.Fatal("FTS must find the evidence entry")
	}
	if hits[0] != "rec_f0000001" {
		t.Fatalf("unexpected FTS result: %v", hits)
	}
}

func TestTombstoneBlocksAndPersists(t *testing.T) {
	s := open(t)
	if err := s.Tombstone("rec_x0000001", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.Tombstone("source:old-src", "revoked"); err != nil {
		t.Fatal(err)
	}
	set := s.RevokedSet()
	if !set["rec_x0000001"] || !set["source:old-src"] {
		t.Fatalf("tombstones must persist: %v", set)
	}
}

func TestWipeRebuildable(t *testing.T) {
	s := open(t)
	rec := contracts.Record{RecordID: "rec_w0000001", SourceID: "kh", Kind: contracts.KindFact, Status: contracts.StatusActive, Sensitivity: "public_general"}
	if err := s.PutRecords([]contracts.Record{rec}, nil); err != nil {
		t.Fatal(err)
	}
	if s.Count() != 1 {
		t.Fatal("setup failed")
	}
	if err := s.Wipe(); err != nil {
		t.Fatal(err)
	}
	if s.Count() != 0 {
		t.Fatal("wipe must empty the store (rebuildable, FR-004)")
	}
}

func TestRecordJSONShapeStable(t *testing.T) {
	rec := contracts.Record{RecordID: "rec_j0000001", Kind: contracts.KindPrinciple, Status: contracts.StatusActive, Confidence: contracts.ConfidenceValidated, Authority: contracts.AuthorityDefault}
	b, _ := json.Marshal(rec)
	for _, must := range []string{`"record_id"`, `"kind":"principle"`, `"status":"active"`, `"confidence":"validated"`, `"authority":"default"`} {
		if !containsStr(string(b), must) {
			t.Fatalf("stable JSON shape missing %s in %s", must, b)
		}
	}
}

func containsStr(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
