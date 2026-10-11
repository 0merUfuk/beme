package brain

import (
	"os"
	"path/filepath"
	"time"
)

// The brain log is the vault's append-only chronological record (Karpathy's
// log.md convention): one greppable line per event —
//
//	## [2026-09-19] promoted | dec-… [proj] sha256:1a2b3c4d
//
// It is an audit trail, not memory: it is never injected into a briefing.
// Appends are single small O_APPEND writes, so concurrent writers interleave
// whole lines.
const logHeader = "# Brain log\n\n<!-- append-only; one line per event: `grep '^## \\[' log.md | tail` -->\n\n"

func (v *Vault) appendLog(now time.Time, op, desc string) {
	p := filepath.Join(v.Root, "log.md")
	if err := os.MkdirAll(v.Root, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() == 0 {
		f.WriteString(logHeader)
	}
	f.WriteString("## [" + now.UTC().Format("2006-01-02") + "] " + oneLine(op, 24) + " | " + oneLine(desc, 200) + "\n")
}
