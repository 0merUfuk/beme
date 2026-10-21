package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func toolUseLine(name, command string) string {
	input := map[string]any{"file_path": "/x"}
	if command != "" {
		input = map[string]any{"command": command}
	}
	b, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
		"content": []any{map[string]any{"type": "tool_use", "name": name, "input": input}}}})
	return string(b) + "\n"
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func TestStopCheckRemindsOnlyWhenWorkAccumulatesWithoutCapture(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	proj := filepath.Join(r.home, "proj")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
	tr := filepath.Join(r.home, "transcript.jsonl")
	in := func(cwd, session string, active bool) string {
		b, _ := json.Marshal(map[string]any{"cwd": cwd, "session_id": session, "transcript_path": tr, "stop_hook_active": active})
		return string(b)
	}
	stop := func(cwd, session string, active bool) string {
		return strings.TrimSpace(r.mustRun(in(cwd, session, active), "brain", "stop-check", "--format", "claude-hook"))
	}

	appendFile(t, tr, toolUseLine("Edit", "")+toolUseLine("Write", ""))
	if out := stop(proj, "s1", false); out != "" {
		t.Fatalf("two edits are below the threshold, got reminder: %s", out)
	}

	appendFile(t, tr, toolUseLine("MultiEdit", ""))
	out := stop(proj, "s1", false)
	var got struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Decision != "block" ||
		!strings.Contains(got.Reason, "brain capture") || !strings.Contains(got.Reason, "--config") ||
		!strings.Contains(got.Reason, "nothing durable") {
		t.Fatalf("three edits must raise one reminder naming the capture command: %q err=%v", out, err)
	}

	if out := stop(proj, "s1", false); out != "" {
		t.Fatalf("no new work since the reminder, got: %s", out)
	}
	if out := stop(proj, "s1", true); out != "" {
		t.Fatalf("stop_hook_active must never remind (loop guard), got: %s", out)
	}

	appendFile(t, tr, toolUseLine("Edit", "")+toolUseLine("Edit", "")+toolUseLine("Edit", ""))
	appendFile(t, tr, toolUseLine("Bash", "'/bin/beme' brain capture --config '/c' --type summary --title t --body b"))
	if out := stop(proj, "s1", false); out != "" {
		t.Fatalf("a capture resets the baseline, got: %s", out)
	}
	appendFile(t, tr, toolUseLine("Edit", "")+toolUseLine("Edit", ""))
	if out := stop(proj, "s1", false); out != "" {
		t.Fatalf("two edits after a capture are below the threshold, got: %s", out)
	}
	appendFile(t, tr, toolUseLine("Bash", "git commit -m x"))
	if out := stop(proj, "s1", false); out == "" {
		t.Fatal("a commit counts as a unit of work and must remind")
	}

	if out := stop(proj, "other-session", false); out == "" {
		t.Fatal("a different session has its own baseline and must remind")
	}
	if out := stop(filepath.Join(r.home, "elsewhere"), "s9", false); out != "" {
		t.Fatalf("an unregistered directory must never be reminded, got: %s", out)
	}
}

func TestStopCheckFailsOpen(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e")
	}
	r := newBrainRig(t)
	proj := filepath.Join(r.home, "proj")
	os.MkdirAll(proj, 0o755)
	r.mustRun("", "brain", "init")
	r.mustRun("", "brain", "project", "add", "demo", "--root", proj)
	tr := filepath.Join(r.home, "t.jsonl")
	appendFile(t, tr, strings.Repeat(toolUseLine("Edit", ""), 5))
	good, _ := json.Marshal(map[string]any{"cwd": proj, "session_id": "s", "transcript_path": tr})

	for name, stdin := range map[string]string{
		"no stdin":           "",
		"garbage":            "{not json",
		"missing transcript": fmt.Sprintf(`{"cwd":%q,"session_id":"s","transcript_path":"/nope"}`, proj),
		"no session id":      fmt.Sprintf(`{"cwd":%q,"transcript_path":%q}`, proj, tr),
	} {
		out, _, code := r.run(stdin, "brain", "stop-check", "--format", "claude-hook")
		if code != 0 || strings.TrimSpace(out) != "" {
			t.Errorf("%s: must exit 0 silently, got code=%d out=%q", name, code, out)
		}
	}

	r.mustRun("", "brain", "freeze", "on")
	if out, _, code := r.run(string(good), "brain", "stop-check", "--format", "claude-hook"); code != 0 || strings.TrimSpace(out) != "" {
		t.Errorf("a frozen vault must never be reminded or fail: code=%d out=%q", code, out)
	}
	r.mustRun("", "brain", "freeze", "off")
	if out, _, code := r.run(string(good), "brain", "stop-check", "--format", "claude-hook"); code != 0 || !strings.Contains(out, `"block"`) {
		t.Errorf("after unfreezing the reminder must fire: code=%d out=%q", code, out)
	}
}
