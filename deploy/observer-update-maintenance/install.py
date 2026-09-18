#!/usr/bin/env python3
"""Root updater maintenance only. Verify signed outer kit BEFORE execution.

Never calls a product installer, bootstrap hook or product service mutation.
The existing original bootstrap policy/journal and application bytes are retained.
"""
import base64,fcntl,hashlib,json,os,pathlib,pwd,re,shutil,subprocess,sys,tempfile
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
P=pathlib.Path
KIT=P(__file__).resolve().parent
sys.path.insert(0,str(KIT/'component'))
from host import protected,private_json,Host,APPLICATION,BOOTSTRAP_JOURNAL
from adapter import atomic_json
from protocol import PROTOCOL
SERVICE='siemcore-cascade-updater'
CONFIG=P('/etc/'+SERVICE+'/config.yaml')
LAYOUT=P('/var/lib/'+SERVICE+'/self-update')
ROOT=P('/var/lib/siemcore-observer-update')
POLICY=P('/etc/'+SERVICE+'/observer-update-policy.json')
WRAPPER=P('/usr/local/sbin/siemcore-observer-update')
SUDO=P('/etc/sudoers.d/siemcore-observer-update')

def run(args):return subprocess.check_output(args,stderr=subprocess.PIPE,text=True,timeout=40).strip()
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def put(path,raw,mode=0o600,uid=0,gid=0):
 fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.observer-maintenance-')
 try:
  with os.fdopen(fd,'wb') as f:
   os.fchmod(f.fileno(),mode);os.fchown(f.fileno(),uid,gid);f.write(raw);f.flush();os.fsync(f.fileno())
  os.replace(tmp,path);d=os.open(path.parent,os.O_RDONLY);os.fsync(d);os.close(d)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)
def snapshot(path):
 if not path.exists():
  if path.is_symlink():raise ValueError('dangling protected target')
  return None
 protected(path);st=path.stat();return (path.read_bytes(),st.st_mode&0o777,st.st_uid,st.st_gid)
def restore(path,old):
 if old is None:path.unlink(missing_ok=True)
 else:put(path,*old)
