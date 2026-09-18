"""Offline standalone admission and durable intent. No host executor is enabled.

Evidence and approved_binding must come from protected root measurements/policy,
not request JSON. A future host adapter must enumerate *all* prior journals under
the lifecycle lock. The evidence digest pins that complete reviewed inventory.
"""
import hashlib
import json
import re
import uuid

PROTOCOL = 'pod-node-standalone-v1'


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()


def digest(value):
    return hashlib.sha256(canonical(value)).hexdigest()


def _sha(value):
    if not isinstance(value, str) or not re.fullmatch('[0-9a-f]{64}', value):
        raise ValueError('invalid_digest')


def validate_binding(binding):
    fields = {'protocol', 'operation_id', 'source', 'source_version', 'source_artifact_sha256',
              'bootstrap_receipt_sha256', 'source_evidence_sha256', 'target',
              'configuration_sha256', 'instance_id', 'source_mode', 'target_mode'}
    if not isinstance(binding, dict) or set(binding) not in (fields, fields | {'previous_operation'}):
        raise ValueError('exact_standalone_binding_required')
    if binding['protocol'] != PROTOCOL or binding['source_mode'] != 'independent-management' or binding['target_mode'] != 'independent-standalone':
        raise ValueError('unsupported_mode_transition')
    if str(uuid.UUID(binding['operation_id'])) != binding['operation_id']:
        raise ValueError('canonical_operation_id_required')
    if 'previous_operation' in binding:
        previous=binding['previous_operation']
        if not isinstance(previous,dict) or set(previous)!={'operation_id','operation_sha256'} or str(uuid.UUID(previous['operation_id']))!=previous['operation_id'] or previous['operation_id']==binding['operation_id']:
            raise ValueError('exact_previous_operation_required')
        _sha(previous['operation_sha256'])
    identity = binding['source']
    if not isinstance(identity, dict) or set(identity) != {'machine_id', 'installation_id', 'updater_instance_id', 'node_id'}:
        raise ValueError('exact_identity_required')
    if not isinstance(identity['machine_id'], str) or not re.fullmatch('[0-9a-f]{32}', identity['machine_id']) or identity['node_id'] not in ('1', '2'):
        raise ValueError('invalid_local_identity')
    for value in (identity['installation_id'], identity['updater_instance_id'], binding['instance_id']):
        if not isinstance(value, str) or not re.fullmatch('[A-Za-z0-9][A-Za-z0-9_-]{0,100}', value):
            raise ValueError('invalid_installation_identity')
    for key in ('source_artifact_sha256', 'bootstrap_receipt_sha256', 'source_evidence_sha256', 'configuration_sha256'):
        _sha(binding[key])
    target = binding['target']
    if not isinstance(target, dict) or set(target) != {'product', 'version', 'architecture', 'source_commit', 'artifact_sha256', 'binary_sha256'}:
        raise ValueError('exact_target_required')
    if target['product'] != 'siemcore' or target['architecture'] != 'linux/amd64':
        raise ValueError('invalid_target')
    for version in (binding['source_version'], target['version']):
        if not isinstance(version, str) or not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+', version):
            raise ValueError('invalid_version')
    if tuple(map(int, target['version'].split('.'))) < tuple(map(int, binding['source_version'].split('.'))):
        raise ValueError('downgrade_refused')
    if not isinstance(target['source_commit'], str) or not re.fullmatch('[0-9a-f]{40}', target['source_commit']):
        raise ValueError('invalid_commit')
    _sha(target['artifact_sha256']); _sha(target['binary_sha256'])


