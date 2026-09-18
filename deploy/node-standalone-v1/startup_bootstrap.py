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
CAPSULE_BASE_URL='RENDER_CAPSULE_HTTPS_BASE'
CAPSULE_MANIFEST_SHA256='RENDER_CAPSULE_MANIFEST_SHA256'


def fetch(name,limit,base=None):
    url=(base or BASE_URL).rstrip('/')+'/'+name
    if not url.startswith('https://updates.mysoc.ai/downloads/'):raise ValueError('approved_https_source_required')
    with urllib.request.urlopen(url,timeout=30) as response:
        if response.url!=url or response.status!=200:raise ValueError('download_redirect_or_status')
        raw=response.read(limit+1)
    if len(raw)>limit:raise ValueError('download_limit')
    return raw


def stage_capsule(kit):
    capsule_raw=fetch('manifest.json',65536,CAPSULE_BASE_URL)
    if hashlib.sha256(capsule_raw).hexdigest()!=CAPSULE_MANIFEST_SHA256:raise ValueError('pinned_capsule_manifest_mismatch')
    capsule_manifest=json.loads(capsule_raw)
    capsule_signature=fetch('manifest.sig',1024,CAPSULE_BASE_URL)
    canonical=json.dumps(capsule_manifest,sort_keys=True,separators=(',',':'),ensure_ascii=True).encode()
    Ed25519PublicKey.from_public_bytes(bytes.fromhex(PUBLIC_KEY)).verify(base64.b64decode(capsule_signature.strip(),validate=True),b'mysoc-standalone-inputs-v1\n'+canonical)
    ciphertext=fetch('inputs.cms',4*1024*1024,CAPSULE_BASE_URL)
    if hashlib.sha256(ciphertext).hexdigest()!=capsule_manifest['ciphertext_sha256']:raise ValueError('capsule_ciphertext_mismatch')
    capsule=kit/'capsule';capsule.mkdir(mode=0o700,exist_ok=True)
    if capsule.is_symlink() or capsule.stat().st_uid!=0 or capsule.stat().st_mode&0o077:raise ValueError('protected_capsule_directory_required')
    for name,content in {'manifest.json':capsule_raw,'manifest.sig':capsule_signature,'inputs.cms':ciphertext}.items():
        path=capsule/name
        if path.is_symlink() or (path.exists() and path.read_bytes()!=content):raise ValueError('capsule_retry_conflict')
        if not path.exists():
            with path.open('xb') as out:out.write(content);out.flush();os.fsync(out.fileno())
            path.chmod(0o600)
    # Installer independently verifies signature, host/recipient identity,
    # expiry and exact decrypted file inventory before installing anything.

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
    if PHASE=='install':stage_capsule(stage/kit_name)
    subprocess.run(['/usr/bin/python3','-I',str(stage/kit_name/'install.py'),PHASE],check=True,timeout=180)


if __name__=='__main__':main()
