package projection

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
)

// ContentDigest hashes deterministic JSON, with a domain separator. Callers
// canonicalize set-valued slices before hashing; JSON map keys are ordered by
// encoding/json. No capture times or random build generations enter a digest.
func ContentDigest(domain string, value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(append([]byte(domain+"\n"), b...))
	return "sha256:" + hex.EncodeToString(h[:]), nil
}

func sortedStrings(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func canonicalRecord(r contracts.Record) contracts.Record {
	r.Scope.Profiles = sortedStrings(r.Scope.Profiles)
	r.Scope.WorkspaceIDs = sortedStrings(r.Scope.WorkspaceIDs)
	r.Scope.PathGlobs = sortedStrings(r.Scope.PathGlobs)
	r.Scope.TaskKinds = sortedStrings(r.Scope.TaskKinds)
	r.Scope.LifecyclePhases = sortedStrings(r.Scope.LifecyclePhases)
	r.Scope.Technologies = sortedStrings(r.Scope.Technologies)
	r.Scope.Harnesses = sortedStrings(r.Scope.Harnesses)
	r.Scope.RiskLevels = sortedStrings(r.Scope.RiskLevels)
	r.Scope.Environments = sortedStrings(r.Scope.Environments)
	r.Scope.Exclude = sortedStrings(r.Scope.Exclude)
	r.Relationships.Supersedes = sortedStrings(r.Relationships.Supersedes)
	r.Relationships.Challenges = sortedStrings(r.Relationships.Challenges)
	r.Relationships.ConflictsWith = sortedStrings(r.Relationships.ConflictsWith)
	r.Relationships.Refines = sortedStrings(r.Relationships.Refines)
	r.Relationships.DerivedFrom = sortedStrings(r.Relationships.DerivedFrom)
	r.ProvenanceRefs = sortedStrings(r.ProvenanceRefs)
	return r
}

// finalizeSnapshot binds the actual normalized snapshot, source-file bytes
// recorded in provenance, and trusted registration policy independently.
func (b *Builder) finalizeSnapshot(sourceIDs []string) error {
	recs, err := b.Store.AllRecords()
	if err != nil {
		return err
	}
	for i := range recs {
		recs[i] = canonicalRecord(recs[i])
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].RecordID < recs[j].RecordID })
	index, err := ContentDigest("normalized-index-v2", recs)
	if err != nil {
		return err
	}
	provs := map[string]contracts.Provenance{}
	for _, rec := range recs {
		for _, ref := range rec.ProvenanceRefs {
			if p, ok := b.Store.Provenance(ref); ok {
				p.CapturedAt = ""
				provs[ref] = p
			}
		}
	}
	sources, err := ContentDigest("source-snapshot-v2", struct {
		Sources    []string
		Provenance map[string]contracts.Provenance
	}{sortedStrings(sourceIDs), provs})
	if err != nil {
		return err
	}
	descriptors := b.PolicySources
	if descriptors == nil {
		descriptors = b.ingestedSources
	}
	// Registration flags are operational policy despite their json:"-" tags.
	type registeredPolicy struct {
		Descriptor contracts.SourceDescriptor
		Registered bool
		Revoked    bool
	}
	policies := make([]registeredPolicy, 0, len(descriptors))
	for _, sd := range descriptors {
		sd.Include = sortedStrings(sd.Include)
		sd.Exclude = sortedStrings(sd.Exclude)
		sd.ProfilesAllowed = append([]contracts.Profile{}, sd.ProfilesAllowed...)
		sort.Slice(sd.ProfilesAllowed, func(i, j int) bool { return sd.ProfilesAllowed[i] < sd.ProfilesAllowed[j] })
		policies = append(policies, registeredPolicy{sd, sd.Registered, sd.Revoked})
	}
	sort.Slice(policies, func(i, j int) bool { return policies[i].Descriptor.SourceID < policies[j].Descriptor.SourceID })
	policy, err := ContentDigest("registered-policy-v2", struct {
		SafeManifest bool
		Sources      []registeredPolicy
		Context      any
	}{b.SafeManifestMode, policies, b.PolicyContext})
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{{"index_revision", "idx_" + strings.TrimPrefix(index, "sha256:")}, {"source_revision_digest", sources}, {"policy_digest", policy}} {
		if err := b.Store.SetMeta(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}
