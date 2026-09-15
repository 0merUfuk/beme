package storage_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/storage"
)

func purgeRecord(id, sourceRecordID, text string, refs []string) contracts.Record {
	return contracts.Record{
		SchemaVersion: "1", RecordID: id, SourceID: "src", SourceRecordID: sourceRecordID,
		Kind: contracts.Kind("principle"), Title: text, Statement: text, CompactText: text,
		Status: contracts.Status("active"), Authority: contracts.Authority("default"),
		SourceRole: contracts.SourceRole("canonical_reusable_knowledge"), Trust: contracts.Trust("canonical"),
		Sensitivity: "personal_private", ProvenanceRefs: refs,
	}
}

func purgeProv(id, sourceRecordID, locator string) contracts.Provenance {
	return contracts.Provenance{ProvenanceID: id, SourceID: "src", SourceRecordID: sourceRecordID, Locator: locator,
		ContentHash: "sha256:" + locator, CapturedAt: "2026-09-15T00:00:00Z", IngestionVersion: 1}
}

// TestPurgeRecordsUsesPayloadProvenanceRefs pins that purge removes exactly
// the provenance a record owns — multiple and nonconventional IDs from its
// payload, plus rows matching its source identity — and never derives an ID
// from the "prov_"+suffix convention (which here belongs to another record).
func TestPurgeRecordsUsesPayloadProvenanceRefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	st, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	target := purgeRecord("rec_target", "T-1", "unique purge target statement", []string{"custom:ref/alpha", "p-β-002"})
	other := purgeRecord("rec_other", "O-1", "unrelated kept statement", []string{"prov_target"})
	provs := []contracts.Provenance{
		purgeProv("custom:ref/alpha", "T-1", "loc-alpha-unique.md"),
		purgeProv("p-β-002", "LEGACY-7", "loc-beta-unique.md"), // reachable only through the payload refs
		purgeProv("late-ref-9", "T-1", "loc-orphan-unique.md"), // owned by source identity, missing from payload
		purgeProv("prov_target", "O-1", "loc-other-keep.md"),   // convention-looking ID owned by rec_other
	}
	if err := st.PutRecords([]contracts.Record{target, other}, provs); err != nil {
		t.Fatal(err)
	}

	n, err := st.PurgeRecords([]storage.PurgeTarget{{RecordID: "rec_target"}})
	if err != nil || n != 1 {
		t.Fatalf("purge: n=%d err=%v", n, err)
	}
	for _, id := range []string{"custom:ref/alpha", "p-β-002", "late-ref-9"} {
		if _, ok := st.Provenance(id); ok {
			t.Fatalf("provenance %q owned by the purged record must be removed", id)
		}
	}
	if _, ok := st.Provenance("prov_target"); !ok {
		t.Fatal("convention-derived ID belonging to another record must be kept")
	}
	if !st.HasRecord("rec_other") || st.HasRecord("rec_target") {
		t.Fatal("only the target record may be removed")
	}

	// Idempotent, and a resumed purge removes plan-carried refs whose record
	// row is already gone.
	if err := st.PutRecords(nil, []contracts.Provenance{purgeProv("resume-only-ref", "GONE-1", "loc-resume-unique.md")}); err != nil {
		t.Fatal(err)
	}
	n, err = st.PurgeRecords([]storage.PurgeTarget{
		{RecordID: "rec_target"},
		{RecordID: "rec_gone", ProvenanceRefs: []string{"resume-only-ref"}},
	})
	if err != nil || n != 0 {
		t.Fatalf("second purge must be a no-op on rows: n=%d err=%v", n, err)
	}
	if _, ok := st.Provenance("resume-only-ref"); ok {
		t.Fatal("plan-carried provenance ref must be removed on resume")
	}

	if err := st.Compact(); err != nil {
		t.Fatal(err)
	}
	st.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(path + suffix)
		if err != nil {
			continue
		}
		for _, needle := range []string{"unique purge target statement", "loc-alpha-unique", "loc-beta-unique", "loc-orphan-unique", "loc-resume-unique"} {
			if bytes.Contains(data, []byte(needle)) {
				t.Fatalf("%q survives on disk in store%s", needle, suffix)
			}
		}
	}
}
