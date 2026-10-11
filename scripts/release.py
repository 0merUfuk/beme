#!/usr/bin/env python3
"""Small alpha packager and fail-closed publisher (Python standard library)."""
from pathlib import Path
import re
import subprocess
import gzip
import hashlib
import io
import json
import stat
import tarfile
import zipfile
import os
import platform
import tempfile
from urllib.parse import quote

from smoke import smoke_binaries

OWNER = "0merUfuk"
REPOSITORY = OWNER + "/beme"
ALPHA = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-alpha\.([1-9][0-9]*)")
TARGETS = ("darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64")
BINARIES = ("beme", "beme-bench", "beme-eval", "beme-threat-corpus")
MAX_MEMBER_BYTES = 128 * 1024 * 1024


def paginated(api, path):
    result = []
    for page in range(1, 101):
        batch = api("GET", f"{path}?per_page=100&page={page}")
        if not isinstance(batch, list):
            raise ValueError("invalid API list response")
        result.extend(batch)
        if len(batch) < 100:
            return result
    raise ValueError("API pagination limit reached; no partial-list decisions")


def require_draft(record, tag):
    if record.get("tag_name") != tag or record.get("draft") is not True or record.get("immutable"):
        raise ValueError("refusing a published, immutable or mismatched release")
    if record.get("author", {}).get("login") != OWNER or not record.get("body", "").strip():
        raise ValueError("an owner-authored draft with reviewed notes is required")
    if not isinstance(record.get("id"), int):
        raise ValueError("invalid release identity")


def find_draft(api, tag):
    # The tag lookup endpoint is documented for published releases; list drafts
    # with the scoped token and paginate rather than silently missing one.
    matches = [r for r in paginated(api, "/releases") if r.get("tag_name") == tag]
    if len(matches) != 1:
        raise ValueError("exactly one pre-created owner draft is required; never recreate it")
    record = api("GET", f"/releases/{matches[0]['id']}")
    require_draft(record, tag)
    return record


def checked_assets(api, release_id, expected, out):
    present = {}
    for asset in paginated(api, f"/releases/{release_id}/assets"):
        name = asset["name"]
        if (name in present or name not in expected or asset.get("state") != "uploaded"
                or asset.get("digest") != "sha256:" + expected.get(name, "")
                or asset.get("size") != (Path(out) / name).stat().st_size):
            raise ValueError("draft asset mismatch; never overwrite or delete assets")
        present[name] = asset
    return present


def publish(api, out, tag, source):
    expected = verify_bundle(out, tag, source)
    record = find_draft(api, tag)
    release_id = record["id"]
    present = checked_assets(api, release_id, expected, out)
    identity = {key: record.get(key) for key in ("id", "author", "body", "name", "tag_name")}
    def read_draft():
        current = api("GET", f"/releases/{release_id}")
        require_draft(current, tag)
        if any(current.get(key) != value for key, value in identity.items()):
            raise ValueError("owner draft changed during publication")
    for name in sorted(expected.keys() - present.keys()):
        read_draft()
        data = (Path(out) / name).read_bytes()
        if digest(data) != expected[name]:
            raise ValueError("local asset changed after verification")
        api("POST", f"/releases/{release_id}/assets?name={quote(name, safe='')}", blob=data)
        if name not in checked_assets(api, release_id, expected, out):
            raise ValueError("uploaded asset missing on readback")
    read_draft()
    if set(checked_assets(api, release_id, expected, out)) != set(expected):
        raise ValueError("incomplete release assets")
    api("PATCH", f"/releases/{release_id}", {"draft": False, "prerelease": True, "make_latest": "false"})
    final = api("GET", f"/releases/{release_id}")
    if (final.get("draft") is not False or final.get("prerelease") is not True
            or any(final.get(key) != value for key, value in identity.items())
            or set(checked_assets(api, release_id, expected, out)) != set(expected)):
        raise ValueError("published release readback failed; do not retry by mutating it")
    return final

def native_target():
    goos = {"Darwin": "darwin", "Linux": "linux", "Windows": "windows"}.get(platform.system())
    goarch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine().lower())
    target = f"{goos}_{goarch}"
    if target not in TARGETS:
        raise ValueError("unsupported native platform")
    return target


