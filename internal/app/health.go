package app

import (
	"encoding/json"

	"github.com/0merUfuk/beme/internal/resolver"
)

// SessionHealth contains class-only agent-facing diagnostics. Source names,
// filesystem errors and counts of failed/denied sources stay operator-local.
// Read on every request so an already-running server observes rebuild recovery.
type SessionHealth struct {
	Status       string                      `json:"status"`
	RecordCount  int                         `json:"record_count"`
	Degradations []resolver.DegradationInput `json:"degradations"`
}

func (s *Session) Health() (SessionHealth, error) {
	h := SessionHealth{Status: "ok", Degradations: []resolver.DegradationInput{}}
	_, restored, err := s.Runtime.EffectiveRevoked(s.Capability.Profile, s.Store)
	if err != nil {
		return h, err
	}
	h.RecordCount, err = s.VisibleCount()
	if err != nil {
		return h, err
	}
	add := func(kind, detail string) {
		h.Degradations = append(h.Degradations, resolver.DegradationInput{Kind: kind, Detail: detail})
		h.Status = "degraded"
	}
	if restored {
		add("stale_projection", "projection is out of date; rebuild before relying on results")
	}
	raw, ok := s.Store.GetMeta(buildFailuresMetaKey)
	var failures []SourceFailure
	if !ok || json.Unmarshal([]byte(raw), &failures) != nil {
		add("stale_projection", "build health is unavailable; rebuild before relying on results")
	} else if len(failures) > 0 {
		add("source_unavailable", "registered source evidence was unavailable at the last build; absence of evidence is not a confirmed preference")
	}
	if h.RecordCount == 0 {
		add("unavailable", "projection has no available evidence; rebuild before relying on results")
		h.Status = "unavailable"
	}
	return h, nil
}
