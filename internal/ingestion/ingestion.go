// Package ingestion reads registered source content safely. It never
// executes repository code (FR-021), never follows symlinks outside
// registered real roots (FR-022), excludes secrets before indexing
// (FR-023), and enforces size/count/depth/time bounds (NFR-009).
package ingestion

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/contracts"
)

// Limits bound ingestion (NFR-009, threat case 29).
type Limits struct {
	MaxFileBytes  int64
	MaxFiles      int
	MaxDepth      int
	MaxTotalBytes int64
	Timeout       time.Duration
}

// DefaultLimits are the conservative v1 bounds.
func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:  2 << 20, // 2 MiB per file
		MaxFiles:      5000,
		MaxDepth:      12,
		MaxTotalBytes: 100 << 20, // 100 MiB per source
		Timeout:       30 * time.Second,
	}
}

// HardExcludes are always excluded regardless of descriptor config.
var HardExcludes = []string{
	".git", ".hg", ".svn",
	".env", ".env.local", ".env.*", "*.env",
	"*.pem", "*.key", "*.p12", "*.pfx", "*.crt",
	"credentials", "secrets", "id_rsa*", "id_ed25519*",
	".DS_Store", "node_modules",
	// protected corpora are excluded via source-descriptor excludes
	// (deployment configuration), not engine hard-coding
}

// Walker walks a registered root safely.
type Walker struct {
	limits Limits
}

func NewWalker(limits Limits) *Walker { return &Walker{limits: limits} }

// File is one ingested file.
type File struct {
	RelPath string // relative to registered root
	Content []byte
	Hash    string
}

// Walk collects allowed files under root matching include globs. It:
//   - resolves the root real path and never escapes it (symlink check);
//   - applies hard excludes + descriptor excludes;
//   - enforces bounds and aborts on violation.
func (w *Walker) Walk(root string, include, exclude []string) ([]File, error) {
	deadline := time.Now().Add(w.limits.Timeout)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("root not resolvable: %w", err)
	}
	files := []File{}
	total := int64(0)
	count := 0

	err = filepath.WalkDir(realRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("ingestion timeout exceeded")
		}
		// Relative paths are slash-separated on every platform: globs,
		// excludes, and provenance locators are portable (NFR-007).
		rel := strings.TrimPrefix(path, realRoot)
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
		if d.IsDir() {
			if rel == "" {
				return nil
			}
			if matchAny(rel, HardExcludes) || matchAny(rel, exclude) {
				return fs.SkipDir
			}
			if strings.Count(rel, "/")+1 > w.limits.MaxDepth {
				return fs.SkipDir
			}
			return nil
		}
		count++
		if count > w.limits.MaxFiles {
			return fmt.Errorf("file count limit exceeded (%d)", w.limits.MaxFiles)
		}
		if matchAny(rel, HardExcludes) || matchAny(rel, exclude) {
			return nil
		}
		// Real-path containment: a symlink file must not point outside root.
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil // unreadable entries are skipped, never fatal to bounds
		}
		if !strings.HasPrefix(real, realRoot+string(filepath.Separator)) && real != realRoot {
			return nil // symlink escape: skipped
		}
		if len(include) > 0 && !matchAny(rel, include) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > w.limits.MaxFileBytes {
			return fmt.Errorf("file %s exceeds size limit (%d > %d)", rel, info.Size(), w.limits.MaxFileBytes)
		}
		if !isTextFile(rel) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		total += int64(len(content))
		if total > w.limits.MaxTotalBytes {
			return fmt.Errorf("total size limit exceeded")
		}
		h := sha256.Sum256(content)
		files = append(files, File{RelPath: rel, Content: content, Hash: "sha256:" + hex.EncodeToString(h[:])})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func matchAny(rel string, patterns []string) bool {
	for _, p := range patterns {
		if matchGlob(p, rel) {
			return true
		}
		if matchGlob(p, path.Base(rel)) {
			return true
		}
		// directory-tree excludes: a plain name matching a leading path
		// segment excludes the whole subtree ("secrets", ".git").
		if !strings.Contains(p, "*") && strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

// matchGlob matches a slash-separated relative path against a pattern
// segment by segment (gitignore semantics). "**" matches zero or more whole
// segments anywhere in the pattern, including before a multi-segment tail
// such as "a/**/b/*.md"; every other segment uses path.Match, so "*" never
// crosses "/". Paths containing ".." never match.
func matchGlob(pattern, rel string) bool {
	if rel == "" {
		return false
	}
	rs := strings.Split(rel, "/")
	for _, seg := range rs {
		if seg == ".." {
			return false
		}
	}
	return matchSegments(strings.Split(strings.Trim(pattern, "/"), "/"), rs)
}

func matchSegments(ps, rs []string) bool {
	for len(ps) > 0 {
		if ps[0] == "**" {
			for len(ps) > 1 && ps[1] == "**" {
				ps = ps[1:]
			}
			if len(ps) == 1 {
				return true // trailing ** matches any remainder
			}
			for i := 0; i <= len(rs); i++ {
				if matchSegments(ps[1:], rs[i:]) {
					return true
				}
			}
			return false
		}
		if len(rs) == 0 {
			return false
		}
		if ok, err := path.Match(ps[0], rs[0]); err != nil || !ok {
			return false
		}
		ps, rs = ps[1:], rs[1:]
	}
	return len(rs) == 0
}

func isTextFile(rel string) bool {
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".md", ".markdown", ".txt", ".yaml", ".yml", ".json":
		return true
	}
	return false
}

// SecretScan rejects content that looks like credentials before indexing
// (threat case 17: defense beyond .gitignore).
func SecretScan(content []byte) error {
	s := strings.ToLower(string(content))
	patterns := []string{
		"-----BEGIN RSA PRIVATE KEY-----",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"-----BEGIN EC PRIVATE KEY-----",
		"-----BEGIN PRIVATE KEY-----",
		"api_key =", "api_key:", "apikey:", "secret_key:", "access_token=",
		"aws_access_key_id", "aws_secret_access_key",
		"password: \"", "password='", "bearer ",
	}
	hits := 0
	for _, p := range patterns {
		if strings.Contains(s, strings.ToLower(p)) {
			hits++
		}
	}
	if hits > 0 {
		return fmt.Errorf("potential secret content detected (%d patterns)", hits)
	}
	return nil
}

// MarkdownEntry is a parsed markdown/frontmatter document from a knowledge
// source (entry-style frontmatter, ADR-style decisions).
type MarkdownEntry struct {
	Frontmatter map[string]any
	Body        string
	RelPath     string
	Hash        string
}

// ParseMarkdown splits frontmatter and body. It never interprets content
// as instructions (ADR-006): frontmatter values are data only.
func ParseMarkdown(content []byte) (map[string]any, string) {
	txt := string(content)
	if !strings.HasPrefix(txt, "---") {
		return map[string]any{}, txt
	}
	rest := txt[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return map[string]any{}, txt
	}
	fm := rest[:end]
	// crude but safe YAML subset parse: key: value lines, lists
	front := map[string]any{}
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		v = strings.Trim(v, "\"'")
		if strings.Contains(v, "#") && !strings.HasPrefix(v, "\"") {
			v = strings.TrimSpace(strings.SplitN(v, "#", 2)[0])
		}
		front[k] = v
	}
	body := rest[end+4:] // skip "\n---" entirely (end points at the newline; +4 passes the closing dashes)
	return front, body
}

