"""Strict common transaction vocabulary; no lifecycle or host side effects."""
import base64
import hashlib
import json
import re
import uuid

PROTOCOL = 'pod-node-update-v1'
PHASES = {'prepared', 'staged', 'switching', 'verifying', 'accepted',
          'recovery_required', 'restoring', 'restored', 'blocked'}
BINDING_FIELDS = {'protocol', 'operation_id', 'machine_id', 'installation_id',
                  'updater_instance_id', 'server_type', 'node_id', 'bootstrap_policy_sha256',
                  'predecessor', 'target', 'signing_public_key_sha256', 'ui_protection_required'}
ARTIFACT_FIELDS = {'product', 'version', 'architecture', 'artifact_sha256',
                   'artifact_signature', 'binary_sha256'}


def strict_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError('duplicate JSON field')
            result[key] = value
        return result
    if len(raw) > 65536:
        raise ValueError('request exceeds limit')
    return json.loads(raw, object_pairs_hook=pairs,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError('nonfinite JSON')))


def canonical(value):
    # Binding has only ASCII strings, objects and true; this is RFC8785 for
    # that restricted vocabulary (no floats, Unicode normalization or arrays).
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def digest(value):
    return hashlib.sha256(canonical(value)).hexdigest()


def hex_digest(value):
    return isinstance(value, str) and re.fullmatch(r'[0-9a-f]{64}', value) is not None


def validate_binding(binding):
    if not isinstance(binding, dict) or set(binding) != BINDING_FIELDS:
        raise ValueError('exact operation binding required')
    if binding['protocol'] != PROTOCOL or binding['server_type'] != 'pod-node' or binding['node_id'] not in ('1','2') or binding['ui_protection_required'] is not True:
        raise ValueError('unlinked protected-UI protocol required')
    operation = binding['operation_id']
    if not isinstance(operation, str) or str(uuid.UUID(operation)) != operation:
        raise ValueError('canonical operation UUID required')
    if not isinstance(binding['machine_id'], str) or not re.fullmatch(r'[0-9a-f]{32}', binding['machine_id']):
        raise ValueError('machine identity required')
    for key in ('installation_id', 'updater_instance_id'):
        if not isinstance(binding[key], str) or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}', binding[key]):
            raise ValueError('installation identity required')
    for key in ('bootstrap_policy_sha256', 'signing_public_key_sha256'):
        if not hex_digest(binding[key]):
            raise ValueError('invalid binding digest')
    for key in ('predecessor', 'target'):
        item = binding[key]
        if not isinstance(item, dict) or set(item) != ARTIFACT_FIELDS:
            raise ValueError('exact signed artifact identity required')
        if item['product'] != 'siemcore' or item['architecture'] != 'linux/amd64':
            raise ValueError('unsupported product architecture')
        if not isinstance(item['version'], str) or not re.fullmatch(r'\d+\.\d+\.\d+\.\d+', item['version']):
            raise ValueError('invalid version')
        if not all(hex_digest(item[k]) for k in ('artifact_sha256', 'binary_sha256')):
            raise ValueError('invalid artifact digest')
        if not isinstance(item['artifact_signature'], str) or len(base64.b64decode(item['artifact_signature'], validate=True)) != 64:
            raise ValueError('invalid release signature')
    if tuple(map(int, binding['target']['version'].split('.'))) <= tuple(map(int, binding['predecessor']['version'].split('.'))):
        raise ValueError('target must be newer; recovery is a distinct action')
    return binding


def validate_response(response, binding):
    required = {'protocol', 'operation_id', 'operation_sha256', 'phase', 'observed_at'}
    optional = {'health', 'error_code', 'mutation', 'target_version', 'artifact_sha256'}
    if not isinstance(response, dict) or not required <= set(response) or set(response) - required - optional:
        raise ValueError('invalid worker response fields')
    if response['protocol'] != PROTOCOL or response['operation_id'] != binding['operation_id'] or response['operation_sha256'] != digest(binding):
        raise ValueError('worker binding mismatch')
    if ('target_version' in response and response['target_version'] != binding['target']['version']) or ('artifact_sha256' in response and response['artifact_sha256'] != binding['target']['artifact_sha256']):
        raise ValueError('worker target mismatch')
    if response['phase'] not in PHASES:
        raise ValueError('invalid worker phase')
    if response.get('mutation') not in (None, 'none', 'possible', 'confirmed'):
        raise ValueError('invalid worker mutation classification')
    from datetime import datetime, timezone
    timestamp = datetime.fromisoformat(response['observed_at'].replace('Z', '+00:00'))
    if timestamp.tzinfo is None or not -5 <= (datetime.now(timezone.utc)-timestamp).total_seconds() <= 30:
        raise ValueError('worker measurement stale')
    return response


def validate_health(health, binding, artifact, require_closed=True):
    expected = dict(version=artifact['version'], binary_sha256=artifact['binary_sha256'],
                    installation_id=binding['installation_id'], updater_id=binding['updater_instance_id'],
                    installation_state='installed-unlinked', node_id=binding['node_id'], management_ready=True, data_ready=True, installation_complete=True, link_ready=False,
                    pod_ready=False, authority_enabled=False, processing_enabled=False)
    if not isinstance(health, dict) or any(type(health.get(k)) is not type(v) or health[k] != v for k,v in expected.items()):
        raise ValueError('exact independent Node health required')
    if require_closed and health.get('ui_closed') is not True:
        raise ValueError('unauthenticated UI must be closed')
    return health
