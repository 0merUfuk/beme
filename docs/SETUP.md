# Be Me — guided setup

`beme setup` writes your personal working rules once and puts them where agents
read them. Dry run by default; nothing leaves your machine.

```sh
beme setup                       # interview in a terminal, shows the plan
beme setup --apply               # write it (backs up each file first)
beme setup --defaults --apply    # recommended rules, no questions
```

## What it writes
- `<config>/rules/answers.json` — your answers, the single source of truth.
- `<config>/rules/entries/RULE-*.md` — one Be Me entry per rule, plus
  `<config>/sources/personal-rules.yaml` (only if absent).
- A rules block inside `<!-- BEGIN beme-rules:managed -->` markers in
  `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`, after a backup in
  `<config>/backups/`. Cursor and Hermes: the text is printed for you to place.
- It then runs `beme build` and `beme doctor`, and prints the MCP registration.
  It never edits a harness's MCP configuration.

## Let your agent run it
Any agent can do the interview for you:

1. `beme setup --print-questions` prints the questions as JSON (key, prompt,
   choices, default).
2. The agent asks you each one and writes the answers to a JSON file.
3. `beme setup --answers FILE` shows the plan; add `--apply` after you agree.

Unknown keys, custom rules longer than 200 characters, or text that looks like
a managed-region marker are rejected before anything is written.

## Change or undo
- Edit `answers.json`, then `beme setup --refresh --apply`.
- `beme setup --remove --apply` removes the block and restores your file's
  other content byte for byte. Your answers and entries are kept.

## Honest limits
The block is the working part: agents read instruction files at session start.
The entries are advisory and only help when an agent calls the resolver, which
Claude Code rarely did in measurements. Whether agents then follow the rules is
not guaranteed; start a new session and ask it to list the rules to confirm they
loaded. A transcript checker for rule violations is not shipped yet.
