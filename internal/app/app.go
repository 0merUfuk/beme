// Package app wires config, sources, projections, and resolution into the
// runtime the CLI and MCP server drive. It owns no harness specifics.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/ingestion"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/projection"
	"github.com/0merUfuk/beme/internal/resolver"
	"github.com/0merUfuk/beme/internal/storage"
	"github.com/0merUfuk/beme/internal/workspace"
	"gopkg.in/yaml.v3"
)

// Config is the deployment configuration (FR-062/063: platform dirs,
// configurable paths; no hard-coded ~/.beme).
type Config struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version"`
	// CanonicalRoot is where trusted registration lives (operator-owned).
	CanonicalRoot string `yaml:"canonical_root" json:"canonical_root"`
	// DataDir hosts projections; CacheDir hosts indexes/packs (separable).
	DataDir  string `yaml:"data_dir" json:"data_dir"`
	CacheDir string `yaml:"cache_dir" json:"cache_dir"`
}

// Runtime is the assembled application.
type Runtime struct {
	Config   Config
	Registry *workspace.Registry
	Sources  []contracts.SourceDescriptor
}

// DefaultDirs resolves platform-appropriate directories (NFR-007, FR-062):
//   - macOS: ~/Library/Application Support/beme (config+data), ~/Library/Caches/beme
//   - Windows: %AppData%\beme (config+data), %LocalAppData%\beme (cache)
//   - Linux/other: XDG — $XDG_CONFIG_HOME/beme, $XDG_DATA_HOME/beme,
//     $XDG_CACHE_HOME/beme (defaults ~/.config, ~/.local/share, ~/.cache)
//
// All overridable via env: BEME_CONFIG_HOME, BEME_DATA_HOME, BEME_CACHE_HOME.
func DefaultDirs() (configHome, dataHome, cacheHome string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", err
	}
	envOr := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	switch runtime.GOOS {
	case "darwin":
		configHome = filepath.Join(home, "Library", "Application Support", "beme")
		dataHome = configHome
		cacheHome = filepath.Join(home, "Library", "Caches", "beme")
	case "windows":
		configHome = filepath.Join(envOr("APPDATA", filepath.Join(home, "AppData", "Roaming")), "beme")
		dataHome = configHome
		cacheHome = filepath.Join(envOr("LOCALAPPDATA", filepath.Join(home, "AppData", "Local")), "beme")
	default:
		configHome = filepath.Join(envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config")), "beme")
		dataHome = filepath.Join(envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share")), "beme")
		cacheHome = filepath.Join(envOr("XDG_CACHE_HOME", filepath.Join(home, ".cache")), "beme")
	}
	configHome = envOr("BEME_CONFIG_HOME", configHome)
	dataHome = envOr("BEME_DATA_HOME", dataHome)
	cacheHome = envOr("BEME_CACHE_HOME", cacheHome)
	return configHome, dataHome, cacheHome, nil
}

// Load reads deployment config + registrations. Missing config = empty
// runtime (degrades honestly, never invents sources).
func Load(configDirOverride string) (*Runtime, error) {
	configDir := configDirOverride
	if configDir == "" {
		var err error
		configDir, _, _, err = DefaultDirs()
		if err != nil {
			return nil, err
		}
	}
	rt := &Runtime{Config: Config{SchemaVersion: contracts.SchemaVersion}}

	cfgPath := filepath.Join(configDir, "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := yaml.Unmarshal(data, &rt.Config); err != nil {
			return nil, fmt.Errorf("config.yaml: %w", err)
		}
	}
	if rt.Config.CanonicalRoot == "" {
		rt.Config.CanonicalRoot = configDir
	}
	if rt.Config.DataDir == "" {
		// Isolation rule (ADR-026): an explicit config dir is a self-contained
		// deployment root — derived data defaults INSIDE it, never to the
		// operator's real data home. The user-home default applies only when
		// Load() is called with no override (the real CLI deployment case).
		if configDirOverride != "" {
			rt.Config.DataDir = filepath.Join(configDir, "data")
		} else {
			_, dataHome, _, err := DefaultDirs()
			if err != nil {
				return nil, err
			}
			rt.Config.DataDir = dataHome
		}
	}
	if rt.Config.CacheDir == "" {
		if configDirOverride != "" {
			rt.Config.CacheDir = filepath.Join(configDir, "cache")
		} else {
			_, _, cacheHome, err := DefaultDirs()
			if err != nil {
				return nil, err
			}
			rt.Config.CacheDir = cacheHome
		}
	}

	// Sources: trusted registration only (FR-020).
	srcDir := filepath.Join(rt.Config.CanonicalRoot, "sources")
	entries, _ := os.ReadDir(srcDir)
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".json")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return nil, err
		}
		var sd contracts.SourceDescriptor
		if strings.HasSuffix(e.Name(), ".json") {
			err = json.Unmarshal(data, &sd)
		} else {
			err = yaml.Unmarshal(data, &sd)
		}
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", e.Name(), err)
		}
		if sd.SchemaVersion == "" {
			sd.SchemaVersion = contracts.SchemaVersion
		}
		sd.Registered = true // registration IS the trust act
		rt.Sources = append(rt.Sources, sd)
	}

	// Workspace registry.
	reg, err := workspace.LoadRegistry(filepath.Join(rt.Config.CanonicalRoot, "policies"))
	if err != nil {
		return nil, err
	}
	rt.Registry = reg
	return rt, nil
}

