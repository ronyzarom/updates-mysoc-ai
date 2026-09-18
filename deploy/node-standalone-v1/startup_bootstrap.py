#!/usr/bin/env python3
"""Pinned startup verifier. Metadata supplies only public URLs/digests/phase.

Render constants for one reviewed kit before installing as GCP startup metadata.
Never execute the downloaded installer before signature/checksum verification.
"""
import base64
import hashlib
import json
import os
from pathlib import Path,PurePosixPath
import subprocess
import tarfile
import urllib.request
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

BASE_URL='RENDER_KIT_HTTPS_BASE'
MANIFEST_SHA256='RENDER_MANIFEST_SHA256'
PUBLIC_KEY='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
PHASE='enroll'


def fetch(name,limit):
    url=BASE_URL.rstrip('/')+'/'+name
    if not url.startswith('https://updates.mysoc.ai/downloads/'):raise ValueError('approved_https_source_required')
    with urllib.request.urlopen(url,timeout=30) as response:
        if response.url!=url or response.status!=200:raise ValueError('download_redirect_or_status')
        raw=response.read(limit+1)
    if len(raw)>limit:raise ValueError('download_limit')
    return raw


def main():
    os.umask(0o077)
    if os.geteuid()!=0 or PHASE not in ('enroll','install'):raise ValueError('root_bootstrap_phase_required')
    raw=fetch('manifest.json',65536)
    if hashlib.sha256(raw).hexdigest()!=MANIFEST_SHA256:raise ValueError('pinned_manifest_mismatch')
    signature=base64.b64decode(fetch('manifest.sig',1024).strip(),validate=True)
    Ed25519PublicKey.from_public_bytes(bytes.fromhex(PUBLIC_KEY)).verify(signature,b'mysoc-installation-repository-v1\n'+raw)
    manifest=json.loads(raw)
    archive_name=manifest['archive']
    if PurePosixPath(archive_name).name!=archive_name:raise ValueError('invalid_archive_name')
    artifact=fetch(archive_name,8*1024*1024)
    if len(artifact)!=manifest['size'] or hashlib.sha256(artifact).hexdigest()!=manifest['sha256']:raise ValueError('kit_checksum_mismatch')
    stage=Path('/root/updates-standalone-prerequisite')/manifest['version']
    stage.mkdir(mode=0o700,parents=True,exist_ok=True)
    for path in (stage,*stage.parents):
        st=path.lstat()
        if st.st_uid!=0 or st.st_mode&0o022 or path.is_symlink():raise ValueError('protected_stage_required')
    archive=stage/'kit.tar.gz';archive.write_bytes(artifact);archive.chmod(0o600)
    kit_name=archive_name.removesuffix('.tar.gz');seen=set()
    with tarfile.open(archive,'r:gz') as stream:
        for member in stream:
            parts=PurePosixPath(member.name).parts
            if not parts or parts[0]!=kit_name or '..' in parts or member.name.startswith('/') or not(member.isdir() or member.isfile()):raise ValueError('unsafe_kit_member')
            path=stage.joinpath(*parts)
            if member.isdir():path.mkdir(mode=0o700,parents=True,exist_ok=True);continue
            name='/'.join(parts[1:])
            if name in seen or name not in manifest['files'] or member.size>2*1024*1024:raise ValueError('unexpected_kit_file')
            content=stream.extractfile(member).read()
            if hashlib.sha256(content).hexdigest()!=manifest['files'][name]:raise ValueError('kit_file_mismatch')
            path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
            if path.exists() and (path.is_symlink() or path.read_bytes()!=content):raise ValueError('staged_file_conflict')
            if not path.exists():path.write_bytes(content);path.chmod(0o600)
            seen.add(name)
    if seen!=set(manifest['files']):raise ValueError('incomplete_kit')
    # Phase2 encrypted capsule download will be pinned by its separate signed
    # capsule manifest before install. This phase1 renderer never fetches secrets.
    if PHASE!='enroll':raise ValueError('phase2_capsule_transport_not_rendered')
    subprocess.run(['/usr/bin/python3','-I',str(stage/kit_name/'install.py'),PHASE],check=True,timeout=180)


if __name__=='__main__':main()
