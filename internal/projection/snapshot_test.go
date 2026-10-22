package projection_test

import (
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/projection"
	"github.com/0merUfuk/beme/internal/storage"
)

func TestSnapshotCanonicalOrderIgnoresCaptureTime(t *testing.T) {
	build := func(reverse bool) map[string]string {
		st, err := storage.Open(filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		b := projection.Builder{Store: st}
		sd := contracts.SourceDescriptor{SourceID: "synthetic", Registered: true, Trust: contracts.TrustCanonical, Sensitivity: contracts.SensPublicGeneral, AuthorityCeiling: contracts.AuthorityRecommended, ProfilesAllowed: []contracts.Profile{contracts.ProfilePersonal}, Include: []string{"a/**", "b/**"}}
		recs := []contracts.Record{{RecordID: "rec_a", SourceID: sd.SourceID, Statement: "Choose local storage.", ProvenanceRefs: []string{"prov_a"}, Scope: contracts.Scope{TaskKinds: []string{"architecture", "deployment"}}}, {RecordID: "rec_b", SourceID: sd.SourceID, Statement: "Check artifact evidence.", ProvenanceRefs: []string{"prov_b"}}}
		provs := []contracts.Provenance{{ProvenanceID: "prov_a", SourceID: sd.SourceID, ContentHash: "synthetic-byte-digest-a", CapturedAt: "2020-01-01T00:00:00Z"}, {ProvenanceID: "prov_b", SourceID: sd.SourceID, ContentHash: "synthetic-byte-digest-b", CapturedAt: "2020-01-01T00:00:00Z"}}
		if reverse {
			sd.Include = []string{"b/**", "a/**"}
			recs[0].Scope.TaskKinds = []string{"deployment", "architecture"}
			recs[0], recs[1] = recs[1], recs[0]
			provs[0], provs[1] = provs[1], provs[0]
			for i := range provs {
				provs[i].CapturedAt = "2026-10-01T00:00:00Z"
			}
		}
		if err = b.IngestSource(sd, recs, provs); err != nil {
			t.Fatal(err)
		}
		if err = b.Finalize([]string{sd.SourceID}); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, key := range []string{"index_revision", "source_revision_digest", "policy_digest"} {
			out[key], _ = st.GetMeta(key)
		}
		return out
	}
	a, b := build(false), build(true)
	for key, value := range a {
		if value == "" || b[key] != value {
			t.Errorf("%s depends on insertion order/capture time: %q vs %q", key, value, b[key])
		}
	}
}
