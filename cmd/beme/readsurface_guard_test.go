package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Read-surface guard (ADR-027 §7, ADR-030 §4). Product code outside the
// layers listed below must not read projection rows, provenance, counts, or
// tombstones directly, and must not open the learning store without the
// ledger filter — either would bypass anti-resurrection.
//
// The guard parses Go source instead of matching lines: a line-based regular
// expression misses a selector split across lines and an aliased import such
// as `learn "…/internal/learning"; learn.Open(dir)`.

// guardedPackages are the packages whose constructors must not be called
// outside the allowed layers, with the functions that open raw state.
var guardedPackages = map[string][]string{
	"github.com/0merUfuk/beme/internal/storage":  {"Open"},
	"github.com/0merUfuk/beme/internal/learning": {"Open", "OpenWith"},
}

// guardedMethods are raw-read methods; they are matched on any receiver,
// because the receiver is usually a field chain (sess.Store.Records()).
var guardedMethods = map[string]bool{
	"Records": true, "AllRecords": true, "RecordByID": true, "Count": true,
	"Provenance": true, "RevokedSet": true, "RevokedKeys": true, "SearchFTS": true,
	"OpenStoreForProfile": true,
}

// readSurfaceAllowed lists packages that may read raw stores: internal/app
// applies store tombstones and the durable ledger at read time; storage,
// projection, resolver, and learning are the layers it composes; the
// adversarial corpus and the evaluation fixture inspect raw stores on
// purpose, to prove what the filtered surfaces hide.
var readSurfaceAllowed = []string{
	"internal/app/", "internal/storage/", "internal/projection/", "internal/resolver/",
	"internal/learning/", "internal/privacycorpus/",
}

// scanDirectReads parses the product code under root and returns every
// violating position. Traversal, parse, and path errors are returned, never
// skipped, so the guard cannot pass without having inspected all code.
func scanDirectReads(root string, dirs ...string) ([]string, error) {
	var violations []string
	fset := token.NewFileSet()
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
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			// local package name -> guarded functions (aliases resolved),
			// plus every import name, so a package-qualified call such as
			// strings.Count is never mistaken for a store method.
			guarded := map[string][]string{}
			imported := map[string]bool{}
			for _, imp := range file.Imports {
				p, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return err
				}
				fns, ok := guardedPackages[p]
				if !ok {
					if imp.Name != nil {
						imported[imp.Name.Name] = true
					} else {
						imported[p[strings.LastIndex(p, "/")+1:]] = true
					}
					continue
				}
				name := p[strings.LastIndex(p, "/")+1:]
				if imp.Name != nil {
					name = imp.Name.Name
				}
				imported[name] = true
				if name == "." || name == "_" {
					// a dot import hides the qualifier entirely
					violations = append(violations, rel+":"+strconv.Itoa(fset.Position(imp.Pos()).Line)+
						": dot/blank import of guarded package "+p)
					continue
				}
				guarded[name] = fns
			}
			report := func(pos token.Pos, what string) {
				violations = append(violations, rel+":"+strconv.Itoa(fset.Position(pos).Line)+": "+what)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok {
					for _, fn := range guarded[id.Name] {
						if sel.Sel.Name == fn {
							report(call.Pos(), id.Name+"."+fn+"() opens raw state outside the ledger filter")
						}
					}
				}
				if guardedMethods[sel.Sel.Name] {
					// skip package-qualified functions (strings.Count, …):
					// only method calls on a value can be a raw store read
					if id, ok := sel.X.(*ast.Ident); ok && imported[id.Name] {
						return true
					}
					report(call.Pos(), "."+sel.Sel.Name+"() reads raw state outside the ledger filter")
				}
				return true
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return violations, nil
}

// TestReadSurfacesUseLedgerFilter pins the rule against the real tree.
func TestReadSurfacesUseLedgerFilter(t *testing.T) {
	violations, err := scanDirectReads(filepath.Join("..", ".."), "cmd", "internal")
	if err != nil {
		t.Fatalf("guard could not inspect all product code: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("direct reads bypass the ledger filter:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestReadSurfaceGuardCatchesEvasions: the guard errors (never passes) when
// it cannot inspect the tree, and flags the forms a line-based matcher
// misses — an aliased import, a selector split across lines, and a dot
// import — while leaving compliant code alone.
func TestReadSurfaceGuardCatchesEvasions(t *testing.T) {
	if _, err := scanDirectReads(t.TempDir(), "does-not-exist"); err == nil {
		t.Fatal("a missing directory must be a guard error")
	}
	root := t.TempDir()
	write := func(name, src string) {
		p := filepath.Join(root, "cmd", name, "main.go")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("plain", "package main\n\nfunc f(sess S) { _ = sess.Store.Records() }\n\ntype S struct{ Store struct{ Records func() []int } }\n")
	write("aliased", "package main\n\nimport learn \"github.com/0merUfuk/beme/internal/learning\"\n\nfunc g(dir string) { _, _ = learn.Open(dir) }\n")
	write("multiline", "package main\n\nfunc h(sess S2) {\n\t_ = sess.\n\t\tStore.\n\t\tAllRecords()\n}\n\ntype S2 struct{ Store struct{ AllRecords func() []int } }\n")
	write("dotimport", "package main\n\nimport . \"github.com/0merUfuk/beme/internal/storage\"\n\nfunc i(p string) { _, _ = Open(p) }\n")
	write("compliant", "package main\n\nfunc j(rt R) { _, _ = rt.OpenLearning() }\n\ntype R struct{ OpenLearning func() (int, error) }\n")
	write("stdlib", "package main\n\nimport \"strings\"\n\nfunc k(s string) int { return strings.Count(s, \"x\") }\n")

	violations, err := scanDirectReads(root, "cmd")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"cmd/plain/", "cmd/aliased/", "cmd/multiline/", "cmd/dotimport/"} {
		found := false
		for _, v := range violations {
			found = found || strings.HasPrefix(v, want)
		}
		if !found {
			t.Errorf("guard missed the evasion in %s (got %v)", want, violations)
		}
	}
	for _, v := range violations {
		if strings.HasPrefix(v, "cmd/compliant/") || strings.HasPrefix(v, "cmd/stdlib/") {
			t.Errorf("guard flagged compliant code: %s", v)
		}
	}
}

// TestReadSurfaceGuardRejectsUnparseableCode: a file the guard cannot parse
// is an error, not a silent skip.
func TestReadSurfaceGuardRejectsUnparseableCode(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "cmd", "broken", "main.go")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("package main\nfunc ( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scanDirectReads(root, "cmd"); err == nil {
		t.Fatal("unparseable product code must fail the guard")
	}
}
