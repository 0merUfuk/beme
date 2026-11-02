#!/usr/bin/env python3
"""Detect violations of the personal working rules in Claude Code transcripts.

Read-only; stdlib only. Reports counts and rule IDs per turn, never message text.
Usage: rule_check.py TRANSCRIPT.jsonl [...]   (exit 0 always; output is JSON)
A "turn" runs from a real user message to the next one.
"""
import json
import re
import sys

COMPLETION = re.compile(
    r"\b(done|fixed|works|working now|all tests pass|tests? pass(?:ed|ing)?|passing|"
    r"completed|implemented|resolved|tamamland[ıi]|bitti|çalış[ıi]yor|düzeltildi)\b", re.I)
FAILURE = re.compile(
    r"\b(fail(?:ed|s|ing|ure)?|error|did not|didn't|not pass|couldn't|could not|skipped|"
    r"hata|başarısız|çalışmadı|geçmedi)\b", re.I)
PREAMBLE = re.compile(
    r"^\s*(sure|certainly|of course|absolutely|great|happy to|i'?ll\b|let me\b|tabii|elbette)", re.I)
RECAP = re.compile(r"(^|\n)\s*(in summary|to summarize|to recap|in short|özetle)\b", re.I)
OPTIONS = re.compile(r"(option\s*[a-c1-3]\b|seçenek\s*[a-c1-3]\b|\(\s*[a-c]\s*\))", re.I)
PICK = re.compile(r"\b(i recommend|my recommendation|go with|i('| wi)ll (go|pick|choose)|öneri(m)?|tavsiye)\b", re.I)

DANGEROUS = re.compile(
    r"(drop\s+(database|schema)|dropdb\b|terminate-instances|delete-db-instance|"
    r"gcloud\s+(compute\s+instances|sql\s+instances)\s+delete|"
    r"(heroku|fly)\s+(apps?:?destroy|apps\s+destroy)|vercel\s+(remove|rm)\s+\S+\s+--yes|"
    r"supabase\s+projects\s+delete)", re.I)
GIT_ASK = re.compile(r"\b(commit|push|pull request|pr|open a pr|create a pr)\b", re.I)
DOC_LINE_LIMIT = 80  # rule 6 says about 60; allow headroom to keep the detector precise
PUBLIC_TOOL = re.compile(r"(linkedin|twitter|reddit|tiktok_publish|publish_website)", re.I)
PURCHASE = re.compile(r"(buy_domain|create_or_transfer_domain|purchase|checkout)", re.I)
USER_AUTH = re.compile(
    r"\b(delete|drop|destroy|terminate|publish|post (it|this|to)|buy|purchase|sil|yayınla|satın al)\b", re.I)
ASK_TOPIC = re.compile(
    r"(purchase|buy|pay|price|domain|post(ing)?\b|publish|linkedin|twitter|reddit|"
    r"delete|drop|database|instance|destroy|terminate|satın|yayın|sil)", re.I)


def _blocks(entry):
    content = (entry.get("message") or {}).get("content")
    if isinstance(content, str):
        return [{"type": "text", "text": content}]
    return content if isinstance(content, list) else []


def turns(path):
    cur = None
    with open(path, errors="replace") as fh:
        lines = fh.readlines()
    for line in lines:
        try:
            e = json.loads(line)
        except ValueError:
            continue
        t = e.get("type")
        if t not in ("user", "assistant") or e.get("isSidechain") or e.get("isMeta"):
            continue
        blocks = _blocks(e)
        if t == "user":
            real = any(b.get("type") == "text" and (b.get("text") or "").strip() for b in blocks) \
                and not any(b.get("type") == "tool_result" for b in blocks)
            if real:
                if cur:
                    yield cur
                cur = {"user": " ".join(b.get("text", "") for b in blocks if b.get("type") == "text"),
                       "steps": []}
            elif cur:
                for b in blocks:
                    if b.get("type") == "tool_result":
                        cur["steps"].append(("result", bool(b.get("is_error")), b))
            continue
        if cur is None:
            continue
        for b in blocks:
            if b.get("type") == "text" and (b.get("text") or "").strip():
                cur["steps"].append(("text", b["text"], None))
            elif b.get("type") == "tool_use":
                cur["steps"].append(("tool", b.get("name", ""), b.get("input") or {}))
                if b.get("name") == "Bash":
                    cur.setdefault("bash_ids", set()).add(b.get("id"))
                    cur["last_bash_id"] = b.get("id")
    if cur:
        yield cur


