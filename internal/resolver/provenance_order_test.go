package resolver

import (
	"encoding/json"
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
)

func TestWorkSafeProvenanceOrderUsesOnlyVisibleTuple(t *testing.T) {
	var want string
	for _, names := range [][]string{{"private-alpha", "private-beta"}, {"private-zulu", "private-alpha"}} {
		records := []scoredRecord{
			{rec: contracts.Record{SourceID: names[0], SourceRole: contracts.RoleDeclassifiedSafe}},
			{rec: contracts.Record{SourceID: names[1], SourceRole: contracts.RoleDeclassifiedSafe}},
			{rec: contracts.Record{SourceID: names[1], SourceRole: contracts.RoleDeclassifiedSafe}},
		}
		for i := 0; i < 200; i++ {
			got := provenanceManifest(records, policy.Capability{Profile: contracts.ProfileWorkSafe}, nil)
			for _, p := range got {
				if p.SourceID != nil || p.ContentHash != nil || p.SourceRevision != nil {
					t.Fatal("hidden identity emitted")
				}
			}
			b, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if want == "" {
				want = string(b)
			} else if string(b) != want {
				t.Fatalf("hidden identity/map order changed visible provenance: %s vs %s", b, want)
			}
		}
	}
}
