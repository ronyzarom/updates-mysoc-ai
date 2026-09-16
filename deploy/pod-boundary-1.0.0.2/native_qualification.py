#!/usr/bin/env python3
"""Disposable native fixture: real signatures/cgroups; replayed B incident evidence."""
import importlib.util,json,os,socket,subprocess,time,urllib.request,shutil,uuid
from pathlib import Path
from unittest.mock import patch
import boundary as b
import reconcile as r
import install as installer
assert os.geteuid()==0 and socket.gethostname().split('.')[0]=='updater-boundary-qual-20260916'
base=Path(__file__).resolve().parent
result_path=Path('/var/lib/updater-boundary-2-native-result.json')
if result_path.exists():raise SystemExit('fixture already completed')
# No pod containers or application services may exist on the disposable VM.
assert not Path('/opt/siemcore-app-app-a').exists() and not Path('/opt/siemcore-app-app-b').exists()
fixture=Path('/var/lib/updater-boundary-2-fixture');fixture.mkdir(mode=0o700,exist_ok=True)
spec=importlib.util.spec_from_file_location('legacy',b.LEGACY);old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)
assert r.digest(b.LEGACY.read_bytes())==b.LEGACY_SHA
old.RELEASES=fixture/'releases';old.RELEASES.mkdir(exist_ok=True)
old.CURRENT=fixture/'current';old.CACHE=fixture/'cache';old.CACHE.mkdir(exist_ok=True)
old.GREENFIELD_JOURNAL=fixture/'bootstrap.json';b.atomic(old.GREENFIELD_JOURNAL,{'status':'complete'})
root=fixture/'root';root.mkdir(mode=0o700,exist_ok=True)
boundary=b.Boundary(old,root)
record=json.loads((base/'b-evidence.json').read_text());state=record['boundary']
policy={'public_key':r.KEY}
app={'cluster_id':'bezeq-pod-test','topology':'pod','pod_role':'b','updater_instance_id':r.UPDATER}
req=urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token',headers={'Metadata-Flavor':'Google'})
token=json.load(urllib.request.urlopen(req,timeout=10))['access_token']
for kind in ('candidate','previous'):
 ref=state[kind];artifact=old.CACHE/('siemcore-'+ref['version']+'.artifact')
 if not artifact.exists():
  cached=Path('/var/lib/siemcore-cascade-updater/cache')/artifact.name
  # Locate previous qualification's controlled cache without trusting bytes.
  matches=list(Path('/var/lib').glob('**/'+artifact.name))
  source=next((p for p in matches if p!=artifact and r.digest(p.read_bytes())==ref['sha256']),None)
  if source:shutil.copyfile(source,artifact)
  else:
   url='https://storage.googleapis.com/osherad-graylog-boundary-qual-20260916/siemcore-universal-'+ref['version']+'.tar.gz'
   with urllib.request.urlopen(urllib.request.Request(url,headers={'Authorization':'Bearer '+token}),timeout=120) as src,artifact.open('wb') as dst:shutil.copyfileobj(src,dst)
 assert r.digest(artifact.read_bytes())==ref['sha256']
 release=old.RELEASES/ref['version'];release.mkdir(exist_ok=True)
 b.atomic(release/'.updater-release.json',dict(ref,product='siemcore'))
 target=root/ref['archive'];target.parent.mkdir(mode=0o700,parents=True,exist_ok=True);shutil.copyfile(artifact,target);target.chmod(0o400)
 with boundary.bundle(ref,r.KEY):pass
