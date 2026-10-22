package evalrunner

import (
	"os"
	"path/filepath"
	"testing"
)

// A bundle lookup is a wire-identifier lookup, not a request to clean an OS
// pathname. Even an indexed alias must not name the same evidence twice.
func TestBundleReadRejectsNoncanonicalWirePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.json"), []byte(`{"value":"synthetic"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sample.json", "./sample.json", "blinded/../sample.json", "blinded//../sample.json", `blinded\..\sample.json`} {
		t.Run(name, func(t *testing.T) {
			v := verifiedBundle{Root: root, Index: map[string]string{name: "indexed"}}
			var got map[string]string
			err := v.read(name, &got)
			if name == "sample.json" {
				if err != nil || got["value"] != "synthetic" {
					t.Fatalf("canonical positive control: %v %v", got, err)
				}
			} else if err == nil {
				t.Fatalf("noncanonical wire path %q was normalized and read", name)
			}
		})
	}
}

func TestBundleFilePathKeepsWireIdentifiersLiteral(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"summary.json", "manifests/sample.json", "corpus/synthetic case.yaml"} {
		got, err := bundleFilePath(root, name)
		if err != nil || got != filepath.Join(root, filepath.FromSlash(name)) {
			t.Fatalf("canonical wire path %q: %q %v", name, got, err)
		}
	}
	for _, name := range []string{"", ".", "..", "./sample.json", "../sample.json", "manifests/../sample.json", "manifests//sample.json", "manifests/./sample.json", "manifests/sample.json/", "/sample.json", `manifests\sample.json`, `C:\sample.json`, "C:/sample.json", "sample.json:stream", `\\host\share\sample.json`, "sample\x00.json"} {
		if got, err := bundleFilePath(root, name); err == nil || got != "" {
			t.Errorf("malformed wire identifier %q was accepted or normalized: %q %v", name, got, err)
		}
	}
}