def main():
 if os.geteuid()!=0 or sys.platform!='linux' or len(sys.argv)!=1:raise ValueError('root Linux, no arguments required')
 protected(KIT)
 package=private_json(KIT/'PACKAGE.json');version=package['updater_version']
 if not re.fullmatch(r'\d+\.\d+\.\d+\.\d+',version):raise ValueError('invalid package version')
 for name,digest in package['files'].items():
  if P(name).is_absolute() or '..' in P(name).parts or sha(protected(KIT/name))!=digest:raise ValueError('package integrity mismatch')
 app=private_json(APPLICATION);journal=private_json(BOOTSTRAP_JOURNAL)
 if app.get('schema')!=4 or app.get('topology')!='observer-unlinked' or journal.get('status')!='complete':raise ValueError('completed independent Observer bootstrap required')
 if app['machine_id']!=P('/etc/machine-id').read_text().strip():raise ValueError('machine binding mismatch')
 link=P('/etc/siemcore-pod-observer/link.json')
 if link.exists() or link.is_symlink():raise ValueError('linked Observer refused')
 bootstrap=private_json(P('/etc/siemcore/greenfield-release.json'))
 policy_sha=hashlib.sha256(json.dumps(app,sort_keys=True).encode()).hexdigest()
 if journal['policy_sha256']!=policy_sha:raise ValueError('bootstrap identity mismatch')
 binary=KIT/'updater-linux-amd64';receipt=package['updater_receipt']
 if receipt['version']!=version or sha(binary)!=receipt['sha256']:raise ValueError('updater checksum mismatch')
 Ed25519PublicKey.from_public_bytes(bytes.fromhex(bootstrap['public_key'])).verify(base64.b64decode(receipt['signature'],validate=True),('mysoc-release-v1\nupdater-linux-amd64\n'+version+'\n'+receipt['sha256']).encode())
 if run([str(binary),'version']).split()[1]!=version:raise ValueError('updater version mismatch')
 oldlink=os.readlink(LAYOUT/'current');oldbinary=(LAYOUT/'current'/SERVICE)
 if sha(oldbinary)!=package['allowed_predecessor_updater_sha256'] or sha(protected(P('/usr/local/sbin/siemcore-apply-update')))!=package['allowed_bootstrap_hook_sha256']:raise ValueError('unknown predecessor components')
 config=snapshot(CONFIG);text=config[0].decode()
 if 'observer_unlinked_update:' in text:raise ValueError('already configured; explicit subsequent maintenance required')
 if not re.search(r'^    server_type: [\"\']?observer-unlinked[\"\']?\s*$',text,re.M):raise ValueError('protected Observer identity required')
 channel=re.findall(r'^    channel: (\S+)',text,re.M)
 if len(channel)!=1 or not re.search(r'(?ms)^self_update:.*?^  channel: stable\s*$',text):raise ValueError('preserve standard self-update and explicit product channel')
 if len(re.findall(r'^  filesystem:\s*$',text,re.M))!=1:raise ValueError('exact filesystem executor required')
 updated=re.sub(r'(^  filesystem:\s*$)',r'\1\n    observer_unlinked_update: true',text,count=1,flags=re.M).encode()
 if ROOT.exists():
  protected(ROOT)
  if (ROOT/'active-operation.json').exists():raise ValueError('retained product transaction')
 else:ROOT.mkdir(mode=0o700)
 if ROOT.stat().st_mode&0o077:raise ValueError('private operation root required')
 maintenance=ROOT/'maintenance.json'
 if maintenance.exists():raise ValueError('maintenance receipt exists; do not overwrite')
 host=Host(ROOT);preserved=host.preservation(None)
 initial_pid=run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID','--value'])
 oldpaths={p:snapshot(p) for p in (POLICY,WRAPPER,SUDO)}
 destination=P('/usr/local/libexec/siemcore-observer-update')/version
 if destination.exists():raise ValueError('immutable component version exists')
 was_active=run(['systemctl','is-active',SERVICE])=='active'
 run(['systemctl','stop',SERVICE])
 activated=False
 try:
  with open('/var/lib/siemcore-greenfield/hook.lock','a') as lock:
   fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
   if (ROOT/'active-operation.json').exists():raise ValueError('product operation raced maintenance')
   if not host.preservation(preserved):raise ValueError('installation changed')
   atomic_json(maintenance,dict(phase='preparing',updater_version=version,previous_updater=oldlink,bootstrap_policy_sha256=policy_sha))
   destination.parent.mkdir(mode=0o755,parents=True,exist_ok=True);protected(destination.parent)
   shutil.copytree(KIT/'component',destination);destination.chmod(0o700)
   for p in destination.iterdir():p.chmod(0o600)
   protected(destination)
   wrapper=('#!/bin/sh\nexec /usr/bin/python3 -I '+str(destination/'cli.py')+' "$@"\n').encode()
   put(WRAPPER,wrapper,0o755)
   put(SUDO,(SERVICE+' ALL=(root) NOPASSWD: '+', '.join(str(WRAPPER)+' '+a for a in ('readiness','apply','status','recover'))+'\n').encode(),0o440)
   run(['/usr/sbin/visudo','-cf',str(SUDO)])
   binding=dict(installation_id=app['installation_id'],updater_instance_id=app['updater_instance_id'])
   health=host.probe(binding,'predecessor')
   if health['version']!=bootstrap['version']:raise ValueError('unexpected installed predecessor')
   atomic_json(POLICY,dict(protocol=PROTOCOL,enabled=True,component_manifest_sha256=sha(destination/'COMPONENT.json'),bootstrap_policy_sha256=policy_sha,bootstrap_binary_sha256=health['binary_sha256'],product_channel=channel[0]))
   # CLI readiness acquires the same lock; release only after all root files are
   # complete, while the updater remains stopped. No product operation is called.
  ready=subprocess.run([str(WRAPPER),'readiness'],input=json.dumps(dict(protocol=PROTOCOL)),text=True,capture_output=True,timeout=40)
  if ready.returncode or json.loads(ready.stdout).get('eligible_for_security_upgrade') is not True:raise ValueError('installed readiness failed')
  if not host.preservation(preserved) or run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID','--value'])!=initial_pid:raise ValueError('product changed during maintenance')
  account=pwd.getpwnam(SERVICE);release=LAYOUT/'releases'/version
  if release.exists():raise ValueError('immutable updater version exists')
  release.mkdir(mode=0o755);os.chown(release,account.pw_uid,account.pw_gid)
  put(release/SERVICE,binary.read_bytes(),0o755,account.pw_uid,account.pw_gid)
  put(CONFIG,updated,config[1],config[2],config[3])
  tmp=LAYOUT/'maintenance-next';tmp.symlink_to(release);os.replace(tmp,LAYOUT/'current')
  atomic_json(maintenance,dict(phase='ready',updater_version=version,previous_updater=oldlink,bootstrap_policy_sha256=policy_sha,component_manifest_sha256=sha(destination/'COMPONENT.json')))
  activated=True
 except BaseException:
  if (ROOT/'active-operation.json').exists():raise RuntimeError('operation started; retain component for reconciliation')
  put(CONFIG,*config)
  for p,old in oldpaths.items():restore(p,old)
  tmp=LAYOUT/'maintenance-restore';tmp.unlink(missing_ok=True);tmp.symlink_to(oldlink);os.replace(tmp,LAYOUT/'current')
  if destination.exists():shutil.rmtree(destination)
  atomic_json(maintenance,dict(phase='rolled_back',updater_version=version))
  raise
 finally:
  if was_active:run(['systemctl','start',SERVICE])
 print(json.dumps(dict(updater_version=version,component_installed=activated,product_execution=False,product_channel=channel[0],self_update_channel='stable')))
if __name__=='__main__':
 try:main()
 except Exception as error:
  print('Observer maintenance refused or rolled back: '+str(error),file=sys.stderr);sys.exit(1)
