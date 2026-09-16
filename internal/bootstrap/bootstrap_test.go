package bootstrap_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/bootstrap"
)

// TestBootstrapMatchesCanonicalAdapterText is the drift guard: the embedded
// copy must be byte-identical to adapters/common/skill/BOOTSTRAP.md.
func TestBootstrapMatchesCanonicalAdapterText(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "adapters", "common", "skill", "BOOTSTRAP.md"))
	if err != nil {
		t.Fatalf("canonical bootstrap unreadable: %v", err)
	}
	if len(canonical) == 0 {
		t.Fatal("positive control failed: canonical bootstrap is empty")
	}
	if !bytes.Equal(canonical, []byte(bootstrap.Text())) {
		t.Fatalf("internal/bootstrap/BOOTSTRAP.md drifted from adapters/common/skill/BOOTSTRAP.md (%d vs %d bytes); copy the canonical file", len(bootstrap.Text()), len(canonical))
	}
	h := sha256.Sum256(canonical)
	if want := "sha256:" + hex.EncodeToString(h[:]); bootstrap.SHA256() != want {
		t.Fatalf("SHA256() = %s, want %s", bootstrap.SHA256(), want)
	}
}
