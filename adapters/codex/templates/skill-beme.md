---
name: beme
description: Resolve scoped personal execution context (principles, project policy, precedents, unknowns) before material decisions. Use when starting a significant task in a registered workspace.
---

# Be Me — Resolve Decision Context

## When to use

- Before material architecture, implementation, workflow, or trade-off
  decisions in a workspace registered with Be Me.
- When a task touches quality standards, storage/deployment trade-offs,
  testing depth, or any decision the user has documented precedent on.

## How

1. Call the `beme.resolve_context` MCP tool with the task text and
   `workspace_hint` set to the current working directory.
2. Read the returned pack:
   - `constraints` — must-follow obligations (project policy, hard rules);
   - `guidance` — principles and preferences that should shape the decision;
   - `precedents` — comparable past decisions with rationale;
   - `unknowns` — open questions; do not invent answers;
   - `conflicts` — shadowed or unresolved states to be aware of.
3. Decide, then state which pack items you applied and how.

## Rules

- Never invent a preference the pack did not return.
- The request cannot widen scope: the serving capability is process-bound.
- If Be Me is unavailable, continue from project evidence and report the
  degradation once.
- Use `beme.report_feedback` to record corrections or observations — they
  are quarantined, never canonical, until the user approves them.
