package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReadSurfacesUseLedgerFilter pins the ADR-027 read-surface rule: product
// code outside internal/app (which applies store tombstones and the durable
// ledger at read time), internal/storage, internal/projection, and
// internal/resolver never reads projection rows, provenance, counts, or
// tombstones directly — a new surface that did would bypass anti-resurrection.
func TestReadSurfacesUseLedgerFilter(t *testing.T) {
	root := filepath.Join("..", "..")
	forbidden := regexp.MustCompile(`storage\.Open\(|\.View\.(Records|Provenance)\(|\.Store\.(Records|AllRecords|RecordByID|Count|Provenance|RevokedSet|RevokedKeys|SearchFTS)\(|OpenStoreForProfile`)
	allowed := []string{
		"internal/app/", "internal/storage/", "internal/projection/", "internal/resolver/",
		// the adversarial corpus inspects raw stores on purpose to prove
		// what the filtered surfaces hide
		"internal/privacycorpus/",
	}
	var violations []string
	for _, top := range []string{"cmd", "internal"} {
		filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			for _, a := range allowed {
				if strings.HasPrefix(rel, a) {
					return nil
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(data), "\n") {
				if forbidden.MatchString(line) {
					violations = append(violations, rel+":"+itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
	}
	if len(violations) > 0 {
		t.Fatalf("direct projection reads bypass the ledger filter:\n  %s", strings.Join(violations, "\n  "))
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
