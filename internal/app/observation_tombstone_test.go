package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/learning"
)

// TestRestoredObservationBackupStaysHidden: purged observations restored from
// a pre-purge backup are invisible on every learning surface — list, list-all,
// inspect, review, family counts, feedback dedup, and the rejection tombstone
// index — and build erases them.
func TestRestoredObservationBackupStaysHidden(t *testing.T) {
	f := newPurgeFixture(t)
	sc := addDerivedCopies(t, f)
	if _, err := sc.obs.Review(sc.obsIDs[0], "reject", "op", "rejected before purge"); err != nil {
		t.Fatal(err)
	}
	obsDir := filepath.Join(f.rt.Config.DataDir, "observations")
	backup := t.TempDir()
	for _, id := range sc.obsIDs {
		copyFile(t, filepath.Join(obsDir, id+".json"), filepath.Join(backup, id+".json"))
	}
	if _, err := f.rt.PhysicalPurge(purgeReq("rec_can-001")); err != nil {
		t.Fatal(err)
	}
	for _, id := range sc.obsIDs {
		copyFile(t, filepath.Join(backup, id+".json"), filepath.Join(obsDir, id+".json"))
	}

	// Positive control: the restore really brought the files back.
	raw, err := learning.Open(f.rt.Config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range sc.obsIDs {
		if _, err := raw.Get(id); err != nil {
			t.Fatalf("fixture: restored observation %s must exist on disk: %v", id, err)
		}
	}

	store, err := f.rt.OpenLearning()
	if err != nil {
		t.Fatal(err)
	}
	purged := map[string]bool{sc.obsIDs[0]: true, sc.obsIDs[1]: true}
	listed := store.List("")
	all, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	for name, obs := range map[string][]learning.Observation{"list": listed, "list all": all} {
		keep := false
		for _, o := range obs {
			if purged[o.ObservationID] || strings.Contains(strings.ToLower(o.Hypothesis), purgeCanary) {
				t.Errorf("%s surfaces a purged observation", name)
			}
			keep = keep || o.ObservationID == sc.unrelatedObs
		}
		if !keep {
			t.Errorf("%s must keep the unrelated observation", name)
		}
	}
	for _, id := range sc.obsIDs {
		if _, err := store.Get(id); !errors.Is(err, learning.ErrObservationNotFound) {
			t.Errorf("inspect of a purged observation must be not-found; got %v", err)
		}
		if _, err := store.Review(id, "defer", "op", ""); !errors.Is(err, learning.ErrObservationNotFound) {
			t.Errorf("review of a purged observation must be not-found; got %v", err)
		}
	}
	counts := store.FamilyCounts()
	if counts["fam-a"] != 0 || counts["fam-b"] != 0 || counts["fam-keep"] != 1 {
		t.Errorf("family counts include purged observations: %v", counts)
	}
	again, err := store.Observe("observation", "Remember: "+strings.ToUpper(purgeCanary)+"!", "fam-b", "personal", "personal_private", "")
	if err != nil || again.ObservationID == sc.obsIDs[1] || again.FamilyCount != 1 {
		t.Errorf("feedback dedup must not merge into a purged observation: %+v %v", again, err)
	}
	if _, err := store.Observe("observation", "The owner has a "+purgeCanary, "fam-a", "personal", "personal_private", ""); err != nil {
		t.Errorf("a purged rejection must not reveal itself through the tombstone refusal: %v", err)
	}

	findings, err := f.rt.ObservationFindings()
	if err != nil || len(findings) != 1 || strings.Contains(findings[0], sc.obsIDs[0]) {
		t.Fatalf("doctor must report restored purged observations without IDs: %v %v", findings, err)
	}
	brep, err := f.rt.BuildProfile(contracts.ProfilePersonal)
	if err != nil {
		t.Fatal(err)
	}
	if brep.PurgedObservationsErased != 2 {
		t.Fatalf("build must erase both restored purged observations; %+v", brep)
	}
	for _, id := range sc.obsIDs {
		if _, err := os.Stat(filepath.Join(obsDir, id+".json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("restored purged observation %s survived build", id)
		}
	}
	if findings, err := f.rt.ObservationFindings(); err != nil || len(findings) != 0 {
		t.Fatalf("doctor finding must clear after build: %v %v", findings, err)
	}
}
