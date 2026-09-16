#!/usr/bin/env python3
"""One-time, exact-host OS Config prerequisite. Never delivers a product release."""
import base64, fcntl, hashlib, io, json, os, pathlib, re, subprocess, tarfile, urllib.request
P = pathlib.Path
base = P(__file__).resolve().parent
expected = json.loads((base/'expected.json').read_text())
receipt = json.loads((base/'receipt.json').read_text())
assert os.geteuid() == 0
req = urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/id', headers={'Metadata-Flavor':'Google'})
assert urllib.request.urlopen(req, timeout=10).read().decode() == expected['gcp_id']
assert P('/etc/machine-id').read_text().strip() == expected['machine_id']
app = json.loads(P('/etc/siemcore/greenfield.json').read_text())
assert app['updater_instance_id'] == expected['updater_instance_id'] and app['pod_role'] == expected['role']
def digest(p): return hashlib.sha256(P(p).read_bytes()).hexdigest()
for path, sha in expected['hashes'].items(): assert digest(path) == sha, 'preexisting input drift: '+path
assert not P('/var/lib/siemcore-pod-boundary/transaction.json').exists()
assert json.loads(P('/opt/siemcore-app-app-'+app['pod_role']+'/.node.json').read_text())['installed_version'] == '3.3.152.30'
assert receipt['public_key'] == '1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
archive = base/'pod-boundary-1.0.0.1.tar.gz'
assert digest(archive) == receipt['sha256']
(base/'key.der').write_bytes(bytes.fromhex('302a300506032b6570032100'+receipt['public_key']))
(base/'signature').write_bytes(base64.b64decode(receipt['signature'],validate=True))
(base/'message').write_text('mysoc-pod-boundary-v1\n1.0.0.1\n'+receipt['sha256'])
subprocess.run(['openssl','pkeyutl','-verify','-pubin','-keyform','DER','-inkey',str(base/'key.der'),'-rawin','-in',str(base/'message'),'-sigfile',str(base/'signature')],check=True,capture_output=True,timeout=15)
package = base/'package'; package.mkdir(mode=0o700, exist_ok=True)
with tarfile.open(archive) as tar:
    names = set()
    for entry in tar:
        assert entry.name in ('boundary.py','install.py','wrapper','files.json') and entry.isfile() and entry.name not in names
        names.add(entry.name); p = package/entry.name; p.write_bytes(tar.extractfile(entry).read()); p.chmod(0o600)
    assert len(names)==4
manifest = json.loads((package/'files.json').read_text())
for name, sha in manifest.items(): assert digest(package/name)==sha
config = P('/etc/siemcore-cascade-updater/config.yaml')
config_sha = digest(config)
assert re.search(r'^\s*state_file:\s*/var/lib/siemcore-cascade-updater/state.json\s*$',config.read_text(),re.M)
service = 'siemcore-cascade-updater.service'
assert subprocess.check_output(['systemctl','is-active',service],text=True).strip()=='active'
# This is the updater's own process-shared cycle lock. Nonblocking acquisition
# refuses an in-flight apply and prevents a new cycle until the service is down.
lockpath = '/var/lib/siemcore-cascade-updater/state.json.cycle-lock'
fd = os.open(lockpath, os.O_RDWR | os.O_NOFOLLOW)
try:
    fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
    assert os.fstat(fd).st_ino == os.stat(lockpath,follow_symlinks=False).st_ino
    try:
        subprocess.run(['systemctl','stop',service],check=True,timeout=45)
        subprocess.run(['/usr/bin/python3',str(package/'install.py'),'--expected-machine-id',expected['machine_id'],'--expected-updater-id',expected['updater_instance_id']],check=True,timeout=30)
        for path, sha in expected['hashes'].items():
            if path != '/usr/local/sbin/siemcore-apply-update': assert digest(path)==sha
        assert digest(config)==config_sha
        assert digest('/usr/local/sbin/siemcore-apply-update')==manifest['wrapper']
        assert digest('/usr/local/lib/siemcore-pod-boundary/1.0.0.1/boundary.py')==manifest['boundary.py']
    finally:
        fcntl.flock(fd, fcntl.LOCK_UN)
        subprocess.run(['systemctl','start',service],check=True,timeout=30)
finally:
    os.close(fd)
assert subprocess.check_output(['systemctl','is-active',service],text=True).strip()=='active'
result={'status':'installed','gcp_id':expected['gcp_id'],'role':expected['role'],'package_sha256':receipt['sha256'],'installed_hashes':manifest,'updater':'active','product_version':'3.3.152.30','policy_sudo_config':'unchanged'}
marker=P('/var/lib/siemcore-pod-boundary-provisioned.json')
marker.write_text(json.dumps(result)+'\n');marker.chmod(0o600)
with open('/dev/ttyS0','w') as serial:serial.write('UPDATES_BOUNDARY_INSTALLED '+json.dumps(result)+'\n')
print(json.dumps(result))
