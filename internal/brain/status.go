package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Status is the operator's one-screen view of the vault.
type Status struct {
	Root              string         `json:"root"`
	Initialized       bool           `json:"initialized"`
	SchemaVersion     int            `json:"schema_version,omitempty"`
	Frozen            bool           `json:"frozen"`
	Projects          []string       `json:"projects,omitempty"`
	Live              map[string]int `json:"live_by_type,omitempty"`
	LiveUnknown       bool           `json:"live_unknown,omitempty"`
	LiveUnknownReason string         `json:"live_unknown_reason,omitempty"`
	Inbox             int            `json:"inbox_pending"`
	Rejected          int            `json:"rejected"`
	Archived          int            `json:"archived"`
	Snapshots         int            `json:"snapshots"`
	LatestSnap        string         `json:"latest_snapshot,omitempty"`
	LastLog           string         `json:"last_log_entry,omitempty"`
	SilentSession     map[string]int `json:"sessions_since_capture,omitempty"`
	Invocations       map[string]int `json:"invocations_since_capture,omitempty"`
	SessionWindow     int            `json:"session_dedup_window"`
	Accounting        string         `json:"accounting_semantics"`
	AccountingUnknown []string       `json:"accounting_unknown_projects,omitempty"`
}

// Status inspects the vault read-only.
func (v *Vault) Status() Status {
	st := Status{Root: v.Root, Initialized: v.Initialized()}
	if !st.Initialized {
		return st
	}
	st.SessionWindow = sessionIDWindow
	st.Accounting = "sessions = identified starts within retained ID window; invocations include unknown IDs and resumes; capture means accepted proposal, not promotion, use or benefit"
	st.Invocations = map[string]int{}
	st.SchemaVersion = v.schemaVersion()
	st.Frozen = v.Frozen()
	st.Live = map[string]int{}
	if ps, err := v.Projects(); err == nil {
		for _, p := range ps {
			st.Projects = append(st.Projects, p.Slug)
			state, err := v.readState(p.Slug)
			if err != nil || state.AccountingUnknown {
				st.AccountingUnknown = append(st.AccountingUnknown, p.Slug)
				continue
			}
			if n := state.InvocationsSinceCapture; n > 0 {
				st.Invocations[p.Slug] = n
			}
			if s := state.SessionsSinceCapture; s > 0 {
				if st.SilentSession == nil {
					st.SilentSession = map[string]int{}
				}
				st.SilentSession[p.Slug] = s
			}
		}
	}
	// An incomplete bounded scan (entry, byte or wall-time limit, or an
	// unreadable projects directory) claims no counts: zero would be a lie.
	notes, bad, err := v.scanLive()
	for _, b := range bad {
		// A single unparseable note is reported by Doctor; an unreadable or
		// over-limit type directory means a whole type went uncounted.
		if err == nil && !strings.HasSuffix(b.Path, ".md") {
			err = fmt.Errorf("%s: %s", filepath.Base(b.Path), b.Err)
		}
	}
	if err != nil {
		st.Live = nil
		st.LiveUnknown = true
		st.LiveUnknownReason = err.Error()
	} else {
		for _, n := range notes {
			st.Live[string(n.Type)]++
		}
	}
	st.Inbox = countMD(v.inboxDir())
	st.Rejected = countMD(v.rejectedDir())
	st.Archived = len(v.scanArchive())
	if snaps, _ := v.ListBackups(); len(snaps) > 0 {
		st.Snapshots = len(snaps)
		st.LatestSnap = filepath.Base(snaps[0].Path)
	}
	if data, err := v.readFile(filepath.Join(v.Root, "log.md")); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.HasPrefix(lines[i], "## [") {
				st.LastLog = lines[i]
				break
			}
		}
	}
	sort.Strings(st.Projects)
	return st
}

func countMD(dir string) int {
	n := 0
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n
}
