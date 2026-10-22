package brain

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Vault layout (all plain files; Obsidian-readable, git-diffable):
//
//	identity.md                          agent identity card + canonical pointers (operator-owned)
//	index.md                             generated map of content (rebuildable)
//	inbox/<id>.md                        proposals awaiting Doctor
//	inbox/rejected/<id>.md               refused proposals (auto-pruned)
//	projects/<slug>/project.yaml         project registration (roots)
//	projects/<slug>/{decisions,open,summaries,learnings}/<id>.md   live notes
//	archive/<slug>/<id>.md               retired notes (superseded/merged/done/archived)
type Vault struct {
	Root string
}

// ErrNotInitialized means the vault directory has not been created by `init`.
var ErrNotInitialized = errors.New("brain not initialized (run: beme brain init)")

// ErrLocked means another process is running maintenance on the vault.
var ErrLocked = errors.New("brain vault is locked by another process")

// ErrFrozen means the operator froze the vault (incident response: a
// suspected poisoned or misbehaving writer). Reads still work.
var ErrFrozen = errors.New("brain is frozen (read-only): run `beme brain freeze off` to resume writes")

// SchemaVersion is the vault format this binary reads and writes. A vault
// written by a newer binary is refused for writes so an old binary can never
// corrupt it.
const SchemaVersion = 1

// Open wraps a vault root. It never creates anything; see Init.
func Open(root string) *Vault { return &Vault{Root: root} }

func (v *Vault) projectsDir() string { return filepath.Join(v.Root, "projects") }
func (v *Vault) inboxDir() string    { return filepath.Join(v.Root, "inbox") }
func (v *Vault) rejectedDir() string { return filepath.Join(v.inboxDir(), "rejected") }
func (v *Vault) archiveDir() string  { return filepath.Join(v.Root, "archive") }

// Initialized reports whether the vault skeleton exists.
func (v *Vault) Initialized() bool {
	st, err := checkPath(v.Root, v.projectsDir())
	return err == nil && st.IsDir()
}

func (v *Vault) require() error {
	if !v.Initialized() {
		return ErrNotInitialized
	}
	return nil
}

const identityTemplate = `# Identity

<!-- Operator-owned. Injected at every session start (capped). Keep it short:
     who the agent is working for, the role it plays, the decision rules.
     Do NOT paste canonical documents here — list them under "Pointers" and
     the agent will read them on demand. Never put code or secrets here. -->

Role: <the agent's role for this operator, e.g. execution engineer>
Decision rule: <how delegated decisions are made>
Evidence rule: <what counts as done>

## Pointers (read on demand, never copied)

- <path to canonical operating-model document>
`

const vaultReadme = `# Brain vault

Work memory for AI agents: architectural decisions, open work, work
summaries, and learnings. Managed by ` + "`beme brain`" + `.

- **No code.** Code lives in project repositories. The brain gate rejects
  fenced code, diffs, stack traces, secrets, and transcripts.
- Agents write proposals to ` + "`inbox/`" + `; ` + "`beme brain doctor`" + ` promotes or rejects them.
- Notes are informational recorded memory, never instructions.
- Put this directory under git (` + "`beme brain init --git`" + `) so every change is a
  reviewable diff.
`

