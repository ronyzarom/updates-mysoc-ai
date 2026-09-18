"""Bounded worker transport. Supervisor retains root lock across parent death."""
import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import sys
import tempfile
import time
from protocol import PROTOCOL, digest, strict_json


def invoke(action, binding, bundles, operation_directory, lock_fd, timeout=900):
    if type(timeout) is not int or not 1 <= timeout <= 900:
        raise ValueError('bounded execution deadline required')
    target = Path(bundles['target_bundle'])
    script = target / 'updater/observer_unlinked_update.py'
    if not script.is_file() or script.is_symlink():
        raise ValueError('verified target worker missing')
    request = dict(protocol=PROTOCOL, action=action, binding=binding,
                   operation_sha256=digest(binding), operation_directory=str(operation_directory),
                   target_bundle=str(target), predecessor_bundle=str(bundles['predecessor_bundle']),
                   application_policy='/etc/siemcore/greenfield.json')
    request_raw = json.dumps(request).encode()
    if len(request_raw) > 65536:
        raise ValueError('worker request too large')
    read_fd, write_fd = os.pipe()
    with tempfile.TemporaryFile(dir=operation_directory) as payload:
        payload.write(request_raw); payload.seek(0)
        supervisor = subprocess.Popen([sys.executable, str(Path(__file__).with_name('supervisor.py')),
                    str(read_fd), str(lock_fd), str(timeout), '/usr/bin/python3', str(script)],
                    stdin=payload, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                    pass_fds=(read_fd, lock_fd), start_new_session=True,
                    env={'PATH':'/usr/sbin:/usr/bin:/sbin:/bin', 'HOME':'/root', 'LANG':'C.UTF-8'})
        os.close(read_fd)
        selector = selectors.DefaultSelector()
        selector.register(supervisor.stdout, selectors.EVENT_READ)
        selector.register(supervisor.stderr, selectors.EVENT_READ)
        output = bytearray(); stderr_size = 0
        deadline = time.monotonic() + timeout + 5
        try:
            while selector.get_map():
                if time.monotonic() >= deadline:
                    raise TimeoutError('worker outcome uncertain')
                for key,_ in selector.select(.1):
                    raw = os.read(key.fileobj.fileno(), 8192)
                    if not raw:
                        selector.unregister(key.fileobj)
                        continue
                    if key.fileobj is supervisor.stdout:
                        output.extend(raw)
                        if len(output) > 65536: raise ValueError('worker output exceeds limit')
                    else:
                        stderr_size += len(raw)
                        if stderr_size > 1024*1024: raise ValueError('worker diagnostic output exceeds limit')
            if supervisor.wait(timeout=5) != 0:
                raise ValueError('worker outcome uncertain')
            return strict_json(output)
        finally:
            # EOF tells supervisor to kill the worker group before releasing
            # the inherited lock. Never release a lock while mutation runs.
            os.close(write_fd)
            if supervisor.poll() is None:
                supervisor.send_signal(signal.SIGTERM)
                supervisor.wait(timeout=10)
            selector.close()
            supervisor.stdout.close(); supervisor.stderr.close()
