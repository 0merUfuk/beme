package evalrunner

import (
	"reflect"
	"testing"
)

func TestFamilyBootstrapDoesNotCountRepeatsAsFamilies(t *testing.T) {
	values := []pairedValue{}
	// Two families with perfectly correlated repeats: treating 100 repeats as
	// independent would produce a misleading narrow interval around zero.
	for i := 0; i < 100; i++ {
		values = append(values, pairedValue{"a", "family-a", 1, 1, 1}, pairedValue{"b", "family-b", -1, -1, -1})
	}
	first := GradeReport{BootstrapSeed: 37, BootstrapSamples: 2000}
	second := first
	familyBootstrap(&first, values)
	familyBootstrap(&second, values)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("seeded intervals are not reproducible")
	}
	if first.Decision.EffectCI.Lower != -1 || first.Decision.EffectCI.Upper != 1 {
		t.Fatalf("pseudoreplication narrowed family interval: %+v", first.Decision.EffectCI)
	}
	if first.QualityCI.Lower != -1 || first.QualityCI.Upper != 1 {
		t.Fatalf("quality interval: %+v", first.QualityCI)
	}
}
