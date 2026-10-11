package evalrunner

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
)

// ReviewerPacket is the ONLY shareable review surface. One content-only JSON
// file contains no per-sample filesystem metadata, owner paths or condition
// labels. The owner bundle (including blinded/) must not be shared. Answer
// wording can still suggest a condition; blinding is not an anonymity claim.
// Reviewers fill Template and return it as the existing hash-bound grades file.
type ReviewerPacket struct {
	SchemaVersion string            `json:"schema_version"`
	Template      GradeDocument     `json:"template"`
	Samples       []json.RawMessage `json:"samples"`
}

func ExportReviewerPacket(bundle, evaluator string) (ReviewerPacket, error) {
	var p ReviewerPacket
	v, err := verifyBundle(bundle)
	if err != nil {
		return p, err
	}
	p.SchemaVersion = "1"
	p.Template = templateFrom(v, evaluator)
	for _, b := range v.Bindings {
		path, err := bundleFilePath(v.Root, b.ArtifactPath)
		if err != nil {
			return ReviewerPacket{}, err
		}
		data, err := readBounded(path)
		if err != nil {
			return ReviewerPacket{}, err
		}
		p.Samples = append(p.Samples, json.RawMessage(data))
	}
	// Fisher-Yates with fresh randomness. Source iteration/creation order is
	// never exposed; the packet has a single file's metadata, not one per arm.
	for i := len(p.Samples) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return ReviewerPacket{}, err
		}
		j := int(n.Int64())
		p.Samples[i], p.Samples[j] = p.Samples[j], p.Samples[i]
	}
	return p, nil
}
