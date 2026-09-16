#!/usr/bin/env python3
"""Updates-owned privileged retained-candidate compensation boundary (candidate)."""
import contextlib
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import uuid

LEGACY = Path('/usr/local/lib/siemcore-cascade/greenfield-hook.py')
LEGACY_SHA = '221fe046129211fcd8b421e495134e5bf5054b9934c74f0029a2e49667b95a88'
ROOT = Path('/var/lib/siemcore-pod-boundary')
PREFIX = 'MYSOC_COMPENSATION_RESULT_V1:'
INSTALLED = Path('/usr/local/lib/siemcore-pod-boundary/1.0.0.1/boundary.py')
UNIT = re.compile(r'^siemcore-pod-boundary-[0-9a-f]{32}\.service$')


def trusted(path, private=False):
    for part in (path, *path.parents):
        st = part.lstat()
        if stat.S_ISLNK(st.st_mode) or st.st_uid != 0 or st.st_mode & 0o022:
            raise ValueError('unprotected boundary path')
    if private and path.stat().st_mode & 0o077:
        raise ValueError('boundary state must be private')


def atomic(path, value):
    raw = (json.dumps(value, sort_keys=True) + '\n').encode()
    fd, name = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as out:
            os.fchmod(out.fileno(), 0o600)
            out.write(raw); out.flush(); os.fsync(out.fileno())
        os.replace(name, path)
        fd = os.open(path.parent, os.O_DIRECTORY)
        try: os.fsync(fd)
        finally: os.close(fd)
    finally:
        if os.path.exists(name): os.unlink(name)


class Units:
    def quiesce(self, unit):
        if not UNIT.fullmatch(unit):
            raise ValueError('invalid persisted unit identity')
        subprocess.run(['/usr/bin/systemctl', 'stop', unit], timeout=25, capture_output=True)
        result = subprocess.run(['/usr/bin/systemctl', 'show', unit, '-p', 'ActiveState', '--value'],
                                timeout=10, capture_output=True, text=True)
        if result.stdout.strip() not in ('inactive', 'failed'):
            # An absent transient unit has no remaining cgroup process.
            loaded = subprocess.run(['/usr/bin/systemctl', 'show', unit, '-p', 'LoadState', '--value'],
                                    timeout=10, capture_output=True, text=True)
            if loaded.stdout.strip() != 'not-found':
                raise RuntimeError('previous unit is not quiescent; rollback refused')

    def run(self, unit, args, env, output, timeout):
        command = ['/usr/bin/systemd-run', '--quiet', '--wait', '--unit='+unit,
                   '--property=Type=exec', '--property=KillMode=control-group',
                   '--property=Restart=no', '--property=SendSIGKILL=yes',
                   '--property=TimeoutStopSec=10s', '--property=RuntimeMaxSec='+str(timeout)+'s',
                   '--property=StandardOutput=file:'+str(output),
                   '--property=StandardError=file:'+str(output)+'.stderr']
        request = output.with_suffix('.request.json')
        atomic(request, {'unit': unit, 'args': [str(x) for x in args], 'env': env})
        try:
            result = subprocess.run(command + ['--', '/usr/bin/python3', str(INSTALLED), '_worker', unit],
                                    timeout=timeout+20, capture_output=True)
        finally:
            self.quiesce(unit)
        if result.returncode:
            raise RuntimeError('supervised product phase failed; retained transaction requires rollback')
        with output.open('rb') as stream:
            data = stream.read(65537)
        return data


