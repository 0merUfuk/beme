package ingestion

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestMatchGlobTable pins ** semantics on slash-separated relative paths —
// the only form the walker emits on macOS, Linux, and Windows (ADR-028).
func TestMatchGlobTable(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		// multi-segment tail after ** (direct and nested)
		{"a/**/b/*.md", "a/b/file.md", true},
		{"a/**/b/*.md", "a/x/b/file.md", true},
		{"a/**/b/*.md", "a/x/y/b/file.md", true},
		{"a/**/b/*.md", "a/x/b/c/file.md", false},
		{"a/**/b/*.md", "a/x/b/file.txt", false},
		{"a/**/b/*.md", "z/a/b/file.md", false},
		{"a/**/b/c/*.md", "a/x/b/c/file.md", true},
		{"a/**/b/c/*.md", "a/b/c/file.md", true},
		{"a/**/b/c/*.md", "a/b/x/c/file.md", false},
		// leading and trailing **
		{"**/*.md", "file.md", true},
		{"**/*.md", "deep/er/file.md", true},
		{"**/node_modules/**", "node_modules/pkg/index.md", true},
		{"**/node_modules/**", "src/node_modules/pkg/index.md", true},
		{"secrets/**", "secrets/prod.env", true},
		{"secrets/**", "secrets/nested/key.yaml", true},
		{"secrets/**", "public/secrets.md", false},
		{"entries/**/*.md", "entries/a.md", true},
		{"entries/**/*.md", "entries/2026/09/a.md", true},
		{"entries/**/*.md", "other/entries/a.md", false},
		// consecutive ** and single-segment wildcards
		{"a/**/**/b.md", "a/b.md", true},
		{"a/**/**/b.md", "a/x/y/b.md", true},
		{"docs/*.md", "docs/a.md", true},
		{"docs/*.md", "docs/sub/a.md", false},
		{"docs/?.md", "docs/a.md", true},
		{"docs/[ab].md", "docs/b.md", true},
		// names that are awkward on some platforms stay literal segments
		{"notes/**/*.md", "notes/Ünïcode dir/a b.md", true},
		{"notes/**/*.md", "notes/C:/a.md", true},
		{"notes/*.md", `notes\a.md`, false},
		// traversal never matches
		{"**/*.md", "../escape.md", false},
		{"a/**", "a/../b.md", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.rel); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.rel, got, c.want)
		}
	}
}

// TestWalkIncludeExcludeNestedTails exercises include and exclude through the
// real walker on the current OS: relative paths come back slash-separated and
// multi-segment ** tails select exactly the intended files.
func TestWalkIncludeExcludeNestedTails(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"a/b/direct.md",
		"a/x/b/nested.md",
		"a/x/y/b/deeper.md",
		"a/x/b/c/too-deep.md",
		"a/other.md",
		"docs/sub/n.md",
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w := NewWalker(Limits{MaxFileBytes: 1 << 20, MaxFiles: 100, MaxDepth: 10, MaxTotalBytes: 1 << 20, Timeout: 10 * time.Second})
	rels := func(include, exclude []string) []string {
		got, err := w.Walk(root, include, exclude)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, f := range got {
			if strings.Contains(f.RelPath, `\`) {
				t.Fatalf("walker emitted a non-slash relative path: %q", f.RelPath)
			}
			out = append(out, f.RelPath)
		}
		sort.Strings(out)
		return out
	}
	if got, want := rels([]string{"a/**/b/*.md"}, nil), []string{"a/b/direct.md", "a/x/b/nested.md", "a/x/y/b/deeper.md"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("include a/**/b/*.md: got %v, want %v", got, want)
	}
	if got, want := rels(nil, []string{"a/**/b/*.md"}), []string{"a/other.md", "a/x/b/c/too-deep.md", "docs/sub/n.md"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("exclude a/**/b/*.md: got %v, want %v", got, want)
	}
}
