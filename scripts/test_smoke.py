#!/usr/bin/env python3
"""Synthetic controls for the same checked runner used by native smokes."""
import os
from pathlib import Path
import subprocess
import shutil
import sys
import tempfile
import unittest

import smoke


class SmokeTests(unittest.TestCase):
    def test_failure_cannot_be_hidden_by_later_success(self):
        self.assertIsNotNone(smoke, "the checked smoke runner is required")
        with tempfile.TemporaryDirectory() as d:
            marker = Path(d) / "success"
            success = [sys.executable, "-c", f"from pathlib import Path; Path({str(marker)!r}).touch()"]
            with self.assertRaises(subprocess.CalledProcessError) as failure:
                smoke.run_commands([[sys.executable, "-c", "raise SystemExit(17)"], success])
            self.assertEqual(failure.exception.returncode, 17)
            self.assertFalse(marker.exists(), "success after failure must never execute")
            smoke.run_commands([success])
            self.assertTrue(marker.exists(), "positive control must actually execute")

    @unittest.skipUnless(os.environ.get("BEME_SMOKE_BIN_DIR"), "native install supplied by CI")
    def test_real_native_install(self):
        self.assertTrue(hasattr(smoke, "smoke_binaries"), "complete native smoke is required")
        repo = Path(__file__).resolve().parent.parent
        result = smoke.smoke_binaries(Path(os.environ["BEME_SMOKE_BIN_DIR"]), repo)
        self.assertEqual(set(result["executables"]), {"beme", "beme-bench", "beme-eval", "beme-threat-corpus"})
        self.assertTrue(result["brain_memory_recalled"])
        self.assertTrue(result["context_pack_checked"])

    @unittest.skipUnless(os.name == "nt" or shutil.which("pwsh"), "PowerShell native control runs on Windows CI")
    def test_powershell_failure_stops_before_success(self):
        helper = Path(__file__).with_name("native.ps1")
        self.assertTrue(helper.is_file())
        with tempfile.TemporaryDirectory() as d:
            marker = Path(d) / "success"
            # Run a real failing native executable, then a real successful one.
            # This is the same function that guards the Windows workflow steps.
            prefix = "$ErrorActionPreference = 'Stop'\n. $env:CHECKED_NATIVE_HELPER\n"
            success = "Invoke-CheckedNative -Command $env:CHECKED_NATIVE_PYTHON -Arguments @('-c', $env:CHECKED_NATIVE_SUCCESS)\n"
            failure = "Invoke-CheckedNative -Command $env:CHECKED_NATIVE_PYTHON -Arguments @('-c', 'raise SystemExit(17)')\n"
            env = dict(os.environ, CHECKED_NATIVE_HELPER=str(helper),
                       CHECKED_NATIVE_PYTHON=sys.executable,
                       CHECKED_NATIVE_SUCCESS=f"from pathlib import Path; Path({str(marker)!r}).touch()")
            script = Path(d) / "control.ps1"
            script.write_text(prefix + failure + success, encoding="utf-8")
            r = subprocess.run(["pwsh", "-NoProfile", "-File", str(script)], env=env,
                               capture_output=True, text=True, timeout=30)
            self.assertNotEqual(r.returncode, 0)
            self.assertIn("native command failed (17)", r.stderr)
            self.assertFalse(marker.exists())
            script.write_text(prefix + success, encoding="utf-8")
            subprocess.run(["pwsh", "-NoProfile", "-File", str(script)], env=env,
                           check=True, timeout=30)
            self.assertTrue(marker.exists())


if __name__ == "__main__":
    unittest.main()