// ProjectionPath returns the store path for a profile (separate files,
// ADR-005).
func (rt *Runtime) ProjectionPath(profile contracts.Profile) string {
	return filepath.Join(rt.Config.DataDir, "projections", string(profile), "store.db")
}

// BuildProfile ingests all registered sources eligible for the profile and
// (re)builds its projection store. The work-safe builder reads ONLY
// approved safe-manifest sources (FR-012, ADR-018): this is construction,
// not filtering.
//
// Corruption recovery (FR-065): a projection store is derived data — if it
// exists but cannot be opened (corruption), it is deleted and recreated from
// the registered sources. Canonical sources are never touched.
func (rt *Runtime) BuildProfile(profile contracts.Profile) (*BuildReport, error) {
	// Serialized with forget, purge, and learning writes: a build never
	// re-ingests content a concurrent purge is erasing (ADR-030).
	lk, err := rt.lock()
	if err != nil {
		return nil, err
	}
	defer lk.Release()
	storePath := rt.ProjectionPath(profile)

	// Detect an unusable existing store and recreate it (recovery path).
	if _, err := os.Stat(storePath); err == nil {
		if probe, perr := storage.Open(storePath); perr != nil {
			// Unusable derived store: safe to remove and rebuild.
			_ = probe
			if err := os.Remove(storePath); err != nil {
				return nil, fmt.Errorf("corrupt store could not be removed for rebuild: %w", err)
			}
			// WAL side files may also exist; remove them too (derived data).
			os.Remove(storePath + "-wal")
			os.Remove(storePath + "-shm")
		} else {
			probe.Close()
		}
	}

	// Durable tombstones (ADR-027): purged content is never re-ingested,
	// whatever the source, sync, or backup state. Unreadable ledger → fail
	// closed.
	ledger, err := rt.LoadLedger()
	if err != nil {
		return nil, err
	}
	erased, err := rt.eraseHiddenObservations(ledger)
	if err != nil {
		return nil, err
	}

	store, err := storage.Open(storePath)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	if err := store.Wipe(); err != nil {
		return nil, err
	}

	safeMode := profile == contracts.ProfileWorkSafe
	builder := &projection.Builder{Store: store, SafeManifestMode: safeMode}
	walker := ingestion.NewWalker(ingestion.DefaultLimits())
	report := &BuildReport{Profile: string(profile), PurgedObservationsErased: erased}
	ingestedIDs := []string{}

	for _, sd := range rt.Sources {
		// Profile eligibility from the trusted descriptor.
		eligible := false
		for _, p := range sd.ProfilesAllowed {
			if p == profile {
				eligible = true
			}
		}
		if !eligible {
			report.Skipped = append(report.Skipped, sd.SourceID+": profile not allowed")
			continue
		}
		if sd.Type != "git_repository" && sd.Type != "directory" {
			report.Skipped = append(report.Skipped, sd.SourceID+": non-filesystem source (contract only in v1)")
			continue
		}
		files, err := walker.Walk(sd.Root, sd.Include, sd.Exclude)
		if err != nil {
			report.Skipped = append(report.Skipped, sd.SourceID+": "+err.Error())
			continue
		}
		recs := []contracts.Record{}
		provs := []contracts.Provenance{}
		for _, f := range files {
			if err := ingestion.SecretScan(f.Content); err != nil {
				report.SecretRejected = append(report.SecretRejected, f.RelPath+" ("+sd.SourceID+")")
				continue
			}
			fm, body := ingestion.ParseMarkdown(f.Content)
			if len(fm) == 0 {
				continue // not an entry-shaped document
			}
			entry := ingestion.MarkdownEntry{Frontmatter: fm, Body: body, RelPath: f.RelPath, Hash: f.Hash}
			rec, ok := ingestion.NormalizeEntry(sd, entry)
			if !ok {
				continue
			}
			if ledger.Purged(rec) {
				report.PurgeBlocked++
				continue
			}
			prov := contracts.Provenance{
				ProvenanceID:     "prov_" + rec.RecordID[len("rec_"):],
				SourceID:         sd.SourceID,
				SourceRecordID:   rec.SourceRecordID,
				Locator:          f.RelPath,
				SourceRevision:   rec.SourceRevision,
				ContentHash:      f.Hash,
				CapturedAt:       nowUTC(),
				IngestionVersion: 1,
			}
			rec.ProvenanceRefs = []string{prov.ProvenanceID}
			recs = append(recs, rec)
			provs = append(provs, prov)
		}
		if err := builder.IngestSource(sd, recs, provs); err != nil {
			return nil, fmt.Errorf("ingest %s: %w", sd.SourceID, err)
		}
		report.RecordsIngested += len(recs)
		ingestedIDs = append(ingestedIDs, sd.SourceID)
	}
	if err := builder.Finalize(ingestedIDs); err != nil {
		return nil, err
	}
	// A fresh generation per build invalidates ContextPacks issued against
	// earlier projection contents (pack-bound expansion, ADR-029).
	if err := store.SetMeta("build_generation", newOpaqueToken()); err != nil {
		return nil, err
	}
	report.SourcesIngested = ingestedIDs
	return report, nil
}

