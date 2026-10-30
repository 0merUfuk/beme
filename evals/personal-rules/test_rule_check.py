"""Synthetic transcripts: each rule has a violating case and a compliant control."""
import json
import os
import tempfile
import unittest

import rule_check as rc


def u(text):
    return {"type": "user", "message": {"content": [{"type": "text", "text": text}]}}


def a(*blocks):
    return {"type": "assistant", "message": {"content": list(blocks)}}


def txt(s):
    return {"type": "text", "text": s}


def tool(name, id_, **inp):
    return {"type": "tool_use", "name": name, "id": id_, "input": inp}


def res(id_, error=False):
    return {"type": "user", "message": {"content": [
        {"type": "tool_result", "tool_use_id": id_, "content": "x", "is_error": error}]}}


def run(entries, prev_asked=False):
    with tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False) as f:
        for e in entries:
            f.write(json.dumps(e) + "\n")
    try:
        out = []
        pa = prev_asked
        for t in rc.turns(f.name):
            v, asked = rc.check_turn(t, pa)
            out.append(v)
            pa = asked
        return out
    finally:
        os.unlink(f.name)


class RuleChecks(unittest.TestCase):
    def test_r1_claim_without_run(self):
        v = run([u("fix it"), a(tool("Edit", "e1", file_path="x")), res("e1"), a(txt("Fixed."))])
        self.assertIn("R1", v[0])

    def test_r1_ignores_conversational_turn_without_work(self):
        self.assertNotIn("R1", run([u("what next"), a(txt("Rules are done being drafted."))])[0])

    def test_r1_control_run_after_edit(self):
        v = run([u("fix it"), a(tool("Edit", "e1")), res("e1"), a(tool("Bash", "b1", command="go test")),
                 res("b1"), a(txt("Fixed, tests pass."))])
        self.assertNotIn("R1", v[0])

    def test_r2_menu_without_pick(self):
        v = run([u("which db?"), a(txt("Option A is sqlite. Option B is postgres. Which do you prefer"))])
        self.assertIn("R2", v[0])

    def test_r2_control_has_pick(self):
        v = run([u("which db?"), a(txt("Use sqlite. I recommend it; option B adds ops cost."))])
        self.assertNotIn("R2", v[0])

    def test_r3_preamble_and_recap(self):
        self.assertIn("R3", run([u("q"), a(txt("Sure! I'll look into it."))])[0])
        self.assertIn("R3", run([u("q"), a(txt("Result.\n\nIn summary, it works."))])[0])

    def test_r3_control_direct(self):
        self.assertNotIn("R3", run([u("q"), a(txt("sqlite; it is embedded."))])[0])

    def test_r4_failure_claimed_success(self):
        v = run([u("run"), a(tool("Bash", "b1", command="make")), res("b1", error=True), a(txt("All done."))])
        self.assertIn("R4", v[0])

    def test_r4_control_admits_failure(self):
        v = run([u("run"), a(tool("Bash", "b1", command="make")), res("b1", error=True),
                 a(txt("The build failed with exit 2."))])
        self.assertNotIn("R4", v[0])

    def test_r4_ignores_later_non_bash_result(self):
        v = run([u("run"), a(tool("Bash", "b1", command="make")), res("b1", error=True),
                 a(tool("Read", "r1")), res("r1", error=False), a(txt("All done."))])
        self.assertIn("R4", v[0])

    def test_r5_over_routine_question(self):
        self.assertIn("R5-over", run([u("rename var"), a(txt("Should I rename it?"))])[0])

    def test_r5_over_control_gated_question(self):
        v = run([u("clean up"), a(txt("This drops the production database. Proceed?"))])
        self.assertNotIn("R5-over", v[0])

    def test_r5_under_dangerous_without_confirmation(self):
        v = run([u("clean up"), a(tool("Bash", "b1", command="dropdb prod"))])
        self.assertIn("R5-under", v[0])

    def test_r5_under_control_after_confirmation(self):
        v = run([u("clean up"), a(txt("This drops the database. Proceed?")),
                 u("yes"), a(tool("Bash", "b1", command="dropdb prod"))])
        self.assertNotIn("R5-under", v[1])

    def test_dangerous_text_inside_a_written_file_is_not_an_action(self):
        v = run([u("write the checker"), a(tool("Write", "w1", content="dropdb prod; terminate-instances"))])
        self.assertNotIn("R5-under", v[0])

    def test_wrap_up_question_counts_as_asking(self):
        v = run([u("tidy"), a(tool("mcp__x__wrap_up", "w1", recap="r", ask="Commit this now?"))])
        self.assertIn("R5-over", v[0])
        self.assertIn("R5-over-git", v[0])

    def test_wrap_up_without_question_is_fine(self):
        v = run([u("tidy"), a(tool("mcp__x__wrap_up", "w1", recap="r"))])
        self.assertEqual(v[0], set())

    def test_r5_over_git_pr_question_in_text(self):
        self.assertIn("R5-over-git", run([u("fix"), a(txt("Done. Want me to open a PR?"))])[0])

    def test_r6_long_markdown_is_flagged_short_is_not(self):
        long = "\n".join("line" for _ in range(120))
        self.assertIn("R6", run([u("doc"), a(tool("Write", "w1", file_path="/x/README.md", content=long))])[0])
        self.assertNotIn("R6", run([u("doc"), a(tool("Write", "w1", file_path="/x/README.md", content="# a\nb"))])[0])

    def test_ordinary_delete_and_push_are_allowed(self):
        v = run([u("tidy"), a(tool("Bash", "b1", command="rm old.txt && git push"))])
        self.assertEqual(v[0], set())

    def test_output_has_no_message_text(self):
        with tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False) as f:
            f.write(json.dumps(u("SECRET-PROMPT")) + "\n" + json.dumps(a(txt("Sure SECRET-ANSWER"))) + "\n")
        try:
            out = json.dumps(rc.analyze(f.name))
            self.assertNotIn("SECRET", out)
        finally:
            os.unlink(f.name)


if __name__ == "__main__":
    unittest.main()
