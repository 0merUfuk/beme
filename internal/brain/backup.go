package brain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Backup-first maintenance (nanobrain snapshots, open-second-brain
// pre-mutation snapshots, Avenox backup-before-update): Doctor snapshots the
// vault before its first mutation of a run, and `restore` is itself preceded
// by a snapshot — so every automated change and every restore is reversible.
//
// Snapshots are tar.gz files with a SHA-256 sidecar, stored in a sibling
// directory (<vault>.backups) so they never enter the vault, its git history,
// or a briefing. The vault's .git, lock, freeze flag and machine-local state
// are not part of a snapshot.
const (
	backupKeep     = 10
	restoreMaxFile = 16 << 20
	restoreMaxAll  = 128 << 20
)

// BackupInfo describes one snapshot.
type BackupInfo struct {
	Path   string    `json:"path"`
	Time   time.Time `json:"time"`
	Bytes  int64     `json:"bytes"`
	SHA256 string    `json:"sha256"`
}

func (v *Vault) backupDir() string { return filepath.Clean(v.Root) + ".backups" }

func excludedFromBackup(rel string) bool {
	top := strings.Split(filepath.ToSlash(rel), "/")[0]
	switch top {
	case ".git", ".lock", ".frozen", ".state":
		return true
	}
	return strings.HasPrefix(filepath.Base(rel), ".tmp-")
}

// Backup writes a verified snapshot and prunes old ones.
func (v *Vault) Backup(reason string, now time.Time) (string, error) {
	if err := v.require(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(v.backupDir(), 0o700); err != nil {
		return "", err
	}
	base := fmt.Sprintf("brain-%s-%s", now.UTC().Format("20060102T150405Z"), slugify(reason, 20))
	name := base + ".tar.gz"
	path := filepath.Join(v.backupDir(), name)
	// Never overwrite an earlier snapshot: two runs inside one second (start
	// and end hooks) each keep their own pre-mutation state.
	for i := 2; ; i++ {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s-%d.tar.gz", base, i)
		path = filepath.Join(v.backupDir(), name)
	}
	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(f, h))
	tw := tar.NewWriter(gz)
	deadline := time.Now().Add(scanDuration)
	var total int64
	werr := walkBounded(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(v.Root, p)
		if rel == "." {
			return nil
		}
		if excludedFromBackup(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil // never archive links
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: filepath.ToSlash(rel), ModTime: info.ModTime(), Mode: 0o600}
		if d.IsDir() {
			hdr.Typeflag, hdr.Name, hdr.Mode = tar.TypeDir, hdr.Name+"/", 0o700
			return tw.WriteHeader(hdr)
		}
		if !info.Mode().IsRegular() {
			return nil
		} // never open FIFOs/devices/sockets
		if info.Size() > restoreMaxFile || total+info.Size() > restoreMaxAll || time.Now().After(deadline) {
			return fmt.Errorf("snapshot size/time budget exceeded; no snapshot published")
		}
		data, err := readRegular(v.Root, p, restoreMaxFile)
		if err != nil {
			return err
		}
		total += int64(len(data))
		hdr.Typeflag, hdr.Size = tar.TypeReg, int64(len(data))
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if werr == nil {
		werr = tw.Close()
	}
	if werr == nil {
		werr = gz.Close()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp)
		return "", werr
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.WriteFile(path+".sha256", []byte(sum+"  "+name+"\n"), 0o600); err != nil {
		return "", err
	}
	v.pruneBackups()
	if !v.Frozen() {
		v.appendLog(now, "backup", name)
	}
	return path, nil
}

func (v *Vault) pruneBackups() {
	list, _ := v.ListBackups()
	for i := backupKeep; i < len(list); i++ { // newest first
		os.Remove(list[i].Path)
		os.Remove(list[i].Path + ".sha256")
	}
}

// ListBackups returns snapshots, newest first.
func (v *Vault) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(v.backupDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []BackupInfo
	mtimes := map[string]time.Time{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(v.backupDir(), e.Name())
		bi := BackupInfo{Path: p, Bytes: info.Size(), Time: info.ModTime()}
		mtimes[p] = info.ModTime()
		if data, err := readRegular(v.backupDir(), p+".sha256", 4096); err == nil {
			if f := strings.Fields(string(data)); len(f) > 0 { // a corrupt sidecar must not crash listing
				bi.SHA256 = f[0]
			}
		}
		// The name carries the snapshot time (sortable, independent of mtime).
		if len(e.Name()) >= 21 {
			if t, err := time.Parse("20060102T150405Z", e.Name()[6:22]); err == nil {
				bi.Time = t
			}
		}
		out = append(out, bi)
	}
	// Newest first: snapshot time from the name, creation order within a second.
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return mtimes[out[i].Path].After(mtimes[out[j].Path])
	})
	return out, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Restore replaces the vault content with a verified snapshot. The snapshot's
// SHA-256 sidecar is mandatory; a non-empty vault requires force and is
// snapshotted first, so a restore can itself be undone. Extraction refuses
// absolute paths, traversal, links, and oversized entries.
func (v *Vault) Restore(snapshot string, force bool, now time.Time) error {
	side, err := readRegular(filepath.Dir(snapshot), snapshot+".sha256", 4096)
	if err != nil {
		return fmt.Errorf("snapshot has no .sha256 sidecar; refusing to restore an unverified archive")
	}
	want := strings.Fields(string(side))
	archive, err := readRegular(filepath.Dir(snapshot), snapshot, restoreMaxAll)
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	if err != nil {
		return err
	}
	if len(want) == 0 || want[0] != got {
		return fmt.Errorf("snapshot checksum mismatch (archive corrupted or altered)")
	}

	// Stage the ENTIRE input before backup retention can prune it and before
	// changing the live vault. Validation/extraction failures leave it intact.
	if err := os.MkdirAll(filepath.Dir(v.Root), 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(v.Root), ".brain-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractSnapshot(archive, stage); err != nil {
		return err
	}
	staged := Open(stage)
	if !staged.Initialized() || staged.checkSchema() != nil {
		return fmt.Errorf("snapshot is not a supported brain vault")
	}
	if err := v.checkSchema(); err != nil {
		return err
	}
	entries, err := os.ReadDir(v.Root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("vault is not empty: pass --force (a pre-restore snapshot is taken first)")
	}
	if err := os.MkdirAll(v.Root, 0o700); err != nil {
		return err
	}
	unlock, err := v.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if len(entries) > 0 {
		if !v.Initialized() {
			return fmt.Errorf("non-empty target is not a brain vault; refusing to replace it")
		}
		if _, err := v.Backup("pre-restore", now); err != nil {
			return fmt.Errorf("pre-restore snapshot failed: %w", err)
		}
	}
	if err := v.installRestore(stage, os.Rename); err != nil {
		return err
	}
	v.appendLog(now, "restore", filepath.Base(snapshot))
	return nil
}

// installRestore preserves the old tree until every staged entry is installed.
// A failed install rolls back; if rollback itself fails its directory is kept
// and named in the error, never silently deleted. The rename seam is only for
// deterministic filesystem-failure testing.
func (v *Vault) installRestore(stage string, rename func(string, string) error) error {
	old, err := os.MkdirTemp(filepath.Dir(v.Root), ".brain-rollback-*")
	if err != nil {
		return err
	}
	var saved, installed []string
	rollback := func(cause error) error {
		var failures []error
		for i := len(installed) - 1; i >= 0; i-- {
			name := installed[i]
			if err := rename(filepath.Join(v.Root, name), filepath.Join(stage, name)); err != nil {
				failures = append(failures, err)
			}
		}
		for i := len(saved) - 1; i >= 0; i-- {
			name := saved[i]
			if err := rename(filepath.Join(old, name), filepath.Join(v.Root, name)); err != nil {
				failures = append(failures, err)
			}
		}
		if len(failures) != 0 {
			return fmt.Errorf("restore failed: %w; rollback incomplete, preserved at %s: %v", cause, old, errors.Join(failures...))
		}
		os.Remove(old)
		return fmt.Errorf("restore failed (previous vault restored): %w", cause)
	}
	entries, err := os.ReadDir(v.Root)
	if err != nil {
		return rollback(err)
	}
	for _, e := range entries {
		if excludedFromBackup(e.Name()) {
			continue
		}
		if err := rename(filepath.Join(v.Root, e.Name()), filepath.Join(old, e.Name())); err != nil {
			return rollback(err)
		}
		saved = append(saved, e.Name())
	}
	entries, err = os.ReadDir(stage)
	if err != nil {
		return rollback(err)
	}
	for _, e := range entries {
		if err := rename(filepath.Join(stage, e.Name()), filepath.Join(v.Root, e.Name())); err != nil {
			return rollback(err)
		}
		installed = append(installed, e.Name())
	}
	if err := os.RemoveAll(old); err != nil {
		return fmt.Errorf("restore installed; prior tree retained at %s: %w", old, err)
	}
	return nil
}

func extractSnapshot(archive []byte, stage string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	entries := 0
	seen := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(hdr.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || excludedFromBackup(name) {
			return fmt.Errorf("snapshot contains a forbidden path %q", hdr.Name)
		}
		entries++
		if name == "." || seen[name] || entries > 10000 {
			return fmt.Errorf("snapshot has duplicate, invalid or excessive entries")
		}
		seen[name] = true
		dest := filepath.Join(stage, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if hdr.Size < 0 || hdr.Size > restoreMaxFile {
				return fmt.Errorf("snapshot entry %q too large", hdr.Name)
			}
			if total += hdr.Size; total > restoreMaxAll {
				return fmt.Errorf("snapshot too large")
			}
			data, err := io.ReadAll(io.LimitReader(tr, hdr.Size))
			if err != nil {
				return err
			}
			if int64(len(data)) != hdr.Size {
				return io.ErrUnexpectedEOF
			}
			if err := writeAtomic(dest, data); err != nil {
				return err
			}
		default:
			return fmt.Errorf("snapshot contains a non-regular entry %q (links are refused)", hdr.Name)
		}
	}
	// Tar EOF alone does not check the gzip trailer.
	_, err = io.Copy(io.Discard, io.LimitReader(gz, restoreMaxAll+1))
	return err
}
