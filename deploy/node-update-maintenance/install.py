#!/usr/bin/env python3
"""Install an outer-signature-verified node update component, never product code.
Updater binary must already have arrived through normal signed self-update.
"""
import fcntl,hashlib,json,os,re,shutil,subprocess,sys,tempfile
from pathlib import Path
KIT=Path(__file__).resolve().parent
sys.path.insert(0,str(KIT/'component'))
from host import protected,private_json,Host,APPLICATION,BOOTSTRAP_JOURNAL
from adapter import atomic_json
from protocol import PROTOCOL
SERVICE='siemcore-cascade-updater'
ROOT=Path('/var/lib/siemcore-node-update')
CONFIG=Path('/etc/siemcore-cascade-updater/config.yaml')
POLICY=CONFIG.parent/'node-update-policy.json'
WRAPPER=Path('/usr/local/sbin/siemcore-node-update')
SUDO=Path('/etc/sudoers.d/siemcore-node-update')

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def run(args):return subprocess.check_output(args,text=True,stderr=subprocess.PIPE,timeout=60).strip()
def put(path,raw,mode,uid=0,gid=0):
 fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.node-maintenance-')
 try:
  with os.fdopen(fd,'wb') as stream:
   os.fchmod(stream.fileno(),mode);os.fchown(stream.fileno(),uid,gid);stream.write(raw);stream.flush();os.fsync(stream.fileno())
  os.replace(tmp,path)
  fd=os.open(path.parent,os.O_RDONLY)
  try:os.fsync(fd)
  finally:os.close(fd)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)

def main():
 if os.geteuid()!=0 or sys.platform!='linux' or len(sys.argv)!=1:raise ValueError('root Linux required')
 package=private_json(KIT/'PACKAGE.json')
 expected={'install.py'}|{'component/'+name for name in ('adapter.py','cli.py','host.py','protocol.py','supervisor.py','worker.py','COMPONENT.json')}
 if set(package['files'])!=expected:raise ValueError('unexpected package files')
 for name,digest in package['files'].items():
  if sha(protected(KIT/name))!=digest:raise ValueError('package integrity mismatch')
 app=private_json(APPLICATION);journal=private_json(BOOTSTRAP_JOURNAL)
 if app.get('schema')!=5 or app.get('topology')!='node-unlinked' or journal.get('status')!='complete':raise ValueError('completed independent node required')
 if {k:app[k] for k in ('machine_id','installation_id','updater_instance_id','node_id')}!=package['identity'] or app['machine_id']!=Path('/etc/machine-id').read_text().strip():raise ValueError('wrong installation')
 installed=run(['/usr/local/bin/siemcore-cascade-updater','version']).splitlines()[0].split()[1]
 if tuple(map(int,installed.split('.')))<tuple(map(int,package['minimum_updater_version'].split('.'))):raise ValueError('normal signed updater self-update required first')
 if sha(protected(Path('/usr/local/lib/siemcore-cascade/greenfield-hook.py')))!=package['original_hook_sha256']:raise ValueError('unexpected predecessor hook')
 original=protected(CONFIG).read_bytes();config_stat=CONFIG.stat();config_mode=config_stat.st_mode&0o777
 text=original.decode()
 if 'independent_node_update:' in text:raise ValueError('already configured; retained component must be reviewed')
 if len(re.findall(r'^  filesystem:\s*$',text,re.M))!=1:raise ValueError('exact executor config required')
 if not re.search(r'^    server_type: [\"\']?pod-node[\"\']?\s*$',text,re.M):raise ValueError('node role required')
 if not re.search(r'(?ms)^self_update:.*?^  channel: stable\s*$',text):raise ValueError('standard self channel required')
 channels=re.findall(r'^    channel: (\S+)',text,re.M)
 if channels!=[package['product_channel']]:raise ValueError('unexpected product channel')
 for path in (POLICY,WRAPPER,SUDO):
  if path.exists() or path.is_symlink():raise ValueError('existing component refused')
 ROOT.mkdir(mode=0o700,exist_ok=True);protected(ROOT)
 if ROOT.stat().st_mode&0o077 or (ROOT/'active-operation.json').exists():raise ValueError('retained operation exists')
 host=Host(ROOT);preserved=host.preservation(None)
 destination=Path('/usr/local/libexec/siemcore-node-update')/package['version']
 if destination.exists():raise ValueError('component version already exists')
 updated=re.sub(r'(^  filesystem:\s*$)',r'\1\n    independent_node_update: true',text,count=1,flags=re.M).encode()
 was_active=run(['systemctl','is-active',SERVICE])=='active'
 run(['systemctl','stop',SERVICE])
 try:
  fd=os.open('/var/lib/siemcore-greenfield/hook.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
  with os.fdopen(fd,'a') as lock:
   fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
   if not host.preservation(preserved):raise ValueError('installation changed')
   destination.parent.mkdir(mode=0o755,parents=True,exist_ok=True);protected(destination.parent)
   shutil.copytree(KIT/'component',destination);destination.chmod(0o700)
   for path in destination.iterdir():path.chmod(0o600)
   put(WRAPPER,('#!/bin/sh\nexec /usr/bin/python3 -I '+str(destination/'cli.py')+' "$@"\n').encode(),0o755)
   put(SUDO,(SERVICE+' ALL=(root) NOPASSWD: '+', '.join(str(WRAPPER)+' '+a for a in ('readiness','apply','status','recover'))+'\n').encode(),0o440)
   run(['/usr/sbin/visudo','-cf',str(SUDO)])
   binding={k:app[k] for k in ('installation_id','updater_instance_id','node_id')}
   health=host.probe(binding,'predecessor')
   if health['version']!=package['allowed_transition']['predecessor']['version']:raise ValueError('wrong predecessor')
   atomic_json(POLICY,dict(protocol=PROTOCOL,enabled=True,component_manifest_sha256=sha(destination/'COMPONENT.json'),bootstrap_policy_sha256=journal['policy_sha256'],bootstrap_binary_sha256=health['binary_sha256'],product_channel=channels[0],allowed_transition=package['allowed_transition']))
  ready=subprocess.run([str(WRAPPER),'readiness'],input=json.dumps(dict(protocol=PROTOCOL)),text=True,capture_output=True,timeout=40)
  if ready.returncode or json.loads(ready.stdout).get('capabilities')!=[PROTOCOL]:raise ValueError('installed readiness failed')
  if not host.preservation(preserved):raise ValueError('product changed during maintenance')
  put(CONFIG,updated,config_mode,config_stat.st_uid,config_stat.st_gid)
  atomic_json(ROOT/'component-install.json',dict(version=package['version'],installed_updater=installed,product_execution=False))
 except BaseException:
  if (ROOT/'active-operation.json').exists():raise RuntimeError('operation exists; retain component')
  put(CONFIG,original,config_mode,config_stat.st_uid,config_stat.st_gid)
  for path in (POLICY,WRAPPER,SUDO):path.unlink(missing_ok=True)
  if destination.exists():shutil.rmtree(destination)
  raise
 finally:
  if was_active:run(['systemctl','start',SERVICE])
 print(json.dumps(dict(component=package['version'],updater=installed,product_execution=False)))

if __name__=='__main__':main()