def require_native(target):
    if target != native_target():
        raise ValueError("native archive execution required; crossbuild is not runtime proof")


def source_identity(root):
    git = lambda *args: command(["git", *args], root)
    paths = sorted(set(git("ls-files", "-z", "--cached", "--others", "--exclude-standard").split("\0")) - {""})
    seal = hashlib.sha256()
    for name in paths:
        path = Path(root) / name
        if path.is_symlink() or not path.is_file():
            raise ValueError("source contains missing or nonregular file")
        seal.update(name.encode() + b"\0" + digest(path.read_bytes()).encode() + b"\n")
    return {"commit": git("rev-parse", "HEAD"), "tree": git("rev-parse", "HEAD^{tree}"),
            "source_sha256": seal.hexdigest(), "dirty": bool(git("status", "--porcelain", "--untracked-files=all"))}


def package(root, out, target):
    root, out = Path(root).resolve(), Path(out).resolve()
    if out.is_relative_to(root):
        raise ValueError("package output must be outside the source checkout")
    tag = "v" + source_version(root)
    archive_name(tag, target)  # reject unsupported targets before any build
    source = source_identity(root)
    goos, goarch = target.split("_")
    env = dict(os.environ, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0", GOTOOLCHAIN="local", GOWORK="off")
    go = os.environ.get("GO", "go")
    goversion = command([go, "env", "GOVERSION"], root, env=env)
    files = {"LICENSE": (root / "LICENSE").read_bytes()}
    with tempfile.TemporaryDirectory(prefix="beme-build-") as d:
        for name in BINARIES:
            filename = name + (".exe" if goos == "windows" else "")
            binary = Path(d) / filename
            command([go, "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", str(binary), "./cmd/" + name], root, env=env)
            files[filename] = binary.read_bytes()
    if source_identity(root) != source:
        raise ValueError("source changed while building")
    return write_archive(out, tag, target, source, files, goversion)


def smoke_archive(path, repo):
    tag = "v" + source_version(repo)
    manifest, files = inspect_archive(path, tag, source_identity(repo))
    require_native(manifest["target"])
    if files["LICENSE"] != (Path(repo) / "LICENSE").read_bytes():
        raise ValueError("archive LICENSE differs from source")
    # Do not use extractall: only exact, flat, verified members can be written.
    with tempfile.TemporaryDirectory(prefix="beme-archive-") as d:
        for name, data in files.items():
            dest = Path(d) / name
            dest.write_bytes(data)
            dest.chmod(0o755 if name not in {"LICENSE", "MANIFEST.json"} else 0o644)
        result = smoke_binaries(Path(d), repo, tag[1:])
    return dict(result, target=manifest["target"], archive_sha256=digest(Path(path).read_bytes()))

def digest(data):
    return hashlib.sha256(data).hexdigest()


def payload_names(target):
    if target not in TARGETS:
        raise ValueError("unsupported target")
    suffix = ".exe" if target.startswith("windows_") else ""
    return {name + suffix for name in BINARIES} | {"LICENSE"}


def archive_name(tag, target):
    if not ALPHA.fullmatch(tag) or target not in TARGETS:
        raise ValueError("invalid alpha tag/target")
    suffix = ".zip" if target.startswith("windows_") else ".tar.gz"
    return f"beme_{tag[1:]}_{target}{suffix}"


def write_archive(out, tag, target, source, files, go_version):
    if set(files) != payload_names(target) or not all(files.values()):
        raise ValueError("archive requires all four executables and LICENSE")
    manifest = {"schema_version": 1, "tag": tag, "target": target, "source": source,
                "go_version": go_version, "files": {name: digest(data) for name, data in files.items()}}
    files = dict(files, **{"MANIFEST.json": (json.dumps(manifest, sort_keys=True, indent=2) + "\n").encode()})
    Path(out).mkdir(parents=True, exist_ok=True)
    path = Path(out) / archive_name(tag, target)
    with path.open("xb") as dest:
        if target.startswith("windows_"):
            with zipfile.ZipFile(dest, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                for name, data in sorted(files.items()):
                    info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
                    info.create_system = 3
                    info.external_attr = (stat.S_IFREG | (0o755 if name.endswith(".exe") else 0o644)) << 16
                    info.compress_type = zipfile.ZIP_DEFLATED
                    archive.writestr(info, data)
        else:
            with gzip.GzipFile(filename="", fileobj=dest, mode="wb", mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as archive:
                    for name, data in sorted(files.items()):
                        info = tarfile.TarInfo(name)
                        info.size = len(data)
                        info.mode = 0o755 if name in BINARIES else 0o644
                        archive.addfile(info, io.BytesIO(data))
    return path


def inspect_archive(path, tag=None, source=None):
    path = Path(path)
    files, modes = {}, {}
    def add(name, data, mode):
        if name in files or "/" in name or "\\" in name or name in {".", ".."}:
            raise ValueError("unsafe or duplicate archive member")
        if not data or len(data) > MAX_MEMBER_BYTES:
            raise ValueError("empty or oversized archive member")
        files[name], modes[name] = data, mode
    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as archive:
            if len(archive.infolist()) != 6:
                raise ValueError("archive must contain exactly six regular files")
            for info in archive.infolist():
                mode = info.external_attr >> 16
                if not stat.S_ISREG(mode) or info.file_size > MAX_MEMBER_BYTES:
                    raise ValueError("nonregular or oversized archive member")
                add(info.filename, archive.read(info), mode)
    else:
        with tarfile.open(path, "r:gz") as archive:
            for info in archive:
                if not info.isfile() or info.size > MAX_MEMBER_BYTES or len(files) >= 6:
                    raise ValueError("nonregular, excess or oversized archive member")
                member = archive.extractfile(info)
                if member is None:
                    raise ValueError("unreadable archive member")
                add(info.name, member.read(), info.mode)
    try:
        manifest = json.loads(files["MANIFEST.json"])
        target, actual_tag = manifest["target"], manifest["tag"]
        expected = payload_names(target)
        if set(files) != expected | {"MANIFEST.json"} or manifest["schema_version"] != 1:
            raise ValueError("incomplete archive")
        if path.name != archive_name(actual_tag, target) or (tag is not None and tag != actual_tag):
            raise ValueError("archive tag/name disagreement")
        if source is not None and manifest["source"] != source:
            raise ValueError("archive source disagreement")
        if manifest["files"] != {name: digest(files[name]) for name in expected}:
            raise ValueError("archive payload checksum mismatch")
        # Generic artifact scan only; never embed a private identifier inventory.
        personal_path = rb"/(?:Users|home)/[A-Za-z0-9_.-]+/|[A-Za-z]:[\\\\/]Users[\\\\/][A-Za-z0-9_.-]+[\\\\/]"
        if any(re.search(personal_path, data) for data in files.values()):
            raise ValueError("artifact contains a personal path; build with trimpath and synthetic inputs")
        if not target.startswith("windows_") and any(modes[name] & 0o777 != 0o755 for name in BINARIES):
            raise ValueError("archive lost executable permissions")
    except (KeyError, TypeError, json.JSONDecodeError) as exc:
        raise ValueError("invalid or missing manifest") from exc
    return manifest, files


def bundle_sums(out, tag, source):
    if source.get("dirty") is not False:
        raise ValueError("release requires clean source, not a local snapshot")
    expected = {archive_name(tag, target) for target in TARGETS}
    present = {p.name for p in Path(out).iterdir()}
    if present - {"SHA256SUMS"} != expected:
        raise ValueError("release bundle requires exactly five platform archives")
    lines = []
    for name in sorted(expected):
        path = Path(out) / name
        if path.is_symlink() or not path.is_file():
            raise ValueError("bundle contains nonregular file")
        inspect_archive(path, tag, source)
        lines.append(f"{digest(path.read_bytes())}  {name}\n")
    return "".join(lines)


def checksums(out, tag, source):
    data = bundle_sums(out, tag, source)
    path = Path(out) / "SHA256SUMS"
    with path.open("x", encoding="utf-8", newline="\n") as dest:
        dest.write(data)
    return path


def verify_bundle(out, tag, source):
    expected = bundle_sums(out, tag, source)
    sums = Path(out) / "SHA256SUMS"
    if sums.is_symlink() or sums.read_text(encoding="utf-8") != expected:
        raise ValueError("release bundle checksum mismatch")
    return {p.name: digest(p.read_bytes()) for p in sorted(Path(out).iterdir())}

def command(args, root, **kwargs):
    return subprocess.run(args, cwd=root, check=True, capture_output=True,
                          text=True, encoding="utf-8", timeout=300, **kwargs).stdout.strip()


def validate_trigger(env, tag):
    tag_push = (env.get("GITHUB_EVENT_NAME") == "push" and env.get("GITHUB_REF_TYPE") == "tag"
                and env.get("GITHUB_REF") == "refs/tags/" + tag)
    dispatch = (env.get("GITHUB_EVENT_NAME") == "workflow_dispatch" and env.get("GITHUB_REF") == "refs/heads/main"
                and env.get("GITHUB_REF_TYPE") == "branch" and env.get("RELEASE_TAG") == tag)
    if (not ALPHA.fullmatch(tag) or env.get("GITHUB_REPOSITORY") != REPOSITORY
            or env.get("GITHUB_REPOSITORY_OWNER") != OWNER
            or env.get("GITHUB_ACTOR") != OWNER
            or env.get("GITHUB_TRIGGERING_ACTOR") != OWNER or not (tag_push or dispatch)):
        raise ValueError("release requires an owner tag push or an owner main-workflow dispatch naming an existing tag")


def verify_source(root, tag, sha):
    if not ALPHA.fullmatch(tag) or not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("invalid tag/source identity")
    git = lambda *args: command(["git", *args], root)
    if git("rev-parse", "HEAD^{commit}") != sha or git("rev-parse", f"refs/tags/{tag}^{{commit}}") != sha:
        raise ValueError("checkout/tag/source disagreement")
    if git("status", "--porcelain", "--untracked-files=all"):
        raise ValueError("release requires a clean source checkout")
    # Fetch only main, never create/move/delete any tag, including on retries.
    git("fetch", "--no-tags", "origin", "+refs/heads/main:refs/remotes/origin/main")
    refs = dict(line.split()[::-1] for line in git("ls-remote", "origin", f"refs/tags/{tag}", f"refs/tags/{tag}^{{}}").splitlines())
    if refs.get(f"refs/tags/{tag}^{{}}", refs.get(f"refs/tags/{tag}")) != sha:
        raise ValueError("remote tag does not bind this source")
    try:
        git("merge-base", "--is-ancestor", sha, "refs/remotes/origin/main")
    except subprocess.CalledProcessError as exc:
        raise ValueError("tag must already be reachable from remote main; never auto-merge") from exc


def source_version(root):
    text = (Path(root) / "cmd/beme/main.go").read_text(encoding="utf-8")
    found = re.findall(r'^const version = "([^"\n]+)"$', text, re.MULTILINE)
    if len(found) != 1:
        raise ValueError("exactly one CLI version constant is required")
    return found[0]


def validate_version(root, tag):
    if not ALPHA.fullmatch(tag):
        raise ValueError("only numbered vX.Y.Z-alpha.N tags are releasable")
    version = source_version(root)
    if tag != "v" + version:
        raise ValueError("tag and CLI version disagree")
    text = (Path(root) / "CHANGELOG.md").read_text(encoding="utf-8")
    sections = re.split(r"(?m)^## ", text)[1:]
    released = [s for s in sections if not s.startswith("[Unreleased]")]
    if not released or not re.match(r"\[" + re.escape(version) + r"\](?: (?:—|-) \d{4}-\d{2}-\d{2})?\n", released[0]):
        raise ValueError("CHANGELOG first released heading must match the tag")
    if not re.search(r"(?m)^- \S", released[0]):
        raise ValueError("CHANGELOG release notes are empty")
    return version


class GitHubAPI:
    """Only release reads, asset uploads, and draft publication; no tag writes."""
    def __init__(self, token, opener=None):
        import urllib.request
        if not token:
            raise ValueError("GH_TOKEN is required")
        self.token = token
        self.opener = opener or urllib.request.urlopen

    def __call__(self, method, path, data=None, blob=None):
        import urllib.request
        reads = r"/releases(?:\?per_page=100&page=[1-9][0-9]*|/[1-9][0-9]*(?:/assets\?per_page=100&page=[1-9][0-9]*)?)"
        upload = r"/releases/[1-9][0-9]*/assets\?name=[A-Za-z0-9_.%-]+"
        allowed = ((method == "GET" and re.fullmatch(reads, path))
                   or (method == "POST" and blob is not None and re.fullmatch(upload, path))
                   or (method == "PATCH" and re.fullmatch(r"/releases/[1-9][0-9]*", path)
                       and data == {"draft": False, "prerelease": True, "make_latest": "false"}))
        if not allowed:
            raise ValueError("API operation outside the scoped release protocol")
        host = "uploads.github.com" if blob is not None else "api.github.com"
        body = blob if blob is not None else (json.dumps(data).encode() if data is not None else None)
        headers = {"Authorization": "Bearer " + self.token, "Accept": "application/vnd.github+json",
                   "X-GitHub-Api-Version": "2026-03-10", "User-Agent": "beme-alpha-release",
                   "Content-Type": "application/octet-stream" if blob is not None else "application/json"}
        request = urllib.request.Request(f"https://{host}/repos/{REPOSITORY}{path}", data=body, headers=headers, method=method)
        with self.opener(request, timeout=120) as response:
            return json.load(response)


def main():
    import argparse
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    commands = parser.add_subparsers(dest="command", required=True)
    build = commands.add_parser("package", help="build four executables; local snapshots are not publishable")
    build.add_argument("--out", type=Path, required=True)
    build.add_argument("--target", choices=(*TARGETS, "native"), default="native")
    build.add_argument("--smoke", action="store_true", help="execute the built archive on its native platform")
    smoke = commands.add_parser("smoke", help="verify and run a native archive outside the repository")
    smoke.add_argument("--archive", type=Path, required=True)
    gate = commands.add_parser("gate", help="owner tag, main ancestry and version gates")
    gate.add_argument("--tag", required=True)
    for name in ("checksums", "publish"):
        p = commands.add_parser(name)
        p.add_argument("--out", type=Path, required=True)
        p.add_argument("--tag", required=True)
    args = parser.parse_args()
    root = args.repo.resolve()
    if args.command == "package":
        target = native_target() if args.target == "native" else args.target
        if args.smoke:
            require_native(target)
        path = package(root, args.out, target)
        print(json.dumps(smoke_archive(path, root), sort_keys=True) if args.smoke else path)
    elif args.command == "smoke":
        print(json.dumps(smoke_archive(args.archive, root), sort_keys=True))
    elif args.command == "checksums":
        validate_version(root, args.tag)
        print(checksums(args.out, args.tag, source_identity(root)))
    else:
        if args.command == "publish" and (os.environ.get("RELEASE_CI_RESULT") != "success"
                                         or os.environ.get("RELEASE_ARTIFACT_RESULT") != "success"):
            raise ValueError("complete CI and native artifacts must succeed in this workflow run")
        validate_trigger(os.environ, args.tag)
        validate_version(root, args.tag)
        if args.command == "publish":
            sha = os.environ.get("RELEASE_SOURCE_SHA", "")
        elif os.environ.get("GITHUB_EVENT_NAME") == "push":
            sha = os.environ.get("GITHUB_SHA", "")
        else:
            # Dispatch's github.sha is main, not necessarily the requested tag.
            sha = command(["git", "rev-parse", "HEAD^{commit}"], root)
        verify_source(root, args.tag, sha)
        if args.command == "gate":
            source = source_identity(root)
            if os.environ.get("GITHUB_OUTPUT"):
                with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8", newline="\n") as dest:
                    dest.write(f"sha={source['commit']}\ntag={args.tag}\n")
            print(json.dumps({"source": source, "tag": args.tag}, sort_keys=True))
        else:
            api = GitHubAPI(os.environ.get("GH_TOKEN", ""))
            result = publish(api, args.out, args.tag, source_identity(root))
            print(json.dumps({"release_id": result["id"], "tag": result["tag_name"], "draft": result["draft"]}))


if __name__ == "__main__":
    import sys
    try:
        main()
    except (ValueError, OSError, subprocess.SubprocessError) as exc:
        print(f"release refused: {exc}", file=sys.stderr)
        if isinstance(exc, subprocess.CalledProcessError):
            print(exc.stdout or "", file=sys.stderr)
            print(exc.stderr or "", file=sys.stderr)
        sys.exit(1)
