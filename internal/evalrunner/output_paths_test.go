package evalrunner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/evalrunner"
)

// No OS skip: an alternate spelling either aliases the bundle on this
// filesystem or names a distinct sibling. Both outcomes have an exact policy.
func TestOutputBoundaryUsesDirectoryIdentity(t *testing.T) {
	parent := t.TempDir()
	bundle := filepath.Join(parent, "BundleRoot")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.Stat(bundle)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "bundleroot")
	aliasInfo, err := os.Stat(alias)
	if os.IsNotExist(err) {
		if err := os.Mkdir(alias, 0700); err != nil {
			t.Fatal(err)
		}
		aliasInfo, err = os.Stat(alias)
	}
	if err != nil {
		t.Fatal(err)
	}
	same := os.SameFile(root, aliasInfo)
	t.Logf("alternate-case spelling aliases bundle: %v", same)
	for _, tc := range []struct {
		name, out string
		inside    bool
	}{
		{"root", bundle, true},
		{"child", filepath.Join(bundle, "template.json"), true},
		{"missing-descendants", filepath.Join(bundle, "new", "nested", "template.json"), true},
		{"case-spelling", alias, same},
		{"case-spelling-child", filepath.Join(alias, "template.json"), same},
		{"case-spelling-missing-descendants", filepath.Join(alias, "new", "nested", "template.json"), same},
		{"similar-prefix-sibling", filepath.Join(parent, "BundleRoot-other", "template.json"), false},
		{"parent", parent, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := evalrunner.CheckOutputOutsideBundle(tc.out, bundle)
			if (err != nil) != tc.inside {
				t.Fatalf("inside=%v output=%q: %v", tc.inside, tc.out, err)
			}
		})
	}
}
