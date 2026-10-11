package evalrunner_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
)

// Run on every OS: wire paths must not acquire the host path separator. Check
// the actual saved bytes and joins used by grading, not a cleaned copy of them.
func TestArtifactWirePathsRoundTrip(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	c := syntheticCorpus(t, d, 2)
	out := filepath.Join(t.TempDir(), "synthetic artifacts")
	s, err := evalrunner.Run(c, evalrunner.RunConfig{Arms: evalrunner.AllArms, OutputDir: out, Refs: d.Refs()}, evalrunner.NewMockProvider("synthetic-only"), nil, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(out, s.RunID)
	read := func(name string, into any) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, into); err != nil {
			t.Fatal(err)
		}
	}
	var saved evalrunner.Summary
	var index map[string]string
	var bindings []evalrunner.SampleBinding
	read("summary.json", &saved)
	read("bundle_index.json", &index)
	read("grading_index.json", &bindings)
	if !reflect.DeepEqual(s.ManifestPaths, saved.ManifestPaths) {
		t.Fatal("returned and stored manifest identifiers disagree")
	}
	manifests := map[string]string{}
	for name, digest := range index {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, `\:`) {
			t.Fatalf("noncanonical wire identifier in bundle index: %q", name)
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || sha(b) != digest {
			t.Fatalf("wire identifier does not bind native file %q: %v", name, err)
		}
		if strings.HasPrefix(name, "manifests/") {
			manifests[name] = digest
		}
	}
	seen, manifestHashes := map[string]bool{}, map[string]bool{}
	for _, name := range saved.ManifestPaths {
		if seen[name] || manifests[name] == "" {
			t.Fatalf("summary manifest must bind one exact wire index key: %q", name)
		}
		seen[name] = true
		manifestHashes[manifests[name]] = true
	}
	want := len(evalrunner.AllArms) * 2
	if len(seen) != want || len(manifests) != want || len(bindings) != want {
		t.Fatalf("manifest/binding population: %d/%d/%d want %d", len(seen), len(manifests), len(bindings), want)
	}
	for _, b := range bindings {
		if b.ArtifactPath != "blinded/"+b.SampleID+".json" || index[b.ArtifactPath] != b.ArtifactSHA || !manifestHashes[b.ManifestSHA] {
			t.Fatalf("grading binding lost exact wire identity: %+v", b)
		}
	}
	g, err := evalrunner.GradeTemplate(root, "synthetic-reviewer")
	if err != nil || len(g.Records) != want {
		t.Fatalf("grade reconstruction: %d records, %v", len(g.Records), err)
	}
	packet, err := evalrunner.ExportReviewerPacket(root, "synthetic-reviewer")
	if err != nil || len(packet.Samples) != want || !reflect.DeepEqual(packet.Template, g) {
		t.Fatalf("reviewer export lost bindings: %d samples, %v", len(packet.Samples), err)
	}
}
