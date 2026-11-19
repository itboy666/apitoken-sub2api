#!/usr/bin/env python3
"""Build a deterministic .s2plugin ZIP; keep the signing key outside the repo."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    signer = parser.add_mutually_exclusive_group(required=True)
    signer.add_argument('--key', type=Path, help='Ed25519 private PEM outside this source tree')
    signer.add_argument('--unsigned', action='store_true', help='Development only; production host rejects this package')
    parser.add_argument('--openssl', default='openssl', help='OpenSSL 3 binary used for Ed25519 signing')
    args = parser.parse_args()
    manifest = json.loads((ROOT / 'manifest.template.json').read_text(encoding='utf-8'))
    assets = {}
    for directory in ('bin', 'ui'):
        for path in sorted((ROOT / directory).rglob('*')):
            if path.is_symlink():
                raise SystemExit('Symlinks are not package assets')
            if path.is_file():
                assets[path.relative_to(ROOT).as_posix()] = path.read_bytes()
    manifest['files'] = {name: hashlib.sha256(data).hexdigest() for name, data in assets.items()}
    manifest_bytes = (json.dumps(manifest, indent=2, ensure_ascii=False) + '\n').encode('utf-8')
    entries = {'manifest.json': manifest_bytes, **assets}
    if args.key:
        key = args.key.resolve(strict=True)
        repository = ROOT.parents[1]
        if key == repository or repository in key.parents:
            raise SystemExit('Signing key must be outside the source repository')
        openssl = shutil.which(args.openssl)
        if not openssl:
            raise SystemExit('OpenSSL binary not found')
        with tempfile.TemporaryDirectory(prefix='usdt-payment-sign-') as temporary:
            manifest_path = Path(temporary) / 'manifest.json'
            signature_path = Path(temporary) / 'signature.bin'
            manifest_path.write_bytes(manifest_bytes)
            subprocess.run([openssl, 'pkeyutl', '-sign', '-rawin', '-inkey', str(key), '-in', str(manifest_path), '-out', str(signature_path)], check=True, stdout=subprocess.DEVNULL)
            signature = signature_path.read_bytes()
        if len(signature) != 64:
            raise SystemExit('Expected an Ed25519 signature')
        entries['signature.json'] = (json.dumps({'algorithm': 'ed25519', 'key_id': 'apitoken-usdt-trc20-v1', 'signature': base64.b64encode(signature).decode('ascii')}, indent=2) + '\n').encode('utf-8')
    output = args.output.resolve()
    if output.suffix != '.s2plugin':
        raise SystemExit('Output extension must be .s2plugin')
    if ROOT in output.parents:
        raise SystemExit('Build outputs belong outside the plugin source tree')
    output.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(output, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as package:
        for name, data in sorted(entries.items()):
            info = zipfile.ZipInfo(name, date_time=(2026, 11, 19, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.create_system = 3
            info.external_attr = (0o100755 if name.startswith("bin/") else 0o100644) << 16
            package.writestr(info, data)
    print(json.dumps({'file': str(output), 'version': manifest['version'], 'signed': bool(args.key), 'bytes': output.stat().st_size, 'sha256': hashlib.sha256(output.read_bytes()).hexdigest()}, ensure_ascii=False))


if __name__ == '__main__':
    main()
