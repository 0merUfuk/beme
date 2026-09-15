package app

// Pack-bound expansion (ADR-029). beme.get_context_item may expand a record
// only when the caller presents a ContextPack this same serving session
// issued, the record was selected into that pack, the pack has not expired,
// the projection has not been rebuilt or restored since, and the record is
// still visible and still passes Stage-A policy under the pack's context.
// Every refusal returns ErrItemUnavailable, so unknown, expired, replayed,
// unselected, denied, revoked, purged, and nonexistent look identical.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/policy"
	"github.com/0merUfuk/beme/internal/resolver"
)

// ErrItemUnavailable is the single refusal for context-item expansion.
var ErrItemUnavailable = errors.New("context item not available")

// DefaultPackTTL bounds how long an issued pack authorizes expansion.
const DefaultPackTTL = 30 * time.Minute

// maxIssuedPacks bounds the per-session registry (oldest evicted first).
const maxIssuedPacks = 256

type issuedPack struct {
	records    map[string]bool
	tc         policy.TaskContext
	generation string
	issuedAt   time.Time
}

type packRegistry struct {
	mu    sync.Mutex
	packs map[string]*issuedPack
	order []string
}

func newOpaqueToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func (s *Session) clock() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Session) registerPack(pack resolver.Pack, tc policy.TaskContext) {
	if pack.PackID == "" || s.issued == nil {
		return
	}
	ids := map[string]bool{}
	for _, section := range [][]resolver.ContextItem{pack.Constraints, pack.Guidance, pack.Precedents, pack.LearnedExperimental} {
		for _, it := range section {
			if it.RecordID != "" {
				ids[it.RecordID] = true
			}
		}
	}
	gen, _ := s.Store.GetMeta("build_generation")
	s.issued.mu.Lock()
	defer s.issued.mu.Unlock()
	if s.issued.packs == nil {
		s.issued.packs = map[string]*issuedPack{}
	}
	s.issued.packs[pack.PackID] = &issuedPack{records: ids, tc: tc, generation: gen, issuedAt: s.clock()}
	s.issued.order = append(s.issued.order, pack.PackID)
	for len(s.issued.order) > maxIssuedPacks {
		delete(s.issued.packs, s.issued.order[0])
		s.issued.order = s.issued.order[1:]
	}
}

// ExpandedItem is the expansion of one pack-authorized record. Source
// identity and provenance refs are included only under the personal
// capability (work-safe expansions omit them, FR-039).
type ExpandedItem struct {
	PackID         string   `json:"pack_id"`
	RecordID       string   `json:"record_id"`
	Kind           string   `json:"kind"`
	Title          string   `json:"title,omitempty"`
	Statement      string   `json:"statement"`
	Status         string   `json:"status"`
	Authority      string   `json:"authority"`
	Confidence     string   `json:"confidence,omitempty"`
	Sensitivity    string   `json:"sensitivity"`
	SourceID       string   `json:"source_id,omitempty"`
	SourceRecordID string   `json:"source_record_id,omitempty"`
	ProvenanceRefs []string `json:"provenance_refs,omitempty"`
}

// ExpandItem expands recordID under packID. Ledger failures are returned as
// ErrLedgerUnusable (independent of the requested record); every other
// refusal is ErrItemUnavailable.
func (s *Session) ExpandItem(packID, recordID string) (*ExpandedItem, error) {
	if packID == "" || recordID == "" || s.issued == nil {
		return nil, ErrItemUnavailable
	}
	ttl := s.PackTTL
	if ttl <= 0 {
		ttl = DefaultPackTTL
	}
	s.issued.mu.Lock()
	ip := s.issued.packs[packID]
	if ip != nil && s.clock().Sub(ip.issuedAt) > ttl {
		delete(s.issued.packs, packID)
		ip = nil
	}
	s.issued.mu.Unlock()
	if ip == nil || !ip.records[recordID] {
		return nil, ErrItemUnavailable
	}
	if gen, _ := s.Store.GetMeta("build_generation"); gen != ip.generation {
		return nil, ErrItemUnavailable
	}
	vis, err := s.Runtime.loadVisibility(s.Capability.Profile, s.Store)
	if err != nil {
		return nil, err
	}
	rec, ok, err := s.Store.RecordByID(recordID)
	if err != nil || !ok || !vis.visible(rec) {
		return nil, ErrItemUnavailable
	}
	if !policy.NewEngine(timeNowUTC()).Evaluate(rec, s.Capability, ip.tc, vis.revoked, s.Capability.ExperimentalLearnedGuidance).Eligible {
		return nil, ErrItemUnavailable
	}
	item := &ExpandedItem{
		PackID: packID, RecordID: rec.RecordID, Kind: string(rec.Kind), Title: rec.Title,
		Statement: rec.Statement, Status: string(rec.Status), Authority: string(rec.Authority),
		Confidence: string(rec.Confidence), Sensitivity: rec.Sensitivity,
	}
	if s.Capability.Profile == contracts.ProfilePersonal {
		item.SourceID, item.SourceRecordID, item.ProvenanceRefs = rec.SourceID, rec.SourceRecordID, rec.ProvenanceRefs
	}
	return item, nil
}
