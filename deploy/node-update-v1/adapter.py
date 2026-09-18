"""Root transaction coordinator. Not installed/enabled by existing kits.

Host integration supplies verified artifact staging, measured health and a
supervised worker. It owns the shared root lock for the entire call. No generic
rollback or bootstrap-policy edits are performed here.
"""
import json
import os
from pathlib import Path
import tempfile
from datetime import datetime, timezone
from protocol import PROTOCOL, digest, strict_json, validate_binding, validate_response, validate_health


def atomic_json(path, value):
    fd, temporary = tempfile.mkstemp(prefix='.journal-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as stream:
            os.fchmod(stream.fileno(), 0o600)
            json.dump(value, stream, sort_keys=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY)
        try: os.fsync(directory)
        finally: os.close(directory)
    finally:
        if os.path.exists(temporary): os.unlink(temporary)


class Adapter:
    def __init__(self, directory, verify_binding, stage, worker, probe, stopped, protected, preservation):
        self.directory = Path(directory)
        self.verify_binding = verify_binding
        self.stage = stage
        self.worker = worker
        self.probe = probe
        self.stopped = stopped
        self.protected = protected
        self.preservation = preservation

    def read(self, path):
        self.protected(path)
        return strict_json(path.read_bytes())

    def save(self, journal, phase, error=None):
        result = dict(journal, phase=phase, observed_at=datetime.now(timezone.utc).isoformat())
        if error: result['error_code'] = error
        else: result.pop('error_code', None)
        atomic_json(self.directory / 'adapter-journal.json', result)
        return result

    def load(self, binding, create):
        validate_binding(binding)
        if self.directory.name != binding['operation_id']:
            raise ValueError('operation directory mismatch')
        self.protected(self.directory)
        binding_path = self.directory / 'binding.json'
        journal_path = self.directory / 'adapter-journal.json'
        preserved_path = self.directory / 'preserved-files.json'
        if binding_path.exists():
            if self.read(binding_path) != binding:
                raise ValueError('retained operation binding changed')
            self.verify_binding(binding, False)
            if not self.preservation(self.read(preserved_path)):
                raise ValueError('protected installation inputs changed')
        else:
            if not create:
                raise ValueError('operation does not exist')
            self.verify_binding(binding, True)
            validate_health(self.probe(binding, 'predecessor'), binding, binding['predecessor'], require_closed=False)
            if preserved_path.exists():
                if not self.preservation(self.read(preserved_path)):
                    raise ValueError('interrupted preparation inputs changed')
            else:
                atomic_json(preserved_path, self.preservation(None))
            atomic_json(binding_path, binding)
        if journal_path.exists():
            journal = self.read(journal_path)
            if journal.get('operation_sha256') != digest(binding) or journal.get('operation_id') != binding['operation_id']:
                raise ValueError('retained journal binding mismatch')
            return journal
        # Crash after immutable binding fsync but before journal creation is
        # recoverable: no worker can have been invoked before prepared fsync.
        return self.save(dict(protocol=PROTOCOL, operation_id=binding['operation_id'],
                              operation_sha256=digest(binding)), 'prepared')

    def run(self, action, binding):
        if action not in ('apply', 'status', 'recover'):
            raise ValueError('unknown operation')
        journal = self.load(binding, create=action == 'apply')
        if journal['phase'] in ('accepted', 'restored'):
            which = 'target' if journal['phase'] == 'accepted' else 'predecessor'
            try:
                if which == 'predecessor':
                    retained = self.stage(binding)
                    if retained.get('predecessor_ui_capable') is not True:
                        raise ValueError('unsafe restored receipt')
                validate_health(self.probe(binding, which), binding, binding[which])
            except Exception:
                return self.save(journal, 'blocked', 'terminal_health_mismatch')
            return journal  # Exact accepted replay never writes or invokes worker.
        if journal['phase'] == 'blocked' and action == 'apply':
            return journal
        if action == 'apply' and journal['phase'] not in ('prepared', 'staged'):
            action = 'status'  # Interrupted mutation must reconcile first.
        try:
            bundles = self.stage(binding)  # Verify both retained signed archives.
        except Exception:
            return self.save(journal, 'blocked', 'artifact_verification_failed')
        if action == 'apply':
            journal = self.save(journal, 'staged')
            journal = self.save(journal, 'switching')  # Before possible mutation.
        elif action == 'recover':
            journal = self.save(journal, 'recovery_required')
        try:
            response = validate_response(self.worker(action, binding, bundles, self.directory), binding)
        except Exception:
            return self.save(journal, 'recovery_required', 'worker_outcome_uncertain')
        if not self.preservation(self.read(self.directory / 'preserved-files.json')):
            return self.save(journal, 'blocked', 'protected_inputs_changed')
        phase = response['phase']
        if phase == 'accepted':
            try:
                validate_health(self.probe(binding, 'target'), binding, binding['target'])
            except Exception:
                return self.save(journal, 'recovery_required', 'target_health_failed')
            return self.save(journal, 'accepted')
        if phase == 'restored':
            # Capability comes only from the root-verified predecessor archive,
            # never from a version string or the worker's restoration claim.
            if bundles.get('predecessor_ui_capable') is not True:
                return self.save(journal, 'blocked', 'unsafe_predecessor_restore')
            try:
                validate_health(self.probe(binding, 'predecessor'), binding, binding['predecessor'])
            except Exception:
                return self.save(journal, 'blocked', 'predecessor_health_failed')
            return self.save(journal, 'restored', 'target_update_failed')
        if phase == 'blocked':
            error = response.get('error_code', 'worker_blocked')
            if error == 'predecessor_ui_unprotected' and not self.stopped():
                error = 'stop_unconfirmed'
            return self.save(journal, 'blocked', error)
        if phase == 'prepared':
            try:
                validate_health(self.probe(binding, 'predecessor'), binding, binding['predecessor'], require_closed=False)
            except Exception:
                return self.save(journal, 'recovery_required', 'prepared_predecessor_health_failed')
        if phase not in ('prepared', 'staged', 'switching', 'verifying', 'recovery_required', 'restoring'):
            return self.save(journal, 'blocked', 'invalid_worker_transition')
        return self.save(journal, phase, response.get('error_code'))