class Boundary:
    def __init__(self, legacy, root=ROOT, units=None):
        self.old, self.root, self.units = legacy, root, units or Units()
        self.journal = root / 'transaction.json'

    def read(self):
        trusted(self.journal, True)
        return json.loads(self.journal.read_text())

    def save(self, state, phase):
        state['phase'] = phase
        atomic(self.journal, state)

    def retain(self, path, directory, policy):
        version, entry = self.old.receipt(path, policy)
        directory.mkdir(mode=0o700)
        # Copies archive privately, validates Ed25519/digest then safely unpacks.
        self.old.verified_bundle(entry, version, policy['public_key'], directory)
        archive = directory / 'release.tar.gz'
        archive.chmod(0o400)
        return {'version': version, 'sha256': entry['sha256'], 'signature': entry['signature'],
                'archive': str(archive.relative_to(self.root))}

    @contextlib.contextmanager
    def bundle(self, ref, key):
        archive = self.root / ref['archive']
        if archive.resolve(strict=True).parent.parent.parent != self.root.resolve():
            raise ValueError('retained reference escapes transaction')
        trusted(archive, True)
        with tempfile.TemporaryDirectory(prefix='verified-', dir=self.root) as temporary:
            yield self.old.verified_bundle({'artifact': str(archive), 'sha256': ref['sha256'],
                                           'signature': ref['signature']}, ref['version'], key, Path(temporary))

    def invoke(self, state, ref, action, compensation=False):
        with self.bundle(ref, state['public_key']) as bundle:
            executable = bundle / 'updater' / ('compensate' if compensation else 'apply')
            if not executable.is_file() or executable.is_symlink() or not os.access(executable, os.X_OK):
                raise ValueError('signed executable entrypoint required')
            unit = 'siemcore-pod-boundary-'+uuid.uuid4().hex+'.service'
            state['pending_unit'] = unit
            # Persist BEFORE starting a unit; a killed boundary leaves a recoverable unit reference.
            self.save(state, 'compensating' if compensation else action)
            output = self.root / (unit+'.stdout')
            env = {'PATH':'/usr/sbin:/usr/bin:/sbin:/bin','HOME':'/root','LANG':'C.UTF-8',
                   'UPDATER_PHASE': action,'PRODUCT':'siemcore','VERSION':ref['version'],
                   'SIEMCORE_POD_RECOVERY_PROTOCOL':'1',
                   'CURRENT_DIR':str(bundle),'INSTALL_ROOT':str(self.old.CURRENT.parents[1]),
                   'SIEMCORE_INSTALL_DIR':'/opt/siemcore-app-app-'+state['role']}
            raw = self.units.run(unit, [executable, action], env, output, 90 if compensation else 600)
            state.pop('pending_unit', None)
            self.save(state, 'compensated' if compensation else action+'-returned')
            if compensation:
                if len(raw)>65536:
                    raise ValueError('compensation output exceeds limit')
                lines=raw.decode().splitlines()
                if len(lines)!=1 or not lines[0].startswith(PREFIX):
                    raise ValueError('exact compensation receipt required')
                receipt=json.loads(lines[0][len(PREFIX):])
                if receipt != {'status':'paused','product':'siemcore','version':ref['version']}:
                    raise ValueError('compensation identity/status mismatch')

    def dispatch(self, policy, application, phase):
        role=application.get('pod_role')
        if role not in ('a','b'):
            raise ValueError('boundary supports qualified data-node updates only')
        state=self.read() if self.journal.exists() else None
        if state and (state['role']!=role or state['public_key']!=policy['public_key']):
            raise ValueError('transaction host policy changed')
        if state and state.get('pending_unit'):
            state['draining_unit'] = state.pop('pending_unit')
            self.save(state,'interrupted')  # revoke delayed unit, retain identity until stopped
        if state and state.get('draining_unit'):
            self.units.quiesce(state['draining_unit'])
            state.pop('draining_unit')
            self.save(state,'interrupted')
        if phase=='apply':
            if state and state['phase'] not in ('healthy','rolled-back'):
                raise RuntimeError('unfinished transaction requires compensation/rollback first')
            previous=self.old.CURRENT.parent/'.previous'
            if not previous.is_file() or previous.is_symlink():
                raise ValueError('signed predecessor required')
            old_path=self.old.release_path(Path(previous.read_text().strip()))
            target=self.old.release_path(self.old.CURRENT)
            if target==old_path:
                raise ValueError('candidate equals predecessor')
            tx=self.root/uuid.uuid4().hex;tx.mkdir(mode=0o700)
            candidate=self.retain(target,tx/'candidate',policy)
            predecessor=self.retain(old_path,tx/'previous',policy)
            state={'role':role,'public_key':policy['public_key'],'candidate':candidate,'previous':predecessor}
            with self.bundle(candidate,state['public_key']) as b:
                e=b/'updater/compensate'
                if not e.is_file() or not os.access(e,os.X_OK):
                    raise ValueError('candidate compensation contract missing')
            self.save(state,'prepared')
            self.invoke(state,candidate,'apply')
            self.save(state,'awaiting-health')
        elif phase=='health':
            if not state or state['phase'] not in ('awaiting-health','healthy'):
                raise RuntimeError('no applied transaction for health')
            version,entry=self.old.receipt(self.old.CURRENT,policy)
            if (version,entry['sha256'])!=(state['candidate']['version'],state['candidate']['sha256']):
                raise ValueError('health target changed')
            self.invoke(state,state['candidate'],'health')
            self.save(state,'healthy')
        elif phase=='rollback':
            if not state:
                raise RuntimeError('no retained transaction; rollback refused')
            if state['phase']=='rolled-back':return
            version,entry=self.old.receipt(self.old.CURRENT,policy)
            if (version,entry['sha256'])!=(state['previous']['version'],state['previous']['sha256']):
                raise ValueError('rollback predecessor changed')
            self.invoke(state,state['candidate'],'rollback',compensation=True)
            self.invoke(state,state['previous'],'rollback')
            self.save(state,'rolled-back')
        else:raise ValueError('unsupported boundary phase')


