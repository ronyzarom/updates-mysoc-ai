"""Exact B incident reconciliation. Review candidate; never infers safety from absence alone."""
import contextlib
import hashlib
import json
import os
from pathlib import Path
import subprocess
import stat
import fcntl
import boundary as b

TX = 'e7d0fe81a55941b1a3ba99567f1582cf'
MACHINE = '19e0966bf88842f08bc180c40044d2ed'
UPDATER = 'bezeq-pod-test-b-5012156962629255074'
KEY = '1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
CANDIDATE = ('3.3.152.33', 'd81a9cf86ebd8a1b81b1c9e962eb5f76e931ddd66faf3c15a76628df42a4e6df')
PREVIOUS = ('3.3.152.30', '61bef7f75d77bc490c4ee4dd5822f3a077a586aa4107118e25fe1b8d8fbe82cc')
APPLY = 'siemcore-pod-boundary-ce8a9787f28a4029beb0937c02e23d56.service'
COMPENSATE = 'siemcore-pod-boundary-d68201082f0f4c368c03e1318e50d6f4.service'
PRODUCT_STATE = Path('/var/lib/siemcore-pod-update/transaction.json')


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def private_read(path):
    b.trusted(path)
    if not stat.S_ISREG(path.stat().st_mode):
        raise ValueError('regular evidence file required')
    with path.open('rb') as stream:
        raw = stream.read(65537)
    if len(raw) > 65536:
        raise ValueError('evidence exceeds limit')
    return raw


def identity(state, policy, application, machine):
    if machine != MACHINE or application.get('updater_instance_id') != UPDATER or application.get('cluster_id') != 'bezeq-pod-test' or application.get('topology') != 'pod' or application.get('pod_role') != 'b':
        raise ValueError('incident host mismatch')
    if state.get('role') != 'b' or state.get('public_key') != KEY or policy.get('public_key') != KEY:
        raise ValueError('incident trust/role mismatch')
    for name, expected in [('candidate', CANDIDATE), ('previous', PREVIOUS)]:
        ref = state[name]
        if (ref['version'], ref['sha256']) != expected or ref['archive'] != TX+'/'+name+'/release.tar.gz':
            raise ValueError('incident artifact/transaction mismatch')


def no_product_transaction():
    # lstat/lexists rejects dangling links too; verify nearest existing ancestor.
    ancestor = PRODUCT_STATE.parent
    while not ancestor.exists():
        if ancestor.is_symlink():
            raise ValueError('unsafe product state ancestor')
        ancestor = ancestor.parent
    b.trusted(ancestor)
    if os.path.lexists(PRODUCT_STATE):
        raise ValueError('product transaction exists; reconciliation refused')


def quiescent(unit):
    """Read-only: do not stop a unit or assume inactive means an empty cgroup."""
    result = subprocess.run(['/usr/bin/systemctl', 'show', unit, '-p', 'LoadState', '-p', 'ActiveState', '-p', 'ControlGroup'], check=True, capture_output=True, text=True, timeout=10)
    fields = dict(line.split('=', 1) for line in result.stdout.splitlines() if '=' in line)
    if fields.get('LoadState') == 'not-found':
        # Also check the conventional cgroup for a collected transient unit.
        group = '/system.slice/'+unit
    elif fields.get('ActiveState') in ('inactive', 'failed'):
        group = fields.get('ControlGroup') or '/system.slice/'+unit
    else:
        raise ValueError('unit not quiescent')
    if group != '/system.slice/'+unit:
        raise ValueError('unexpected unit cgroup')
    path = Path('/sys/fs/cgroup'+group)
    if path.exists() and 'populated 0' not in (path/'cgroup.events').read_text().splitlines():
        raise ValueError('unit cgroup still populated')


def verify_evidence(boundary):
    # Recipe ships with the versioned package. Never accept caller supplied pins.
    recipe = Path(__file__).with_name('incident.json')
    b.trusted(recipe)
    expected = json.loads(recipe.read_text())
    evidence = {}
    requests = set()
    for path in boundary.root.glob('*.request.json'):
        raw = private_read(path)
        request = json.loads(raw)
        env = request.get('env', {})
        if env.get('VERSION') == CANDIDATE[0]:
            requests.add(request.get('unit'))
    if requests != {APPLY, COMPENSATE}:
        raise ValueError('unexpected candidate execution history')
    for unit, action, entry in [(APPLY, 'apply', 'apply'), (COMPENSATE, 'rollback', 'compensate')]:
        quiescent(unit)
        request_raw = private_read(boundary.root/(unit+'.request.json'))
        request = json.loads(request_raw)
        env = request['env']
        bundle = env.get('CURRENT_DIR', '')
        # Original verified temporary paths were removed. Bind to retained signed
        # artifact via exact historical unit, reviewed logs and root request.
        if not bundle.startswith(str(boundary.root)+'/verified-') or '/unpacked/siemcore-universal-3.3.152.33' not in bundle:
            raise ValueError('unexpected execution path')
        if request['unit'] != unit or request['args'] != [bundle+'/updater/'+entry, action] or env != {
            'PATH':'/usr/sbin:/usr/bin:/sbin:/bin','HOME':'/root','LANG':'C.UTF-8',
            'UPDATER_PHASE':action,'PRODUCT':'siemcore','VERSION':CANDIDATE[0],
            'SIEMCORE_POD_RECOVERY_PROTOCOL':'1','CURRENT_DIR':bundle,
            'INSTALL_ROOT':str(boundary.old.CURRENT.parents[1]),
            'SIEMCORE_INSTALL_DIR':'/opt/siemcore-app-app-b'}:
            raise ValueError('phase request mismatch')
        for suffix in ('.stdout', '.stdout.stderr'):
            name = unit+suffix
            raw = private_read(boundary.root/name)
            if digest(raw) != expected['logs'][name]:
                raise ValueError('reviewed failure evidence mismatch')
            evidence[name] = digest(raw)
        evidence[unit+'.request.json'] = digest(request_raw)
    return evidence


