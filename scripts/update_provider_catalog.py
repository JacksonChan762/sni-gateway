#!/usr/bin/env python3
"""Build a compact, reproducible server snapshot from qdm12/gluetun-servers."""
import argparse
import gzip
import ipaddress
import json
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / 'backend/internal/catalog/data/servers.json.gz'


def build(source: Path) -> dict:
    directory = source / 'pkg/servers'
    if not (source / 'LICENSE').is_file() or not directory.is_dir():
        raise ValueError('source must be a gluetun-servers checkout with its LICENSE')
    commit = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    providers = {}
    for path in sorted(directory.glob('*.json')):
        if path.name == 'manifest.json':
            continue
        entries = []
        for server in json.loads(path.read_text())['servers']:
            protocol = server.get('vpn')
            if protocol not in ('wireguard', 'openvpn'):
                continue
            addresses = []
            for raw in server.get('ips', []):
                try:
                    addresses.append(str(ipaddress.ip_address(raw)))
                except ValueError:
                    pass
            if not addresses:
                continue
            entries.append({
                'id': len(entries), 'protocol': protocol,
                'country': server.get('country', ''), 'region': server.get('region', ''),
                'city': server.get('city', ''), 'name': server.get('server_name', '') or server.get('hostname', ''),
                'hostname': server.get('hostname', ''), 'ip': addresses[0],
                'public_key': server.get('wgpubkey', '') if protocol == 'wireguard' else '',
                'udp': bool(server.get('udp')), 'tcp': bool(server.get('tcp')),
            })
        providers[path.stem] = entries
    return {'source_commit': commit, 'providers': providers}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path, help='local gluetun-servers Git checkout')
    args = parser.parse_args()
    data = build(args.source)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with OUTPUT.open('wb') as file:
        with gzip.GzipFile(fileobj=file, mode='wb', filename='', mtime=0, compresslevel=9) as stream:
            stream.write(json.dumps(data, ensure_ascii=False, separators=(',', ':'), sort_keys=True).encode())
    shutil.copyfile(args.source / 'LICENSE', OUTPUT.parent / 'LICENSE.gluetun-servers')
    print(f"{len(data['providers'])} providers, {sum(map(len, data['providers'].values()))} servers, {OUTPUT.stat().st_size} bytes, {data['source_commit']}")


if __name__ == '__main__':
    main()
