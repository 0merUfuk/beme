package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Brain Doctor: the lightweight maintenance loop. It is the ONLY writer of
// live notes (Capture writes proposals). Everything it does is deterministic
// and reversible: promoted notes are plain files, retired notes move to
// archive/, and the vault is meant to sit under git so every run is a diff.
const (
	overlapThreshold = 0.45 // related claims → flag for a human
	maxActive        = 30   // dossier cap: beyond this agents go instruction-blind
	keepSummaries    = 3
	staleAfter       = 30 * 24 * time.Hour
	rejectedTTL      = 30 * 24 * time.Hour
	agingAfter       = 180 * 24 * time.Hour // untouched decisions/learnings deserve review
	maxMergeRetries  = 128                  // unresolved retries per target, not a lifetime recapture limit
)

// Options tunes a Doctor run.
type Options struct {
	DryRun bool
	Now    time.Time
}

// Action is something Doctor did (or, in a dry run, would do).
type Action struct {
	Kind    string `json:"kind"` // promoted|merged|superseded|rejected|retired|recovered|pruned
	ID      string `json:"id"`
	Project string `json:"project,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Flag is something a human (or the next session) should look at. Flags are
// recomputed from the vault on every run, so they cannot go stale.
type Flag struct {
	Kind    string   `json:"kind"` // conflict|overlap|stale_open|cap_exceeded|lint|invalid_note|foreign_file|unregistered_project
	Project string   `json:"project,omitempty"`
	IDs     []string `json:"ids,omitempty"`
	Detail  string   `json:"detail"`
}

// Report is the outcome of one run.
type Report struct {
	DryRun  bool           `json:"dry_run"`
	Inbox   int            `json:"inbox_seen"`
	Actions []Action       `json:"actions"`
	Flags   []Flag         `json:"flags"`
	Live    map[string]int `json:"live_notes_by_project"`
	Backup  string         `json:"pre_run_snapshot,omitempty"`
}

// Changed reports whether the run modified (or would modify) the vault.
func (r *Report) Changed() bool { return len(r.Actions) > 0 }

// Human renders the report for terminals.
func (r *Report) Human() string {
	var b strings.Builder
	mode := ""
	if r.DryRun {
		mode = " (dry run — nothing written)"
	}
	fmt.Fprintf(&b, "brain doctor%s: %d proposal(s) seen, %d action(s), %d flag(s)\n", mode, r.Inbox, len(r.Actions), len(r.Flags))
	for _, a := range r.Actions {
		fmt.Fprintf(&b, "  %-10s %s", a.Kind, a.ID)
		if a.Project != "" {
			fmt.Fprintf(&b, " [%s]", a.Project)
		}
		if a.Detail != "" {
			fmt.Fprintf(&b, " — %s", a.Detail)
		}
		b.WriteString("\n")
	}
	for _, f := range r.Flags {
		fmt.Fprintf(&b, "  ! %-12s", f.Kind)
		if f.Project != "" {
			fmt.Fprintf(&b, " [%s]", f.Project)
		}
		fmt.Fprintf(&b, " %s", f.Detail)
		if len(f.IDs) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(f.IDs, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

type doctor struct {
	v    *Vault
	dry  bool
	now  time.Time
	r    *Report
	live []*Note
	// backedUp records that this run already took its pre-mutation snapshot.
	backedUp bool
}

// ensureBackup takes the pre-mutation snapshot once, before the first change
// of a run (backup-first maintenance). A failed snapshot is surfaced as a
// flag; hygiene continues because the archive keeps every retired note anyway.
func (d *doctor) ensureBackup() {
	if d.dry || d.backedUp {
		return
	}
	d.backedUp = true
	path, err := d.v.Backup("doctor", d.now)
	if err != nil {
		d.flag("backup_failed", "", "pre-run snapshot failed: "+err.Error())
		return
	}
	d.r.Backup = filepath.Base(path)
}

func hashNote(n *Note) string {
	data, err := n.Marshal()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func (d *doctor) act(kind, id, project, detail string) {
	d.r.Actions = append(d.r.Actions, Action{Kind: kind, ID: id, Project: project, Detail: detail})
}

func (d *doctor) flag(kind, project, detail string, ids ...string) {
	d.r.Flags = append(d.r.Flags, Flag{Kind: kind, Project: project, IDs: ids, Detail: detail})
}

func (d *doctor) ts() string { return d.now.Format(time.RFC3339) }

// Doctor runs one maintenance pass under the vault lock.
func (v *Vault) Doctor(opt Options) (*Report, error) {
	if err := v.require(); err != nil {
		return nil, err
	}
	if !opt.DryRun {
		if err := v.writable(); err != nil {
			return nil, err
		}
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if !opt.DryRun {
		unlock, err := v.lock()
		if err != nil {
			return nil, err
		}
		defer unlock()
		if err := v.writable(); err != nil {
			return nil, err
		}
	}

	d := &doctor{v: v, dry: opt.DryRun, now: opt.Now.UTC(),
		r: &Report{DryRun: opt.DryRun, Live: map[string]int{}}}
	live, bad, err := v.scanLive()
	if err != nil {
		return nil, err
	}
	d.live = live
	for _, b := range bad {
		rel, _ := filepath.Rel(v.Root, b.Path)
		d.flag("invalid_note", "", fmt.Sprintf("%s: %s", rel, b.Err))
	}

	d.reconcileSupersessions()
	d.processInbox()
	d.retainSummaries()
	d.audit()
	d.pruneRejected()
	if !d.dry {
		for _, a := range d.r.Actions {
			v.appendLog(d.now, a.Kind, strings.TrimSpace(a.ID+" ["+a.Project+"] "+a.Detail))
		}
	}
	if err := d.writeIndex(); err != nil {
		d.flag("maintenance_failed", "", "index update failed; completed note actions are retained")
		return d.r, err
	}
	return d.r, nil
}

// ---- inbox → live ----------------------------------------------------------

func (d *doctor) processInbox() {
	entries, err := readDirBounded(d.v.Root, d.v.inboxDir())
	if err != nil {
		d.flag("maintenance_failed", "", "inbox unavailable; proposals preserved")
		return
	}
	type prop struct {
		path string
		n    *Note
		err  error
	}
	var props []prop
	deadline := time.Now().Add(scanDuration)
	total := int64(0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(d.v.inboxDir(), e.Name())
		if st, err := e.Info(); err == nil && st.Mode().IsRegular() {
			total += st.Size()
		}
		if len(props) >= 128 || total > maxScanBytes || time.Now().After(deadline) {
			d.flag("maintenance_budget", "", "inbox scan budget reached; remaining proposals preserved for a later pass")
			break
		}
		n, err := d.v.readNote(p)
		props = append(props, prop{p, n, err})
	}
	sort.SliceStable(props, func(i, j int) bool {
		ci, cj := "", ""
		if props[i].n != nil {
			ci = props[i].n.Created
		}
		if props[j].n != nil {
			cj = props[j].n.Created
		}
		ti, _ := time.Parse(time.RFC3339Nano, ci)
		tj, _ := time.Parse(time.RFC3339Nano, cj)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return props[i].path < props[j].path
	})
	d.r.Inbox = len(props)
	if len(props) > 0 {
		d.ensureBackup()
	}
	for _, p := range props {
		if time.Now().After(deadline) {
			d.flag("maintenance_budget", "", "promotion time budget reached; remaining proposals preserved for a later pass")
			break
		}
		if p.err != nil {
			d.rejectRaw(p.path, "invalid_note: "+p.err.Error())
			continue
		}
		d.handle(p.path, p.n)
	}
}

func (d *doctor) handle(path string, n *Note) {
	// Crash recovery: a previous run wrote the promoted note but died before
	// removing the proposal. The promotion is done; finish the cleanup.
	for _, target := range []string{d.v.notePath(n), d.v.archivePath(n)} {
		if prior, err := d.v.readNote(target); err == nil {
			if prior.ID != n.ID || !sameClaim(prior, n) {
				d.flag("mutation_failed", n.Project, "ID collision with different content; proposal preserved", n.ID)
				return
			}
			if prior.Status == StatusMerged {
				if prior.MergeApplied {
					if d.remove(path) {
						d.act("recovered", n.ID, n.Project, "merge committed; removed leftover proposal")
					}
					return
				}
				for _, into := range d.live {
					if into.ID == prior.MergedInto && into.Project == n.Project {
						d.merge(path, n, into)
						return
					}
				}
				d.flag("mutation_failed", n.Project, "incomplete merge target unavailable; proposal preserved", n.ID)
				return
			}
			if d.remove(path) {
				d.act("recovered", n.ID, n.Project, "already persisted; removed leftover proposal")
			}
			return
		}
	}
	if unsupportedScope(n) {
		d.flag("unsupported_scope", n.Project, "global open/summary proposal preserved; register the intended project and recapture with --project <slug>", n.ID)
		return
	}
	if !d.v.hasProject(n.Project) {
		d.reject(path, n, fmt.Sprintf("project: unknown project %q", n.Project))
		return
	}
	if vs := Check(n); len(vs) > 0 {
		parts := make([]string, len(vs))
		for i, v := range vs {
			parts[i] = v.String()
		}
		d.reject(path, n, strings.Join(parts, "; "))
		return
	}

	var peers []*Note
	for _, e := range d.live {
		if e.Project == n.Project && e.Type == n.Type {
			peers = append(peers, e)
		}
	}

	// Decisions with a topic are versioned by topic: a differing statement
	// replaces the old decision (history kept in archive/) — unless the old
	// one was human-validated, in which case both stay and audit flags a
	// conflict instead of silently overwriting (anti memory-poisoning).
	if n.Type == TypeDecision && n.Topic != "" {
		var same []*Note
		for _, e := range peers {
			if e.Topic == n.Topic {
				same = append(same, e)
			}
		}
		for _, e := range same {
			if sameClaim(n, e) {
				d.merge(path, n, e)
				return
			}
		}
		var olds []*Note
		for _, e := range same {
			if e.Confidence != ConfValidated && capturedTime(n).After(capturedTime(e)) {
				olds = append(olds, e)
				n.Supersedes = append(n.Supersedes, e.ID)
			}
		}
		// Promote first: if the new note cannot be written, the old decision
		// must stay live rather than vanish into the archive.
		if d.promote(path, n) {
			for _, e := range olds {
				d.supersede(e, n)
			}
		}
		return
	}

	// A summary is the latest state of the work, not a claim to confirm: a
	// reworded newer one must replace the old view (retention retires the
	// old), never be dropped for looking similar.
	if n.Type == TypeSummary {
		d.promote(path, n)
		return
	}

	var best *Note
	bestSim := 0.0
	for _, e := range peers {
		if !sameClaim(n, e) {
			continue // opposite claims are never confirmations; audit flags them
		}
		if s := similarity(n, e); s > bestSim {
			best, bestSim = e, s
		}
	}
	if best != nil {
		d.merge(path, n, best)
		return
	}
	d.promote(path, n)
}

func (d *doctor) promote(path string, n *Note) bool {
	n.Status = n.Type.liveStatus()
	n.Updated = n.Created // promotion is maintenance, not new semantic evidence
	n.LastCaptured = n.Created
	dest := d.v.notePath(n)
	if !d.dry {
		if err := d.v.writeNote(dest, n); err != nil {
			d.flag("invalid_note", n.Project, "promotion failed for "+n.ID+": "+err.Error())
			return false
		}
		d.remove(path)
	}
	d.act("promoted", n.ID, n.Project, fmt.Sprintf("%s → %s sha256:%s", n.Type, filepath.Base(filepath.Dir(dest)), hashNote(n)))
	n.Path = dest
	d.live = append(d.live, n)
	return true
}

func (d *doctor) merge(path string, n, into *Note) {
	updated, archived := *into, *n
	// Successful cleanup has no future retry. Never evict an unresolved ID:
	// if too many cleanups fail, stop merging rather than double-count later.
	updated.RecapturedBy = nil
	applied := false
	for _, id := range into.RecapturedBy {
		if id == n.ID {
			applied = true
		}
		if _, err := os.Lstat(filepath.Join(d.v.inboxDir(), id+".md")); !os.IsNotExist(err) {
			updated.RecapturedBy = append(updated.RecapturedBy, id)
		}
	}
	if !applied {
		if len(updated.RecapturedBy) >= maxMergeRetries {
			d.flag("mutation_failed", n.Project, "merge retry bound reached; resolve pending cleanup before more merges", n.ID)
			return
		}
		updated.Confirmed++
		updated.RecapturedBy = append(updated.RecapturedBy, n.ID)
		if capturedTime(n).After(capturedTime(into)) {
			updated.Updated = n.Created
			updated.LastCaptured = n.Created
		}
	}
	archived.Status, archived.MergedInto, archived.Updated = StatusMerged, into.ID, d.ts()
	if !d.dry {
		// Archive before altering the target; the proposal stays until BOTH writes
		// commit. Applied IDs make a crash between target write and cleanup safe.
		if err := d.v.writeNote(d.v.archivePath(&archived), &archived); err != nil {
			d.flag("mutation_failed", n.Project, "merge archive failed: "+err.Error(), n.ID)
			return
		}
		if err := d.v.writeNote(into.Path, &updated); err != nil {
			d.flag("mutation_failed", n.Project, "merge target failed: "+err.Error(), n.ID)
			return
		}
		*into = updated // committed, even if receipt/cleanup below fails
		archived.MergeApplied = true
		if err := d.v.writeNote(d.v.archivePath(&archived), &archived); err != nil {
			d.flag("mutation_failed", n.Project, "merge receipt failed; proposal retained for retry", n.ID)
			return
		}
	}
	*into = updated
	if !d.remove(path) {
		return
	}
	d.act("merged", n.ID, n.Project, fmt.Sprintf("exact recapture of %s (recaptured ×%d; not independent evidence)", into.ID, into.Confirmed))
}

func (d *doctor) retireLive(old, by *Note, kind, detail string) bool {
	retired := *old
	retired.Status, retired.SupersededBy, retired.Updated = StatusSuperseded, by.ID, d.ts()
	if !d.dry {
		if err := d.v.writeNote(d.v.archivePath(&retired), &retired); err != nil {
			d.flag("mutation_failed", old.Project, "retirement failed; will retry: "+err.Error(), old.ID)
			return false
		}
		if !d.remove(old.Path) {
			return false
		}
	}
	d.dropLive(old)
	d.act(kind, old.ID, old.Project, detail)
	return true
}

func (d *doctor) supersede(old, by *Note) {
	d.retireLive(old, by, "superseded", "replaced by "+by.ID+" (same topic "+by.Topic+")")
}

func (d *doctor) reconcileSupersessions() {
	// A promotion can commit before retirement fails or the process stops.
	// Persisted lineage is an intent, not proof of a completed retirement.
	for _, by := range append([]*Note{}, d.live...) {
		for _, id := range by.Supersedes {
			for _, old := range append([]*Note{}, d.live...) {
				if old.ID == id && old.Project == by.Project && old.Type == TypeDecision && by.Type == TypeDecision && old.Topic != "" && old.Topic == by.Topic && old.Confidence != ConfValidated && capturedTime(by).After(capturedTime(old)) {
					d.ensureBackup()
					d.supersede(old, by)
				}
			}
		}
	}
}

func (d *doctor) dropLive(n *Note) {
	out := d.live[:0]
	for _, e := range d.live {
		if e != n {
			out = append(out, e)
		}
	}
	d.live = out
}

func (d *doctor) remove(path string) bool {
	if !d.dry {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			d.flag("mutation_failed", "", "cleanup failed; retry required: "+err.Error())
			return false
		}
	}
	return true
}

func (d *doctor) reject(path string, n *Note, reason string) {
	if !d.dry {
		rejected := *n
		rejected.Status, rejected.Reason, rejected.Updated = StatusRejected, reason, d.ts()
		if err := d.v.writeNote(filepath.Join(d.v.rejectedDir(), filepath.Base(path)), &rejected); err != nil {
			d.flag("mutation_failed", n.Project, "rejection failed; proposal preserved: "+err.Error(), n.ID)
			return
		}
		if !d.remove(path) {
			return
		}
	}
	d.act("rejected", n.ID, n.Project, reason)
}

func (d *doctor) rejectRaw(path, reason string) {
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	if !d.dry {
		raw, err := d.v.readFile(path)
		if err != nil {
			d.flag("mutation_failed", "", "cannot safely read rejected proposal: "+err.Error(), name)
			return
		}
		body := "<!-- rejected: " + strings.ReplaceAll(reason, "-->", "--") + " -->\n" + string(raw)
		if err := writeAtomic(filepath.Join(d.v.rejectedDir(), filepath.Base(path)), []byte(body)); err != nil {
			d.flag("mutation_failed", "", "rejection failed; proposal preserved: "+err.Error(), name)
			return
		}
		if !d.remove(path) {
			return
		}
	}
	d.act("rejected", name, "", reason)
}

// ---- retention ------------------------------------------------------------

// retainSummaries keeps the newest summaries per project: a summary is the
// latest state, so older ones are superseded by definition.
func (d *doctor) retainSummaries() {
	byProject := map[string][]*Note{}
	for _, n := range d.live {
		if n.Type == TypeSummary && !unsupportedScope(n) {
			byProject[n.Project] = append(byProject[n.Project], n)
		}
	}
	for _, list := range byProject {
		sort.SliceStable(list, func(i, j int) bool {
			if !createdTime(list[i]).Equal(createdTime(list[j])) {
				return createdTime(list[i]).After(createdTime(list[j]))
			}
			return list[i].ID > list[j].ID
		})
		for _, old := range list[min(keepSummaries, len(list)):] {
			d.ensureBackup()
			d.retireLive(old, list[0], "retired", "summary retention (keep newest "+fmt.Sprint(keepSummaries)+")")
		}
	}
}

func (d *doctor) pruneRejected() {
	entries, err := readDirBounded(d.v.Root, d.v.rejectedDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		if d.now.Sub(info.ModTime()) > rejectedTTL {
			if d.remove(filepath.Join(d.v.rejectedDir(), e.Name())) {
				d.act("pruned", strings.TrimSuffix(e.Name(), ".md"), "", "rejected proposal older than 30 days")
			}
		}
	}
}

// ---- audit (stateless flags) ----------------------------------------------

func (d *doctor) auditNotes() {
	byProject := map[string][]*Note{}
	for _, n := range d.live {
		byProject[n.Project] = append(byProject[n.Project], n)
		d.r.Live[n.Project]++
	}
	projects := make([]string, 0, len(byProject))
	for p := range byProject {
		projects = append(projects, p)
	}
	sort.Strings(projects)

	for _, p := range projects {
		notes := byProject[p]
		if len(notes) > maxActive {
			d.flag("cap_exceeded", p, fmt.Sprintf("%d live notes (> %d): archive what no longer matters or agents lose the signal", len(notes), maxActive))
		}

		var stale []string
		topics := map[string][]string{}
		var decisions []*Note
		for _, n := range notes {
			if unsupportedScope(n) {
				d.flag("unsupported_scope", n.Project, "global open/summary is preserved but never broadcast; recapture under the intended registered project", n.ID)
				continue // no note-derived warning may broadcast this project state
			}
			if n.Type == TypeOpen && d.now.Sub(lastTouched(n)) > staleAfter {
				stale = append(stale, n.ID)
			}
			if n.Type == TypeDecision {
				decisions = append(decisions, n)
				if n.Topic != "" {
					topics[n.Topic] = append(topics[n.Topic], n.ID)
				}
			}
			if vs := Check(n); unsafe(vs) {
				codes := make([]string, 0, len(vs))
				for _, v := range vs {
					if v.Safety {
						codes = append(codes, v.Code)
					}
				}
				d.flag("lint", p, "live note violates the brain gate ("+strings.Join(codes, ",")+"); excluded from session briefings", n.ID)
			}
		}
		var aging []string
		for _, n := range notes {
			if (n.Type == TypeDecision || n.Type == TypeLearning) && d.now.Sub(lastTouched(n)) > agingAfter {
				aging = append(aging, n.ID)
			}
		}
		if len(aging) > 0 {
			d.flag("aging", p, "decisions/learnings untouched for >180 days — verify against evidence or explicitly archive; recapture is not independent confirmation", aging...)
		}
		if len(stale) > 0 {
			d.flag("stale_open", p, "open items untouched for >30 days — verify, update or mark done", stale...)
		}
		var topicKeys []string
		for t := range topics {
			topicKeys = append(topicKeys, t)
		}
		sort.Strings(topicKeys)
		for _, t := range topicKeys {
			if len(topics[t]) > 1 {
				d.flag("conflict", p, fmt.Sprintf("multiple active decisions share topic %q; verify chronology/validation and resolve by archiving one", t), topics[t]...)
			}
		}
		reported := 0
		for i := 0; i < len(decisions) && reported < 5; i++ {
			for j := i + 1; j < len(decisions) && reported < 5; j++ {
				a, b := decisions[i], decisions[j]
				if a.Topic != "" && a.Topic == b.Topic {
					continue
				}
				if s := similarity(a, b); s >= overlapThreshold && !sameClaim(a, b) {
					kind := "possible contradiction or duplicate"
					if contradicts(a, b) {
						kind = "one negates the other — likely a contradiction"
					}
					d.flag("overlap", p, fmt.Sprintf("decisions overlap (similarity %.2f): %s", s, kind), a.ID, b.ID)
					reported++
				}
			}
		}
	}

}

func (d *doctor) audit() {
	d.auditNotes()
	d.auditRefs()
	d.auditIdentity()
	for _, f := range d.v.foreignFiles() {
		d.flag("foreign_file", "", "non-markdown file in the brain vault (code belongs in project repos): "+f)
	}
	if entries, err := os.ReadDir(d.v.projectsDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != GlobalProject && !d.v.hasProject(e.Name()) {
				d.flag("unregistered_project", e.Name(), "directory has no project.yaml; notes there are not injected until registered")
			}
		}
	}
}

func createdTime(n *Note) time.Time { t, _ := time.Parse(time.RFC3339Nano, n.Created); return t }

// Only capture/recapture advances semantic freshness. For older live notes,
// Updated was the recapture instant when a recapture count was present.
func capturedTime(n *Note) time.Time {
	t := createdTime(n)
	s := n.LastCaptured
	if s == "" && n.Confirmed > 0 && (n.Status == StatusActive || n.Status == StatusOpen) {
		s = n.Updated
	}
	if newer, err := time.Parse(time.RFC3339Nano, s); err == nil && newer.After(t) {
		return newer
	}
	return t
}

func lastTouched(n *Note) time.Time {
	for _, s := range []string{n.Updated, n.Created} {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Now()
}

// ---- index (MOC) -----------------------------------------------------------

// writeIndex regenerates the map of content. It is rebuildable, carries no
// timestamp (no diff noise), and lists titles only.
func (d *doctor) writeIndex() error {
	projects, err := d.v.Projects()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Brain index\n\n<!-- generated by `beme brain doctor`; rebuildable — do not edit -->\n")
	for _, p := range projects {
		var mine []*Note
		for _, n := range d.live {
			if n.Project == p.Slug && !unsafe(Check(n)) {
				mine = append(mine, n)
			}
		}
		if len(mine) == 0 {
			continue
		}
		sort.SliceStable(mine, func(i, j int) bool { return mine[i].ID < mine[j].ID })
		fmt.Fprintf(&b, "\n## %s (`%s`)\n", oneLine(p.Name, 80), p.Slug)
		for _, t := range allTypes {
			first := true
			for _, n := range mine {
				if n.Type != t {
					continue
				}
				if first {
					fmt.Fprintf(&b, "\n**%s**\n\n", t)
					first = false
				}
				fmt.Fprintf(&b, "- [[%s]] %s — %s\n", n.ID, oneLine(n.Title, 100), oneLine(n.Statement, 110))
			}
		}
	}
	if d.dry {
		return nil
	}
	path := filepath.Join(d.v.Root, "index.md")
	if old, err := d.v.readFile(path); err == nil && string(old) == b.String() {
		return nil
	}
	return writeAtomic(path, []byte(b.String()))
}

// auditRefs flags lineage links that point at notes that no longer exist
// (Karpathy lint: broken references).
func (d *doctor) auditRefs() {
	known := map[string]bool{}
	for _, n := range d.live {
		known[n.ID] = true
	}
	for id := range d.v.archivedIDs() {
		known[id] = true
	}
	for _, n := range d.live {
		if unsupportedScope(n) {
			continue
		}
		for _, ref := range append(append([]string{}, n.Supersedes...), n.SupersededBy, n.MergedInto) {
			if ref != "" && !known[ref] {
				d.flag("broken_ref", n.Project, "note references a note that no longer exists: "+ref, n.ID, ref)
			}
		}
	}
}

// auditIdentity checks that the identity card's pointers still resolve. The
// card points at canonical documents and never copies them, so a moved or
// deleted document silently blinds the agent.
func (d *doctor) auditIdentity() {
	if _, withheld := d.v.identityText(identityCap); withheld {
		d.flag("identity_unsafe", "", "identity.md fails the safety screen (secret or instruction-shaped text) and is withheld from briefings")
	}
	for _, p := range d.v.identityPointers() {
		if _, err := os.Stat(p); err != nil {
			d.flag("identity_pointer", "", "identity.md points at a path that does not exist: "+p)
		}
	}
}
