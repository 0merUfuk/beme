package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// directReadPattern matches product code that reads projection rows,
// provenance, counts, or tombstones directly, or opens the learning store
// without the ledger filter.
var directReadPattern = regexp.MustCompile(`storage\.Open\(|\.View\.(Records|Provenance)\(|\.Store\.(Records|AllRecords|RecordByID|Count|Provenance|RevokedSet|RevokedKeys|SearchFTS)\(|OpenStoreForProfile|learning\.Open`)

// readSurfaceAllowed lists packages that may read raw stores: internal/app
// applies store tombstones and the durable ledger at read time; storage,
// projection, resolver, and learning are the layers it composes; the
// adversarial corpus inspects raw stores on purpose to prove what the
// filtered surfaces hide.
var readSurfaceAllowed = []string{
	"internal/app/", "internal/storage/", "internal/projection/", "internal/resolver/",
	"internal/learning/", "internal/privacycorpus/",
}

// scanDirectReads walks the product code under root and returns every
// violating line. Traversal and read errors are returned, never skipped, so
// the guard cannot pass without having inspected all code.
func scanDirectReads(root string, dirs ...string) ([]string, error) {
	var violations []string
	for _, top := range dirs {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for _, a := range readSurfaceAllowed {
				if strings.HasPrefix(rel, a) {
					return nil
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(data), "\n") {
				if directReadPattern.MatchString(line) {
					violations = append(violations, rel+":"+itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return violations, nil
}

// TestReadSurfacesUseLedgerFilter pins the ADR-027 read-surface rule: product
// code outside the allowed layers never reads projection or learning state
// directly — a new surface that did would bypass anti-resurrection.
func TestReadSurfacesUseLedgerFilter(t *testing.T) {
	violations, err := scanDirectReads(filepath.Join("..", ".."), "cmd", "internal")
	if err != nil {
		t.Fatalf("guard could not inspect all product code: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("direct reads bypass the ledger filter:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestReadSurfaceGuardFailsOnTraversalErrors: the guard must error, not pass,
// when it cannot walk the tree; and it must flag a planted violation.
func TestReadSurfaceGuardFailsOnTraversalErrors(t *testing.T) {
	if _, err := scanDirectReads(t.TempDir(), "does-not-exist"); err == nil {
		t.Fatal("a missing directory must be a guard error")
	}
	root := t.TempDir()
	planted := filepath.Join(root, "cmd", "x", "main.go")
	if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planted, []byte("package main\nfunc f() { recs := sess.Store.Records() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	violations, err := scanDirectReads(root, "cmd")
	if err != nil || len(violations) != 1 {
		t.Fatalf("positive control: planted direct read must be flagged; got %v %v", violations, err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
