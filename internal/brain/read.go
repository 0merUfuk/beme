package brain

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxDocumentBytes = 64 << 10
	maxScanEntries   = 4096
	maxScanBytes     = 16 << 20
	scanDuration     = 2 * time.Second
)

// checkPath rejects links in every component below the caller's trusted root.
// This is a cooperative-local guard, not isolation from a hostile same-user
// process concurrently replacing filesystem objects.
func checkPath(root, path string) (fs.FileInfo, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path escapes read scope")
	}
	parts := []string{root}
	if rel != "." {
		p := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			p = filepath.Join(p, part)
			parts = append(parts, p)
		}
	}
	var st fs.FileInfo
	for i, p := range parts {
		st, err = os.Lstat(p)
		if err != nil {
			return nil, err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlinks are not allowed in the vault")
		}
		if i < len(parts)-1 && !st.IsDir() {
			return nil, fmt.Errorf("non-directory path component")
		}
	}
	return st, nil
}

func readRegular(root, path string, limit int64) ([]byte, error) {
	before, err := checkPath(root, path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("non-regular document refused")
	}
	if before.Size() > limit {
		return nil, fmt.Errorf("document exceeds %d-byte limit", limit)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, fmt.Errorf("document changed during read")
	}
	if _, err := checkPath(root, path); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("document exceeds %d-byte limit", limit)
	}
	return data, nil
}

func (v *Vault) readFile(path string) ([]byte, error) {
	return readRegular(v.Root, path, maxDocumentBytes)
}
func (v *Vault) readNote(path string) (*Note, error) {
	data, err := v.readFile(path)
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

// walkBounded is for operator/maintenance scans, never read-time scope selection.
func walkBounded(root string, visit fs.WalkDirFunc) error {
	deadline := time.Now().Add(scanDuration)
	count := 0
	var walk func(string, int) error
	walk = func(path string, depth int) error {
		if depth > 32 || time.Now().After(deadline) {
			return fmt.Errorf("vault scan depth/time limit reached")
		}
		entries, err := readDirBounded(root, path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			count++
			if count > maxScanEntries || time.Now().After(deadline) {
				return fmt.Errorf("vault scan entry/time limit reached")
			}
			p := filepath.Join(path, e.Name())
			err := visit(p, e, nil)
			if err == filepath.SkipDir {
				continue
			}
			if err != nil {
				return err
			}
			if e.IsDir() {
				if err := walk(p, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(root, 0)
}

// Bounded enumeration happens before sorting: ReadDir(-1) itself is unbounded.
func readDirBounded(root, path string) ([]fs.DirEntry, error) {
	st, err := checkPath(root, path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("not a directory")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	es, err := f.ReadDir(maxScanEntries + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(es) > maxScanEntries {
		return nil, fmt.Errorf("directory exceeds %d-entry scan limit", maxScanEntries)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].Name() < es[j].Name() })
	return es, nil
}
