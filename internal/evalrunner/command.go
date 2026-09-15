package evalrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// DefaultCommandTimeout bounds one external generation.
const DefaultCommandTimeout = 10 * time.Minute

// DefaultCommandMaxOutput bounds one generation's stdout.
const DefaultCommandMaxOutput = 4 << 20

// CommandProvider runs an external command per generation: the rendered
// prompt is written to its stdin and its stdout is the generated text. The
// command is executed directly (no shell). It receives only the prompt — no
// arm label, case ID, or pack structure.
//
// A command cannot report its model settings, so Declared carries what the
// owner declares for the run; unset identity fields report "unreported".
type CommandProvider struct {
	Argv      []string
	Timeout   time.Duration
	MaxOutput int
	Declared  ModelSettings
}

// NewCommandProvider parses command with SplitCommand.
func NewCommandProvider(command string, declared ModelSettings, timeout time.Duration) (*CommandProvider, error) {
	argv, err := SplitCommand(command)
	if err != nil {
		return nil, err
	}
	return &CommandProvider{Argv: argv, Timeout: timeout, MaxOutput: DefaultCommandMaxOutput, Declared: declared}, nil
}

func (c *CommandProvider) Name() string { return "command" }

func (c *CommandProvider) Settings() ModelSettings {
	s := c.Declared
	if s.Provider == "" {
		s.Provider = "command"
	}
	if s.ModelID == "" {
		s.ModelID = "unreported"
	}
	if s.ModelVersion == "" {
		s.ModelVersion = "unreported"
	}
	return s
}

func (c *CommandProvider) Generate(req GenerationRequest) (GenerationResult, error) {
	if len(c.Argv) == 0 {
		return GenerationResult{}, errors.New("command provider has no command")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	limit := c.MaxOutput
	if limit <= 0 {
		limit = DefaultCommandMaxOutput
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Stdin = strings.NewReader(req.Prompt)
	stdout := &cappedBuffer{limit: limit}
	stderr := &cappedBuffer{limit: 8 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return GenerationResult{}, fmt.Errorf("command timed out after %s", timeout)
	}
	if err != nil {
		return GenerationResult{}, fmt.Errorf("command failed: %v; stderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.overflow {
		return GenerationResult{}, fmt.Errorf("command output exceeded %d bytes", limit)
	}
	return GenerationResult{Text: stdout.String(), Meta: map[string]string{"provider": "command", "tokens_used": "unreported"}}, nil
}

// cappedBuffer keeps the first limit bytes and records overflow without
// failing the writer (so the child never sees a broken pipe). The buffer is
// a named field, not embedded: an embedded bytes.Buffer would promote
// ReadFrom, and io.Copy would bypass Write and the cap.
type cappedBuffer struct {
	buf      bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room < len(p) {
		b.overflow = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) String() string { return b.buf.String() }

// SplitCommand splits a command line into argv without a shell: whitespace
// separates arguments; single or double quotes group text (quotes removed).
// There is no escape character, so Windows paths with backslashes pass
// through unchanged.
func SplitCommand(s string) ([]string, error) {
	args := []string{}
	var cur strings.Builder
	inArg := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inArg = r, true
		case unicode.IsSpace(r):
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote in command", quote)
	}
	if inArg {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, errors.New("empty command")
	}
	return args, nil
}
