#!/usr/bin/env python3
"""Offline, synthetic native executable smoke. Every subprocess fails closed."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile


def isolated_env(home):
    env = {key: value for key, value in os.environ.items()
           if key.upper() in {"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT"}}
    for key in ("HOME", "USERPROFILE", "TMPDIR", "TMP", "TEMP"):
        env[key] = str(home)
    env.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
               GIT_CONFIG_COUNT="2", GIT_CONFIG_KEY_0="commit.gpgsign", GIT_CONFIG_VALUE_0="false",
               GIT_CONFIG_KEY_1="tag.gpgsign", GIT_CONFIG_VALUE_1="false")
    for key, subdir in {"BEME_CONFIG_HOME": "cfg", "BEME_DATA_HOME": "data", "BEME_CACHE_HOME": "cache",
                        "APPDATA": "appdata", "LOCALAPPDATA": "localappdata", "XDG_CONFIG_HOME": "config",
                        "XDG_DATA_HOME": "xdg-data", "XDG_CACHE_HOME": "xdg-cache"}.items():
        env[key] = str(home / subdir)
    return env


def run_commands(commands, **kwargs):
    return [subprocess.run(command, check=True, capture_output=True, text=True, encoding="utf-8",
                           timeout=180, **kwargs) for command in commands]


def smoke_binaries(bin_dir, repo, expected_version=None):
    bin_dir, repo = Path(bin_dir).resolve(), Path(repo).resolve()
    suffix = ".exe" if os.name == "nt" else ""
    names = ("beme", "beme-bench", "beme-eval", "beme-threat-corpus")
    binaries = {name: str(bin_dir / (name + suffix)) for name in names}
    with tempfile.TemporaryDirectory(prefix="beme-smoke-") as d:
        home = Path(d)
        env = isolated_env(home)
        cfg = home / "cfg"
        (cfg / "sources").mkdir(parents=True)
        entries = home / "src" / "entries"
        entries.mkdir(parents=True)
        def invoke(name, *args):
            return run_commands([[binaries[name], *map(str, args)]], cwd=home, env=env)[0].stdout
        def beme(*args):
            # The top-level flag parser stops at subcommands/positionals.
            pos = len(args) if args[0] in {"brain", "candidate"} else 1
            return invoke("beme", *args[:pos], "--config", cfg, *args[pos:])
        version = invoke("beme", "version").strip()
        if expected_version is not None and version != "beme " + expected_version:
            raise ValueError("archive CLI version disagrees with its manifest")
        (entries / "SMOKE-001.md").write_text(
            '---\nid: SMOKE-001\ntitle: "Smoke preference"\ntype: preference\nstatus: active\n---\n\n'
            'Keep installed-binary smoke tests synthetic and isolated.\n', encoding="utf-8")
        (cfg / "sources/smoke.yaml").write_text(
            'schema_version: "1"\nsource_id: smoke-src\ntype: directory\nroot: ' + json.dumps((home / "src").as_posix()) + '\n'
            'purpose: [reusable_knowledge]\ntrust: canonical\ninstruction_semantics: registered_files_only\n'
            'authority_ceiling: default\nsensitivity: personal_private\nprofiles_allowed: [personal]\n'
            'ingestion_mode: index_content\ninclude: ["entries/**/*.md"]\n', encoding="utf-8")
        beme("build", "--profile", "personal")
        json.loads(beme("doctor", "--json"))
        json.loads(beme("status", "--json"))
        pack = json.loads(beme("preview", "--task", "installed binary smoke", "--json"))
        if not pack.get("pack_id") or "synthetic and isolated" not in json.dumps(pack):
            raise ValueError("preview did not return the synthetic evidence in a context pack")
        json.loads(beme("candidate", "list", "--json"))
        beme("forget", "rec_smoke-001", "smoke")
        beme("purge", "--confirm", "rec_smoke-001", "--dry-run", "rec_smoke-001")
        invoke("beme-threat-corpus", "--repo", repo)
        invoke("beme-bench", "--seed", repo / "testdata/synthetic/records.json", "--scales", "1",
               "--iterations", "2", "--warmup", "1", "--build-runs", "1", "--out", "-")
        if "grade-import" not in invoke("beme-eval", "--help"):
            raise ValueError("evaluation executable lacks its expected command surface")
        project = home / "project"
        project.mkdir()
        beme("brain", "init")
        beme("brain", "project", "add", "demo", "--root", project)
        beme("brain", "capture", "--cwd", project, "--type", "learning", "--title", "Synthetic release lesson",
             "--body", "SYNTHETIC_RELEASE_MEMORY persists across independent CLI processes.")
        beme("brain", "doctor", "--quiet")
        if "SYNTHETIC_RELEASE_MEMORY" not in beme("brain", "briefing", "--cwd", project):
            raise ValueError("brain did not recall the promoted synthetic memory")
        json.loads(beme("brain", "status", "--json"))
    return {"executables": list(names), "version": version,
            "context_pack_checked": True, "brain_memory_recalled": True}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin-dir", type=Path, required=True)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    args = parser.parse_args()
    print(json.dumps(smoke_binaries(args.bin_dir, args.repo), sort_keys=True))
