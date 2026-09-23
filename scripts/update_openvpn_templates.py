#!/usr/bin/env python3
"""Regenerate public OpenVPN provider templates from a Gluetun source checkout."""

import argparse
import gzip
import json
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / 'backend/internal/provider/data'
KEEP = {
    'proto', 'remote', 'remote-cert-tls', 'verify-x509-name',
    'key-direction', 'auth', 'data-ciphers', 'data-ciphers-fallback',
    'cipher', 'tls-cipher', 'mssfix', 'fragment', 'comp-lzo',
    'ping', 'reneg-sec', 'tun-mtu',
}


def compact(lines):
    result = []
    section = False
    for line in lines:
        if line.startswith('<') and not line.startswith('</'):
            section = True
            result.append(line)
        elif line.startswith('</'):
            section = False
            result.append(line)
        elif section or line and line.split()[0] in KEEP:
            result.append(line)
    if '<tls-crypt>' in result:
        result = [line for line in result if not line.startswith('key-direction ')]
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path, help='local qdm12/gluetun Git checkout')
    args = parser.parse_args()
    source = args.source.resolve()
    if not (source / 'LICENSE').is_file() or not (source / 'go.mod').is_file():
        parser.error('source must be a Gluetun source checkout')
    helper_dir = source / 'cmd/sni-profile-dump'
    if helper_dir.exists():
        parser.error(f'{helper_dir} already exists; refusing to overwrite it')
    helper_dir.mkdir(parents=True)
    helper_file = helper_dir / 'main.go'
    try:
        shutil.copyfile(ROOT / 'scripts/gluetun_profile_export.go', helper_file)
        raw = subprocess.check_output(['go', 'run', './cmd/sni-profile-dump'], cwd=source)
    finally:
        helper_file.unlink(missing_ok=True)
        helper_dir.rmdir()
    templates = json.loads(raw)
    for variants in templates.values():
        for protocol, lines in variants.items():
            variants[protocol] = compact(lines)
    OUTPUT.mkdir(parents=True, exist_ok=True)
    with (OUTPUT / 'openvpn_profiles.json.gz').open('wb') as file:
        with gzip.GzipFile(fileobj=file, mode='wb', filename='', mtime=0, compresslevel=9) as stream:
            stream.write(json.dumps(templates, separators=(',', ':'), sort_keys=True).encode())
    shutil.copyfile(source / 'LICENSE', OUTPUT / 'LICENSE.gluetun')
    commit = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    (OUTPUT / 'openvpn_source_commit.txt').write_text(commit + '\n')
    print(f'{len(templates)} OpenVPN providers from Gluetun {commit}')


if __name__ == '__main__':
    main()
