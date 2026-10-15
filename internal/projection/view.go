// Package projection adapts storage stores to the resolver's read
// interface, and builds projections from registered sources.
package projection

import (
	"fmt"
	"strings"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/storage"
)

// View is a read-only adapter over a projection store, bound to one
// capability. It implements resolver.Store.
type View struct {
	store        *storage.Store
	capabilityID string
	profile      contracts.Profile
}

func NewView(store *storage.Store, capabilityID string, profile contracts.Profile) *View {
	return &View{store: store, capabilityID: capabilityID, profile: profile}
}

func (v *View) Records() []contracts.Record                       { return v.store.Records() }
func (v *View) Provenance(id string) (contracts.Provenance, bool) { return v.store.Provenance(id) }

func (v *View) IndexRevision() string {
	if v.profile == contracts.ProfileWorkSafe {
		return "withheld"
	}
	if s, ok := v.store.GetMeta("index_revision"); ok {
		return s
	}
	return "unknown"
}

func (v *View) PolicyDigest() string {
	if v.profile == contracts.ProfileWorkSafe {
		return "withheld"
	}
	if s, ok := v.store.GetMeta("policy_digest"); ok {
		return s
	}
	return "unknown"
}

func (v *View) SourceRevisionDigest() string {
	if v.profile == contracts.ProfileWorkSafe {
		return ""
	}
	if s, ok := v.store.GetMeta("source_revision_digest"); ok {
		return s
	}
	return ""
}

// CapabilityID returns the bound capability.
func (v *View) CapabilityID() string { return v.capabilityID }

// Profile returns the bound profile.
func (v *View) Profile() contracts.Profile { return v.profile }

// Builder ingests registered sources into a projection store. The work-safe
// builder receives only sources whose descriptor was explicitly moved into
// the approved safe manifest (ADR-018) — construction enforces it.
type Builder struct {
	Store *storage.Store
	// SafeManifestMode: when true, only descriptors with
	// purpose safe_declassified are ingested.
	SafeManifestMode bool
	// PolicySources includes registered sources that failed ingestion too.
	// PolicyContext holds trusted workspace policy (owner-local only).
	PolicySources   []contracts.SourceDescriptor
	PolicyContext   any
	ingestedSources []contracts.SourceDescriptor
}

// IngestResult reports what a build produced.
type IngestResult struct {
	RecordsIngested int
	SourcesIngested []string
	Skipped         map[string]string // source_id -> reason
}

// IngestSource normalizes one registered source's content into the store.
// Content is data; authority comes only from the descriptor's trusted
// registration (ADR-006).
func (b *Builder) IngestSource(sd contracts.SourceDescriptor, recs []contracts.Record, provs []contracts.Provenance) error {
	if b.SafeManifestMode {
		ok := false
		for _, p := range sd.Purpose {
			if p == "safe_declassified" {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("work-safe builder: source %s not in approved safe manifest (no safe_declassified purpose)", sd.SourceID)
		}
	}
	// Clamp authority to the registered ceiling (content cannot raise it).
	for i := range recs {
		if authorityRank(recs[i].Authority) > authorityRank(sd.AuthorityCeiling) {
			recs[i].Authority = sd.AuthorityCeiling
		}
		// Sensitivity monotonicity: derived may retain or increase, never lower.
		if sensitivityRank(recs[i].Sensitivity) < sensitivityRank(sd.Sensitivity) {
			recs[i].Sensitivity = sd.Sensitivity
		}
		// Untrusted sources can never carry directive force.
		if sd.Trust == contracts.TrustUntrustedData {
			recs[i].Authority = contracts.AuthorityInformational
			recs[i].SourceRole = contracts.RoleEpisodicEvidence
		}
		if sd.InstructionSemantics == "data_only" {
			recs[i].Authority = contracts.AuthorityInformational
		}
		for pi := range provs {
			// locator sanity: never absolute machine paths in provenance
			if strings.HasPrefix(provs[pi].Locator, "/") {
				provs[pi].Locator = strings.TrimPrefix(provs[pi].Locator, "/")
			}
		}
	}
	b.ingestedSources = append(b.ingestedSources, sd)
	return b.Store.PutRecords(recs, provs)
}

func authorityRank(a contracts.Authority) int {
	switch a {
	case contracts.AuthorityDefault:
		return 2
	case contracts.AuthorityRecommended:
		return 1
	default:
		return 0
	}
}

func sensitivityRank(s string) int {
	switch {
	case strings.HasPrefix(s, contracts.SensWorkRestrictedPrefix):
		return 2
	case s == contracts.SensPersonalPrivate:
		return 2
	case s == contracts.SensPublicGeneral:
		return 1
	default:
		return 0
	}
}

// Finalize computes deterministic index metadata for the built store.
func (b *Builder) Finalize(sourceIDs []string) error {
	return b.finalizeSnapshot(sourceIDs)
}
