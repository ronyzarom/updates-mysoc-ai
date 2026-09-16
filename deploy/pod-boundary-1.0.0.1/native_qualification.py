#!/usr/bin/env python3
"""Disposable GCP/systemd fixture only; no application or database deployment."""
import hashlib,json,os,pathlib,signal,socket,subprocess,sys,time,uuid
import boundary as b
base=pathlib.Path(__file__).resolve().parent
assert os.geteuid()==0
assert socket.gethostname().split('.')[0]=='updater-boundary-qual-20260916'
assert not pathlib.Path('/opt/siemcore-cascade').exists()
# Simulate the pinned preexisting hook on this otherwise-empty disposable VM.
b.LEGACY.parent.mkdir(parents=True,exist_ok=True)
legacy=(base/'legacy-hook.py').read_bytes();assert hashlib.sha256(legacy).hexdigest()==b.LEGACY_SHA
b.LEGACY.write_bytes(legacy);b.LEGACY.chmod(0o644)
p=pathlib.Path('/etc/siemcore');p.mkdir(exist_ok=True)
app=p/'greenfield.json';app.write_text(json.dumps({'topology':'pod','pod_role':'a','updater_instance_id':'boundary-fixture-a'}));app.chmod(0o600)
w=pathlib.Path('/usr/local/sbin/siemcore-apply-update')
w.write_text('#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 /usr/local/lib/siemcore-cascade/greenfield-hook.py "$@"\n');w.chmod(0o755)
# Provisioning precondition: inactive updater; no real updater is installed.
u=pathlib.Path('/etc/systemd/system/siemcore-cascade-updater.service')
u.write_text('[Unit]\nDescription=Qualification fixture only\n[Service]\nExecStart=/usr/bin/sleep infinity\n');subprocess.run(['systemctl','daemon-reload'],check=True)
args=[sys.executable,str(base/'install.py'),'--expected-machine-id',pathlib.Path('/etc/machine-id').read_text().strip(),'--expected-updater-id','boundary-fixture-a']
wrong=args.copy();wrong[3]='wrong-machine'
assert subprocess.run(wrong,capture_output=True).returncode!=0
subprocess.run(['systemctl','start','siemcore-cascade-updater'],check=True)
assert subprocess.run(args,capture_output=True).returncode!=0
subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True)
subprocess.run(args,check=True);subprocess.run(args,check=True)
b.ROOT.mkdir(mode=0o700,exist_ok=True)
runner=b.Units();passed=['installer-wrong-machine-refused','installer-active-updater-refused','installer-first-repeat-pass']
def makeunit():return 'siemcore-pod-boundary-'+uuid.uuid4().hex+'.service'
def setup(unit):b.atomic(b.ROOT/'transaction.json',{'pending_unit':unit})
def alive(pid):
 try:return pathlib.Path('/proc/'+str(pid)+'/stat').read_text().split()[2]!='Z'
 except FileNotFoundError:return False
unit=makeunit();setup(unit)
out=runner.run(unit,['/usr/bin/printf','ok'],{},b.ROOT/(unit+'.stdout'),5)
assert out==b'ok';passed.append('native-systemd-phase-success')
script=b.ROOT/'spawn.py'
script.write_text('import subprocess,pathlib,time,sys\np=subprocess.Popen(["/usr/bin/sleep","300"],start_new_session=True)\npathlib.Path(sys.argv[1]).write_text(str(p.pid))\ntime.sleep(300)\n')
unit=makeunit();setup(unit);pidfile=b.ROOT/'timeout-child.pid'
try:runner.run(unit,['/usr/bin/python3',script,pidfile],{},b.ROOT/(unit+'.stdout'),1)
except RuntimeError:pass
else:raise AssertionError('timeout accepted')
assert pidfile.exists() and not alive(int(pidfile.read_text()));passed.append('timeout-kills-new-session-descendant')
unit=makeunit();pidfile=b.ROOT/'crash-child.pid';setup(unit)
pid=os.fork()
if pid==0:
 try:runner.run(unit,['/usr/bin/python3',script,pidfile],{},b.ROOT/(unit+'.stdout'),60)
 finally:os._exit(0)
end=time.monotonic()+15
while not pidfile.exists() and time.monotonic()<end:time.sleep(.1)
assert pidfile.exists();os.kill(pid,signal.SIGKILL);os.waitpid(pid,0)
b.atomic(b.ROOT/'transaction.json',{'draining_unit':unit})
runner.quiesce(unit)
assert not alive(int(pidfile.read_text()));passed.append('outer-wrapper-kill-recovery-quiesces-descendants')
late=makeunit()
r=subprocess.run(['/usr/bin/python3',str(b.INSTALLED),'_worker',late],capture_output=True)
assert r.returncode!=0;passed.append('late-worker-refused')
subprocess.run([sys.executable,'-m','unittest','discover','-s',str(base),'-p','test_boundary.py'],check=True)
result={'status':'passed','checks':passed,'scope':'native systemd supervision and installer; artifact verifier mocked in boundary unit tests; no real product/database/retained .30 execution'}
pathlib.Path('/var/lib/updater-boundary-qualification-result.json').write_text(json.dumps(result)+'\n')
with open('/dev/ttyS0','w') as serial:serial.write('UPDATES_BOUNDARY_QUALIFICATION '+json.dumps(result)+'\n')
print(json.dumps(result))
