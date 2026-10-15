package ingestion_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/0merUfuk/beme/internal/contracts"
	"github.com/0merUfuk/beme/internal/ingestion"
)

func TestEntryMetadataOnlyNarrowsRegistration(t *testing.T) {
	sd := contracts.SourceDescriptor{SourceID: "synthetic", Trust: contracts.TrustCanonical, Purpose: []string{"reusable_knowledge"}, AuthorityCeiling: contracts.AuthorityRecommended, Sensitivity: contracts.SensPersonalPrivate, ProfilesAllowed: []contracts.Profile{contracts.ProfilePersonal}}
	parse := func(meta, body string) (contracts.Record, bool) {
		fm, b := ingestion.ParseMarkdown([]byte("---\nid: SYN-META\ntype: heuristic\n" + meta + "\n---\n" + body))
		return ingestion.NormalizeEntry(sd, ingestion.MarkdownEntry{Frontmatter: fm, Body: b})
	}
	t.Run("supported", func(t *testing.T) {
		r, ok := parse("key: storage.embedded\ndecision_key: storage.engine\nscope:\n  profiles: [personal, work-safe]\n  workspace_ids: [synthetic-alpha]\n  task_kinds:\n    - deployment\nvalidity:\n  effective_from: 2020-01-01\n  expires_at: 2099-01-01\nauthority: default\nsensitivity: public_general\nsource_role: trusted_project_policy\nrelationships:\n  supersedes: [rec_other]", "# Title\n\n## Decision\n\nDeploy with an embedded store.")
		if !ok || r.Key != "storage.embedded" || r.DecisionKey != "storage.engine" || len(r.Scope.Profiles) != 1 || r.Scope.Profiles[0] != "personal" || len(r.Scope.WorkspaceIDs) != 1 || len(r.Scope.TaskKinds) != 1 || r.Scope.TaskKinds[0] != "deployment" || r.Validity.ExpiresAt == nil {
			t.Fatalf("metadata lost or broadened: %+v %v", r, ok)
		}
		if r.Authority != contracts.AuthorityInformational || r.Sensitivity != sd.Sensitivity || r.SourceRole != contracts.RoleCanonicalKnowledge || len(r.Relationships.Supersedes) != 0 {
			t.Fatalf("source self-assigned authority: %+v", r)
		}
	})
	for _, meta := range []string{"scope:\n  profiles: [work-safe]", "scope:\n  task_kinds: 42", "scope:\n  path_globs: [src/private/**]", "validity:\n  expires_at: not-a-date", "status: retired"} {
		t.Run(meta, func(t *testing.T) {
			if _, ok := parse(meta, "Useful database recommendation."); ok {
				t.Fatal("unusable/narrowed metadata became unrestricted active evidence")
			}
		})
	}
}

func TestSummaryKeepsInlineLinksAndTaskLists(t *testing.T) {
	for _, body := range []string{
		"[SQLite](storage.md) is preferred for the local batch database.",
		"- [ ] Verify container health before deployment.",
	} {
		fm, parsed := ingestion.ParseMarkdown([]byte("---\nid: SYN-LINK\ntype: heuristic\n---\n# Policy\n\n[Home](index.md) | [Contents](#summary)\n\n## Summary\n\n" + body + "\n"))
		r, ok := ingestion.NormalizeEntry(contracts.SourceDescriptor{}, ingestion.MarkdownEntry{Frontmatter: fm, Body: parsed})
		if !ok || r.Statement != body {
			t.Errorf("decision markup discarded or navigation selected: %+v %v", r, ok)
		}
	}
}

func TestMarkdownUnusableBodiesRejectedAndUnicodeBounded(t *testing.T) {
	sd := contracts.SourceDescriptor{SourceID: "synthetic", Sensitivity: contracts.SensPublicGeneral}
	for _, body := range []string{"# Title\n\n## Summary\n\n[Home](../)", "Title\n=====\n\n<!-- comment -->\n\n```\nexample only\n```"} {
		r, ok := ingestion.NormalizeEntry(sd, ingestion.MarkdownEntry{Frontmatter: map[string]any{"id": "SYN-EMPTY", "type": "heuristic"}, Body: body})
		if ok {
			t.Errorf("unusable document indexed: %+v", r)
		}
	}
	r, ok := ingestion.NormalizeEntry(sd, ingestion.MarkdownEntry{Frontmatter: map[string]any{"id": "SYN-UNICODE"}, Body: strings.Repeat("界", 800)})
	if !ok || !utf8.ValidString(r.CompactText) || !utf8.ValidString(r.Statement) {
		t.Fatal("bounded text cuts a UTF-8 sequence")
	}
}
