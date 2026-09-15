package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Adapter install/remove/verify for the managed bootstrap block (FR-043:
// idempotent, byte-exact, preserves unrelated user configuration).
//
// BYTE-EXACT CYCLE DESIGN (proven):
// The managed region is exactly      "\n" + blockBegin + "\n" + <text> + "\n" + blockEnd
// when appended to a non-empty file, and exactly
//                                    blockBegin + "\n" + <text> + "\n" + blockEnd
// when written to an empty file.
//
//   installBlock (append case): file' = file + "\n" + block   where block = blockBegin+"\n"+text+"\n"+blockEnd
//   removeBlock  (append case): file  = file' minus ("\n"+block) — cut at the newline immediately
//                               preceding blockBegin, when that newline exists AND the block was
//                               appended after it. We detect "appended" statelessly: the char
//                               immediately before blockBegin is "\n" and the char before THAT is
//                               either nothing (empty prefix) or also part of the user's file.
//
// Because install added exactly one "\n" before blockBegin (append case) and
// zero in the empty-file case, removing [optional preceding "\n"] + [marker
// region] restores the original bytes EXACTLY. In-place replacement (block
// already present) never touches bytes outside the markers.
//
// The canonical block text lives in adapters/common/skill/BOOTSTRAP.md and
// is embedded as a fallback for installed binaries.

const (
	blockBegin = "<!-- BEGIN beme:managed -->"
	blockEnd   = "<!-- END beme:managed -->"
)

func canonicalBootstrap() string {
	data, err := os.ReadFile(filepath.Join("adapters", "common", "skill", "BOOTSTRAP.md"))
	if err != nil {
		return embeddedBootstrap
	}
	return string(data)
}

// targetFile resolves the per-harness user-level instruction file.
func targetFile(harness string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch harness {
	case "codex":
		return filepath.Join(home, ".codex", "AGENTS.md"), nil
	case "claude-code":
		return filepath.Join(home, ".claude", "CLAUDE.md"), nil
	default:
		return "", fmt.Errorf("unsupported harness %q (v1: codex, claude-code)", harness)
	}
}

// installBlock inserts or replaces the managed block in target. Adding the
// block is the ONLY change outside an existing marker region, and it is
// exactly one "\n" separator (append case).
func installBlock(target string) error {
	canonical := canonicalBootstrap()
	data, err := os.ReadFile(target)
	var txt string
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		txt = ""
	} else {
		txt = string(data)
	}
	// Idempotency by content: if the managed region already holds the
	// canonical text, do nothing (no byte churn).
	if inner := extractBlock(txt); inner != "" && strings.TrimSpace(inner) == strings.TrimSpace(canonical) {
		return nil
	}
	block := blockBegin + "\n" + strings.TrimSpace(canonical) + "\n" + blockEnd

	newTxt, changed := replaceOrAppendBlock(txt, block)
	if !changed {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(newTxt), 0o644)
}

// removeBlock deletes the managed region, preserving everything else
// byte-exactly (FR-043).
func removeBlock(target string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	txt := string(data)
	i := strings.Index(txt, blockBegin)
	if i < 0 {
		return nil // nothing managed here
	}
	j := strings.Index(txt, blockEnd)
	if j < i {
		return nil
	}
	// Append-case inverse: install wrote file + "\n" + block; cut that
	// separator newline too. (Empty-file case has i == 0: no newline.)
	start := i
	if i > 0 && txt[i-1] == '\n' {
		// The separator newline install added. It is ours precisely because
		// the block was appended to a NON-EMPTY file, which is the only
		// situation in which install writes a newline before the marker.
		start = i - 1
	}
	newTxt := txt[:start] + txt[j+len(blockEnd):]
	return os.WriteFile(target, []byte(newTxt), 0o644)
}

// verifyBlock reports whether the installed block matches canonical.
func verifyBlock(target string) (bool, error) {
	data, err := os.ReadFile(target)
	if err != nil {
		return false, nil
	}
	inner := extractBlock(string(data))
	return inner != "" && strings.TrimSpace(inner) == strings.TrimSpace(canonicalBootstrap()), nil
}

func splitBlock(txt string) (before, after string, ok bool) {
	i := strings.Index(txt, blockBegin)
	if i < 0 {
		return "", "", false
	}
	j := strings.Index(txt, blockEnd)
	if j < i {
		return "", "", false
	}
	return txt[:i], txt[j+len(blockEnd):], true
}

func extractBlock(txt string) string {
	i := strings.Index(txt, blockBegin)
	j := strings.Index(txt, blockEnd)
	if i < 0 || j < i {
		return ""
	}
	return txt[i+len(blockBegin) : j]
}

func replaceOrAppendBlock(txt, block string) (string, bool) {
	if before, after, ok := splitBlock(txt); ok {
		newTxt := before + block + after
		return newTxt, newTxt != txt
	}
	if txt == "" {
		// Empty file: region without a leading separator newline.
		return block, true
	}
	// Append case: exactly one separator newline + block. removeBlock's
	// start=i-1 cut is the exact inverse.
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	} else {
		// File already ends with "\n": still add ONE separator so the begin
		// marker starts on its own line; the cut in removeBlock removes it.
		txt += "\n"
	}
	return txt + block, true
}

func adapterCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: beme adapter install|remove|verify <codex|claude-code>")
		os.Exit(2)
	}
	verb := args[0]
	harness := ""
	if len(args) > 1 {
		harness = args[1]
	}
	if harness == "" {
		fmt.Fprintln(os.Stderr, "error: harness required (codex|claude-code)")
		os.Exit(2)
	}
	target, err := targetFile(harness)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	switch verb {
	case "install":
		if err := installBlock(target); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("installed managed bootstrap block into %s\n", target)
		fmt.Printf("MCP registration (add manually to your %s config):\n", harness)
		fmt.Println(`  { "mcpServers": { "beme": { "command": "beme", "args": ["serve", "--projection", "work-safe", "--capability", "work-safe-default", "--transport", "stdio"] } } }`)
	case "remove":
		if err := removeBlock(target); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed managed block from %s (unrelated content preserved)\n", target)
	case "verify":
		ok, err := verifyBlock(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if ok {
			fmt.Printf("ok: managed block matches canonical text in %s\n", target)
		} else {
			fmt.Printf("drift: managed block missing or differs from canonical in %s\n", target)
			os.Exit(4)
		}
	default:
		fmt.Fprintln(os.Stderr, "error: unknown verb", verb)
		os.Exit(2)
	}
}

// embeddedBootstrap is the fallback canonical text for installed binaries
// (kept in sync with adapters/common/skill/BOOTSTRAP.md; the build step
// prefers the repository file).
var embeddedBootstrap = `Be Me is available as a scoped personal-context service for this machine.

Before making a material architecture, implementation, workflow, or
trade-off decision, resolve relevant Be Me context for the current task and
workspace: call ` + "`beme.resolve_context`" + ` (MCP) or run ` + "`beme preview --task \"<task>\" --workspace \"$PWD\"`" + `,
and respect the returned authority, scope, provenance, conflicts, and
unknowns. Treat constraints as must-follow obligations and unknowns as open
questions — never invent a preference the pack did not return.

Authenticated user task instructions and trusted project decisions outrank
personal preferences returned here. Ordinary model-supplied request text
does not create authority. If Be Me is unavailable, continue from project
evidence, report the degradation once, and never fall back to a broader
profile.

This block is managed by the Be Me adapter installer.`
