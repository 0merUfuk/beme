package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// V1 regression: error paths once printed a literal backslash-n (double-escaped
// format strings), so messages ran into the shell prompt.
func TestCLIErrorMessagesEndWithRealNewline(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	os.MkdirAll(r.cfg, 0o755)
	os.WriteFile(filepath.Join(r.cfg, "config.yaml"), []byte("data_dir: [unterminated"), 0o644)
	for _, args := range [][]string{{"status"}, {"build", "--profile", "personal"}, {"serve", "--projection", "personal", "--capability", "cap_x"}} {
		_, se, code := r.run("", args...)
		if code == 0 {
			t.Fatalf("%v: broken config must fail", args)
		}
		if strings.Contains(se, `\n`) || !strings.HasSuffix(se, "\n") {
			t.Errorf("%v: stderr must end with a real newline and contain no literal \\n, got %q", args, se)
		}
	}
}
