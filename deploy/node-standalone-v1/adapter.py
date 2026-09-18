"""Standalone coordinator. Host owns lock, verified staging and live measurement."""
import json
import os
from pathlib import Path
import tempfile
from datetime import datetime, timezone
from transaction import digest, persist_intent
from protocol import strict_json, validate_response


def atomic_json(path, value):
    fd, temporary = tempfile.mkstemp(prefix='.standalone-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as stream:
            stream.write(json.dumps(value, sort_keys=True, separators=(',', ':')).encode())
            stream.flush(); os.fsync(stream.fileno())
        os.replace(temporary, path)
        parent = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try: os.fsync(parent)
        finally: os.close(parent)
    finally:
        if os.path.exists(temporary): os.unlink(temporary)


class Adapter:
    def __init__(self, directory, host, worker):
        self.directory = Path(directory)
        self.host, self.worker = host, worker

    def save(self, binding, phase, mutation, error=None):
        receipt = dict(protocol=binding['protocol'], operation_id=binding['operation_id'],
                       operation_sha256=digest(binding), phase=phase, mutation=mutation,
                       observed_at=datetime.now(timezone.utc).isoformat())
        if error: receipt['error_code'] = error
        atomic_json(self.directory/'adapter-journal.json', receipt)
        return receipt

    def run(self, action, binding):
        if action not in ('readiness', 'apply', 'status', 'recover'):
            raise ValueError('unsupported_action')
        if self.directory.name != binding['operation_id']:
            raise ValueError('operation_directory_mismatch')
        self.host.protected(self.directory)
        # Re-measure protected source history, signatures/configuration and data
        # identity on every attempt. No receipt alone can reauthorize a transition.
        intent = self.host.admit(binding)
        persist_intent(self.directory, intent)
        path = self.directory/'adapter-journal.json'
        retained = None
        if path.exists():
            self.host.protected(path)
            retained = strict_json(path.read_bytes())
            if retained['operation_sha256'] != digest(binding):
                raise ValueError('retained_binding_mismatch')
        elif action in ('status', 'recover'):
            raise ValueError('unknown_operation')
        bundles = self.host.stage(binding)
        if retained and retained['phase'] == 'accepted':
            evidence = self.host.verify_accepted(binding)
            return dict(retained, health=evidence, observed_at=datetime.now(timezone.utc).isoformat())
        # Re-extract signatures on every call; no trusting stale extracted code.
        if action == 'readiness':
            response = validate_response(self.worker(action, binding, bundles), binding)
            if response['phase'] not in ('prepared', 'blocked') or response['mutation'] != 'none':
                raise ValueError('readiness_must_not_mutate')
            return response
        if not retained:
            self.save(binding, 'prepared', 'none')
        # Once invocation could have begun, a retry is recovery, never a fresh
        # apply. Persist uncertainty before crossing the process boundary.
        selected = action
        if action == 'apply' and retained and retained['phase'] != 'prepared':
            selected = 'recover'
        self.save(binding, 'recovery_required', 'possible')
        try:
            response = validate_response(self.worker(selected, binding, bundles), binding)
            if response['phase'] == 'accepted':
                evidence = self.host.verify_accepted(binding)
                # Separate root acceptance; no updater installation reclassification.
                self.host.persist_effective_mode(binding, evidence)
                return dict(self.save(binding, 'accepted', 'confirmed'), health=evidence)
            if response['phase'] == 'restored':
                self.host.verify_restored(binding)
            return self.save(binding, response['phase'], response['mutation'], response.get('error_code'))
        except Exception:
            self.save(binding, 'recovery_required', 'possible', 'worker_outcome_unverified')
            raise
