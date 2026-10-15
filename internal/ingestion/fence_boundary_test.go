package ingestion_test

import (
	"testing"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/ingestion"
)

// RF02: an example cannot end a surrounding fence with another marker,
// a shorter run, or a closing marker carrying trailing non-whitespace.
func TestMarkdownFenceBoundaries(t *testing.T) {
	descriptor := contracts.SourceDescriptor{SourceID: "synthetic", Trust: contracts.TrustCanonical, Purpose: []string{"reusable_knowledge"}, AuthorityCeiling: contracts.AuthorityRecommended, Sensitivity: contracts.SensPublicGeneral, ProfilesAllowed: []contracts.Profile{contracts.ProfilePersonal}}
	for _, tc := range []struct{ name, example string }{
		{"mixed_markers", "~~~markdown\n```\n## Summary\nWrong fenced example.\n~~~\n"},
		{"shorter_backticks", "````markdown\n```\n## Summary\nWrong fenced example.\n````\n"},
		{"shorter_tildes", "~~~~markdown\n~~~\n## Summary\nWrong fenced example.\n~~~~\n"},
		{"longer_closure", "```markdown\nWrong fenced example.\n`````\n"},
		{"trailing_text_is_not_closure", "~~~markdown\n~~~not-a-close\n## Summary\nWrong fenced example.\n~~~ \t\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.example + "\n## Summary\n\nActual storage decision.\n"
			record, ok := ingestion.NormalizeEntry(descriptor, ingestion.MarkdownEntry{Frontmatter: map[string]any{"id": "SYN-001", "type": "heuristic", "status": "active"}, Body: body})
			if !ok || record.Statement != "Actual storage decision." {
				t.Fatalf("fence boundary exposed example or hid decision: ok=%v statement=%q", ok, record.Statement)
			}
		})
	}
}
