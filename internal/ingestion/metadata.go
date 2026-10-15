package ingestion

import (
	"sort"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
)

// applyMetadata accepts only applicability dimensions enforced by Stage A.
// Authority, source role, sensitivity, criticality and supersession cannot
// be self-assigned here. Unsupported scope dimensions fail closed rather
// than quietly publishing a globally applicable record.
func applyMetadata(rec *contracts.Record, sd contracts.SourceDescriptor, fm map[string]any) bool {
	for _, field := range []struct {
		key string
		dst *string
	}{{"key", &rec.Key}, {"decision_key", &rec.DecisionKey}} {
		if v, exists := fm[field.key]; exists {
			s, ok := v.(string)
			if !ok || len(s) > 256 {
				return false
			}
			*field.dst = strings.TrimSpace(s)
		}
	}
	for _, p := range sd.ProfilesAllowed {
		rec.Scope.Profiles = append(rec.Scope.Profiles, string(p))
	}
	if v, exists := fm["scope"]; exists {
		scope, ok := v.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range scope {
			list, ok := metadataStrings(v)
			if !ok {
				return false
			}
			switch k {
			case "profiles":
				if len(list) == 0 {
					continue
				}
				allowed := map[string]bool{}
				for _, p := range rec.Scope.Profiles {
					allowed[p] = true
				}
				narrowed := []string{}
				for _, p := range list {
					if allowed[p] {
						narrowed = append(narrowed, p)
					}
				}
				if len(narrowed) == 0 {
					return false
				}
				rec.Scope.Profiles = narrowed
			case "workspace_ids":
				rec.Scope.WorkspaceIDs = list
			case "task_kinds":
				rec.Scope.TaskKinds = list
			case "exclude":
				rec.Scope.Exclude = list
			default:
				if len(list) > 0 {
					return false
				}
			}
		}
	}
	if v, exists := fm["validity"]; exists {
		validity, ok := v.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range validity {
			var s string
			switch d := v.(type) {
			case string:
				s = d
			case time.Time:
				s = d.Format("2006-01-02")
			case nil:
				continue
			default:
				return false
			}
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return false
			}
			switch k {
			case "effective_from":
				rec.Validity.EffectiveFrom = s
			case "expires_at":
				rec.Validity.ExpiresAt = &s
			case "review_after":
				rec.Validity.ReviewAfter = &s
			default:
				return false
			}
		}
	}
	return true
}

func metadataStrings(v any) ([]string, bool) {
	values, ok := v.([]any)
	if !ok {
		return nil, false
	}
	if len(values) > 64 {
		return nil, false
	}
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		s = strings.TrimSpace(s)
		if s == "" || len(s) > 128 {
			return nil, false
		}
		if !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	sort.Strings(out)
	return out, true
}