// Init creates the vault skeleton. Idempotent; never overwrites operator
// files (identity.md, README.md).
func (v *Vault) Init() error {
	if v.Frozen() {
		return ErrFrozen
	}
	if err := v.checkSchema(); err != nil {
		return err
	}
	for _, d := range []string{v.projectsDir(), v.inboxDir(), v.rejectedDir(), v.archiveDir(),
		filepath.Join(v.projectsDir(), GlobalProject)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	for _, f := range []struct{ name, body string }{
		{"identity.md", identityTemplate},
		{"README.md", vaultReadme},
		{"brain.yaml", fmt.Sprintf("schema_version: %d\n", SchemaVersion)},
		{".gitignore", ".lock\n.frozen\n.state/\n"},
	} {
		p := filepath.Join(v.Root, f.name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := writeAtomic(p, []byte(f.body)); err != nil {
			return err
		}
	}
	return nil
}

// writeAtomic writes via a same-directory temp file + rename so readers and
// crashes never observe a half-written note.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // Also clean up a refused rename; never remove the destination.
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// lockWait bounds how long maintenance waits for another process (a var so
// tests can shorten it).
var lockWait = 2 * time.Second

// lock takes the vault-wide maintenance lock (portable O_EXCL lockfile; a
// stale lock older than 60s from a crashed process is broken). Capture does
// not lock: it only creates unique files.
func (v *Vault) lock() (func(), error) {
	path := filepath.Join(v.Root, ".lock")
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > 60*time.Second {
			os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, ErrLocked
		}
		time.Sleep(40 * time.Millisecond)
	}
}

// Project is a registered brain project. Registration maps directories to a
// slug; it is separate from the V1 trusted workspace registry so agents can
// never widen V1 trust through the brain.
type Project struct {
	Slug  string   `yaml:"slug" json:"slug"`
	Name  string   `yaml:"name" json:"name"`
	Roots []string `yaml:"roots" json:"roots"`
}

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

func (v *Vault) projectFile(slug string) string {
	return filepath.Join(v.projectsDir(), slug, "project.yaml")
}

// Projects lists registered projects (global first).
func (v *Vault) Projects() ([]Project, error) {
	if err := v.require(); err != nil {
		return nil, err
	}
	out := []Project{{Slug: GlobalProject, Name: "Global (cross-project)"}}
	entries, err := readDirBounded(v.Root, v.projectsDir())
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == GlobalProject {
			continue
		}
		data, err := v.readFile(v.projectFile(e.Name()))
		if err != nil {
			continue // directory without registration: Doctor flags it
		}
		var p Project
		if yaml.Unmarshal(data, &p) != nil || p.Slug != e.Name() {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out[1:], func(i, j int) bool { return out[1+i].Slug < out[1+j].Slug })
	return out, nil
}

func (v *Vault) hasProject(slug string) bool {
	if slug == GlobalProject {
		return true
	}
	if !slugRe.MatchString(slug) { // never let a slug reach the filesystem unvalidated
		return false
	}
	_, err := v.readFile(v.projectFile(slug))
	return err == nil
}

// AddProject registers (or extends) a project with one real root path.
func (v *Vault) AddProject(slug, name, root string) (*Project, error) {
	if err := v.writable(); err != nil {
		return nil, err
	}
	if slug == GlobalProject {
		return nil, fmt.Errorf("%q is reserved", GlobalProject)
	}
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("slug must match %s", slugRe)
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("root must be an absolute path")
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("root not resolvable: %w", err)
	}
	unlock, err := v.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := v.writable(); err != nil {
		return nil, err
	}
	p := Project{Slug: slug, Name: name}
	if data, err := v.readFile(v.projectFile(slug)); err == nil {
		_ = yaml.Unmarshal(data, &p)
	}
	if p.Name == "" {
		p.Name = slug
	}
	if name != "" {
		p.Name = name
	}
	for _, r := range p.Roots {
		if r == real {
			return &p, nil
		}
	}
	p.Roots = append(p.Roots, real)
	data, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(v.projectFile(slug), data); err != nil {
		return nil, err
	}
	return &p, nil
}

// ProjectMatch is the outcome of resolving a working directory.
type ProjectMatch struct {
	Slug      string
	Name      string
	Ambiguous bool
	Reason    string
}

// ResolveProject maps a working directory to a registered project by
// longest real-path root match. Git worktrees created outside the main
// checkout resolve through their main repository. Ties between different
// projects fail closed (no project), mirroring V1 workspace identity (FR-014).
func (v *Vault) ResolveProject(cwd string) ProjectMatch {
	if cwd == "" {
		return ProjectMatch{Reason: "no working directory"}
	}
	real, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return ProjectMatch{Reason: "working directory not resolvable"}
	}
	projects, err := v.Projects()
	if err != nil {
		return ProjectMatch{Reason: err.Error()}
	}
	// Explicit roots win over inferred aliases, even when the main checkout
	// has a longer path. Equally specific explicit roots remain ambiguous.
	match := matchRoots(projects, []string{real})
	if match.Slug != "" || match.Ambiguous {
		return match
	}
	return matchRoots(projects, worktreeMainRoot(real))
}

