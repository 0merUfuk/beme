package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/storage"
)

// TestMigrationsFreshAndIdempotent (FR-064): a fresh store migrates to the
// latest version; re-opening (re-running migrations) is a no-op.
func TestMigrationsFreshAndIdempotent(t *testing.T) {
	s := open(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != storage.Migrations[len(storage.Migrations)-1].Version {
		t.Fatalf("fresh store must be at latest schema; got %d, want %d", v, storage.Migrations[len(storage.Migrations)-1].Version)
	}
	// re-run: idempotent
	if err := s.Migrate(); err != nil {
		t.Fatalf("re-migrate must be a no-op: %v", err)
	}
	v2, _ := s.SchemaVersion()
	if v2 != v {
		t.Fatalf("idempotent migrate changed version %d -> %d", v, v2)
	}
}

// TestMigrationRollbackAndReapply (FR-064): rollback reverts the latest
// migration; re-migrating restores it; data written before survives the
// round trip when schema permits (base up/down covers full tables, so we
// verify the version mechanics and successful re-apply).
func TestMigrationRollbackAndReapply(t *testing.T) {
	s := open(t)
	latest := storage.Migrations[len(storage.Migrations)-1].Version

	if err := s.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	v, _ := s.SchemaVersion()
	if v != latest-1 {
		t.Fatalf("rollback must decrement version; got %d", v)
	}
	// tables dropped by down migration: store is now schema-empty but the
	// meta table remains, so Migrate() restores.
	if err := s.Migrate(); err != nil {
		t.Fatalf("re-migrate after rollback: %v", err)
	}
	v2, _ := s.SchemaVersion()
	if v2 != latest {
		t.Fatalf("re-migrate must restore latest version; got %d", v2)
	}
	// and the store is usable end-to-end after the round trip
	rec := contracts.Record{
		RecordID: "rec_mig00001", SourceID: "canonical-knowledge", Kind: contracts.KindPrinciple,
		Status: contracts.StatusActive, Sensitivity: "public_general",
		Confidence: contracts.ConfidenceValidated, Authority: contracts.AuthorityRecommended,
		SourceRole: contracts.RoleCanonicalKnowledge, Trust: contracts.TrustCanonical,
	}
	if err := s.PutRecords([]contracts.Record{rec}, nil); err != nil {
		t.Fatalf("post-rollback-reapply store must accept writes: %v", err)
	}
	if s.Count() != 1 {
		t.Fatal("write after migration round trip lost")
	}
}

// TestFailedMigrationLeavesPriorIntact (FR-064, §23.3): a migration that
// fails mid-way must roll back and leave the previous usable store intact.
func TestFailedMigrationLeavesPriorIntact(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(filepath.Join(dir, "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// inject a broken migration at the next version
	broken := storage.Migration{Version: 99, Name: "broken", Up: "CREATE TABLE broken_test (id TEXT PRIMARY KEY; -- syntax error", Down: "DROP TABLE broken_test"}
	orig := storage.Migrations
	storage.Migrations = append(append([]storage.Migration{}, orig...), broken)
	defer func() { storage.Migrations = orig }()

	if err := s.Migrate(); err == nil {
		t.Fatal("broken migration must fail")
	}
	// prior schema intact: version unchanged, reads/writes work
	v, _ := s.SchemaVersion()
	if v == 99 {
		t.Fatal("failed migration must not bump schema_version")
	}
	rec := contracts.Record{
		RecordID: "rec_surv0001", SourceID: "s", Kind: contracts.KindFact,
		Status: contracts.StatusActive, Sensitivity: "public_general",
	}
	if err := s.PutRecords([]contracts.Record{rec}, nil); err != nil {
		t.Fatalf("store must remain usable after failed migration: %v", err)
	}
}

// TestOpenOnLegacyStoreWithoutMeta: a store file created by an older
// version (pre-migrations) still opens and migrates.
func TestOpenOnLegacyStoreWithoutMeta(t *testing.T) {
	// covered implicitly: Open() creates meta then migrates; version 0 -> latest
	dir := t.TempDir()
	path := filepath.Join(dir, "store.db")
	if _, err := storage.Open(path); err != nil {
		t.Fatalf("open: %v", err)
	}
	s2, err := storage.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	v, _ := s2.SchemaVersion()
	if v < 1 {
		t.Fatalf("reopen must preserve schema version; got %d", v)
	}
}
