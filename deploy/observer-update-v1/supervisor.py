#!/usr/bin/env python3
"""Own the inherited root flock until worker group cleanup, including parent death."""
import os
import selectors
import signal
import subprocess
import sys
import time

watch_fd, lock_fd, deadline_seconds = map(int, sys.argv[1:4])
# lock_fd intentionally remains open here, not in the product worker.
stop = False
signal.signal(signal.SIGTERM, lambda *_: globals().__setitem__('stop', True))
signal.signal(signal.SIGINT, lambda *_: globals().__setitem__('stop', True))
worker = subprocess.Popen(sys.argv[4:], stdin=sys.stdin.buffer, stdout=sys.stdout.buffer,
                          stderr=sys.stderr.buffer, start_new_session=True, close_fds=True)
selector = selectors.DefaultSelector()
selector.register(watch_fd, selectors.EVENT_READ)
deadline = time.monotonic() + deadline_seconds
expired = False
try:
    while worker.poll() is None:
        if stop or time.monotonic() >= deadline or selector.select(.05):
            expired = True
            break
finally:
    try: os.killpg(worker.pid, signal.SIGKILL)
    except ProcessLookupError: pass
    worker.wait()
    selector.close()
    os.close(watch_fd)
    os.close(lock_fd)
sys.exit(124 if expired else worker.returncode if worker.returncode >= 0 else 125)
