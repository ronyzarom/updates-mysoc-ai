"""Offline pod-adoption-v1 admission. No installed host executor or authority."""
import base64
import hashlib
import json
import re
import uuid
from urllib.parse import urlsplit

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

PROTOCOL = 'pod-adoption-v1'
DOMAIN = b'mysoc-pod-adoption-v1\n'
MAX_LIFETIME = 900


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True,
                      allow_nan=False).encode('ascii')


def digest(value):
    return hashlib.sha256(canonical(value)).hexdigest()


def unique(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError('duplicate_field')
        out[key] = value
    return out


def exact(value, fields):
    if not isinstance(value, dict) or set(value) != set(fields.split()):
        raise ValueError('invalid_fields')


def text(value, pattern):
    if not isinstance(value, str) or not re.fullmatch(pattern, value):
        raise ValueError('invalid_value')


def sha(value):
    text(value, r'[0-9a-f]{64}')


def identity(value):
    text(value, r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}')


def integer(value):
    if type(value) is not int or not 0 < value < 2**53:
        raise ValueError('invalid_integer')


def endpoint(value):
    if not isinstance(value, str) or not value.isascii() or any(c.isspace() for c in value):
        raise ValueError('invalid_endpoint')
    p = urlsplit(value)
    if (p.scheme != 'https' or not p.hostname or p.username is not None or
            p.password is not None or p.query or p.fragment or p.path not in ('', '/')
            or p.port not in (None, 443)):
        raise ValueError('invalid_endpoint')
    text(p.hostname, r'[A-Za-z0-9][A-Za-z0-9.-]{0,252}')


def parse(raw):
    if not isinstance(raw, bytes) or len(raw) > 32768:
        raise ValueError('invalid_request_size')
    q = json.loads(raw, object_pairs_hook=unique,
                   parse_constant=lambda _: (_ for _ in ()).throw(ValueError('invalid_number')))
    exact(q, 'protocol binding authorization_signature')
    if q['protocol'] != PROTOCOL:
        raise ValueError('unsupported_protocol')
    b = q['binding']
    exact(b, 'protocol operation_id source target membership observer desired_settings approved_node_key_sha256 adoption_plan_sha256 issued_at expires_at expected_state')
    if b['protocol'] != PROTOCOL or b['expected_state'] != 'linked-paused':
        raise ValueError('activation_not_permitted')
    if not isinstance(b['operation_id'], str) or str(uuid.UUID(b['operation_id'])) != b['operation_id']:
        raise ValueError('invalid_operation_id')
    source = b['source']
    exact(source, 'installation_class machine_id installation_id updater_instance_id version artifact_sha256 bootstrap_receipt_sha256')
    if source['installation_class'] not in ('normal', 'observer-unlinked'):
        raise ValueError('unsupported_source_class')
    text(source['machine_id'], r'[0-9a-f]{32}')
    for field in ('installation_id', 'updater_instance_id'):
        identity(source[field])
    text(source['version'], r'[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')
    for field in ('artifact_sha256', 'bootstrap_receipt_sha256'):
        sha(source[field])
    target = b['target']
    exact(target, 'product version architecture source_commit artifact_sha256 binary_sha256')
    if target['product'] != 'siemcore' or target['architecture'] != 'linux/amd64':
        raise ValueError('unsupported_target')
    text(target['version'], r'[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')
    if tuple(map(int, target['version'].split('.'))) < tuple(map(int, source['version'].split('.'))):
        raise ValueError('downgrade_refused')
    text(target['source_commit'], r'[0-9a-f]{40}')
    sha(target['artifact_sha256']); sha(target['binary_sha256'])
    exact(b['membership'], 'pod_id node_id')
    identity(b['membership']['pod_id'])
    expected_nodes = ('1', '2') if source['installation_class'] == 'normal' else ('witness',)
    if b['membership']['node_id'] not in expected_nodes:
        raise ValueError('source_role_mismatch')
    o = b['observer']
    exact(o, 'installation_id endpoint public_key_sha256 registry_revision registry_sha256')
    identity(o['installation_id']); endpoint(o['endpoint']); sha(o['public_key_sha256'])
    integer(o['registry_revision']); sha(o['registry_sha256'])
    if (source['installation_class'] == 'observer-unlinked') != (o['installation_id'] == source['installation_id']):
        raise ValueError('observer_identity_mismatch')
    exact(b['desired_settings'], 'revision sha256')
    integer(b['desired_settings']['revision']); sha(b['desired_settings']['sha256'])
    sha(b['approved_node_key_sha256']); sha(b['adoption_plan_sha256'])
    integer(b['issued_at']); integer(b['expires_at'])
    if not 0 < b['expires_at'] - b['issued_at'] <= MAX_LIFETIME:
        raise ValueError('invalid_authorization_lifetime')
    if not isinstance(q['authorization_signature'], str):
        raise ValueError('invalid_signature')
    if len(base64.b64decode(q['authorization_signature'], validate=True)) != 64:
        raise ValueError('invalid_signature')
    return q


def verify(raw, trusted_key, measured, now):
    """Pure verification against INDEPENDENT trusted host/product measurements.

    measured is a root-integration seam, never deserialized from the request.
    No production loader exists yet. The CLI deliberately cannot call this.
    """
    q = parse(raw)
    b = q['binding']
    Ed25519PublicKey.from_public_bytes(trusted_key).verify(
        base64.b64decode(q['authorization_signature'], validate=True), DOMAIN + canonical(b))
    if type(now) is not int or not b['issued_at'] <= now < b['expires_at']:
        raise ValueError('authorization_expired_or_not_yet_valid')
    exact(measured, 'source target membership observer desired_settings approved_node_key_sha256 adoption_plan_sha256')
    for key in measured:
        if measured[key] != b[key]:
            raise ValueError('measured_binding_mismatch:' + key)
    return q


def verify_release(blob, product, version, checksum, signature, trusted_key):
    """Verify retained source and target archives with existing release trust."""
    if product != 'siemcore' or hashlib.sha256(blob).hexdigest() != checksum:
        raise ValueError('artifact_checksum_mismatch')
    Ed25519PublicKey.from_public_bytes(trusted_key).verify(
        base64.b64decode(signature, validate=True),
        ('mysoc-release-v1\n'+product+'\n'+version+'\n'+checksum).encode('ascii'))
