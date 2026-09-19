#!/usr/bin/env python3
"""Prepare an offline, disposable Linux fixture, never a product installation.

Run only on a fresh disposable Linux AMD64 VM with synthetic inputs.
Uses synthetic identities and a synthetic CA; does NOT qualify SSL.com issuance,
real MySoc/archive integration, CVE acceptance, or a new application release.
The actual Updates kit must execute the signed fixture through its usual hook.
"""
import argparse
import re
import base64
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tarfile
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization

ROOT=Path('/etc/normal-qualification')


def run(args):
    subprocess.run(args,check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)

def private(path,data):
    path.write_bytes(data if isinstance(data,bytes) else data.encode());path.chmod(0o600)

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifact',type=Path,required=True)
    parser.add_argument('--version',required=True)
    parser.add_argument('--sha256',required=True)
    a=parser.parse_args()
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+',a.version):
        raise SystemExit('explicit four-part candidate version required')
    if hashlib.sha256(a.artifact.read_bytes()).hexdigest()!=a.sha256:
        raise SystemExit('candidate artifact digest mismatch')
    with tarfile.open(a.artifact) as t:
        manifest=json.load(t.extractfile('siemcore-universal-'+a.version+'/MANIFEST.json'))
    if manifest.get('version')!=a.version or 'normal-prerequisites-v1' not in manifest.get('normal_capabilities',[]):
        raise SystemExit('candidate identity/capability mismatch')
    if os.geteuid()!=0 or os.environ.get('NORMAL_CLEAN_FIXTURE')!='1' or Path('/.dockerenv').exists():
        raise SystemExit('explicit disposable root fixture required')
    if ROOT.exists() or Path('/var/lib/siemcore-greenfield').exists():
        raise SystemExit('fresh fixture required')
    containers=subprocess.check_output(['docker','ps','-aq'],text=True).strip()
    volumes=subprocess.check_output(['docker','volume','ls','-q'],text=True).strip()
    if containers or volumes:raise SystemExit('disposable Docker host must be empty')
    ROOT.mkdir(mode=0o700);c=ROOT/'credentials';c.mkdir(mode=0o700)
    # Synthetic CA is installed only in this disposable VM's trust store.
    run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(c/'ca.key'),'-out',str(c/'ca.crt'),'-days','3','-subj','/CN=SSL.com SYNTHETIC TEST ONLY CA'])
    run(['openssl','req','-newkey','rsa:2048','-nodes','-keyout',str(c/'server.key'),'-out',str(c/'server.csr'),'-subj','/CN=normal.fixture'])
    private(c/'ext','subjectAltName=DNS:normal.fixture,DNS:mysoc.fixture\nextendedKeyUsage=serverAuth\n')
    run(['openssl','x509','-req','-in',str(c/'server.csr'),'-CA',str(c/'ca.crt'),'-CAkey',str(c/'ca.key'),'-CAcreateserial','-out',str(c/'server.crt'),'-days','3','-extfile',str(c/'ext')])
    for p in c.iterdir():p.chmod(0o600)
    shutil.copyfile(c/'ca.crt','/usr/local/share/ca-certificates/normal-fixture.crt');run(['update-ca-certificates'])
    with Path('/etc/hosts').open('a') as f:f.write('\n127.0.0.1 normal.fixture mysoc.fixture\n')
    refs={'etcd':'siemcore/etcd:3.5.33-security2','patroni':'siemcore/patroni-pg16:normal-security-20260919','redis':'redis:7.2-alpine','nginx':'nginx:1.30.5-alpine'}
    images={k:dict(reference=v,image_id=json.loads(subprocess.check_output(['docker','image','inspect',v]))[0]['Id']) for k,v in refs.items()}
    private(ROOT/'handoff.json',json.dumps(dict(schema=1,profile='normal',images=images,
        security_qualification=dict(status='approved',scope='disposable synthetic test ONLY, not live security acceptance'),
        management_tls=dict(hostname='normal.fixture',certificate=str(c/'server.crt'),private_key=str(c/'server.key')))))
    app=dict(schema=3,topology='single',cluster_id='normal-fixture',instance_id='fixture-customer',updater_instance_id='fixture-normal-updater',
        database_name='siemcore',frontend_url='https://normal.fixture',mysoc_url='https://mysoc.fixture',admin_email='fixture@example.invalid',mysoc_api_key=secrets.token_hex(32),
        normal_prerequisites=dict(schema=1,path=str(ROOT/'handoff.json'),sha256=hashlib.sha256((ROOT/'handoff.json').read_bytes()).hexdigest()))
    archive=ROOT/'release.tar.gz'
    shutil.copyfile(a.artifact,archive)
    digest=hashlib.sha256(archive.read_bytes()).hexdigest();key=Ed25519PrivateKey.generate()
    private(ROOT/'fixture-signing-key.raw',key.private_bytes(serialization.Encoding.Raw,serialization.PrivateFormat.Raw,serialization.NoEncryption()))
    public=key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
    signature=base64.b64encode(key.sign(('mysoc-release-v1\nsiemcore\n'+a.version+'\n'+digest).encode())).decode()
    release=dict(version=a.version,sha256=digest,public_key=public,signature=signature,channel='normal-fixture-only',required_capabilities=['normal-prerequisites-v1'])
    private(ROOT/'envelope.json',json.dumps(dict(application=app,release=release)))
    print('Prepared synthetic signed Normal fixture; not a release or live security approval')

if __name__=='__main__':main()
