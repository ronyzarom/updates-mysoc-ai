#!/usr/bin/env python3
"""Root prerequisite: install the read-only observer from the signed .30 cache.

Never replaces or restarts the pod controller. Run after normal cascade bootstrap.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import urllib.request

VERSION = '3.3.152.30'
DIGEST = '61bef7f75d77bc490c4ee4dd5822f3a077a586aa4107118e25fe1b8d8fbe82cc'


def atomic_bytes(target, data, mode):
    fd, temporary = tempfile.mkstemp(prefix='.observer-', dir=target.parent)
    try:
        with os.fdopen(fd, 'wb') as out:
            out.write(data)
            os.fchmod(out.fileno(), mode)
            out.flush()
            os.fsync(out.fileno())
        os.replace(temporary, target)
        directory = os.open(target.parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vm-id', required=True)
    parser.add_argument('--role', choices=['a', 'b'], required=True)
    args = parser.parse_args()
    assert os.geteuid() == 0
    req = urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/id',
                                 headers={'Metadata-Flavor': 'Google'})
    assert urllib.request.urlopen(req, timeout=5).read().decode() == args.vm_id
    path = Path('/usr/local/lib/siemcore-cascade/greenfield-hook.py')
    assert not path.is_symlink() and path.stat().st_uid == 0 and not path.stat().st_mode & 0o022
    spec = importlib.util.spec_from_file_location('trusted_hook', path)
    hook = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(hook)
    receipt = hook.protected(Path('/etc/siemcore/greenfield-release.json'))
    assert receipt['version'] == VERSION and receipt['sha256'] == DIGEST
    app = hook.protected(Path('/etc/siemcore/greenfield.json'))
    assert app['cluster_id'] == 'bezeq-pod-test' and app['pod_role'] == args.role
    assert app['machine_id'] == Path('/etc/machine-id').read_text().strip()
    cfg = hook.protected(Path('/etc/siemcore-pod-controller/controller.json'))
    peer = 'b' if args.role == 'a' else 'a'
    assert cfg['pod_id'] == 'bezeq-pod-test' and cfg['node_id'] == args.role and cfg['peer_id'] == peer
    assert cfg['peer_agent']['url'] == 'https://10.89.0.' + ('3' if peer == 'b' else '2') + ':9444'
    minimal = {'pod_id': cfg['pod_id'], 'node_id': args.role, 'peer_id': peer,
               'endpoint': cfg['peer_agent']['url']}
    for key, name in [('ca', 'agent-ca.crt'), ('cert', 'agent.crt'), ('key', 'agent.key')]:
        expected = '/etc/siemcore-pod-agent/' + name
        assert cfg['peer_agent'][key] == expected and Path(expected).is_file()
        minimal[key] = expected
    assert Path('/run/siemcore-pod-controller').is_dir()
    root = Path('/usr/local/libexec/siemcore-pod-observer')
    assert not root.is_symlink()
    root.mkdir(mode=0o755, exist_ok=True)
    assert root.stat().st_uid == 0 and not root.stat().st_mode & 0o022
    with tempfile.TemporaryDirectory(prefix='verified-observer-', dir='/var/lib/siemcore-greenfield') as directory:
        entry = {'artifact': '/var/lib/siemcore-cascade-updater/artifacts/siemcore-' + VERSION + '.artifact',
                 'sha256': DIGEST, 'signature': receipt['signature']}
        bundle = hook.verified_bundle(entry, VERSION, receipt['public_key'], Path(directory))
        binary = (bundle / 'pod/bin/siemcore').read_bytes()
        unit = (bundle / 'pod/units/siemcore-pod-observer.service').read_bytes()
        version_dir = root / VERSION
        assert not version_dir.is_symlink()
        version_dir.mkdir(mode=0o755, exist_ok=True)
        assert version_dir.stat().st_uid == 0 and not version_dir.stat().st_mode & 0o022
        target = version_dir / 'siemcore'
        if target.exists():
            assert not target.is_symlink() and target.read_bytes() == binary
        else:
            atomic_bytes(target, binary, 0o755)
        config_dir = Path('/etc/siemcore-pod-observer')
        assert not config_dir.is_symlink()
        config_dir.mkdir(mode=0o700, exist_ok=True)
        assert config_dir.stat().st_uid == 0 and not config_dir.stat().st_mode & 0o077
        hook.atomic_private_json(config_dir / 'config.json', minimal)
        unit_path = Path('/etc/systemd/system/siemcore-pod-observer.service')
        assert not unit_path.is_symlink()
        if unit_path.exists():
            assert unit_path.read_bytes() == unit
        else:
            atomic_bytes(unit_path, unit, 0o644)
        current = root / 'current'
        if current.is_symlink():
            assert current.resolve() == version_dir
        else:
            assert not current.exists()
            current.symlink_to(VERSION)
        subprocess.run(['systemd-analyze', 'verify', str(unit_path)], check=True, timeout=15)
        subprocess.run(['systemctl', 'daemon-reload'], check=True, timeout=15)
        subprocess.run(['systemctl', 'enable', '--now', 'siemcore-pod-observer'], check=True, timeout=20)
        subprocess.run(['systemctl', 'is-active', '--quiet', 'siemcore-pod-observer'], check=True, timeout=10)
        print(json.dumps({'observer_installed': VERSION, 'vm_id': args.vm_id, 'role': args.role,
                          'artifact_sha256': DIGEST, 'binary_sha256': hashlib.sha256(binary).hexdigest(),
                          'controller_restarted': False}))


if __name__ == '__main__':
    main()
