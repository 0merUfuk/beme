package evalrunner_test

import (
	"path/filepath"
	"testing"

	"github.com/0merUfuk/beme/internal/evalrunner"
	fx "github.com/0merUfuk/beme/internal/evalrunner/evalfixture"
)

func TestEveryConfiguredArmReconstructsForGrading(t *testing.T) {
	d := buildFixture(t, fx.Options{})
	c := syntheticCorpus(t, d, 2)
	out := t.TempDir()
	s, err := evalrunner.Run(c, evalrunner.RunConfig{Arms: evalrunner.AllArms, OutputDir: out, Refs: d.Refs()}, evalrunner.NewMockProvider("synthetic-only"), nil, d.Inputs(true))
	if err != nil {
		t.Fatal(err)
	}
	g, err := evalrunner.GradeTemplate(filepath.Join(out, s.RunID), "synthetic-reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Records) != len(evalrunner.AllArms)*2 {
		t.Fatal("configured arm/repeat population lost", len(g.Records))
	}
}
