#!/usr/bin/env python3
"""Requires Python cryptography. Uses an independently pinned public key."""
import argparse,base64,hashlib,json,pathlib
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
p=argparse.ArgumentParser();p.add_argument('--public-key',required=True);p.add_argument('archive');a=p.parse_args()
manifest=pathlib.Path('manifest.json').read_bytes()
Ed25519PublicKey.from_public_bytes(bytes.fromhex(a.public_key)).verify(base64.b64decode(pathlib.Path('manifest.sig').read_text()),b'mysoc-installation-repository-v1\n'+manifest)
archive=pathlib.Path(a.archive);entries=[e for e in json.loads(manifest)['packages'] if e['filename']==archive.name]
assert len(entries)==1,'package not present in signed manifest'
data=archive.read_bytes();assert len(data)==entries[0]['size'] and hashlib.sha256(data).hexdigest()==entries[0]['sha256'],'package checksum mismatch'
print('Verified signed repository manifest and package:',archive.name)