type BuildReport struct {
	Profile         string   `json:"profile"`
	RecordsIngested int      `json:"records_ingested"`
	SourcesIngested []string `json:"sources_ingested"`
	Skipped         []string `json:"skipped,omitempty"`
	SecretRejected  []string `json:"secret_rejected,omitempty"`
	// PurgeBlocked counts entries refused because they match a physical-purge
	// tombstone (anti-resurrection, ADR-027).
	PurgeBlocked int `json:"purge_blocked,omitempty"`
	// PurgedObservationsErased counts restored copies of purged observation
	// files the build erased.
	PurgedObservationsErased int `json:"purged_observations_erased,omitempty"`
}

// Serve opens a projection store read-only and binds one immutable
// capability (FR-010). The process never sees another profile's store.
func (rt *Runtime) Serve(profile contracts.Profile, capabilityID string, experimentalLearned bool) (*Session, error) {
	if profile != contracts.ProfilePersonal && profile != contracts.ProfileWorkSafe {
		return nil, fmt.Errorf("invalid profile %q", profile)
	}
	store, err := storage.Open(rt.ProjectionPath(profile))
	if err != nil {
		return nil, err
	}
	view := projection.NewView(store, capabilityID, profile)
	cap := policy.Capability{CapabilityID: capabilityID, Profile: profile, ExperimentalLearnedGuidance: experimentalLearned && profile == contracts.ProfilePersonal}
	sess := &Session{View: view, Capability: cap, Runtime: rt, Store: store, issued: &packRegistry{}}
	// Freshness honesty (§13.7). The notice never says why a rebuild is
	// needed, so it cannot reveal that suppressed (purged) records sit in a
	// restored store; doctor gives the operator the specifics.
	if _, restored, err := rt.EffectiveRevoked(profile, store); err == nil && restored {
		sess.degradations = append(sess.degradations, resolver.DegradationInput{
			Kind:   "stale_projection",
			Detail: "projection is out of date; run `beme build --profile " + string(profile) + "` before relying on results",
		})
	}
	if store.Count() == 0 {
		sess.degradations = append(sess.degradations, resolver.DegradationInput{
			Kind:   "stale_projection",
			Detail: "projection store is empty; run `beme build --profile " + string(profile) + "` before relying on results",
		})
	}
	return sess, nil
}

