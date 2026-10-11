#!/usr/bin/env python3
"""Workflow policy regressions. Uses the existing public validation PyYAML dep."""
from pathlib import Path
import unittest

import yaml

ROOT = Path(__file__).resolve().parent.parent


class WorkflowTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue((ROOT / ".github/workflows/release.yml").is_file(), "release workflow is required")
        self.ci = yaml.load((ROOT / ".github/workflows/ci.yml").read_text(), Loader=yaml.BaseLoader)
        self.release = yaml.load((ROOT / ".github/workflows/release.yml").read_text(), Loader=yaml.BaseLoader)

    def test_dispatch_binds_every_checkout_to_validated_tag_sha(self):
        self.assertEqual(self.ci["on"]["workflow_call"]["inputs"]["source-ref"]["default"], "")
        self.assertIn("tag", self.release["on"]["workflow_dispatch"]["inputs"])
        jobs = self.release["jobs"]
        self.assertEqual(jobs["ci"]["uses"], "./.github/workflows/ci.yml")
        self.assertEqual(jobs["ci"]["with"]["source-ref"], "${{ needs.gate.outputs.sha }}")
        for name, job in self.ci["jobs"].items():
            for step in job.get("steps", []):
                if step.get("uses", "").startswith("actions/checkout@"):
                    self.assertEqual(step["with"]["ref"], "${{ inputs.source-ref || github.sha }}", name)
                    self.assertEqual(step["with"]["persist-credentials"], "false")
        for name in ("artifacts", "publish"):
            checkout = next(s for s in jobs[name]["steps"] if s.get("uses", "").startswith("actions/checkout@"))
            self.assertEqual(checkout["with"]["ref"], "${{ needs.gate.outputs.sha }}", name)
        self.assertEqual(jobs["publish"]["env"]["RELEASE_SOURCE_SHA"], "${{ needs.gate.outputs.sha }}")

    def test_only_complete_ci_then_five_native_artifacts_can_publish(self):
        jobs = self.release["jobs"]
        self.assertEqual(set(jobs["artifacts"]["needs"]), {"gate", "ci"})
        self.assertEqual(set(jobs["publish"]["needs"]), {"gate", "ci", "artifacts"})
        self.assertEqual(self.release["permissions"], {"contents": "read"})
        self.assertEqual(jobs["publish"]["permissions"], {"contents": "write"})
        self.assertIn("needs.ci.result == 'success'", jobs["publish"]["if"])
        self.assertIn("needs.artifacts.result == 'success'", jobs["publish"]["if"])
        self.assertIn("github.actor == '0merUfuk'", jobs["gate"]["if"])
        self.assertIn("github.triggering_actor == '0merUfuk'", jobs["gate"]["if"])
        targets = {item["target"] for item in jobs["artifacts"]["strategy"]["matrix"]["include"]}
        self.assertEqual(targets, {"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64"})
        steps = jobs["artifacts"]["steps"]
        smoke_positions = [i for i, s in enumerate(steps) if "--smoke" in s.get("run", "")]
        upload_position = next(i for i, s in enumerate(steps) if s.get("uses", "").startswith("actions/upload-artifact@"))
        self.assertTrue(smoke_positions)
        self.assertLess(max(smoke_positions), upload_position)
        download = next(s for s in jobs["publish"]["steps"] if s.get("uses", "").startswith("actions/download-artifact@"))
        self.assertNotIn("run-id", download["with"], "must never read another workflow run's artifacts")
        self.assertIn("github.run_attempt", download["with"]["pattern"])
        for workflow in (self.ci, self.release):
            for job in workflow["jobs"].values():
                self.assertNotIn("continue-on-error", job)
                for step in job.get("steps", []):
                    self.assertNotIn("continue-on-error", step)

    def test_windows_smoke_executes_real_negative_control(self):
        job = self.ci["jobs"]["installed-binary-smoke"]
        self.assertIn("windows-latest", job["strategy"]["matrix"]["os"])
        for step in job["steps"]:
            if "test_smoke.py" in step.get("run", ""):
                self.assertIn("BEME_SMOKE_BIN_DIR", step["env"])
        windows = [s["run"] for s in job["steps"] if s.get("shell") == "pwsh"]
        self.assertEqual(len(windows), 1)
        for script in ("test_smoke.py", "test_release.py", "release.py"):
            self.assertIn("Invoke-CheckedNative -Command python -Arguments @('scripts/" + script, windows[0])
        self.assertIn("'--smoke'", windows[0])
        self.assertFalse(any("upload-artifact" in s.get("uses", "") for s in job["steps"]))


if __name__ == "__main__":
    unittest.main()
