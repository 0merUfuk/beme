// Package bootstrap exposes the canonical managed bootstrap text that the
// adapter installer writes into harness instruction files and that the
// evaluation runner gives to every bootstrap-bearing arm (B1–B4, ablations).
//
// The canonical source is adapters/common/skill/BOOTSTRAP.md. go:embed cannot
// reach outside this package directory, so BOOTSTRAP.md here is a byte copy;
// TestBootstrapMatchesCanonicalAdapterText fails on any drift.
package bootstrap

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed BOOTSTRAP.md
var text string

// Text returns the canonical bootstrap text, byte-exact.
func Text() string { return text }

// SHA256 returns "sha256:<hex>" of Text().
func SHA256() string {
	h := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(h[:])
}
