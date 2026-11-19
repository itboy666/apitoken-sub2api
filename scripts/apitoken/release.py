#!/usr/bin/env python3
"""Verify, test and package the APIToken distribution; never upload or deploy."""
import argparse
from datetime import datetime, timezone
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

from sync_upstream import assert_protected_migrations

ROOT = Path(__file__).resolve().parents[2]
VERSION = ROOT / "backend/cmd/server/VERSION"
HOST_PATTERN = r"\d+\.\d+\.\d+-apitoken\.[1-9]\d*"
GO = os.environ.get("APITOKEN_GO", "go")
NODE = os.environ.get("APITOKEN_NODE", "node")


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(args, cwd=ROOT, env=None):
    subprocess.run([str(item) for item in args], cwd=cwd, env=env, check=True)


def verify_inputs():
    version = VERSION.read_text().strip()
    if not re.fullmatch(HOST_PATTERN, version):
        raise ValueError("A vX.Y.Z-apitoken.N distribution version is required")
    upstream = json.loads((ROOT / "deploy/apitoken-upstream.json").read_text())
    if version.split("-")[0] != upstream["tag"].removeprefix("v"):
        raise ValueError("VERSION differs from the recorded upstream base")
    run(["git", "merge-base", "--is-ancestor", upstream["commit"], "HEAD"])
    assert_protected_migrations(upstream)
    lock = json.loads((ROOT / "deploy/apitoken-plugins.lock.json").read_text())
    for collection in (lock["packages"], lock["source_files"]):
        for name, digest in collection.items():
            original = ROOT / name
            path = original.resolve()
            if not path.is_relative_to(ROOT) or original.is_symlink() or sha256(path) != digest:
                raise ValueError(f"Signed plugin input changed; rebuild, test and lock it first: {name}")
    return version, upstream, lock


def tests():
    version, _, lock = verify_inputs()
    if not os.environ.get("USDT_TEST_POSTGRES_DSN"):
        raise ValueError("USDT_TEST_POSTGRES_DSN must point at a disposable database; do not skip payment checks")
    packages = [ROOT / name for name in lock["packages"]]
    usdt = next(p for p in packages if "usdt-trc20" in p.name)
    support = next(p for p in packages if "support-widget" in p.name)
    target = {("Linux", "x86_64"): "linux-amd64", ("Darwin", "arm64"): "darwin-arm64"}.get((platform.system(), platform.machine()))
    if not target:
        raise ValueError("This release pipeline supports Linux amd64 and macOS arm64 test runners")
    with tempfile.TemporaryDirectory(prefix="apitoken-plugin-") as temp:
        binary = Path(temp) / "usdt-trc20"
        with zipfile.ZipFile(usdt) as package:
            binary.write_bytes(package.read(f"bin/{target}/usdt-trc20"))
        binary.chmod(0o755)
        env = dict(os.environ, USDT_PLUGIN_BINARY=str(binary), USDT_PLUGIN_PACKAGE=str(usdt), SUB2API_TEST_WEBSITE_PLUGIN_PACKAGE=str(support), SUB2API_TEST_WEBSITE_HOST_VERSION=version, SUB2API_TEST_CANDIDATE_HOST_VERSION=version)
        run([GO, "test", "-race", "-tags", "unit", "./internal/service", "./internal/handler/admin", "./internal/repository", "./cmd/server", "./internal/server/routes", "-run", "^(TestUpdateService|TestCombined|TestSourceBuild|TestProvideServiceBuildInfo|TestSystemHandler|TestUSDT|TestWebsite|TestPlugin|TestPublicWebsite|TestLegacySupportWidget|TestPluginPackage|TestEvaluatePluginCompatibility|TestAutoConfig|TestOAuthAutoConfig|TestDeleteGroup)", "-count=1"], ROOT / "backend", env)
        run([GO, "test", "-race", "./internal/tronpayment", "-count=1"], ROOT / "backend", env)
    frontend = ROOT / "frontend"
    run([NODE, "node_modules/vitest/vitest.mjs", "run", "src/components/common/__tests__/VersionBadge.spec.ts", "src/components/payment/__tests__/paymentFlow.spec.ts", "src/views/user/__tests__/CryptoPaymentView.spec.ts", "src/views/user/__tests__/PaymentView.spec.ts", "src/views/admin/__tests__/PluginsView.spec.ts", "src/stores/__tests__/app.spec.ts", "src/api/__tests__/admin.system.rollback.spec.ts", "src/i18n/__tests__/localeKeyCompleteness.spec.ts"], frontend)
    run([NODE, "--test", "extensions/support-widget/__tests__/package.test.mjs", "extensions/usdt-trc20/__tests__/config.test.mjs"])
    run([NODE, "node_modules/vue-tsc/bin/vue-tsc.js", "-b"], frontend)
    run([NODE, "node_modules/vite/bin/vite.js", "build"], frontend)


