package evalrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	rel, err := filepath.Rel(root, out)
	if err != nil {
		return err
	}
	if rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("output must be outside immutable bundle")
	}
	return nil
}
