package evalrunner_test

import (
	"encoding/json"
	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestCorpusValidatedFormats(t *testing.T) {
	dir := t.TempDir()
	if err := fx.WriteCorpus(dir, "", 1); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "arms-case.json"))
	var v map[string]any
	json.Unmarshal(b, &v)
	for _, ext := range []string{"yaml", "yml"} {
		v["id"] = "decision.synthetic." + ext + ".v1"
		y, _ := yaml.Marshal(v)
		os.WriteFile(filepath.Join(dir, "case."+ext), y, 0600)
	}
	c, err := evalrunner.LoadCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Cases) != 3 {
		t.Fatalf("silently lost YAML: got %d want 3", len(c.Cases))
	}
	for _, mutation := range []string{"missing", "candidate", "retired", "unapproved", "capability", "version", "unknown", "duplicate"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			var x map[string]any
			json.Unmarshal(b, &x)
			switch mutation {
			case "missing":
				delete(x, "gold")
			case "candidate", "retired":
				x["status"] = mutation
			case "unapproved":
				x["provenance"].(map[string]any)["approved_by_user"] = false
			case "capability":
				x["capability"] = "typo"
			case "version":
				x["schema_version"] = "2"
			case "unknown":
				x["typo"] = true
			}
			data, _ := json.Marshal(x)
			if mutation == "duplicate" {
				data = append([]byte(`{"id":"decision.fake.v1",`), data[1:]...)
			}
			os.WriteFile(filepath.Join(dir, "bad.json"), data, 0600)
			if _, err := evalrunner.LoadCorpus(dir); err == nil {
				t.Fatal("invalid corpus accepted")
			}
		})
	}
}
