#!/usr/bin/env python3
"""Restart-safe exact-incident package activation, invoked by approved OS Config.
The caller verifies the signed package before running this code and stops only
its updater after acquiring the updater cycle lock. No product services touched.
"""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import boundary as b
import reconcile as r

OLD = Path('/usr/local/lib/siemcore-pod-boundary/1.0.0.1/boundary.py')
OLD_SHA = 'ce846a61a6252670cad87a77a0f67e4ec44476e847201af8665469d59a3523ac'
WRAPPER = Path('/usr/local/sbin/siemcore-apply-update')
OLD_WRAPPER = b'#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 /usr/local/lib/siemcore-pod-boundary/1.0.0.1/boundary.py "$@"\n'
NEW_WRAPPER = OLD_WRAPPER.replace(b'/1.0.0.1/', b'/1.0.0.2/')
FILES = {'boundary.py','reconcile.py','incident.json'}


def write_file(path, raw, mode=0o644):
    fd, temporary = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd,'wb') as stream:
            os.fchmod(stream.fileno(),mode)
            stream.write(raw);stream.flush();os.fsync(stream.fileno())
        os.replace(temporary,path)
        directory=os.open(path.parent,os.O_DIRECTORY)
        try:os.fsync(directory)
        finally:os.close(directory)
    finally:
        if os.path.exists(temporary):os.unlink(temporary)


def activate(base, boundary, policy, application, machine):
    # Called while all three execution locks are held and updater is inactive.
    b.trusted(base);b.trusted(OLD);b.trusted(WRAPPER)
    if r.digest(OLD.read_bytes()) != OLD_SHA:
        raise ValueError('installed boundary drift')
    if WRAPPER.read_bytes() not in (OLD_WRAPPER,NEW_WRAPPER):
        raise ValueError('installed wrapper drift')
    manifest=json.loads((base/'files.json').read_text())
    if set(manifest) != FILES | {'install.py'}:
        raise ValueError('unexpected package contents')
    for name, sha in manifest.items():
        b.trusted(base/name)
        if r.digest((base/name).read_bytes()) != sha:
            raise ValueError('package hash mismatch')
    state=boundary.read()
    r.identity(state,policy,application,machine)
    # Stage the complete versioned package before touching the incident journal.
    b.INSTALLED.parent.mkdir(mode=0o755,parents=True,exist_ok=True)
    b.trusted(b.INSTALLED.parent)
    for name in sorted(FILES):
        target=b.INSTALLED.parent/name
        if target.exists() or target.is_symlink():
            b.trusted(target)
            if target.read_bytes() != (base/name).read_bytes():
                raise ValueError('versioned installation drift')
        else:
            write_file(target,(base/name).read_bytes())
    if state['phase']=='preflight-refused':
        # Crash after terminal write is resumable; validate preserved evidence,
        # current predecessor and original failure evidence again.
        r.validate_terminal(boundary,state,policy,application)
        version, entry=boundary.old.receipt(boundary.old.CURRENT,policy)
        if (version,entry['sha256']) != r.PREVIOUS:
            raise ValueError('current changed during package recovery')
        r.verify_evidence(boundary)
        for name in ('candidate','previous'):
            with boundary.bundle(state[name],r.KEY):pass
    else:
        r.reconcile(boundary,policy,application,machine)
    # If killed here, .1 refuses preflight-refused. Rerun completes the switch.
    if WRAPPER.read_bytes() != NEW_WRAPPER:
        write_file(WRAPPER,NEW_WRAPPER,0o755)
    return {'boundary_version':'1.0.0.2','status':'preflight-refused',
            'transaction_id':r.TX,'rollback_dispatched':False,'health_verified':False}


def main():
    if os.geteuid()!=0:raise ValueError('root required')
    active=subprocess.check_output(['/usr/bin/systemctl','show','siemcore-cascade-updater.service','-p','ActiveState','--value'],text=True,timeout=10).strip()
    if active!='inactive':raise ValueError('updater must already be inactive')
    b.trusted(b.LEGACY)
    if r.digest(b.LEGACY.read_bytes())!=b.LEGACY_SHA:raise ValueError('legacy drift')
    spec=importlib.util.spec_from_file_location('legacy',b.LEGACY)
    old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)
    with r.locks():
        result=activate(Path(__file__).resolve().parent,b.Boundary(old),old.protected(old.POLICY),
                        old.protected(Path('/etc/siemcore/greenfield.json')),
                        Path('/etc/machine-id').read_text().strip())
    print(json.dumps(result,sort_keys=True))

if __name__=='__main__':main()
