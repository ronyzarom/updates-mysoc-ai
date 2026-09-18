"""Offline signed-byte verification using the existing release trust domain.

Keys and detached signatures must be loaded from protected policy by the future
root integration. This module never trusts a key/signature in a caller request.
Archive manifest/architecture/binary validation remains a separate staging gate.
"""
import base64
import hashlib

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey


def verify_archives(binding, source_bytes, target_bytes, public_key, signatures):
    if set(signatures) != {'source', 'target'}:
        raise ValueError('exact_release_signatures_required')
    key = Ed25519PublicKey.from_public_bytes(public_key)
    for role, version, checksum, content in (
        ('source', binding['source_version'], binding['source_artifact_sha256'], source_bytes),
        ('target', binding['target']['version'], binding['target']['artifact_sha256'], target_bytes),
    ):
        if not isinstance(content, bytes) or hashlib.sha256(content).hexdigest() != checksum:
            raise ValueError('artifact_checksum_mismatch')
        signature = base64.b64decode(signatures[role], validate=True)
        message = ('mysoc-release-v1\nsiemcore\n' + version + '\n' + checksum).encode('ascii')
        key.verify(signature, message)
