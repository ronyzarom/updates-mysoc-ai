"""Offline durable admission only; never records successful host adoption."""
import fcntl
import json
import os
import stat
import uuid

from contract import canonical, digest, unique, verify


def private(fd, directory=False):
    s = os.fstat(fd)
    expected = stat.S_ISDIR if directory else stat.S_ISREG
    if not expected(s.st_mode) or s.st_uid != os.geteuid() or s.st_mode & 0o077:
        raise ValueError('private_owned_path_required')
    if not directory and s.st_nlink != 1:
        raise ValueError('hardlink_refused')


def read(directory_fd):
    try:
        fd = os.open('admission.json', os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK,
                     dir_fd=directory_fd)
    except FileNotFoundError:
        return None
    try:
        private(fd)
        with os.fdopen(os.dup(fd), 'rb') as stream:
            raw = stream.read(65537)
        if len(raw) > 65536:
            raise ValueError('journal_too_large')
        return json.loads(raw, object_pairs_hook=unique)
    finally:
        os.close(fd)


def record(directory_fd, raw, trusted_key, measure, clock):
    """Trusted in-process measurement/clock callbacks, never request callbacks.

    Caller must open a protected journal directory through a qualified root
    loader. The descriptor anchors accesses despite path replacement. This
    offline function serializes ALL operations for one installation; there is no
    completion/archive operation, so conflicting operations remain blocked.
    """
    private(directory_fd, directory=True)
    lock = os.open('admission.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK,
                   0o600, dir_fd=directory_fd)
    try:
        private(lock)
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        q = verify(raw, trusted_key, measure(), clock())
        receipt = dict(protocol='pod-adoption-v1', operation_id=q['binding']['operation_id'],
                       binding=q['binding'], operation_sha256=digest(q['binding']),
                       authorization_signature=q['authorization_signature'],
                       phase='admitted-awaiting-product', host_mutation='none',
                       processing_authorized=False)
        retained = read(directory_fd)
        if retained is not None:
            if retained != receipt:
                raise ValueError('retained_operation_conflict')
            return retained
        temporary = '.admission-' + uuid.uuid4().hex
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
                     0o600, dir_fd=directory_fd)
        try:
            with os.fdopen(fd, 'wb') as stream:
                stream.write(canonical(receipt)); stream.flush(); os.fsync(stream.fileno())
            os.replace(temporary, 'admission.json', src_dir_fd=directory_fd, dst_dir_fd=directory_fd)
            os.fsync(directory_fd)
        finally:
            try:
                os.unlink(temporary, dir_fd=directory_fd)
            except FileNotFoundError:
                pass
        return receipt
    finally:
        os.close(lock)
