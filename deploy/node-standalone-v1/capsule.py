"""Signed host-bound CMS input capsule. Never used as a TLS certificate."""
import base64
from datetime import datetime, timezone
import hashlib
import json
from pathlib import PurePosixPath
import subprocess
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from protocol import strict_json
from transaction import canonical

CAPSULE_PROTOCOL = 'pod-node-standalone-inputs-v1'
ALLOWED_FILES = {'mysoc-bootstrap.json', 'archive-credentials/gcp-archiver.json'}


def decrypt_capsule(manifest, ciphertext, signature, public_key, identity, recipient_pem, private_key_path):
    required={'protocol','identity','recipient_sha256','ciphertext_sha256','expires_at'}
    if not isinstance(manifest,dict) or set(manifest)!=required or manifest['protocol']!=CAPSULE_PROTOCOL:
        raise ValueError('exact_capsule_manifest_required')
    Ed25519PublicKey.from_public_bytes(public_key).verify(base64.b64decode(signature,validate=True),
        b'mysoc-standalone-inputs-v1\n'+canonical(manifest))
    if manifest['identity']!=identity or manifest['recipient_sha256']!=hashlib.sha256(recipient_pem).hexdigest():
        raise ValueError('capsule_recipient_mismatch')
    expires=datetime.fromisoformat(manifest['expires_at'].replace('Z','+00:00'))
    if expires.tzinfo is None or not 0 < (expires-datetime.now(timezone.utc)).total_seconds() <= 86400:
        raise ValueError('capsule_expired_or_unbounded')
    if len(ciphertext)>4*1024*1024 or hashlib.sha256(ciphertext).hexdigest()!=manifest['ciphertext_sha256']:
        raise ValueError('capsule_ciphertext_mismatch')
    result=subprocess.run(['openssl','cms','-decrypt','-binary','-inform','DER','-inkey',str(private_key_path)],
                          input=ciphertext,capture_output=True,timeout=20)
    if result.returncode or len(result.stdout)>2*1024*1024:
        raise ValueError('capsule_decryption_failed')
    # Config payload may exceed worker request limit, but each credential is
    # bounded separately and duplicate JSON keys remain forbidden.
    def pairs(items):
        value={}
        for key,item in items:
            if key in value:raise ValueError('duplicate_capsule_field')
            value[key]=item
        return value
    payload=json.loads(result.stdout,object_pairs_hook=pairs)
    if not isinstance(payload,dict) or set(payload)!={'protocol','identity','files','policy'} or payload['protocol']!=CAPSULE_PROTOCOL or payload['identity']!=identity:
        raise ValueError('invalid_capsule_payload')
    if not isinstance(payload['files'],dict) or set(payload['files'])!=ALLOWED_FILES:
        raise ValueError('exact_secret_inventory_required')
    files={}
    for name,encoded in payload['files'].items():
        raw=base64.b64decode(encoded,validate=True)
        if not raw or len(raw)>1024*1024:raise ValueError('credential_size_limit')
        files[name]=raw
    return files,payload['policy']
