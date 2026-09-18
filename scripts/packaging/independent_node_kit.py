#!/usr/bin/env python3
"""Build a versioned independent-node kit from clean, committed, signed inputs.

This packages only; it never publishes, enrolls, or changes fleet targeting.
Requires cryptography for verification against the independently supplied key.
"""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tarfile
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey


def clean_commit(root):
    if subprocess.check_output(['git', 'status', '--porcelain'], cwd=root, text=True).strip():
        raise ValueError('source must be committed and clean: ' + str(root))
    return subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()


def package(args, root):
    source_commit = clean_commit(root)
    provisioning = Path(args.provisioning_source).resolve()
    provisioning_commit = clean_commit(provisioning)
    version = (root / 'VERSION').read_text().strip()
    if not re.fullmatch(r'r[1-9][0-9]*', args.package_revision):
        raise ValueError('package revision must be r followed by a positive integer')
    receipt = json.loads(Path(args.receipt).read_text())
    binary = Path(args.binary).read_bytes()
    digest = hashlib.sha256(binary).hexdigest()
    product = 'updater-linux-' + args.architecture
    if receipt['version'] != version or receipt['sha256'] != digest or receipt['product'] != product:
        raise ValueError('updater receipt/version/architecture mismatch')
    if len(binary) < 20 or binary[:6] != b'\x7fELF\x02\x01' or struct.unpack('<H', binary[18:20])[0] != {'amd64':62,'arm64':183}[args.architecture]:
        raise ValueError('updater ELF architecture mismatch')
    Ed25519PublicKey.from_public_bytes(bytes.fromhex(args.public_key)).verify(
        base64.b64decode(receipt['signature'], validate=True),
        ('mysoc-release-v1\n'+product+'\n'+version+'\n'+digest).encode())
    modules = ['greenfield-hook.py', 'recovery/artifact.py', 'recovery/recovery.py', 'recovery/supervise.py']
    for name in modules:
        path = provisioning / 'deploy/cascade' / name
        if path.is_symlink() or not path.is_file():
            raise ValueError('missing regular provisioning module: '+name)
    out = Path(args.output)
    out.mkdir(parents=True, exist_ok=False)
    name = f'siemcore-updater-kit-{version}-{args.package_revision}-linux-{args.architecture}'
    kit = out / name
    shutil.copytree(root / 'kits/siemcore', kit, ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
    for path in kit.iterdir():
        if path.is_symlink() or not path.is_file():
            raise ValueError('unexpected kit template entry')
        path.write_bytes(path.read_bytes().replace(b'@VERSION@', version.encode()))
    (kit / 'bin').mkdir()
    executable = kit / 'bin' / ('siemcore-cascade-updater-linux-'+args.architecture)
    executable.write_bytes(binary)
    executable.chmod(0o755)
    (kit / 'install.sh').chmod(0o755)
    for module in modules:
        target = kit / module
        target.parent.mkdir(exist_ok=True)
        shutil.copyfile(provisioning / 'deploy/cascade' / module, target)
    (kit / 'PROVISIONING_COMMIT').write_text(provisioning_commit+'\n')
    marker = dict(schema=1, protocol='pod-node-bootstrap-v1', provisioning_commit=provisioning_commit,
                  hook_sha256=hashlib.sha256((kit/'greenfield-hook.py').read_bytes()).hexdigest())
    (kit/'INDEPENDENT-NODE-BOOTSTRAP.json').write_text(json.dumps(marker, indent=2)+'\n')
    (kit/'UPDATER-SIGNATURE.json').write_text(json.dumps(receipt, indent=2)+'\n')
    metadata = dict(package_revision=version+'-'+args.package_revision, architecture='linux-'+args.architecture,
                    template_commit=source_commit, provisioning_commit=provisioning_commit,
                    updater_version=version, updater_sha256=digest, protocol='pod-node-bootstrap-v1')
    (kit/'PACKAGE.json').write_text(json.dumps(metadata, indent=2)+'\n')
    (kit/'docs').mkdir()
    for doc in ['INDEPENDENT-POD-NODE-BOOTSTRAP.md','SIEMCORE-INSTALLATION-ROLES.md','SIEMCORE-CLEAN-INSTALL-PARAMETERS.md']:
        shutil.copyfile(root/'docs'/doc, kit/'docs'/doc)
    files = sorted(path for path in kit.rglob('*') if path.is_file())
    (kit/'SHA256SUMS').write_text(''.join(hashlib.sha256(path.read_bytes()).hexdigest()+'  '+str(path.relative_to(kit))+'\n' for path in files))
    archive = out/(name+'.tar.gz')
    with tarfile.open(archive, 'w:gz') as stream:
        stream.add(kit, arcname=name)
    metadata.update(filename=archive.name, size=archive.stat().st_size, sha256=hashlib.sha256(archive.read_bytes()).hexdigest())
    (out/'manifest.json').write_text(json.dumps(dict(schema=1, packages=[metadata]), indent=2)+'\n')
    (out/'SHA256SUMS').write_text(metadata['sha256']+'  '+archive.name+'\n')
    shutil.rmtree(kit)
    return metadata


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    for arg in ['binary','receipt','public-key','provisioning-source','package-revision','output']:
        parser.add_argument('--'+arg, required=True)
    parser.add_argument('--architecture', required=True, choices=['amd64','arm64'])
    print(json.dumps(package(parser.parse_args(), Path(__file__).resolve().parents[2]), indent=2))