def admit(binding, approved_binding, evidence, bootstrap_bytes, measured_configuration_sha256):
    validate_binding(binding)
    if binding != approved_binding:
        raise ValueError('protected_binding_mismatch')
    if digest(evidence) != binding['source_evidence_sha256']:
        raise ValueError('reviewed_source_inventory_mismatch')
    if set(evidence) != {'application', 'journals', 'current_version', 'current_artifact_sha256', 'linked_evidence', 'inventory_complete'} or evidence['inventory_complete'] is not True:
        raise ValueError('complete_root_inventory_required')
    app = evidence['application']
    if app.get('schema') != 5 or app.get('topology') != 'node-unlinked' or app.get('pod_id'):
        raise ValueError('original_independent_installation_required')
    if any(app.get(k) != v for k, v in binding['source'].items()):
        raise ValueError('original_identity_mismatch')
    if evidence['linked_evidence'] != []:
        raise ValueError('linked_history_refused')
    if hashlib.sha256(bootstrap_bytes).hexdigest() != binding['bootstrap_receipt_sha256']:
        raise ValueError('original_bootstrap_changed')
    receipt = json.loads(bootstrap_bytes)
    if receipt.get('status') != 'complete' or receipt.get('installation_state') != 'installed-unlinked':
        raise ValueError('completed_independent_bootstrap_required')
    policy_sha = hashlib.sha256(json.dumps(app, sort_keys=True).encode()).hexdigest()
    if receipt.get('policy_sha256') != policy_sha:
        raise ValueError('bootstrap_policy_mismatch')
    # The inventory records every historical transition, not just a missing link
    # file. Unknown/unfinished operations block. History loader remains required.
    if not isinstance(evidence['journals'], list):
        raise ValueError('invalid_journal_inventory')
    for record in evidence['journals']:
        if set(record) != {'protocol', 'phase', 'identity', 'receipt_sha256'}:
            raise ValueError('invalid_journal_record')
        _sha(record['receipt_sha256'])
        allowed = {'pod-node-update-v1': {'accepted', 'restored'}, 'node-bootstrap-repair': {'journal_committed'}}
        if record['protocol'] not in allowed or record['phase'] not in allowed[record['protocol']] or record['identity'] != binding['source']:
            raise ValueError('prior_operation_unreconciled')
    if evidence['current_version'] != binding['source_version'] or evidence['current_artifact_sha256'] != binding['source_artifact_sha256']:
        raise ValueError('current_source_mismatch')
    if measured_configuration_sha256 != binding['configuration_sha256']:
        raise ValueError('configuration_mismatch')
    # This is an intent, NOT a launch receipt or health acceptance.
    return {'protocol': PROTOCOL, 'operation_id': binding['operation_id'],
            'operation_sha256': digest(binding), 'binding': binding,
            'phase': 'admitted-awaiting-product', 'source_mode': binding['source_mode'],
            'target_mode': binding['target_mode'], 'effective_mode': binding['source_mode'],
            'routine_updates_allowed': False, 'mutation': 'none'}


def persist_intent(directory, intent):
    """Private offline journal with exclusive lock; no accepted mode writer.

    Caller supplies an already-admitted intent under its lifecycle lock. Root
    host integration must provide a protected fixed directory and ancestor chain.
    """
    import fcntl
    import os
    from pathlib import Path
    import stat
    import tempfile
    directory = Path(directory)
    st = directory.lstat()
    if not stat.S_ISDIR(st.st_mode) or st.st_uid != os.geteuid() or st.st_mode & 0o077:
        raise ValueError('private_owned_directory_required')
    validate_binding(intent['binding'])
    if (intent['phase'] != 'admitted-awaiting-product' or intent['effective_mode'] != 'independent-management'
            or intent['routine_updates_allowed'] is not False or intent['operation_sha256'] != digest(intent['binding'])):
        raise ValueError('admitted_intent_only')
    fd = os.open(directory / 'standalone.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        lock = os.fstat(fd)
        if not stat.S_ISREG(lock.st_mode) or lock.st_uid != os.geteuid() or lock.st_mode & 0o077:
            raise ValueError('private_lock_required')
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        path = directory / 'standalone-intent.json'
        if path.exists() or path.is_symlink():
            st = path.lstat()
            if not stat.S_ISREG(st.st_mode) or st.st_uid != os.geteuid() or st.st_mode & 0o077:
                raise ValueError('private_intent_required')
            if path.read_bytes() != canonical(intent):
                raise ValueError('retained_operation_conflict')
            return
        out, temporary = tempfile.mkstemp(prefix='.standalone-', dir=directory)
        try:
            with os.fdopen(out, 'wb') as stream:
                stream.write(canonical(intent)); stream.flush(); os.fsync(stream.fileno())
            os.replace(temporary, path)
            parent = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
            try: os.fsync(parent)
            finally: os.close(parent)
        finally:
            if os.path.exists(temporary): os.unlink(temporary)
    finally:
        os.close(fd)