// NormalizeEntry maps a parsed canonical-knowledge-style entry into a Record using
// the source's trusted registration. Mapping never invents authority:
// unknown frontmatter types become unmapped_reference (§8.2).
func NormalizeEntry(sd contracts.SourceDescriptor, entry MarkdownEntry) (contracts.Record, bool) {
	fm := entry.Frontmatter
	idRaw, _ := fm["id"].(string)
	if idRaw == "" {
		return contracts.Record{}, false
	}
	rec := contracts.Record{
		SchemaVersion:  contracts.SchemaVersion,
		RecordID:       "rec_" + sanitizeID(idRaw),
		SourceID:       sd.SourceID,
		SourceRecordID: idRaw,
		SourceRevision: contentDigestOrCommitHelper(sd.Revision),
		Kind:           mapKind(fm["type"]),
		Status:         mapStatus(fm["status"]),
		Confidence:     mapConfidence(fm["confidence"]),
		Authority:      contracts.AuthorityInformational, // content never self-assigns more
		SourceRole:     mapRole(sd),
		Trust:          sd.Trust,
		Sensitivity:    sd.Sensitivity,
		Relationships:  contracts.Relationships{},
		ProvenanceRefs: []string{},
	}
	if title, ok := fm["title"].(string); ok {
		rec.Title = title
	}
	// Statement from body: first paragraph under Summary if present, else first line.
	rec.Statement = firstMeaningful(entry.Body)
	rec.CompactText = truncate(rec.Statement, 240)
	if rec.Status != contracts.StatusActive {
		// keep deprecated entries out of the index entirely
		return contracts.Record{}, false
	}
	return rec, true
}

func sanitizeID(s string) string {
	out := strings.Builder{}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func mapKind(v any) contracts.Kind {
	s, _ := v.(string)
	switch strings.TrimSpace(s) {
	case "principle":
		return contracts.KindPrinciple
	case "heuristic":
		return contracts.KindHeuristic
	case "pattern":
		return contracts.KindPattern
	case "workflow":
		return contracts.KindWorkflow
	case "failure-mode", "failure_mode":
		return contracts.KindFailureMode
	case "fact":
		return contracts.KindFact
	case "precedent", "decision":
		return contracts.KindPrecedent
	case "preference":
		return contracts.KindPreference
	default:
		return contracts.KindUnmapped
	}
}

func mapStatus(v any) contracts.Status {
	s, _ := v.(string)
	if strings.TrimSpace(s) == "deprecated" {
		return contracts.StatusDeprecated
	}
	return contracts.StatusActive
}

func mapConfidence(v any) contracts.Confidence {
	s, _ := v.(string)
	if strings.TrimSpace(s) == "validated" {
		return contracts.ConfidenceValidated
	}
	return contracts.ConfidenceObserved
}

func mapRole(sd contracts.SourceDescriptor) contracts.SourceRole {
	for _, p := range sd.Purpose {
		switch p {
		case "canonical_foundation":
			return contracts.RoleCanonicalFoundation
		case "reusable_knowledge":
			return contracts.RoleCanonicalKnowledge
		case "trusted_project_policy":
			return contracts.RoleTrustedProjectPolicy
		case "safe_declassified":
			return contracts.RoleDeclassifiedSafe
		case "evidence":
			return contracts.RoleTrustedReference
		}
	}
	return contracts.RoleTrustedReference
}

func firstMeaningful(body string) string {
	for _, para := range strings.Split(body, "\n\n") {
		p := strings.TrimSpace(para)
		p = strings.TrimPrefix(p, "#")
		p = strings.TrimSpace(p)
		if len(p) > 0 && !strings.HasPrefix(p, "[") && !strings.HasPrefix(p, "!") {
			return truncate(p, 600)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// ContentDigestOrCommit helper on SourceRevision.
func contentDigestOrCommitHelper(sr *contracts.SourceRevision) string {
	if sr == nil {
		return ""
	}
	if sr.ApprovedDigest != "" {
		return sr.ApprovedDigest
	}
	return sr.Kind
}
