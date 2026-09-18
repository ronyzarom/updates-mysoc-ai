"""Offline durable admission journal, with no product executor or activation.

The directory must be private and owned by the executing identity (root in a
future root adapter). Admission verifies current inputs while holding one shared
lock, then records one immutable binding. No accepted/completed phase exists yet.
"""
import fcntl
import json
import os
from pathlib import Path
import stat
import tempfile

from product_contract import binding_digest, parse_request, _object


def _private(path):
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise ValueError('private_owned_path_required')
    return info


def _atomic(path, value):
    fd, temporary = tempfile.mkstemp(dir=path.parent, prefix='.admission-')
    try:
        with os.fdopen(fd, 'w') as stream:
            json.dump(value, stream, sort_keys=True, separators=(',', ':'), ensure_ascii=True)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def record_admission(directory, request, verify_current_inputs):
    """Record offline admission only after caller's verification succeeds.

    verify_current_inputs is an integration seam, not a trust decision supplied
    by a remote caller. The unavailable CLI deliberately never calls this.
    """
    request = parse_request(json.dumps(request))
    directory = Path(directory)
    if not stat.S_ISDIR(_private(directory).st_mode):
        raise ValueError('private_directory_required')
    fd = os.open(directory / 'admission.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
            raise ValueError('private_lock_required')
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        binding = request['binding']
        checksum = binding_digest(binding)
        if request['operation_sha256'] != checksum:
            raise ValueError('binding_digest_mismatch')
        path = directory / 'admission.json'
        retained = None
        if path.exists() or path.is_symlink():
            if not stat.S_ISREG(_private(path).st_mode):
                raise ValueError('private_journal_required')
            retained = json.loads(path.read_bytes(), object_pairs_hook=_object)
            if retained != dict(protocol='pod-node-link-v1', binding=binding,
                                operation_sha256=checksum, phase='admitted-awaiting-product', mutation='none'):
                raise ValueError('retained_operation_conflict')
        # Reverify even an exact replay; never infer current authority from a
        # retained receipt or treat admission as completed product adoption.
        if verify_current_inputs() is not True:
            raise ValueError('current_inputs_not_verified')
        if retained is not None:
            return retained
        receipt = dict(protocol='pod-node-link-v1', binding=binding,
                       operation_sha256=checksum, phase='admitted-awaiting-product', mutation='none')
        _atomic(path, receipt)
        return receipt
    finally:
        os.close(fd)