def worker(unit):
    if os.geteuid()!=0 or not UNIT.fullmatch(unit):
        raise ValueError('invalid supervised worker')
    trusted(ROOT, True)
    with (ROOT/'work.lock').open('a') as lock:
        os.chmod(ROOT/'work.lock', 0o600)
        fcntl.flock(lock, fcntl.LOCK_EX|fcntl.LOCK_NB)
        journal=ROOT/'transaction.json';trusted(journal,True)
        if json.loads(journal.read_text()).get('pending_unit')!=unit:
            raise ValueError('superseded phase cannot start')
        request=ROOT/(unit+'.request.json');trusted(request,True)
        data=json.loads(request.read_text())
        if data['unit']!=unit:raise ValueError('worker identity mismatch')
        # Children may create sessions but remain inside the systemd control group.
        subprocess.run(data['args'],env=data['env'],check=True)


def main():
    if os.geteuid()!=0 or len(sys.argv)!=2 or sys.argv[1] not in ('apply','health','rollback'):
        raise ValueError('Linux root boundary phase required')
    trusted(LEGACY)
    if hashlib.sha256(LEGACY.read_bytes()).hexdigest()!=LEGACY_SHA:
        raise ValueError('installed legacy boundary drift')
    spec=importlib.util.spec_from_file_location('legacy_boundary',LEGACY)
    old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)
    application=old.protected(Path('/etc/siemcore/greenfield.json'))
    if application.get('topology')!='pod' or application.get('pod_role') not in ('a','b') or not old.completed_greenfield():
        old.main();return
    ROOT.mkdir(mode=0o700,exist_ok=True);trusted(ROOT,True)
    with (ROOT/'boundary.lock').open('a') as lock:
        os.chmod(ROOT/'boundary.lock',0o600)
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        Boundary(old).dispatch(old.protected(old.POLICY),application,sys.argv[1])

if __name__=='__main__':
    try:
        if len(sys.argv)==3 and sys.argv[1]=='_worker':worker(sys.argv[2])
        else:main()
    except Exception as error:
        print('Updates pod boundary refused: '+str(error),file=sys.stderr);sys.exit(1)
