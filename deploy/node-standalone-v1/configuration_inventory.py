"""Pure standalone configuration inventory hashing; never writes secret inputs.

Caller supplies the reviewed allowlist and expected original owner/mode metadata
from protected policy. Use an exclusively locked, private staging tree before
mode.json creation; this function is not a concurrent hostile-tree sandbox.
This offline helper is not wired into an executor.
"""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import stat


def configuration_digest(root, expected):
    root = Path(root)
    if root.is_symlink() or not root.is_dir():
        raise ValueError('real_configuration_directory_required')
    required = {'application.env', 'installation/tls.crt', 'installation/tls.key'}
    if not isinstance(expected, dict) or not required <= set(expected):
        raise ValueError('required_configuration_missing')
    for name, metadata in expected.items():
        path = PurePosixPath(name)
        if (path.is_absolute() or str(path) != name or '..' in path.parts
                or name == 'installation/mode.json' or len(path.parts) > 2):
            raise ValueError('invalid_configuration_name')
        if name not in required and not (len(path.parts) == 2 and path.parts[0] == 'archive-credentials') and name != 'mysoc-bootstrap.json':
            raise ValueError('unapproved_configuration_name')
        if not isinstance(metadata, dict) or set(metadata) != {'uid', 'gid', 'mode'}:
            raise ValueError('exact_input_metadata_required')
    found = set()
    allowed_directories = {str(PurePosixPath(name).parent) for name in expected} - {'.'}
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs:
            p = Path(directory) / name
            if p.is_symlink() or p.relative_to(root).as_posix() not in allowed_directories:
                raise ValueError('unexpected_configuration_directory')
        for name in files:
            found.add((Path(directory) / name).relative_to(root).as_posix())
    if found != set(expected):
        raise ValueError('configuration_inventory_mismatch')
    hashes = {}
    for name, metadata in expected.items():
        fd = os.open(root / name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode):
                raise ValueError('regular_configuration_file_required')
            if {'uid': info.st_uid, 'gid': info.st_gid, 'mode': stat.S_IMODE(info.st_mode)} != metadata:
                raise ValueError('configuration_metadata_mismatch')
            checksum = hashlib.sha256()
            with os.fdopen(fd, 'rb', closefd=False) as stream:
                for block in iter(lambda: stream.read(65536), b''):
                    checksum.update(block)
            after = os.fstat(fd)
            if (info.st_size, info.st_mtime_ns, info.st_ctime_ns) != (after.st_size, after.st_mtime_ns, after.st_ctime_ns):
                raise ValueError('configuration_changed_during_read')
            hashes[name] = checksum.hexdigest()
        finally:
            os.close(fd)
    return hashlib.sha256(json.dumps(hashes, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()).hexdigest()