func matchRoots(projects []Project, candidates []string) ProjectMatch {
	bestLen, best := -1, []Project{}
	for _, p := range projects {
		for _, root := range p.Roots {
			for _, c := range candidates {
				if c == root || strings.HasPrefix(c, root+string(filepath.Separator)) {
					if len(root) > bestLen {
						bestLen, best = len(root), []Project{p}
					} else if len(root) == bestLen && best[len(best)-1].Slug != p.Slug {
						best = append(best, p)
					}
				}
			}
		}
	}
	if len(best) == 1 {
		return ProjectMatch{Slug: best[0].Slug, Name: best[0].Name}
	}
	if len(best) > 1 {
		return ProjectMatch{Ambiguous: true, Reason: "multiple projects match equally; failing closed"}
	}
	return ProjectMatch{Reason: "directory is not inside a registered project root"}
}

// worktreeMainRoot accepts only reciprocal git-created metadata. A repository
// .git string alone is an untrusted hint, never a grant of project scope.
func worktreeMainRoot(dir string) []string {
	for i := 0; i < 40; i++ {
		marker := filepath.Join(dir, ".git")
		if st, err := os.Lstat(marker); err == nil {
			if !st.Mode().IsRegular() {
				return nil
			}
			data, err := readRegular(dir, marker, 4096)
			if err != nil {
				return nil
			}
			line := strings.TrimSpace(string(data))
			if !strings.HasPrefix(line, "gitdir: ") || strings.ContainsAny(line, "\r\n") {
				return nil
			}
			gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(dir, gitdir)
			}
			gitdir = filepath.Clean(gitdir)
			common := filepath.Dir(filepath.Dir(gitdir))
			if filepath.Base(filepath.Dir(gitdir)) != "worktrees" || filepath.Base(common) != ".git" {
				return nil
			}
			main := filepath.Dir(common)
			if st, err := checkPath(main, gitdir); err != nil || !st.IsDir() {
				return nil
			}
			cd, err := readRegular(main, filepath.Join(gitdir, "commondir"), 4096)
			if err != nil {
				return nil
			}
			c := strings.TrimSpace(string(cd))
			if !filepath.IsAbs(c) {
				c = filepath.Join(gitdir, c)
			}
			if filepath.Clean(c) != common {
				return nil
			}
			back, err := readRegular(main, filepath.Join(gitdir, "gitdir"), 4096)
			if err != nil {
				return nil
			}
			bp := strings.TrimSpace(string(back))
			if !filepath.IsAbs(bp) {
				bp = filepath.Join(gitdir, bp)
			}
			if filepath.Clean(bp) != marker {
				return nil
			}
			for _, p := range []string{filepath.Join(gitdir, "HEAD"), filepath.Join(common, "HEAD"), filepath.Join(common, "config")} {
				if _, err := readRegular(main, p, maxDocumentBytes); err != nil {
					return nil
				}
			}
			if st, err := checkPath(main, filepath.Join(common, "objects")); err != nil || !st.IsDir() {
				return nil
			}
			return []string{main}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
	return nil
}

// ---- note storage ----------------------------------------------------------

func (v *Vault) liveDir(project string, t Type) string {
	return filepath.Join(v.projectsDir(), project, t.dir())
}

func (v *Vault) notePath(n *Note) string {
	return filepath.Join(v.liveDir(n.Project, n.Type), n.ID+".md")
}

func (v *Vault) archivePath(n *Note) string {
	return filepath.Join(v.archiveDir(), n.Project, n.ID+".md")
}

func (v *Vault) writeNote(path string, n *Note) error {
	data, err := readableNote(n)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// A successful publication must be a valid input to the production reader.
func readableNote(n *Note) ([]byte, error) {
	data, err := n.Marshal()
	if err != nil {
		return nil, err
	}
	if len(data) > maxDocumentBytes {
		return nil, fmt.Errorf("note exceeds %d-byte reader limit", maxDocumentBytes)
	}
	if _, err := ParseNote(data); err != nil {
		return nil, err
	}
	return data, nil
}

func readNote(path string) (*Note, error) {
	data, err := readRegular(filepath.Dir(path), path, maxDocumentBytes)
	if err != nil {
		return nil, err
	}
	n, err := ParseNote(data)
	if err != nil {
		return nil, err
	}
	n.Path = path
	return n, nil
}

// InvalidFile is a refused or unreadable document. Never inject its contents.
type InvalidFile struct {
	Path string
	Err  string
}

func (v *Vault) scanLive() ([]*Note, []InvalidFile, error) { return v.scanScoped(nil) }

// Scope is selected BEFORE opening notes. Nil is the explicit operator-wide
// scan; briefings always supply project+global. Count/byte/time bounds apply
// even to regular documents and enumeration is bounded before allocation.
func (v *Vault) scanScoped(scope []string) ([]*Note, []InvalidFile, error) {
	if err := v.require(); err != nil {
		return nil, nil, err
	}
	var notes []*Note
	var bad []InvalidFile
	if scope == nil {
		ps, err := readDirBounded(v.Root, v.projectsDir())
		if err != nil {
			return nil, nil, err
		}
		for _, p := range ps {
			if p.IsDir() {
				scope = append(scope, p.Name())
			}
		}
	}
	deadline := time.Now().Add(scanDuration)
	count, total := 0, int64(0)
	for _, slug := range scope {
		if !slugOK(slug) {
			continue
		}
		for _, typ := range allTypes {
			dir := v.liveDir(slug, typ)
			files, err := readDirBounded(v.Root, dir)
			if err != nil {
				if !os.IsNotExist(err) {
					bad = append(bad, InvalidFile{dir, err.Error()})
				}
				continue
			}
			for _, f := range files {
				if !strings.HasSuffix(f.Name(), ".md") || strings.HasPrefix(f.Name(), ".") {
					continue
				}
				p := filepath.Join(dir, f.Name())
				count++
				st, err := checkPath(v.Root, p)
				if err == nil && st.Mode().IsRegular() {
					total += st.Size()
				}
				if count > maxScanEntries || total > maxScanBytes || time.Now().After(deadline) {
					return notes, bad, fmt.Errorf("live-note scan limit reached")
				}
				n, err := v.readNote(p)
				if err != nil {
					bad = append(bad, InvalidFile{p, err.Error()})
					continue
				}
				if n.Project != slug || n.Type != typ || n.ID+".md" != f.Name() {
					bad = append(bad, InvalidFile{p, "frontmatter project/type/id disagrees with location"})
					continue
				}
				notes = append(notes, n)
			}
		}
	}
	sort.SliceStable(notes, func(i, j int) bool { return notes[i].ID < notes[j].ID })
	return notes, bad, nil
}

// hasID reports whether a note with this ID exists live or archived (used to
// make an interrupted promotion idempotent).
func (v *Vault) hasID(n *Note) bool {
	if _, err := os.Stat(v.notePath(n)); err == nil {
		return true
	}
	if _, err := os.Stat(v.archivePath(n)); err == nil {
		return true
	}
	return false
}

// foreignFiles lists files that are not part of the brain's document model.
// The vault holds markdown and yaml only: anything else is presumed code and
// violates the domain-separation rule (ADR-033).
func (v *Vault) foreignFiles() []string {
	var out []string
	_ = walkBounded(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".obsidian" || d.Name() == ".state" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(v.Root, p)
			out = append(out, rel+" (symlink)")
			return nil
		}
		switch strings.ToLower(filepath.Ext(d.Name())) {
		case ".md", ".yaml", ".yml":
			return nil
		}
		switch d.Name() {
		case ".gitignore", ".lock", ".frozen":
			return nil
		}
		if strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		rel, _ := filepath.Rel(v.Root, p)
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out
}

// Find locates a live note by ID.
func (v *Vault) Find(id string) (*Note, error) {
	notes, _, err := v.scanLive()
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, fmt.Errorf("note %q not found among live notes", id)
}

// ReadAny returns the raw document for a live, archived, or inbox note.
func (v *Vault) ReadAny(id string) ([]byte, string, error) {
	if err := v.require(); err != nil {
		return nil, "", err
	}
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(id) {
		return nil, "", fmt.Errorf("invalid note id")
	}
	var found string
	_ = walkBounded(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == id+".md" {
			found = p
		}
		return nil
	})
	if found == "" {
		return nil, "", fmt.Errorf("note %q not found", id)
	}
	data, err := v.readFile(found)
	rel, _ := filepath.Rel(v.Root, found)
	return data, rel, err
}

// scanArchive loads retired notes (read-only listing).
func (v *Vault) scanArchive() []*Note {
	var out []*Note
	dirs, err := readDirBounded(v.Root, v.archiveDir())
	if err != nil {
		return nil
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, _ := readDirBounded(v.Root, filepath.Join(v.archiveDir(), d.Name()))
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			if n, err := v.readNote(filepath.Join(v.archiveDir(), d.Name(), f.Name())); err == nil {
				out = append(out, n)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// List returns notes filtered by project and/or type ("" = any), optionally
// including the archive.
func (v *Vault) List(project string, t Type, includeArchive bool) ([]*Note, error) {
	live, _, err := v.scanLive()
	if err != nil {
		return nil, err
	}
	all := live
	if includeArchive {
		all = append(append([]*Note{}, live...), v.scanArchive()...)
	}
	var out []*Note
	for _, n := range all {
		if (project == "" || n.Project == project) && (t == "" || n.Type == t) {
			out = append(out, n)
		}
	}
	return out, nil
}

// ---- schema / freeze / state ------------------------------------------------

func (v *Vault) schemaVersion() int {
	version, _ := v.readSchemaVersion()
	return version // zero means unknown, never permission to write
}

func (v *Vault) readSchemaVersion() (int, error) {
	data, err := v.readFile(filepath.Join(v.Root, "brain.yaml"))
	if os.IsNotExist(err) {
		return SchemaVersion, nil // actual absence: new or legacy vault
	}
	if err != nil {
		return 0, fmt.Errorf("schema marker unavailable: %w", err)
	}
	var m struct {
		Version int `yaml:"schema_version"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil || m.Version < 1 {
		return 0, fmt.Errorf("invalid schema marker")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return 0, fmt.Errorf("schema marker must contain exactly one document")
	}
	return m.Version, nil
}

func (v *Vault) checkSchema() error {
	sv, err := v.readSchemaVersion()
	if err != nil {
		return err
	}
	if sv != SchemaVersion {
		return fmt.Errorf("vault schema v%d is newer than this binary (v%d): upgrade beme before writing", sv, SchemaVersion)
	}
	return nil
}

// Frozen reports whether writes are locked.
func (v *Vault) Frozen() bool {
	_, err := os.Stat(filepath.Join(v.Root, ".frozen"))
	return err == nil
}

// Freeze locks (or unlocks) every writer: capture, Doctor, done/archive.
func (v *Vault) Freeze(on bool, now time.Time) error {
	if err := v.require(); err != nil {
		return err
	}
	if err := v.checkSchema(); err != nil {
		return err
	}
	p := filepath.Join(v.Root, ".frozen")
	if on {
		if err := writeAtomic(p, []byte(now.UTC().Format(time.RFC3339)+"\n")); err != nil {
			return err
		}
		v.appendLog(now, "freeze", "vault frozen (read-only)")
		return nil
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	v.appendLog(now, "unfreeze", "vault writable again")
	return nil
}

// writable gates every mutating operation.
func (v *Vault) writable() error {
	if err := v.require(); err != nil {
		return err
	}
	if v.Frozen() {
		return ErrFrozen
	}
	return v.checkSchema()
}

// archivedIDs lists archived note IDs from file names alone (an ID is its
// file name, enforced when notes are written). Parsing the whole archive on
// every maintenance pass would grow linearly for the life of the vault.
func (v *Vault) archivedIDs() map[string]bool {
	ids := map[string]bool{}
	dirs, err := readDirBounded(v.Root, v.archiveDir())
	if err != nil {
		return ids
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, _ := readDirBounded(v.Root, filepath.Join(v.archiveDir(), d.Name()))
		for _, f := range files {
			if name := f.Name(); strings.HasSuffix(name, ".md") && !strings.HasPrefix(name, ".") {
				ids[strings.TrimSuffix(name, ".md")] = true
			}
		}
	}
	return ids
}
