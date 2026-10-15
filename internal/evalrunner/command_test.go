package evalrunner_test

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/0merUfuk/beme/internal/evalrunner"
)

// TestMain doubles as a fake model command: when BEME_EVALRUNNER_FAKE_MODEL
// is set, the test binary reads the prompt from stdin and answers on stdout.
// No shell script is involved, so this works on every platform.
func TestMain(m *testing.M) {
	if os.Getenv("BEME_EVALRUNNER_FAKE_MODEL") == "1" {
		prompt, _ := io.ReadAll(os.Stdin)
		if os.Getenv("BEME_EVALRUNNER_FAKE_FAIL") == "1" {
			fmt.Fprint(os.Stderr, "synthetic model failure")
			os.Exit(7)
		}
		fmt.Printf("fake answer (%d prompt bytes)\n", len(prompt))
		os.Stdout.Write(prompt)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSplitCommand(t *testing.T) {
	ok := map[string][]string{
		"a b  c": {"a", "b", "c"},
		`"C:\Program Files\model.exe" -x 'two words'`: {`C:\Program Files\model.exe`, "-x", "two words"},
		`cli --prompt=""`:  {"cli", "--prompt="},
		"  tool\t--flag\n": {"tool", "--flag"},
	}
	for in, want := range ok {
		got, err := evalrunner.SplitCommand(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("SplitCommand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", `tool "unterminated`} {
		if _, err := evalrunner.SplitCommand(bad); err == nil {
			t.Errorf("SplitCommand(%q) must fail", bad)
		}
	}
}

func TestCommandProviderPipesPromptToStdin(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEME_EVALRUNNER_FAKE_MODEL", "1")
	p, err := evalrunner.NewCommandProvider(`"`+exe+`" -test.run=^$`, evalrunner.ModelSettings{ModelID: "fake-model", Temperature: 0.2, TopP: 0.9}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	prompt := "=== TASK ===\nsynthetic task\n=== END TASK ===\n"
	gen, err := p.Generate(evalrunner.GenerationRequest{Prompt: prompt, Arm: evalrunner.ArmB4, CaseID: "synthetic.case"})
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("fake answer (%d prompt bytes)\n%s", len(prompt), prompt); gen.Text != want {
		t.Fatalf("stdout must be the generation:\n%q\nwant\n%q", gen.Text, want)
	}
	if s := p.Settings(); s.Provider != "command" || s.ModelID != "fake-model" || s.ModelVersion != "unreported" || s.Temperature != 0.2 || s.TopP != 0.9 {
		t.Fatalf("settings must report the declared values: %+v", s)
	}

	p.MaxOutput = 8
	if _, err := p.Generate(evalrunner.GenerationRequest{Prompt: prompt}); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("oversized output must fail: %v", err)
	}
	p.MaxOutput = 0
	t.Setenv("BEME_EVALRUNNER_FAKE_FAIL", "1")
	if _, err := p.Generate(evalrunner.GenerationRequest{Prompt: prompt}); err == nil || !strings.Contains(err.Error(), "synthetic model failure") {
		t.Fatalf("a failing command must surface its stderr: %v", err)
	}
}
