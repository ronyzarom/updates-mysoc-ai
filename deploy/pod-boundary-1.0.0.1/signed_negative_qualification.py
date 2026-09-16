#!/usr/bin/env python3
"""Disposable-only exact signed artifact negative test; never runs product apply."""
import hashlib, importlib.util, json, os, pathlib, shutil, socket, urllib.request, uuid
import boundary as b
from pod_compensation_inputs import inputs

assert os.geteuid() == 0
assert socket.gethostname().split('.')[0] == 'updater-boundary-qual-20260916'
P = pathlib.Path
assert not P('/var/lib/siemcore-pod-update/transaction.json').exists()
assert not P('/opt/siemcore-app-app-a').exists()
assert hashlib.sha256(b.LEGACY.read_bytes()).hexdigest() == b.LEGACY_SHA
spec = importlib.util.spec_from_file_location('legacy', b.LEGACY)
old = importlib.util.module_from_spec(spec); spec.loader.exec_module(old)
base = P(__file__).resolve().parent
receipts = json.loads((base/'signed-receipts.json').read_text())
policy = {'public_key': receipts['candidate']['public_key']}
app, controller = inputs(P('/etc/machine-id').read_text().strip())
app['updater_instance_id'] = 'boundary-fixture-a'
for target, data in [('/etc/siemcore/greenfield.json', app), ('/etc/siemcore-pod-controller/controller.json', controller)]:
    path = P(target); path.parent.mkdir(parents=True, exist_ok=True)
    b.atomic(path, data)
assert not old.RELEASES.exists(), 'disposable fixture requires empty release directory'
old.RELEASES.mkdir(parents=True)
old.CACHE.mkdir(parents=True, exist_ok=True)
req = urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token', headers={'Metadata-Flavor':'Google'})
token = json.load(urllib.request.urlopen(req, timeout=15))['access_token']
b.ROOT.mkdir(mode=0o700, exist_ok=True)
tx = b.ROOT/uuid.uuid4().hex; tx.mkdir(mode=0o700)
boundary = b.Boundary(old)
retained = {}
for kind, receipt in receipts.items():
    version = receipt['version']
    artifact = old.CACHE/('siemcore-'+version+'.artifact')
    url = 'https://storage.googleapis.com/osherad-graylog-boundary-qual-20260916/siemcore-universal-'+version+'.tar.gz'
    request = urllib.request.Request(url, headers={'Authorization':'Bearer '+token})
    with urllib.request.urlopen(request, timeout=120) as src, artifact.open('xb') as dst:
        shutil.copyfileobj(src, dst)
    release = old.RELEASES/version; release.mkdir()
    b.atomic(release/'.updater-release.json', dict(receipt, product='siemcore'))
    retained[kind] = boundary.retain(release, tx/kind, policy)
old.CURRENT.symlink_to(old.RELEASES/receipts['previous']['version'])
state = dict(retained, role='a', public_key=policy['public_key'])
boundary.save(state, 'awaiting-health')

class AuditedUnits(b.Units):
    def __init__(self): self.calls = []
    def run(self, unit, args, env, output, timeout):
        self.calls.append({'unit':unit, 'entrypoint':P(args[0]).name, 'version':env['VERSION']})
        b.atomic(base/'invocations.json', self.calls)
        return super().run(unit, args, env, output, timeout)

audit = AuditedUnits(); boundary.units = audit
try:
    boundary.dispatch(policy, app, 'rollback')
except RuntimeError as error:
    assert 'supervised product phase failed' in str(error), str(error)
else:
    raise AssertionError('missing product transaction accepted')
assert len(audit.calls) == 1 and audit.calls[0]['entrypoint'] == 'compensate'
assert audit.calls[0]['version'] == '3.3.152.32'
unit = audit.calls[0]['unit']
stderr = (b.ROOT/(unit+'.stdout.stderr')).read_text()
assert 'no candidate transaction; cannot claim paused recovery' in stderr, stderr
assert not (b.ROOT/(unit+'.stdout')).read_text().strip()
audit.quiesce(unit)
assert not P('/var/lib/siemcore-pod-update/transaction.json').exists()
assert not P('/opt/siemcore-app-app-a').exists()
result = {'status':'passed', 'checks':['both-exact-artifacts-ed25519-and-checksum-verified', 'retained-private-archives', 'actual-signed-32-compensate-missing-transaction-refused', 'no-30-rollback-invocation', 'native-unit-quiescent'], 'artifacts':{k:{f:v[f] for f in ('version','sha256')} for k,v in receipts.items()}, 'scope':'negative full signed entrypoint only; no successful compensation, full retained rollback, Docker, database or availability qualification'}
b.atomic(P('/var/lib/updater-boundary-signed-negative-result.json'), result)
with open('/dev/ttyS0','w') as serial: serial.write('UPDATES_SIGNED_NEGATIVE '+json.dumps(result)+'\n')
print(json.dumps(result))
