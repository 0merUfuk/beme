package evalrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	evaluationschema "github.com/0merUfuk/beme/schemas/evaluation"
	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"
)

const maxEvidenceBytes = 16 << 20

type Coverage struct {
	Physical int               `json:"physical_case_files"`
	Loaded   int               `json:"loaded"`
	Selected int               `json:"selected"`
	Excluded map[string]string `json:"excluded"`
}

// readBounded rejects links, directories and oversized inputs before decoding.
func readBounded(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxEvidenceBytes {
		return nil, fmt.Errorf("not a bounded regular file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxEvidenceBytes+1))
	if len(b) > maxEvidenceBytes {
		return nil, fmt.Errorf("input too large")
	}
	return b, err
}

// decodeDocument accepts one JSON/YAML document, with no duplicate keys,
// aliases, merges, non-string object keys or ambiguous trailing documents.
func decodeDocument(data []byte, jsonOnly bool, out any) error {
	if len(data) > maxEvidenceBytes {
		return fmt.Errorf("input too large")
	}
	if jsonOnly && !json.Valid(data) {
		return fmt.Errorf("invalid JSON")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var n yaml.Node
	if err := dec.Decode(&n); err != nil {
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one document")
	}
	var walk func(*yaml.Node, int) (any, error)
	walk = func(n *yaml.Node, depth int) (any, error) {
		if depth > 64 {
			return nil, fmt.Errorf("document depth exceeded")
		}
		switch n.Kind {
		case yaml.DocumentNode:
			if len(n.Content) != 1 {
				return nil, fmt.Errorf("empty document")
			}
			return walk(n.Content[0], depth+1)
		case yaml.MappingNode:
			m := map[string]any{}
			for i := 0; i < len(n.Content); i += 2 {
				k := n.Content[i]
				if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
					return nil, fmt.Errorf("object keys must be strings")
				}
				if _, ok := m[k.Value]; ok {
					return nil, fmt.Errorf("duplicate key %q", k.Value)
				}
				v, err := walk(n.Content[i+1], depth+1)
				if err != nil {
					return nil, err
				}
				m[k.Value] = v
			}
			return m, nil
		case yaml.SequenceNode:
			a := []any{}
			for _, c := range n.Content {
				v, err := walk(c, depth+1)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			return a, nil
		case yaml.ScalarNode:
			if n.Tag == "!!timestamp" {
				return n.Value, nil
			}
			switch n.Tag {
			case "!!str", "!!int", "!!float", "!!bool", "!!null":
			default:
				return nil, fmt.Errorf("unsupported scalar tag")
			}
			var v any
			err := n.Decode(&v)
			return v, err
		default:
			return nil, fmt.Errorf("aliases and merges are not permitted")
		}
	}
	value, err := walk(&n, 0)
	if err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func validateSchema(name string, v any) error {
	b, err := evaluationschema.Files.ReadFile(name)
	if err != nil {
		return err
	}
	var schema jsonschema.Schema
	if err = json.Unmarshal(b, &schema); err != nil {
		return err
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return err
	}
	return resolved.Validate(v)
}

func decodeCase(data []byte, jsonOnly bool) (Case, error) {
	var cs Case
	var v any
	if err := decodeDocument(data, jsonOnly, &v); err != nil {
		return cs, err
	}
	if err := validateSchema("golden-case.schema.json", v); err != nil {
		return cs, err
	}
	if err := decodeDocument(data, jsonOnly, &cs); err != nil {
		return cs, err
	}
	if (cs.Status != "approved" && cs.Status != "locked") || !cs.Provenance.ApprovedByUser || cs.Provenance.CandidateNote != "" {
		return cs, fmt.Errorf("case must be approved/locked with explicit user approval and no candidate note")
	}
	if strings.TrimSpace(cs.Scenario.Task) == "" {
		return cs, fmt.Errorf("empty task")
	}
	if cs.Provenance.ValidAt != "" {
		if _, err := time.Parse("2006-01-02", cs.Provenance.ValidAt); err != nil {
			return cs, fmt.Errorf("invalid provenance date")
		}
	}
	return cs, nil
}

// LoadCorpus never silently drops supported formats. split-manifest.json is
// a reserved administrative file, not a golden case. Other directories are
// rejected: a corpus is a flat directory, not a recursive private-data scan.
func LoadCorpus(dir string) (*Corpus, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := &Corpus{Dir: dir, Files: map[string]string{}, caseFile: map[string]string{}, Bytes: map[string][]byte{}, Coverage: Coverage{Excluded: map[string]string{}}}
	for _, e := range entries {
		name := e.Name()
		if name == "split-manifest.json" {
			continue
		}
		if e.IsDir() {
			return nil, fmt.Errorf("nested corpus directory %q is not supported", name)
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			c.Coverage.Excluded[name] = "not a supported case format"
			continue
		}
		c.Coverage.Physical++
		data, err := readBounded(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		cs, err := decodeCase(data, ext == ".json")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if _, ok := c.caseFile[cs.ID]; ok {
			return nil, fmt.Errorf("duplicate case id %s", cs.ID)
		}
		c.Files[name] = sha256Hex(string(data))
		c.Bytes[name] = data
		c.caseFile[cs.ID] = name
		c.Cases = append(c.Cases, cs)
	}
	c.Coverage.Loaded = len(c.Cases)
	if len(c.Cases) == 0 {
		return nil, fmt.Errorf("no cases in corpus")
	}
	return c, nil
}

// ensureUnchanged protects bindings against callers changing the exported
// case slice after loading, without modifying the frozen corpus bytes.
func (c *Corpus) ensureUnchanged() error {
	if len(c.Cases) != len(c.Files) {
		return fmt.Errorf("corpus coverage changed since loading")
	}
	for _, cs := range c.Cases {
		name := c.caseFile[cs.ID]
		b := c.Bytes[name]
		if sha256Hex(string(b)) != c.Files[name] {
			return fmt.Errorf("corpus bytes changed")
		}
		original, err := decodeCase(b, strings.ToLower(filepath.Ext(name)) == ".json")
		if err != nil || !reflect.DeepEqual(cs, original) {
			return fmt.Errorf("case changed since loading: %s", cs.ID)
		}
	}
	return nil
}