if not old.CURRENT.exists():old.CURRENT.symlink_to(old.RELEASES/r.PREVIOUS[0])
(old.CURRENT.parent/'.previous').write_text(str(old.RELEASES/r.PREVIOUS[0]))
b.atomic(boundary.journal,state)
# Replayed historical requests use the fixture's verified path; log bytes remain exact.
for entry in record['phase_logs']:
 unit=entry['unit'];action=entry['phase'];bundle=str(root)+'/verified-fixture/unpacked/siemcore-universal-3.3.152.33'
 env={'PATH':'/usr/sbin:/usr/bin:/sbin:/bin','HOME':'/root','LANG':'C.UTF-8','UPDATER_PHASE':action,
      'PRODUCT':'siemcore','VERSION':r.CANDIDATE[0],'SIEMCORE_POD_RECOVERY_PROTOCOL':'1','CURRENT_DIR':bundle,
      'INSTALL_ROOT':str(old.CURRENT.parents[1]),'SIEMCORE_INSTALL_DIR':'/opt/siemcore-app-app-b'}
 b.atomic(root/(unit+'.request.json'),{'unit':unit,'args':[bundle+'/updater/'+entry['entrypoint'],action],'env':env})
 for suffix,key in [('.stdout','stdout_tail'),('.stdout.stderr','stderr_tail')]:
  p=root/(unit+suffix);p.write_text(entry[key]);p.chmod(0o600)
checks=['actual-ed25519-checksum-both-retained-artifacts','exact-historical-log-byte-pins']
original=boundary.journal.read_bytes()
try:r.reconcile(boundary,policy,app,Path('/etc/machine-id').read_text().strip())
except ValueError:pass
else:raise AssertionError('real disposable identity accepted as B')
assert boundary.journal.read_bytes()==original;checks.append('wrong-real-machine-refused')
# Native unit and cgroup probes, including a separate-session child.
unit='siemcore-pod-boundary-'+uuid.uuid4().hex+'.service'
subprocess.run(['systemd-run','--unit='+unit,'--property=KillMode=control-group','--','/usr/bin/python3','-c','import subprocess,time; subprocess.Popen(["sleep","120"],start_new_session=True); time.sleep(120)'],check=True,capture_output=True)
try:
 try:r.quiescent(unit)
 except ValueError:pass
 else:raise AssertionError('active unit accepted')
finally:subprocess.run(['systemctl','stop',unit],check=True,capture_output=True)
r.quiescent(unit);checks.append('native-active-unit-refused-and-stopped-cgroup-empty')
# Never change the real machine id. Direct API receives the fixture incident identity.
result=r.reconcile(boundary,policy,app,r.MACHINE)
assert result['status']=='preflight-refused' and not result['rollback_dispatched'] and not result['health_verified']
assert json.loads((root/(r.TX+'.preflight-evidence.json')).read_text())['original_journal']==json.loads(original)
assert not r.PRODUCT_STATE.exists();checks.append('exact-incident-replay-terminal-and-preserved-evidence')
# Installer native filesystem test with fixture destinations, existing real .1 hash.
wrapper=fixture/'wrapper';wrapper.write_bytes(installer.OLD_WRAPPER);wrapper.chmod(0o755)
# Actual machine mismatch already tested; substitute fixture expected identity for restart validation only.
with patch.object(b,'INSTALLED',fixture/'installed/boundary.py'),patch.object(installer,'WRAPPER',wrapper),patch.object(r,'MACHINE',Path('/etc/machine-id').read_text().strip()):
 installer.activate(base,boundary,policy,app,r.MACHINE)
 installer.activate(base,boundary,policy,app,r.MACHINE)
assert wrapper.read_bytes()==installer.NEW_WRAPPER;checks.append('native-versioned-install-and-repeat')
subprocess.run(['/usr/bin/python3','-m','unittest','discover','-s',str(base),'-q'],check=True)
result={'status':'passed','checks':checks,'scope':'real signed archives and native systemd/filesystem; historical incident replay, fixture identities/paths; no product apply, DB, activation or host B mutation'}
b.atomic(result_path,result)
with open('/dev/ttyS0','w') as serial:serial.write('UPDATES_BOUNDARY_2_NATIVE '+json.dumps(result)+'\n')
print(json.dumps(result))
