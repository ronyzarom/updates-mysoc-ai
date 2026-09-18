"""Admission for the separate, not-yet-qualified independent-node link workflow.

Validation grants no authority. Updates must authenticate the source installation,
verify signed archives, hold the operation lock and validate protected inputs.
No update or greenfield entrypoint imports this module until integration qualifies.
"""
import hashlib
import json
import re
import uuid
from pathlib import PurePosixPath
from urllib.parse import urlsplit

PROTOCOL = 'pod-node-link-v1'
ROOT = '/var/lib/siemcore-node-link/operations/'

def _exact(value, fields):
    if not isinstance(value, dict) or set(value) != set(fields.split()):
        raise ValueError('invalid_fields')

def _text(value, pattern):
    if not isinstance(value, str) or not re.fullmatch(pattern, value):
        raise ValueError('invalid_value')

def _digest(value):
    _text(value, '[0-9a-f]{64}')

def _identity(value):
    _exact(value, 'machine_id installation_id updater_instance_id node_id')
    _text(value['machine_id'], '[0-9a-f]{32}')
    for key in ('installation_id', 'updater_instance_id'):
        _text(value[key], '[A-Za-z0-9][A-Za-z0-9_-]{0,100}')
    if value['node_id'] not in ('1', '2'):
        raise ValueError('invalid_node_id')

def _url(value):
    if not isinstance(value, str) or not value.isascii() or any(c.isspace() for c in value):
        raise ValueError('invalid_endpoint')
    parsed = urlsplit(value)
    if (parsed.scheme != 'https' or not parsed.hostname or parsed.username is not None
            or parsed.password is not None or parsed.query or parsed.fragment
            or parsed.path not in ('', '/') or parsed.port not in (None, 443)):
        raise ValueError('invalid_endpoint')
    _text(parsed.hostname, '[A-Za-z0-9][A-Za-z0-9.-]{0,252}')
    return (parsed.hostname.lower().rstrip('.'), parsed.port or 443)

def _object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('duplicate_field')
        result[key] = value
    return result

def binding_digest(binding):
    return hashlib.sha256(json.dumps(binding, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()).hexdigest()

def parse_request(raw):
    if not isinstance(raw, (bytes, str)) or len(raw) > 32768:
        raise ValueError('invalid_request_size')
    q = json.loads(raw, object_pairs_hook=_object)
    _exact(q, 'protocol action binding operation_sha256 operation_directory target_bundle adoption_plan')
    if q['protocol'] != PROTOCOL or q['action'] not in ('readiness', 'apply', 'status', 'recover'):
        raise ValueError('unsupported_operation')
    b = q['binding']
    _exact(b, 'protocol operation_id source source_version source_artifact_sha256 bootstrap_receipt_sha256 target pod observer peer adoption_plan_sha256 expected_state')
    if b['protocol'] != PROTOCOL or b['expected_state'] != 'linked-paused':
        raise ValueError('link_cannot_activate')
    if not isinstance(b['operation_id'], str) or str(uuid.UUID(b['operation_id'])) != b['operation_id']:
        raise ValueError('invalid_operation_id')
    _identity(b['source']); _identity(b['peer'])
    for key in b['source']:
        if b['source'][key] == b['peer'][key]:
            raise ValueError('distinct_peer_identity_required')
    _text(b['source_version'], r'[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')
    for key in ('source_artifact_sha256', 'bootstrap_receipt_sha256', 'adoption_plan_sha256'):
        _digest(b[key])
    target = b['target']
    _exact(target, 'product version architecture source_commit artifact_sha256 binary_sha256')
    if target['product'] != 'siemcore' or target['architecture'] != 'linux/amd64':
        raise ValueError('invalid_target')
    _text(target['version'], r'[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')
    if tuple(map(int, target['version'].split('.'))) < tuple(map(int, b['source_version'].split('.'))):
        raise ValueError('downgrade_refused')
    _text(target['source_commit'], '[0-9a-f]{40}')
    for key in ('artifact_sha256', 'binary_sha256'):
        _digest(target[key])
    _exact(b['pod'], 'pod_id instance_id customer_url')
    for key in ('pod_id', 'instance_id'):
        _text(b['pod'][key], '[A-Za-z0-9][A-Za-z0-9_-]{0,100}')
    _url(b['pod']['customer_url'])
    o = b['observer']
    _exact(o, 'installation_id endpoint registry_sha256 generation')
    _text(o['installation_id'], '[A-Za-z0-9][A-Za-z0-9_-]{0,100}')
    _url(o['endpoint']); _digest(o['registry_sha256'])
    if o['installation_id'] in (b['source']['installation_id'], b['peer']['installation_id']):
        raise ValueError('distinct_observer_identity_required')
    if type(o['generation']) is not int or not 0 < o['generation'] < 2**63:
        raise ValueError('invalid_generation')
    if _url(o['endpoint']) == _url(b['pod']['customer_url']):
        raise ValueError('observer_must_use_fixed_endpoint')
    _digest(q['operation_sha256'])
    if binding_digest(b) != q['operation_sha256']:
        raise ValueError('binding_digest_mismatch')
    directory = ROOT + b['operation_id']
    if q['operation_directory'] != directory or q['adoption_plan'] != directory + '/adoption-plan.json':
        raise ValueError('invalid_operation_path')
    p = q['target_bundle']
    if not isinstance(p, str) or not p.startswith('/') or str(PurePosixPath(p)) != p or '..' in PurePosixPath(p).parts:
        raise ValueError('invalid_bundle_path')
    return q
