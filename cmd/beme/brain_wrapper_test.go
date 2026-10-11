package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBrainExplicitWrapperSurvivesGeneratedChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper fixture")
	}
	r := newBrainRig(t)
	cfg := filepath.Join(r.home, "wrapper's configuration")
	proj := filepath.Join(r.home, "project")
	os.MkdirAll(cfg, 0700)
	os.MkdirAll(proj, 0700)
	os.WriteFile(filepath.Join(cfg, "config.yaml"), []byte("brain_mode: manual\n"), 0600)
	wrapper := filepath.Join(r.home, "wrapper's command")
	os.WriteFile(wrapper, []byte("#!/bin/sh\nexport BEME_CONFIG_HOME="+shellQuote(cfg)+"\nexport BEME_DATA_HOME="+shellQuote(filepath.Join(r.home, "wrapper-data"))+"\nexport BEME_BRAIN_HOME="+shellQuote(filepath.Join(r.home, "wrapper-brain"))+"\nexec "+shellQuote(r.bin)+" \"$@\"\n"), 0700)
	shell := func(command, stdin string) string {
		t.Helper()
		c := exec.Command("/bin/sh", "-c", command)
		c.Dir = proj
		c.Env = []string{"HOME=" + r.home, "PATH=/usr/bin:/bin", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
		c.Stdin = strings.NewReader(stdin)
		b, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("generated command failed: %v %s", err, b)
		}
		return string(b)
	}
	command := func(text, verb string) string {
		t.Helper()
		for _, s := range strings.Split(text, "`") {
			if strings.Contains(s, " brain "+verb) {
				return s
			}
		}
		t.Fatal("no generated command", verb, text)
		return ""
	}
	base := shellQuote(wrapper) + " brain "
	shell(base+"init", "")
	shell(base+"project add demo --root "+shellQuote(proj), "")
	var hooks struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	json.Unmarshal([]byte(shell(base+"hooks --binary "+shellQuote(wrapper), "")), &hooks)
	var output struct {
		Hook struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	payload, _ := json.Marshal(map[string]string{"cwd": proj, "source": "fork", "session_id": "synthetic-wrapper"})
	json.Unmarshal([]byte(shell(hooks.Hooks["SessionStart"][0].Hooks[0].Command, string(payload))), &output)
	pull := command(output.Hook.Context, "briefing")
	text := shell(pull, "")
	cap := strings.Split(command(text, "capture"), " --type ")[0] + " --type learning --title 'Wrapper followup' --body 'The generated chain must preserve the wrapper selected vault.'"
	if !strings.HasPrefix(cap, shellQuote(wrapper)+" brain capture") {
		t.Fatal("capture bypassed wrapper", cap)
	}
	receipt := shell(cap, "")
	marker := "(or run: "
	start := strings.Index(receipt, marker)
	if start < 0 {
		t.Fatal(receipt)
	}
	doctor := strings.TrimSuffix(strings.TrimSpace(receipt[start+len(marker):]), ")")
	if !strings.HasPrefix(doctor, shellQuote(wrapper)+" brain doctor") {
		t.Fatal("accepted capture lost wrapper for next generated command", doctor)
	}
	shell(doctor, "")
	if !strings.Contains(shell(base+"briefing --mode normal", ""), "wrapper selected vault") {
		t.Fatal("followup failed to promote to original deployment")
	}
}
