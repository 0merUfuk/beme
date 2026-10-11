package app

import (
	"sort"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/projection"
)

// effectivePolicyDigest binds policy facts actually used by this session,
// not the human-chosen capability label. It is owner-side evidence: work-safe
// packs/status deliberately withhold correlatable fingerprints.
func (s *Session) effectivePolicyDigest(revoked map[string]bool) (string, error) {
	registration, _ := s.Store.GetMeta("policy_digest")
	keys := []string{}
	for k, v := range revoked {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return projection.ContentDigest("effective-stage-a-policy-v2", struct {
		Registration string
		Profile      contracts.Profile
		Learned      bool
		Revoked      []string
		Workspaces   map[string]contracts.Workspace
	}{registration, s.Capability.Profile, s.Capability.ExperimentalLearnedGuidance, keys, s.Runtime.Registry.Workspaces})
}