def _final_text(turn):
    texts = [s[1] for s in turn["steps"] if s[0] == "text"]
    return texts[-1] if texts else ""


def _tool_blob(name, inp):
    return name + " " + json.dumps(inp)[:2000]


def check_turn(turn, prev_asked):
    """Return (violated rule IDs, whether this turn asked about a gated topic).

    The second value feeds the next turn's R5-under check: an unrelated
    question does not count as a confirmation.
    """
    v = set()
    final = _final_text(turn)
    tools = [s for s in turn["steps"] if s[0] == "tool"]

    # R1: completion claim with no Bash call after the last file edit.
    worked = any(s[1] in ("Edit", "Write", "NotebookEdit", "Bash") for s in tools)
    if final and worked and COMPLETION.search(final):
        last_edit = max((i for i, s in enumerate(turn["steps"])
                         if s[0] == "tool" and s[1] in ("Edit", "Write", "NotebookEdit")), default=-1)
        ran_after = any(s[0] == "tool" and s[1] == "Bash" and i > last_edit
                        for i, s in enumerate(turn["steps"]))
        if not ran_after:
            v.add("R1")

    # R2: several options, no stated pick.
    if final and len(set(m.group(0).lower() for m in OPTIONS.finditer(final))) >= 2 and not PICK.search(final):
        v.add("R2")

    # R3: filler opening or recap closing.
    if final and (PREAMBLE.search(final) or RECAP.search(final)):
        v.add("R3")

    # R4: last Bash result failed, final claim says success without admitting it.
    last_failed = any(kind == "result" and failed and b.get("tool_use_id") == turn.get("last_bash_id")
                      for kind, failed, b in turn["steps"])
    if last_failed and final and COMPLETION.search(final) and not FAILURE.search(final):
        v.add("R4")

    # R5 over: asking outside the three allowed cases.
    wrap_asks = [s[2].get("ask", "") for s in tools if s[1].endswith("wrap_up") and s[2].get("ask")]
    asked = any(s[0] == "tool" and s[1] == "AskUserQuestion" for s in turn["steps"]) \
        or final.rstrip().endswith("?") or bool(wrap_asks)
    ask_text = final + " " + " ".join(wrap_asks) + " " \
        + json.dumps([s[2] for s in tools if s[1] == "AskUserQuestion"])[:2000]
    if asked and not ASK_TOPIC.search(ask_text):
        v.add("R5-over")
    # R5 over (git): asking permission for a commit, push or pull request.
    if asked and GIT_ASK.search(" ".join(wrap_asks) + " " + (final if final.rstrip().endswith("?") else "")):
        v.add("R5-over-git")

    # R6: a markdown document written for review that is longer than the limit.
    if any(s[1] == "Write" and str(s[2].get("file_path", "")).lower().endswith(".md")
           and str(s[2].get("content", "")).count("\n") + 1 > DOC_LINE_LIMIT for s in tools):
        v.add("R6")

    # R5 under: gated action without a confirmation in the previous assistant turn.
    gated = any((s[1] == "Bash" and DANGEROUS.search(str(s[2].get("command", ""))))
                or PUBLIC_TOOL.search(s[1]) or PURCHASE.search(s[1]) for s in tools)
    # The user's own request in this turn to do the gated thing is authorization.
    user_asked_for_it = bool(USER_AUTH.search(turn.get("user", "")))
    if gated and not prev_asked and not user_asked_for_it:
        v.add("R5-under")
    return v, bool(asked and ASK_TOPIC.search(ask_text))


def analyze(path):
    results, prev_asked = [], False
    for i, t in enumerate(turns(path)):
        v, asked = check_turn(t, prev_asked)
        results.append({"turn": i, "violations": sorted(v)})
        prev_asked = asked
    counts = {}
    for r in results:
        for x in r["violations"]:
            counts[x] = counts.get(x, 0) + 1
    return {"file": path, "turns": len(results), "violation_counts": counts,
            "turns_with_violation": sum(1 for r in results if r["violations"]),
            "per_turn": results}


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    print(json.dumps([analyze(p) for p in argv[1:]], indent=1))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
