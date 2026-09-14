package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Adapter install/remove/verify for the managed bootstrap block (FR-043:
// idempotent, preserves unrelated user configuration).
//
// The managed block is delimited by markers; operations touch ONLY the
// region between them. The canonical block text lives in the repository
// (adapters/common/skill/BOOTSTRAP.md) and is embedded at build time.

const (
	blockBegin = "<!-- BEGIN beme:managed -->"
	blockEnd   = "<!-- END beme:managed -->"
)

func canonicalBootstrap() string {
	data, err := os.ReadFile(filepath.Join("adapters", "common", "skill", "BOOTSTRAP.md"))
	if err != nil {
		// fall back to embedded copy for installed binaries
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

// installBlock inserts or replaces the managed block in target.
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
	// canonical text, do nothing (no byte churn, no newline accumulation).
	if inner := extractBlock(txt); inner != "" && strings.TrimSpace(inner) == strings.TrimSpace(canonical) {
		return nil
	}
	block := blockBegin + "\n\n" + strings.TrimSpace(canonical) + "\n\n" + blockEnd

	newTxt, changed := replaceOrAppendBlock(txt, block)
	if !changed && txt == newTxt {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(newTxt), 0o644)
}

// removeBlock deletes the managed region, preserving everything else.
func removeBlock(target string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	txt := string(data)
	before, after, ok := splitBlock(txt)
	if !ok {
		return nil // nothing managed here
	}
	newTxt := strings.TrimRight(before, "\n") + "\n" + after
	newTxt = strings.TrimLeft(newTxt, "\n")
	return os.WriteFile(target, []byte(newTxt), 0o644)
}

// verifyBlock reports whether the installed block matches canonical.
func verifyBlock(target string) (bool, error) {
	data, err := os.ReadFile(target)
	if err != nil {
		return false, nil
	}
	before, after, ok := splitBlock(string(data))
	if !ok {
		return false, nil
	}
	_ = before
	_ = after
	inner := extractBlock(string(data))
	return strings.TrimSpace(inner) == strings.TrimSpace(canonicalBootstrap()), nil
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
		return block, true
	}
	if !strings.HasSuffix(txt, "\n\n") {
		txt = strings.TrimRight(txt, "\n") + "\n\n"
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
