#!/usr/bin/env python3
"""Prepare a combined release in a clean checkout. Never deploy or push."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[2]
OFFICIAL = "ranxi2001/sub2api"
TAG = re.compile(r"v(\d+\.\d+\.\d+)")
CUSTOM = re.compile(r"(\d+\.\d+\.\d+)-apitoken\.([1-9]\d*)")


def git(*args, check=True):
    return subprocess.run(["git", *args], cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=check)


def stable_release(tag=""):
    if tag and not TAG.fullmatch(tag):
        raise ValueError("Only official vX.Y.Z stable tags are accepted")
    endpoint = "tags/" + tag if tag else "latest"
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "APIToken-combined-release"}
    token = os.environ.get("GH_TOKEN")
    if token:
        headers["Authorization"] = "Bearer " + token
    request = Request(f"https://api.github.com/repos/{OFFICIAL}/releases/{endpoint}", headers=headers)
    with urlopen(request, timeout=30) as response:
        release = json.load(response)
    selected = release.get("tag_name", "")
    if release.get("draft") or release.get("prerelease") or not TAG.fullmatch(selected) or (tag and selected != tag):
        raise ValueError("The selected upstream tag is not a published stable release")
    return selected


def choose_version(tag, current, tags):
    base = TAG.fullmatch(tag).group(1)
    match = CUSTOM.fullmatch(current)
    revision = int(match[2]) if match and match[1] == base else 1
    used = [int(m[2]) for item in tags if (m := CUSTOM.fullmatch(item.removeprefix("v"))) and m[1] == base]
    if used:
        revision = max(revision, max(used) + 1)
    return f"{base}-apitoken.{revision}"


def assert_protected_migrations(metadata):
    import hashlib
    for name, expected in metadata["protected_migrations"].items():
        contents = (ROOT / "backend/migrations" / name).read_bytes()
        if hashlib.sha256(contents).hexdigest() != expected:
            raise ValueError(f"Applied migration must not be rewritten: {name}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", default="", help="Official stable tag; omit to resolve latest")
    parser.add_argument("--output", type=Path, required=True, help="Candidate metadata outside the checkout")
    args = parser.parse_args()
    if git("status", "--porcelain").stdout.strip():
        raise ValueError("Use a clean checkout; commit custom changes before merging")
    if git("rev-parse", "--is-shallow-repository").stdout.strip() == "true":
        raise ValueError("A full Git history is required; fetch --unshallow first")
    metadata_path = ROOT / "deploy/apitoken-upstream.json"
    metadata = json.loads(metadata_path.read_text())
    assert_protected_migrations(metadata)
    original = git("rev-parse", "HEAD").stdout.strip()
    tag = stable_release(args.tag)
    tracking_ref = "refs/remotes/apitoken-upstream/" + tag
    git("fetch", "--no-tags", f"https://github.com/{OFFICIAL}.git", f"refs/tags/{tag}:{tracking_ref}")
    upstream_commit = git("rev-parse", tracking_ref + "^{commit}").stdout.strip()
    if git("merge-base", "--is-ancestor", metadata["commit"], upstream_commit, check=False).returncode:
        raise ValueError("Upstream tag is older than, or unrelated to, the recorded official base")
    current = (ROOT / "backend/cmd/server/VERSION").read_text().strip()
    version = choose_version(tag, current, git("tag", "--list", "v*-apitoken.*").stdout.splitlines())
    merged = git("merge", "--no-ff", "--no-commit", tracking_ref, check=False)
    if merged.returncode:
        conflicts = git("diff", "--name-only", "--diff-filter=U").stdout.splitlines()
        # VERSION intentionally differs in the distribution. Only this known
        # metadata conflict can be resolved automatically; source conflicts stop.
        if not conflicts or set(conflicts) != {"backend/cmd/server/VERSION"}:
            raise ValueError("Upstream merge stopped; review conflicts before publishing: " + ", ".join(conflicts))
    assert_protected_migrations(metadata)
    (ROOT / "backend/cmd/server/VERSION").write_text(version + "\n")
    metadata.update(tag=tag, commit=upstream_commit)
    metadata_path.write_text(json.dumps(metadata, indent=2) + "\n")
    git("add", "backend/cmd/server/VERSION", "deploy/apitoken-upstream.json")
    if git("diff", "--cached", "--quiet", check=False).returncode:
        git("commit", "-m", f"chore(apitoken): merge {tag} for v{version}")
    candidate = {"version": version, "tag": "v"+version, "sha": git("rev-parse", "HEAD").stdout.strip(), "original_sha": original, "upstream_tag": tag, "upstream_commit": upstream_commit}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(candidate, indent=2) + "\n")
    print(json.dumps(candidate))


if __name__ == "__main__":
    main()