// Session is one bound serving context.
type Session struct {
	View       *projection.View
	Capability policy.Capability
	Runtime    *Runtime
	Store      *storage.Store
	// PackTTL bounds how long an issued ContextPack authorizes expansion
	// (DefaultPackTTL when zero). Now overrides the clock; both are
	// verification hooks.
	PackTTL time.Duration
	Now     func() time.Time

	// degradations carry honesty notices surfaced into every resolved pack.
	degradations []resolver.DegradationInput
	issued       *packRegistry
}

// Resolve runs the two-stage pipeline for one request.
func (s *Session) Resolve(req contracts.ResolutionRequest) (resolver.Pack, []resolver.TraceStep, error) {
	// Task-origin rule (ADR-016): without a trusted integrity binding, task
	// text is a retrieval hint only — enforced downstream by precedence
	// tiers, recorded here.
	tc := policy.TaskContext{
		Task:      req.Task,
		TaskKinds: clampKinds(req.TaskKindHints),
	}
	// Workspace identity: untrusted hint -> trusted registry; ambiguity
	// fails closed (FR-014).
	if req.WorkspaceHint != "" {
		m := s.Runtime.Registry.Resolve(req.WorkspaceHint)
		if m.Ambiguous {
			return resolver.Pack{}, nil, fmt.Errorf("workspace ambiguous: %s", m.Reason)
		}
		tc.WorkspaceID = m.WorkspaceID
	}
	if req.RiskHint == "low" || req.RiskHint == "medium" || req.RiskHint == "high" {
		tc.Risk = req.RiskHint
	}
	// Store tombstones + durable ledger (ADR-027); an unreadable ledger
	// fails closed for personalization.
	revoked, _, err := s.Runtime.EffectiveRevoked(s.Capability.Profile, s.Store)
	if err != nil {
		return resolver.Pack{}, nil, err
	}
	opts := resolver.Options{
		Policy:       policy.NewEngine(timeNowUTC()),
		Revoked:      revoked,
		Now:          timeNowUTC(),
		Degradations: s.degradations,
	}
	pack, trace := resolver.Resolve(s.View, s.Capability, tc, req, opts)
	s.registerPack(pack, tc)
	return pack, trace, nil
}

func clampKinds(hints []string) []string {
	out := []string{}
	for _, h := range hints {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			continue
		}
		if len(h) > 64 {
			h = h[:64]
		}
		out = append(out, h)
	}
	return out
}

// ResolveOnly resolves a pack for a task without CLI output/trace-persistence
// side effects — the interface evaluation harnesses use.
func (s *Session) ResolveOnly(task, workspaceHint string) (resolver.Pack, error) {
	req := contracts.ResolutionRequest{
		SchemaVersion: contracts.SchemaVersion,
		Task:          task,
		WorkspaceHint: workspaceHint,
	}
	pack, _, err := s.Resolve(req)
	return pack, err
}
