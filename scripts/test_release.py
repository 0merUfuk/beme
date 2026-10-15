#!/usr/bin/env python3
"""Release invariants tested with synthetic files; never accesses GitHub."""

from pathlib import Path
import tempfile
import unittest

import release


class ReleaseTests(unittest.TestCase):
    def test_tag_cli_and_changelog_must_agree(self):
        self.assertIsNotNone(release, "release helper is required")
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            (root / "cmd/beme").mkdir(parents=True)
            (root / "cmd/beme/main.go").write_text('const version = "0.1.0-alpha.2"\n')
            (root / "CHANGELOG.md").write_text("# Changelog\n\n## [Unreleased]\n\n## [0.1.0-alpha.2] — 2026-01-01\n\n### Fixed\n- Synthetic change.\n", encoding="utf-8")
            self.assertEqual(release.validate_version(root, "v0.1.0-alpha.2"), "0.1.0-alpha.2")
            (root / "CHANGELOG.md").write_text("## [Unreleased]\n\n## [0.1.0-alpha.2]\n\n- Synthetic change.\n")
            self.assertEqual(release.validate_version(root, "v0.1.0-alpha.2"), "0.1.0-alpha.2")
            for tag in ["v0.1.0-alpha.1", "v0.1.0", "v0.1.0-alpha.02", "../v0.1.0-alpha.2", "v0.1.0-alpha.2\n"]:
                with self.subTest(tag=tag), self.assertRaises(ValueError):
                    release.validate_version(root, tag)
            (root / "CHANGELOG.md").write_text("## [Unreleased]\n- Not released.\n")
            with self.assertRaisesRegex(ValueError, "CHANGELOG"):
                release.validate_version(root, "v0.1.0-alpha.2")


