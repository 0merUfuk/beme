package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/0merUfuk/beme/internal/app"
	"github.com/0merUfuk/beme/internal/brain"
)

// Stop-hook capture reminder. Claude Code fires Stop after every assistant turn,
// so a reminder is raised only when enough file changes have accumulated since
// the last capture or the last reminder. The decision is deterministic, reads
// the harness transcript read-only, and never blocks on any error: a reminder is
// a nudge, not a gate. The agent still writes, and the gate still screens, the
// note itself.
const (
	nudgeMutations    = 3
	maxTranscriptSize = 256 << 20
	stopCheckBudget   = 3 * time.Second
)

// A commit counts only as an invocation (command start or after a shell
// separator), not as text inside an echo, a grep pattern or a message.
var gitCommitRe = regexp.MustCompile(`(?:^|[;&|(]\s*)git\s+commit\b`)

var mutatingTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

type transcriptWork struct {
	Mutations     int // weighted count of changes in the whole transcript
	AtLastCapture int // Mutations when the last brain capture ran
}

func scanTranscript(path string) (transcriptWork, error) {
	var w transcriptWork
	// Stat before open: a FIFO would block open, a device never ends.
	st, err := os.Stat(path)
	if err != nil {
		return w, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxTranscriptSize {
		return w, fmt.Errorf("transcript is not a regular file within the size bound")
	}
	f, err := os.Open(path)
	if err != nil {
		return w, err
	}
	defer f.Close()
	deadline := time.Now().Add(stopCheckBudget)
	r := bufio.NewReaderSize(io.LimitReader(f, maxTranscriptSize), 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && bytes.Contains(line, []byte(`"tool_use"`)) {
			countToolUses(line, &w)
		}
		if err == io.EOF {
			return w, nil
		}
		if err != nil {
			return w, err
		}
		if time.Now().After(deadline) {
			return w, fmt.Errorf("transcript scan budget exceeded")
		}
	}
}

func countToolUses(line []byte, w *transcriptWork) {
	var e struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &e) != nil {
		return
	}
	var blocks []struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Input struct {
			Command string `json:"command"`
		} `json:"input"`
	}
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
		}
		switch {
		case mutatingTools[b.Name]:
			w.Mutations++
		case b.Name == "Bash" && strings.Contains(b.Input.Command, "brain capture"):
			w.AtLastCapture = w.Mutations
		case b.Name == "Bash" && gitCommitRe.MatchString(b.Input.Command):
			// A commit completes a unit of work, but only uncaptured edits
			// make it worth a reminder: capture-then-commit stays silent.
			if w.Mutations > w.AtLastCapture {
				w.Mutations = max(w.Mutations, w.AtLastCapture+nudgeMutations)
			}
		}
	}
}

func brainStopCheck(args []string) {
	defer func() { _ = recover() }() // never block the agent on a bug
	c := newBrainFlags("stop-check")
	_ = c.fs.String("format", "claude-hook", "claude-hook")
	binary := c.fs.String("binary", "", "executable named in the reminder (default: this binary)")
	if _, err := parseInterspersed(c.fs, args); err != nil {
		return
	}

	var in struct {
		CWD            string `json:"cwd"`
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
		StopHookActive bool   `json:"stop_hook_active"`
	}
	data := readHookStdin()
	if len(data) == 0 || json.Unmarshal(data, &in) != nil {
		return
	}
	if in.StopHookActive || in.SessionID == "" || in.TranscriptPath == "" {
		return
	}
	cfg, _, err := app.ResolveConfig(c.config)
	if err != nil {
		return
	}
	v := brain.Open(cfg.BrainDir)
	if !v.Initialized() || v.Frozen() {
		return
	}
	match := v.ResolveProject(in.CWD)
	if match.Slug == "" || match.Slug == brain.GlobalProject {
		return
	}
	work, err := scanTranscript(in.TranscriptPath)
	if err != nil {
		return
	}
	const harness = "claude-code"
	base := max(work.AtLastCapture, v.NudgeMark(match.Slug, harness, in.SessionID))
	pending := work.Mutations - base
	if pending < nudgeMutations {
		return
	}
	if err := v.SetNudgeMark(match.Slug, harness, in.SessionID, work.Mutations); err != nil {
		return
	}
	bin := *binary
	if bin == "" {
		if exe, err := os.Executable(); err == nil {
			bin = exe
		} else {
			bin = "beme"
		}
	}
	emitJSON(map[string]any{
		"decision": "block",
		"reason": fmt.Sprintf("Before you finish: this session has changed files since the last record (%d weighted edits). "+
			"If it produced durable outcomes — a decision with its reason, unfinished work, or a distilled learning — "+
			"record each with `%s brain capture%s --type decision|open|summary|learning --title \"…\" --body \"…\" [--why \"…\"]`. "+
			"If nothing durable came out of it, reply with one line saying so and stop. Never record code, secrets or transcripts.",
			pending, shellQuote(bin), cfgFlag(c.config)),
	})
}
