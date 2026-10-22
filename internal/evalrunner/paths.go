package evalrunner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// bundleFilePath crosses from a canonical, slash-separated wire identifier to
// an OS-native filename. Validate before joining: cleaning or replacing
// separators here would turn malformed evidence into an alias of valid data.
func bundleFilePath(root, name string) (string, error) {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00") {
		return "", fmt.Errorf("unsafe bundle path")
	}
	return filepath.Join(root, filepath.FromSlash(name)), nil
}

// CanonicalOutputPath resolves existing ancestors, including symlinks, before
// appending the not-yet-created suffix. It does not create directories.
func CanonicalOutputPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	suffix := []string{}
	p := abs
	for {
		_, err := os.Lstat(p)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		suffix = append(suffix, filepath.Base(p))
		p = parent
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		p = filepath.Join(p, suffix[i])
	}
	return p, nil
}
func CheckOutputOutsideBundle(output, bundle string) error {
	out, err := CanonicalOutputPath(output)
	if err != nil {
		return err
	}
	root, err := CanonicalOutputPath(bundle)
	if err != nil {
		return err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("bundle must be a directory")
	}
	// EvalSymlinks need not canonicalize case on a case-insensitive filesystem.
	// Compare each existing ancestor's identity instead of path spellings. Walk
	// past absent output suffixes without creating them or weakening the guard.
	for p := out; ; p = filepath.Dir(p) {
		info, err := os.Stat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && os.SameFile(rootInfo, info) {
			return fmt.Errorf("output must be outside immutable bundle")
		}
		if filepath.Dir(p) == p {
			return nil
		}
	}
}
