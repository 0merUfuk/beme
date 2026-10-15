package main

import (
	"flag"
	"regexp"
	"testing"
)

func TestDefaultEvaluationCapabilityIsSchemaConformant(t *testing.T) {
	var c common
	c.register(flag.NewFlagSet("synthetic", flag.ContinueOnError))
	if !regexp.MustCompile(`^cap_[0-9a-z]{8,}$`).MatchString(c.capability) {
		t.Fatalf("default evaluator emits a nonconforming ContextPack capability: %q", c.capability)
	}
}