class SourceGateTests(unittest.TestCase):
    def test_only_owner_tag_trigger_can_release(self):
        self.assertTrue(hasattr(release, "validate_trigger"), "owner/tag gate is required")
        env = {"GITHUB_REPOSITORY": "0merUfuk/beme", "GITHUB_REPOSITORY_OWNER": "0merUfuk",
               "GITHUB_ACTOR": "0merUfuk", "GITHUB_TRIGGERING_ACTOR": "0merUfuk",
               "GITHUB_EVENT_NAME": "push", "GITHUB_REF_TYPE": "tag",
               "GITHUB_REF": "refs/tags/v0.1.0-alpha.2"}
        release.validate_trigger(env, "v0.1.0-alpha.2")
        release.validate_trigger(dict(env, GITHUB_EVENT_NAME="workflow_dispatch", GITHUB_REF="refs/heads/main",
                                      GITHUB_REF_TYPE="branch", RELEASE_TAG="v0.1.0-alpha.2"), "v0.1.0-alpha.2")
        with self.assertRaises(ValueError):
            release.validate_trigger(dict(env, GITHUB_EVENT_NAME="workflow_dispatch", GITHUB_REF="refs/heads/topic",
                                          GITHUB_REF_TYPE="branch", RELEASE_TAG="v0.1.0-alpha.2"), "v0.1.0-alpha.2")
        for key, value in [("GITHUB_REPOSITORY", "someone/beme"), ("GITHUB_ACTOR", "someone"),
                           ("GITHUB_TRIGGERING_ACTOR", "someone"), ("GITHUB_EVENT_NAME", "pull_request"),
                           ("GITHUB_REF_TYPE", "branch"), ("GITHUB_REF", "refs/heads/main")]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                release.validate_trigger(dict(env, **{key: value}), "v0.1.0-alpha.2")

    def test_dispatch_cli_uses_tag_source_not_newer_main_version(self):
        import os
        import subprocess
        import sys
        with tempfile.TemporaryDirectory() as d:
            root = Path(d) / "repo"
            root.mkdir()
            remote = Path(d) / "remote.git"
            env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
                       GIT_AUTHOR_NAME="Synthetic", GIT_AUTHOR_EMAIL="test@example.invalid",
                       GIT_COMMITTER_NAME="Synthetic", GIT_COMMITTER_EMAIL="test@example.invalid")
            def git(*args):
                return subprocess.check_output(["git", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", *args],
                                               cwd=root, env=env, text=True).strip()
            git("init", "-q", "-b", "main")
            (root / "cmd/beme").mkdir(parents=True)
            def version(value):
                (root / "cmd/beme/main.go").write_text(f'const version = "{value}"\n')
                (root / "CHANGELOG.md").write_text(f"## [{value}]\n\n- Synthetic change.\n")
            version("0.1.0-alpha.2")
            git("add", ".")
            git("commit", "-q", "-m", "synthetic tag source")
            tagged_sha = git("rev-parse", "HEAD")
            git("tag", "-a", "v0.1.0-alpha.2", "-m", "synthetic annotated tag")
            version("0.1.0-alpha.3")
            git("add", ".")
            git("commit", "-q", "-m", "newer synthetic main")
            main_sha = git("rev-parse", "HEAD")
            git("init", "-q", "--bare", str(remote))
            git("remote", "add", "origin", str(remote))
            git("push", "-q", "origin", "main", "refs/tags/v0.1.0-alpha.2")
            git("checkout", "-q", "--detach", tagged_sha)
            output = Path(d) / "outputs"
            env.update(GITHUB_REPOSITORY="0merUfuk/beme", GITHUB_REPOSITORY_OWNER="0merUfuk",
                       GITHUB_ACTOR="0merUfuk", GITHUB_TRIGGERING_ACTOR="0merUfuk", GITHUB_EVENT_NAME="workflow_dispatch",
                       GITHUB_REF="refs/heads/main", GITHUB_REF_TYPE="branch", GITHUB_SHA=main_sha,
                       RELEASE_TAG="v0.1.0-alpha.2", GITHUB_OUTPUT=str(output))
            result = subprocess.run([sys.executable, str(Path(__file__).with_name("release.py")), "--repo", str(root),
                                     "gate", "--tag", "v0.1.0-alpha.2"], env=env, capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(f"sha={tagged_sha}\n", output.read_text())
            self.assertNotIn(main_sha, output.read_text())
            self.assertEqual(git("rev-parse", "refs/tags/v0.1.0-alpha.2^{commit}"), tagged_sha)

    def test_remote_main_and_remote_tag_must_bind_exact_clean_source(self):
        self.assertTrue(hasattr(release, "verify_source"), "remote source gate is required")
        import os
        import subprocess
        with tempfile.TemporaryDirectory() as d:
            root = Path(d) / "repo"
            remote = Path(d) / "remote.git"
            env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
                       GIT_AUTHOR_NAME="Synthetic", GIT_AUTHOR_EMAIL="test@example.invalid",
                       GIT_COMMITTER_NAME="Synthetic", GIT_COMMITTER_EMAIL="test@example.invalid")
            def git(*args):
                return subprocess.check_output(["git", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", *args],
                                               cwd=root, env=env, text=True).strip()
            root.mkdir()
            git("init", "-q", "-b", "main")
            git("commit", "-q", "--allow-empty", "-m", "synthetic main")
            sha = git("rev-parse", "HEAD")
            git("tag", "v0.1.0-alpha.2")
            git("init", "-q", "--bare", str(remote))
            git("remote", "add", "origin", str(remote))
            git("push", "-q", "origin", "main", "refs/tags/v0.1.0-alpha.2")
            release.verify_source(root, "v0.1.0-alpha.2", sha)
            (root / "uncommitted").write_text("synthetic")
            with self.assertRaisesRegex(ValueError, "clean"):
                release.verify_source(root, "v0.1.0-alpha.2", sha)
            (root / "uncommitted").unlink()
            git("checkout", "-q", "-b", "topic")
            git("commit", "-q", "--allow-empty", "-m", "unmerged synthetic topic")
            topic = git("rev-parse", "HEAD")
            git("tag", "v0.1.0-alpha.3")
            git("push", "-q", "origin", "refs/tags/v0.1.0-alpha.3")
            with self.assertRaisesRegex(ValueError, "main"):
                release.verify_source(root, "v0.1.0-alpha.3", topic)
            with self.assertRaisesRegex(ValueError, "source"):
                release.verify_source(root, "v0.1.0-alpha.2", sha)
            git("checkout", "-q", "main")
            git("tag", "v0.1.0-alpha.4")
            with self.assertRaisesRegex(ValueError, "remote tag"):
                release.verify_source(root, "v0.1.0-alpha.4", sha)


class ArchiveTests(unittest.TestCase):
    def test_five_archives_bind_all_four_binaries_license_and_source(self):
        self.assertTrue(hasattr(release, "write_archive"), "deterministic archive writer is required")
        import hashlib
        source = {"commit": "a" * 40, "tree": "b" * 40, "source_sha256": "c" * 64, "dirty": False}
        with tempfile.TemporaryDirectory() as d:
            out = Path(d)
            for target in ("darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64"):
                suffix = ".exe" if target.startswith("windows_") else ""
                files = {name + suffix: b"synthetic executable " + name.encode() for name in
                         ("beme", "beme-bench", "beme-eval", "beme-threat-corpus")}
                files["LICENSE"] = b"synthetic license\n"
                archive = release.write_archive(out, "v0.1.0-alpha.2", target, source, files, "go1.25.6")
                manifest, payload = release.inspect_archive(archive, "v0.1.0-alpha.2", source)
                self.assertEqual(manifest["target"], target)
                self.assertEqual(manifest["source"], source)
                self.assertEqual({name: payload[name] for name in files}, files)
                self.assertEqual(manifest["files"], {name: hashlib.sha256(data).hexdigest() for name, data in files.items()})
                before = archive.read_bytes()
                with tempfile.TemporaryDirectory() as other:
                    same = release.write_archive(Path(other), "v0.1.0-alpha.2", target, source, files, "go1.25.6")
                    self.assertEqual(before, same.read_bytes(), "archives must be deterministic for safe draft retries")
                with self.assertRaisesRegex(ValueError, "source"):
                    release.inspect_archive(archive, "v0.1.0-alpha.2", dict(source, commit="f" * 40))
            sums = release.checksums(out, "v0.1.0-alpha.2", source)
            self.assertEqual(len(sums.read_text().splitlines()), 5)
            release.verify_bundle(out, "v0.1.0-alpha.2", source)
            sums.write_text("0" * 64 + "  wrong.zip\n")
            with self.assertRaisesRegex(ValueError, "checksum"):
                release.verify_bundle(out, "v0.1.0-alpha.2", source)
            with self.assertRaisesRegex(ValueError, "clean"):
                release.checksums(out, "v0.1.0-alpha.2", dict(source, dirty=True))

    def test_artifacts_reject_personal_paths_and_changed_payloads(self):
        import zipfile
        source = {"commit": "a" * 40, "tree": "b" * 40, "source_sha256": "c" * 64, "dirty": False}
        with tempfile.TemporaryDirectory() as d:
            files = {name: b"synthetic " + name.encode() for name in release.payload_names("windows_amd64")}
            files["beme.exe"] += b"/" + b"Users/" + b"synthetic/accidental-build-path"
            path = release.write_archive(Path(d), "v0.1.0-alpha.2", "windows_amd64", source, files, "go1.25.6")
            with self.assertRaisesRegex(ValueError, "personal path"):
                release.inspect_archive(path)
            path.unlink()
            files["beme.exe"] = b"synthetic executable"
            path = release.write_archive(Path(d), "v0.1.0-alpha.2", "windows_amd64", source, files, "go1.25.6")
            with zipfile.ZipFile(path) as z:
                entries = [(i, z.read(i)) for i in z.infolist()]
            with zipfile.ZipFile(path, "w") as z:
                for info, data in entries:
                    z.writestr(info, data + b"tampered" if info.filename == "beme.exe" else data)
            with self.assertRaisesRegex(ValueError, "checksum"):
                release.inspect_archive(path)

    def test_rejects_tampered_unsafe_or_incomplete_archives(self):
        self.assertTrue(hasattr(release, "inspect_archive"), "archive verifier is required")
        import zipfile
        with tempfile.TemporaryDirectory() as d:
            archive = Path(d) / "beme_0.1.0-alpha.2_windows_amd64.zip"
            for names in [["../escape"], ["beme.exe"], ["MANIFEST.json", "MANIFEST.json"]]:
                with self.subTest(names=names):
                    import warnings
                    with warnings.catch_warnings():
                        warnings.simplefilter("ignore", UserWarning)
                        with zipfile.ZipFile(archive, "w") as z:
                            for name in names:
                                z.writestr(name, b"{}")
                    with self.assertRaises(ValueError):
                        release.inspect_archive(archive)


class NativeArchiveTests(unittest.TestCase):
    def test_crossbuild_is_not_native_runtime_proof(self):
        self.assertTrue(hasattr(release, "require_native"), "native artifact guard is required")
        target = release.native_target()
        release.require_native(target)
        other = next(t for t in release.TARGETS if t != target)
        with self.assertRaisesRegex(ValueError, "native"):
            release.require_native(other)

    def test_source_identity_changes_with_uncommitted_bytes(self):
        self.assertTrue(hasattr(release, "source_identity"), "actual source binding is required")
        import os
        import subprocess
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
                       GIT_AUTHOR_NAME="Synthetic", GIT_AUTHOR_EMAIL="test@example.invalid",
                       GIT_COMMITTER_NAME="Synthetic", GIT_COMMITTER_EMAIL="test@example.invalid")
            for args in [["init", "-q"], ["-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "synthetic"]]:
                subprocess.run(["git", *args], cwd=root, env=env, check=True)
            clean = release.source_identity(root)
            (root / "fixture").write_text("uncommitted synthetic bytes")
            dirty = release.source_identity(root)
            self.assertFalse(clean["dirty"])
            self.assertTrue(dirty["dirty"])
            self.assertEqual(clean["commit"], dirty["commit"])
            self.assertNotEqual(clean["source_sha256"], dirty["source_sha256"])

    @unittest.skipUnless(__import__("os").environ.get("BEME_RELEASE_NATIVE_TEST") == "1", "native build exercised by CI artifact job")
    def test_real_native_archive_roundtrip(self):
        self.assertTrue(hasattr(release, "package"), "real Go packaging is required")
        repo = Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory() as d:
            path = release.package(repo, Path(d), release.native_target())
            result = release.smoke_archive(path, repo)
            self.assertEqual(result["target"], release.native_target())
            self.assertEqual(len(result["executables"]), 4)
            self.assertTrue(result["brain_memory_recalled"])


class DraftPublicationTests(unittest.TestCase):
    def fixture(self, directory):
        source = {"commit": "a" * 40, "tree": "b" * 40, "source_sha256": "c" * 64, "dirty": False}
        for target in release.TARGETS:
            files = {name: b"synthetic " + name.encode() for name in release.payload_names(target)}
            release.write_archive(directory, "v0.1.0-alpha.2", target, source, files, "go1.25.6")
        release.checksums(directory, "v0.1.0-alpha.2", source)
        return source

    def test_preserves_owner_draft_and_retries_only_matching_assets(self):
        self.assertTrue(hasattr(release, "publish"), "draft-only publication is required")
        with tempfile.TemporaryDirectory() as d:
            out = Path(d)
            source = self.fixture(out)
            api = SyntheticAPI()
            first = sorted(out.iterdir())[0]
            api.assets = [api.asset(first.name, first.read_bytes())]
            release.publish(api, out, "v0.1.0-alpha.2", source)
            self.assertEqual(api.record["author"]["login"], "0merUfuk")
            self.assertEqual(api.record["body"], "Owner-authored synthetic notes.")
            self.assertEqual(api.record["id"], 42)
            self.assertFalse(api.record["draft"])
            self.assertTrue(api.record["prerelease"])
            self.assertEqual(len([w for w in api.writes if w[0] == "POST"]), 5)
            self.assertEqual(api.writes[-1], ("PATCH", "/releases/42", {"draft": False, "prerelease": True, "make_latest": "false"}))
            count = len(api.writes)
            with self.assertRaisesRegex(ValueError, "published"):
                release.publish(api, out, "v0.1.0-alpha.2", source)
            self.assertEqual(len(api.writes), count, "published releases must never be mutated")

    def test_missing_foreign_published_or_mismatching_draft_refuses_before_writes(self):
        self.assertTrue(hasattr(release, "publish"), "draft-only publication is required")
        with tempfile.TemporaryDirectory() as d:
            out = Path(d)
            source = self.fixture(out)
            for case in ("missing", "foreign", "published", "mismatch", "extra", "immutable", "duplicate"):
                with self.subTest(case=case):
                    api = SyntheticAPI()
                    if case == "missing": api.missing = True
                    if case == "foreign": api.record["author"]["login"] = "someone"
                    if case == "published": api.record["draft"] = False
                    if case == "immutable": api.record["immutable"] = True
                    if case == "duplicate": api.duplicate = True
                    if case == "mismatch": api.assets = [api.asset("SHA256SUMS", b"mismatch")]
                    if case == "extra": api.assets = [api.asset("unexpected", b"extra")]
                    with self.assertRaises(ValueError):
                        release.publish(api, out, "v0.1.0-alpha.2", source)
                    self.assertEqual(api.writes, [])

    def test_failed_upload_readback_never_publishes(self):
        self.assertTrue(hasattr(release, "publish"), "draft-only publication is required")
        with tempfile.TemporaryDirectory() as d:
            out = Path(d)
            source = self.fixture(out)
            api = SyntheticAPI()
            api.corrupt_readback = True
            with self.assertRaises(ValueError):
                release.publish(api, out, "v0.1.0-alpha.2", source)
            self.assertTrue(api.record["draft"])
            self.assertFalse(any(w[0] == "PATCH" for w in api.writes))


class SyntheticAPI:
    """No network; stands in only for the explicitly prohibited release writes."""
    def __init__(self):
        self.record = {"id": 42, "tag_name": "v0.1.0-alpha.2", "draft": True, "prerelease": True,
                       "author": {"login": "0merUfuk"}, "body": "Owner-authored synthetic notes.", "name": "Synthetic alpha"}
        self.assets, self.writes = [], []
        self.missing = self.duplicate = self.corrupt_readback = False

    def asset(self, name, data):
        return {"id": len(self.assets) + 1, "name": name, "state": "uploaded", "size": len(data),
                "digest": "sha256:" + release.digest(data)}

    def __call__(self, method, path, data=None, blob=None):
        import copy
        from urllib.parse import parse_qs, urlsplit
        if method == "GET":
            if path.startswith("/releases?"):
                return [] if self.missing else [copy.deepcopy(self.record)] * (2 if self.duplicate else 1)
            if path.startswith("/releases/42/assets?"):
                assets = copy.deepcopy(self.assets)
                if self.corrupt_readback and assets:
                    assets[0]["digest"] = None
                return assets
            if path == "/releases/42": return copy.deepcopy(self.record)
        self.writes.append((method, path, data))
        if method == "POST" and path.startswith("/releases/42/assets?"):
            name = parse_qs(urlsplit(path).query)["name"][0]
            self.assets.append(self.asset(name, blob))
            return copy.deepcopy(self.assets[-1])
        if method == "PATCH" and path == "/releases/42":
            self.record.update(data)
            return copy.deepcopy(self.record)
        raise AssertionError((method, path))


class CommandTests(unittest.TestCase):
    def test_cli_help_and_publish_without_complete_ci_fail_closed(self):
        import os
        import subprocess
        import sys
        script = Path(__file__).with_name("release.py")
        help_result = subprocess.run([sys.executable, str(script), "--help"], capture_output=True, text=True, timeout=30)
        self.assertEqual(help_result.returncode, 0)
        self.assertIn("package", help_result.stdout, "helper must expose the tested packaging CLI")
        with tempfile.TemporaryDirectory() as d:
            for ci, artifacts in [("", ""), ("success", "failure"), ("skipped", "success")]:
                env = {k: v for k, v in os.environ.items() if not k.startswith(("GH_", "GITHUB_", "RELEASE_"))}
                env.update(RELEASE_CI_RESULT=ci, RELEASE_ARTIFACT_RESULT=artifacts)
                result = subprocess.run([sys.executable, str(script), "publish", "--out", d, "--tag", "v0.1.0-alpha.2"],
                                        env=env, capture_output=True, text=True, timeout=30)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("complete CI and native artifacts", result.stderr)

    def test_api_transport_is_repository_scoped_and_never_creates_releases(self):
        self.assertTrue(hasattr(release, "GitHubAPI"), "scoped API adapter is required")
        import io
        import json
        calls = []
        def opener(request, timeout):
            calls.append(request)
            return io.BytesIO(b'{"id":42}')
        api = release.GitHubAPI("synthetic", opener=opener)
        self.assertEqual(api("GET", "/releases/42"), {"id": 42})
        self.assertEqual(calls[-1].full_url, "https://api.github.com/repos/0merUfuk/beme/releases/42")
        api("POST", "/releases/42/assets?name=synthetic.zip", blob=b"synthetic artifact")
        self.assertEqual(calls[-1].full_url, "https://uploads.github.com/repos/0merUfuk/beme/releases/42/assets?name=synthetic.zip")
        self.assertEqual(calls[-1].data, b"synthetic artifact")
        api("PATCH", "/releases/42", {"draft": False, "prerelease": True, "make_latest": "false"})
        self.assertEqual(json.loads(calls[-1].data)["draft"], False)
        for method, path in [("DELETE", "/releases/42"), ("POST", "/releases"), ("GET", "https://other.invalid"),
                             ("PATCH", "/git/refs/tags/v0.1.0-alpha.1")]:
            with self.subTest(method=method, path=path), self.assertRaises(ValueError):
                api(method, path)


if __name__ == "__main__":
    unittest.main()