def build(repository, output):
    version, upstream, lock = verify_inputs()
    if subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT, text=True).strip():
        raise ValueError("Commit reviewed changes before packaging; source and binary must refer to the same commit")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*", repository) or repository.lower() in {"ranxi2001/sub2api", "wei-shaw/sub2api"}:
        raise ValueError("Use the dedicated combined-release repository")
    if not (ROOT / "backend/internal/web/dist/index.html").is_file():
        raise ValueError("Build the tested frontend before packaging")
    sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    date = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    output = output.resolve()
    if output.exists() and any(output.iterdir()):
        raise ValueError("Use an empty output directory to avoid mixing releases")
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="apitoken-build-") as temp:
        binary = Path(temp) / "sub2api"
        flags = f"-s -w -X main.Version={version} -X main.Commit={sha} -X main.Date={date} -X main.BuildType=release -X main.UpdateChannel=apitoken -X main.UpdateRepository={repository} -X main.UpstreamVersion={upstream['tag'].removeprefix('v')}"
        run([GO, "build", "-trimpath", "-tags", "embed", "-ldflags", flags, "-o", binary, "./cmd/server"], ROOT / "backend", dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="amd64"))
        manifest = {"version": version, "repository": repository, "upstream_version": upstream["tag"].removeprefix("v"), "upstream_commit": upstream["commit"], "source_commit": sha, "customization_abi": upstream["customization_abi"], "binary_sha256": sha256(binary)}
        archive = output / f"sub2api_{version}_linux_amd64.tar.gz"
        with tarfile.open(archive, "w:gz") as tar:
            tar.add(binary, arcname="sub2api")
            payload = (json.dumps(manifest, indent=2) + "\n").encode()
            info = tarfile.TarInfo("apitoken-release.json")
            info.size, info.mode = len(payload), 0o644
            tar.addfile(info, io.BytesIO(payload))
            for name in ["LICENSE", "THIRD_PARTY_NOTICES.md"]:
                if (ROOT / name).exists():
                    tar.add(ROOT / name, arcname=name)
    for name in lock["packages"]:
        shutil.copyfile(ROOT / name, output / Path(name).name)
    source = output / f"sub2api_{version}_source.tar.gz"
    run(["git", "archive", "--format=tar.gz", "--output", source, "HEAD"])
    artifacts = sorted(p for p in output.iterdir() if p.name != "checksums.txt" and p.is_file())
    (output / "checksums.txt").write_text("".join(f"{sha256(p)}  {p.name}\n" for p in artifacts))
    print(json.dumps(manifest, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["verify", "test", "build"])
    parser.add_argument("--repository", default=os.environ.get("GITHUB_REPOSITORY", ""))
    parser.add_argument("--output", type=Path, default=ROOT / "dist/apitoken")
    args = parser.parse_args()
    if args.command == "test":
        tests()
    elif args.command == "build":
        build(args.repository, args.output)
    else:
        verify_inputs()
        print("Official base, applied migrations and signed plugin inputs verified")


if __name__ == "__main__":
    main()
