package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGradeOutputsCannotUseBundleDirectoryAlias(t *testing.T) {
	for _, command := range []string{"grade-template", "reviewer-packet", "grade-import"} {
		t.Run(command, func(t *testing.T) {
			// Independent bundles ensure a defective command cannot invalidate
			// the next command's positive input control.
			bundle, grades := syntheticGrades(t)
			gradeFile := writeGrade(t, grades)
			alias := filepath.Join(filepath.Dir(bundle), strings.ToUpper(filepath.Base(bundle)))
			info, err := os.Stat(alias)
			if os.IsNotExist(err) {
				// Case-sensitive filesystems still exercise an aliased pathname;
				// unit coverage also proves distinct-case siblings stay allowed.
				// Do not skip a portable boundary test on any OS.
				alias = bundle + string(filepath.Separator) + "."
				info, err = os.Stat(alias)
			}
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.Stat(bundle)
			if err != nil || !os.SameFile(root, info) {
				t.Fatalf("fixture alias must identify the bundle: %v", err)
			}
			target := filepath.Join(alias, command+"-must-not-exist")
			args := []string{command, "--bundle", bundle, "--out", target}
			if command == "grade-import" {
				args = append(args, "--grades", gradeFile)
			} else {
				args = append(args, "--evaluator", "synthetic-reviewer")
			}
			code, _, raw := runJSON(t, args...)
			if code != 2 || !strings.Contains(raw, "outside immutable bundle") {
				t.Errorf("aliased output was not refused at boundary: %d %s", code, raw)
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Errorf("refused output was created: %v", err)
			}
		})
	}
}