def reconcile(boundary, policy, application, machine):
    b.trusted(boundary.root, True)
    state = boundary.read()
    if not boundary.old.completed_greenfield():
        raise ValueError('completed bootstrap receipt required')
    identity(state, policy, application, machine)
    if state.get('phase') != 'compensating' or state.get('pending_unit') != COMPENSATE or 'draining_unit' in state:
        raise ValueError('unexpected incident phase')
    version, entry = boundary.old.receipt(boundary.old.CURRENT, policy)
    if (version, entry['sha256']) != PREVIOUS:
        raise ValueError('installed predecessor digest mismatch')
    no_product_transaction()
    for name in ('candidate', 'previous'):
        with boundary.bundle(state[name], KEY):
            pass  # existing checksum/signature verification and safe unpack; never execute
    evidence = verify_evidence(boundary)
    no_product_transaction()
    original = private_read(boundary.journal)
    preserved = boundary.root/(TX+'.preflight-evidence.json')
    record = {'schema_version':1, 'transaction_id':TX, 'original_journal':json.loads(original),
              'original_journal_sha256':digest(original), 'evidence_sha256':evidence}
    if preserved.exists():
        if json.loads(private_read(preserved)) != record:
            raise ValueError('preserved evidence conflict')
    else:
        b.atomic(preserved, record)
    state.pop('pending_unit')
    state['reconciliation'] = {'transaction_id':TX, 'reason_code':'active_controller_prerequisite_refused',
                               'evidence_sha256':digest(private_read(preserved)),
                               'compensation_accepted':False, 'rollback_dispatched':False}
    state['phase_failures'] = [
        {'schema_version':1, 'transaction_id':TX, 'phase':phase, 'product':'siemcore',
         'version':CANDIDATE[0], 'sha256':CANDIDATE[1], 'reason_code':code}
        for phase, code in [('apply','active_controller_prerequisite_refused'), ('compensate','missing_product_transaction')]]
    boundary.save(state, 'preflight-refused')
    return {'status':'preflight-refused', 'transaction_id':TX, 'phase_failures':state['phase_failures'],
            'compensation_accepted':False, 'rollback_dispatched':False, 'health_verified':False}


def validate_terminal(boundary, state, policy, application):
    identity(state, policy, application, Path('/etc/machine-id').read_text().strip())
    evidence = private_read(boundary.root/(TX+'.preflight-evidence.json'))
    if state.get('reconciliation', {}).get('evidence_sha256') != digest(evidence):
        raise ValueError('terminal reconciliation evidence mismatch')
    no_product_transaction()
    # The next normal apply has already switched current to its new candidate.
    # Check the executor's retained predecessor instead of that new current.
    previous = boundary.old.CURRENT.parent/'.previous'
    if previous.is_symlink() or not previous.is_file():
        raise ValueError('invalid predecessor pointer')
    version, entry = boundary.old.receipt(Path(previous.read_text().strip()), policy)
    if (version, entry['sha256']) != PREVIOUS:
        raise ValueError('next apply predecessor changed')


@contextlib.contextmanager
def locks():
    # Same order as updater -> boundary -> supervised worker. Nonblocking throughout.
    with contextlib.ExitStack() as stack:
        for path in [Path('/var/lib/siemcore-cascade-updater/state.json.cycle-lock'), b.ROOT/'boundary.lock', b.ROOT/'work.lock']:
            # The cycle lock is intentionally owned by the unprivileged updater.
            # Do not apply root ownership rules to it; pin the opened inode.
            if path.parent == b.ROOT:
                b.trusted(path)
            fd = os.open(path, os.O_RDWR | os.O_NOFOLLOW)
            stream = stack.enter_context(os.fdopen(fd, 'r+'))
            if not stat.S_ISREG(os.fstat(fd).st_mode):
                raise ValueError('regular lock required')
            fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
            actual = path.stat(follow_symlinks=False)
            opened = os.fstat(fd)
            if (actual.st_dev, actual.st_ino) != (opened.st_dev, opened.st_ino):
                raise ValueError('lock inode changed')
        yield


def main():
    import importlib.util
    if os.geteuid() != 0:
        raise ValueError('root required')
    b.trusted(b.ROOT, True)
    b.trusted(b.LEGACY)
    if digest(b.LEGACY.read_bytes()) != b.LEGACY_SHA:
        raise ValueError('legacy boundary drift')
    spec = importlib.util.spec_from_file_location('legacy', b.LEGACY)
    old = importlib.util.module_from_spec(spec); spec.loader.exec_module(old)
    with locks():
        report = reconcile(b.Boundary(old), old.protected(old.POLICY),
                           old.protected(Path('/etc/siemcore/greenfield.json')),
                           Path('/etc/machine-id').read_text().strip())
    print(json.dumps(report, sort_keys=True))


if __name__ == '__main__':
    main()
