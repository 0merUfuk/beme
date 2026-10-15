package app_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
)

// TestMaintenanceOperationsDoNotLoseUpdates runs forgets, a purge, and a build
// concurrently. Each rewrites the ledger or derived state; without the
// maintenance lock, concurrent load-modify-save cycles drop each other's
// entries and a build can re-ingest what a purge is erasing.
func TestMaintenanceOperationsDoNotLoseUpdates(t *testing.T) {
	f := newPurgeFixture(t)
	const forgets = 16
	var wg sync.WaitGroup
	errs := make(chan error, forgets+2)
	for i := 0; i < forgets; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- f.rt.Forget(contracts.ProfilePersonal, fmt.Sprintf("rec_forget-%02d", i), "concurrent forget")
		}(i)
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := f.rt.PhysicalPurge(purgeReq("rec_can-001"))
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := f.rt.BuildProfile(contracts.ProfilePersonal)
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	l, err := f.rt.LoadLedger()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range l.Revocations {
		if strings.HasPrefix(r.Key, "rec_forget-") {
			n++
		}
	}
	if n != forgets {
		t.Fatalf("lost ledger updates: %d of %d forgets recorded", n, forgets)
	}
	if !l.PurgedKey("rec_can-001") {
		t.Fatal("lost ledger update: the purge fingerprint is missing")
	}
	if _, err := f.rt.BuildProfile(contracts.ProfilePersonal); err != nil {
		t.Fatal(err)
	}
	if hits := bytesUnder(t, f.rt.Config.DataDir, purgeCanary); len(hits) > 0 {
		t.Fatalf("purged content on disk after concurrent maintenance: %v", hits)
	}
	if text, _ := resolvedText(t, f.rt); strings.Contains(text, purgeCanary) {
		t.Fatal("purged content resolves after concurrent maintenance")
	}
}
