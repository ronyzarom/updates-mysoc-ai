#!/usr/bin/env python3
"""Updates signed staged-recovery runner. Local candidate; never activates a pod."""
import contextlib
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import stat
import subprocess
import sys
import tempfile
import uuid

ROOT=Path('/var/lib/updates-pod-recovery')
INSTALLED=Path('/usr/local/lib/updates-pod-recovery/1.0.0.1/runner.py')
BOUNDARY=Path('/usr/local/lib/siemcore-pod-boundary/1.0.0.3/boundary.py')
BOUNDARY_SHA='c6b83932733cb40b124a32502ea2d76d1d9d3ccfd416ee2c6c3a617891b4819d'
LEGACY=Path('/usr/local/lib/siemcore-cascade/greenfield-hook.py')
LEGACY_SHA='221fe046129211fcd8b421e495134e5bf5054b9934c74f0029a2e49667b95a88'
KEY='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
INTENT=Path('/etc/siemcore-pod-recovery/intent.json')
RELEASE=Path('/etc/siemcore-pod-recovery/release.json')
UNIT=re.compile(r'^updates-pod-recovery-[0-9a-f]{32}\.service$')
ID=re.compile(r'^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$')
VERSION=re.compile(r'^\d+\.\d+\.\d+\.\d+$')
SHA=re.compile(r'^[a-f0-9]{64}$')
PREFIX=b'MYSOC_RECOVERY_RESULT_V1:'
INTENT_FIELDS={'schema','product','operation_id','pod_id','node_id','machine_id','version','artifact_sha256','maintenance','expected_owner','activation_allowed'}


def sha(raw):return hashlib.sha256(raw).hexdigest()

def trusted(path, private=False):
 for p in (path,*path.parents):
  st=p.lstat()
  if stat.S_ISLNK(st.st_mode) or st.st_uid!=0 or st.st_mode&0o022:raise ValueError('unprotected recovery path')
 if private and path.stat().st_mode&0o077:raise ValueError('private recovery input required')

def read(path, limit=65536, private=True):
 trusted(path,private)
 if not stat.S_ISREG(path.stat().st_mode):raise ValueError('regular recovery input required')
 with path.open('rb') as source:raw=source.read(limit+1)
 if len(raw)>limit:raise ValueError('recovery input exceeds limit')
 return raw

def parse(raw):
 def pairs(items):
  value={}
  for key,item in items:
   if key in value:raise ValueError('duplicate JSON field')
   value[key]=item
  return value
 def constant(value):raise ValueError('non-finite JSON number')
 return json.loads(raw,object_pairs_hook=pairs,parse_constant=constant)

def atomic(path,value):
 trusted(path.parent,True)
 if os.path.lexists(path):read(path)
 fd,name=tempfile.mkstemp(dir=path.parent)
 try:
  with os.fdopen(fd,'wb') as out:
   os.fchmod(out.fileno(),0o600);out.write((json.dumps(value,sort_keys=True)+'\n').encode());out.flush();os.fsync(out.fileno())
  os.replace(name,path)
  fd=os.open(path.parent,os.O_DIRECTORY)
  try:os.fsync(fd)
  finally:os.close(fd)
 finally:
  if os.path.exists(name):os.unlink(name)

def validate_intent(value,machine,policy):
 if not isinstance(value,dict) or set(value)!=INTENT_FIELDS:raise ValueError('unsupported intent fields')
 if type(value['schema']) is not int or value['schema']!=1 or value['product']!='siemcore':raise ValueError('unsupported intent schema/product')
 for key in ('operation_id','pod_id','machine_id'):
  if not isinstance(value[key],str) or not ID.fullmatch(value[key]):raise ValueError('invalid identity')
 if not isinstance(value['version'],str) or not VERSION.fullmatch(value['version']):raise ValueError('invalid version')
 if not isinstance(value['artifact_sha256'],str) or not SHA.fullmatch(value['artifact_sha256']):raise ValueError('invalid digest')
 marker=value['maintenance']
 if not isinstance(marker,dict) or set(marker)!={'operation_id','generation'} or not isinstance(marker['operation_id'],str) or not ID.fullmatch(marker['operation_id']) or type(marker['generation']) is not int or marker['generation']<=0:raise ValueError('invalid maintenance identity')
 if value['activation_allowed'] is not False or value['expected_owner']!='':raise ValueError('activation forbidden')
 if value['node_id'] not in ('a','b') or value['machine_id']!=machine or policy.get('machine_id')!=machine or policy.get('topology')!='pod' or policy.get('pod_role')!=value['node_id'] or policy.get('cluster_id')!=value['pod_id']:raise ValueError('host/pod identity mismatch')

def validate_release(ref,intent):
 if set(ref)!={'product','version','sha256','signature','public_key','source_commit','architecture','artifact'}:raise ValueError('unsupported release receipt')
 if ref['product']!='siemcore' or ref['version']!=intent['version'] or ref['sha256']!=intent['artifact_sha256'] or ref['public_key']!=KEY:raise ValueError('release identity mismatch')
 if not re.fullmatch(r'[a-f0-9]{40}',ref['source_commit']) or ref['architecture'] not in ('amd64','arm64'):raise ValueError('release provenance missing')
 host={'x86_64':'amd64','aarch64':'arm64','arm64':'arm64'}.get(platform.machine())
 if host!=ref['architecture']:raise ValueError('host architecture mismatch')
 if not isinstance(ref['signature'],str) or not ref['signature']:raise ValueError('signature missing')


def validate_receipt(raw,intent,runtime_sha):
 if len(raw)>8192 or len(raw.splitlines())!=1 or not raw.startswith(PREFIX):raise ValueError('exact bounded recovery receipt required')
 value=parse(raw[len(PREFIX):])
 expected=dict(intent,status='staged-paused',runtime_sha256=runtime_sha,prerequisites_verified=True,unchanged_state_verified=True,application_health_verified=False)
 if not isinstance(value,dict) or set(value)!=set(expected):raise ValueError('receipt fields mismatch')
 for key,item in expected.items():
  if value[key]!=item or type(value[key]) is not type(item):raise ValueError('receipt identity/result mismatch')
 if type(value['maintenance'].get('generation')) is not int:raise ValueError('receipt generation type mismatch')
 return value


def load_legacy():
 trusted(LEGACY)
 if sha(LEGACY.read_bytes())!=LEGACY_SHA:raise ValueError('legacy verifier drift')
 spec=importlib.util.spec_from_file_location('recovery_legacy',LEGACY);old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)
 return old


class Runner:
 def __init__(self,old,root=None):
  self.old,self.root=old,root if root is not None else ROOT;self.journal=self.root/'transaction.json'
 def save(self,state):atomic(self.journal,state)
 @contextlib.contextmanager
 def bundle(self,state):
  archive=self.root/'retained.tar.gz';trusted(archive,True)
  with tempfile.TemporaryDirectory(prefix='verified-',dir=self.root) as tmp:
   ref=state['release'];bundle=self.old.verified_bundle({'artifact':str(archive),'sha256':ref['sha256'],'signature':ref['signature']},ref['version'],KEY,Path(tmp))
   manifest=parse((bundle/'MANIFEST.json').read_bytes())
   commit=manifest.get('build',{}).get('git_commit','')
   if manifest.get('product')!='siemcore' or manifest.get('version')!=ref['version'] or manifest.get('architecture')!=ref['architecture'] or not re.fullmatch(r'[a-f0-9]{40}',commit) or ref['source_commit']!=commit:raise ValueError('signed manifest provenance mismatch')
   entry=bundle/'updater/recover';binary=bundle/'pod/bin/siemcore'
   if not entry.is_file() or entry.is_symlink() or not os.access(entry,os.X_OK) or not binary.is_file() or binary.is_symlink():raise ValueError('signed recovery/runtime required')
   yield bundle,sha(binary.read_bytes())
 def prepare(self,intent,ref):
  if self.journal.exists():
   state=parse(read(self.journal))
   if state['intent']!=intent or state['release']!=ref:raise ValueError('unfinished/different recovery operation')
   return state
  artifact=Path(ref['artifact'])
  if not artifact.is_absolute():raise ValueError('absolute protected artifact required')
  trusted(artifact,True)
  if not stat.S_ISREG(artifact.stat().st_mode):raise ValueError('regular artifact required')
  target=self.root/'retained.tar.gz'
  if target.exists():
   trusted(target,True)
   if sha(target.read_bytes())!=ref['sha256']:raise ValueError('retained artifact conflict')
  else:
   with artifact.open('rb') as src,target.open('xb') as dst:
    os.fchmod(dst.fileno(),0o400)
    import shutil
    shutil.copyfileobj(src,dst);dst.flush();os.fsync(dst.fileno())
  state={'schema':1,'intent':intent,'release':ref,'phase':'prepared','runtime_sha256':None}
  with self.bundle(state) as (_,runtime):state['runtime_sha256']=runtime
  atomic(self.root/'intent.json',intent);self.save(state);return state
 def drain(self,state):
  unit=state.get('pending_unit') or state.get('draining_unit')
  if not unit:return
  if not UNIT.fullmatch(unit):raise ValueError('invalid pending recovery unit')
  state.pop('pending_unit',None);state['draining_unit']=unit;state['phase']='interrupted';self.save(state)
  try:quiesce(unit)
  except Exception:
   state['drain_failure']='quiescence_unconfirmed';self.save(state);raise
  state.pop('draining_unit');self.save(state)
 def run(self,state,phase):
  self.drain(state)
  with self.bundle(state) as (_,runtime):
   if runtime!=state['runtime_sha256']:raise ValueError('retained runtime mismatch')
  unit='updates-pod-recovery-'+uuid.uuid4().hex+'.service'
  state.update(phase=phase,pending_unit=unit);self.save(state)
  output=self.root/(unit+'.stdout')
  command=['/usr/bin/systemd-run','--quiet','--wait','--unit='+unit,'--property=Type=exec','--property=KillMode=control-group','--property=Restart=no','--property=SendSIGKILL=yes','--property=TimeoutStopSec=10s','--property=RuntimeMaxSec='+('600s' if phase=='stage' else '120s'),'--property=StandardOutput=file:'+str(output),'--property=StandardError=file:'+str(output)+'.stderr','--','/usr/bin/python3',str(INSTALLED),'_worker',unit]
  try:
   result=subprocess.run(command,capture_output=True,timeout=630 if phase=='stage' else 150)
   quiesce(unit)
   if result.returncode:raise RuntimeError('signed recovery phase failed')
   receipt=validate_receipt(read(output,8192,private=False),state['intent'],state['runtime_sha256'])
  except Exception:
   # Revoke even delayed workers before attempting drainage; retain pending
   # reference if process termination cannot be proved.
   state['failure']={'phase':phase,'reason_code':'recovery_phase_failed'};self.save(state)
   self.drain(state)
   state['phase']='failed';state['reason_code']='recovery_phase_failed';self.save(state)
   raise
  state.pop('pending_unit');state['phase']='stage-returned' if phase=='stage' else 'recovery-staged'
  state['receipt']=receipt;self.save(state)
  return receipt


def quiesce(unit):
 if not UNIT.fullmatch(unit):raise ValueError('invalid recovery unit')
 subprocess.run(['/usr/bin/systemctl','stop',unit],capture_output=True,timeout=25)
 result=subprocess.run(['/usr/bin/systemctl','show',unit,'-p','LoadState','-p','ActiveState','-p','ControlGroup'],capture_output=True,text=True,timeout=10)
 fields=dict(line.split('=',1) for line in result.stdout.splitlines() if '=' in line)
 if fields.get('LoadState')!='not-found' and fields.get('ActiveState') not in ('inactive','failed'):raise RuntimeError('recovery unit not quiescent')
 group=fields.get('ControlGroup') or '/system.slice/'+unit
 if group!='/system.slice/'+unit:raise ValueError('unexpected recovery cgroup')
 path=Path('/sys/fs/cgroup'+group)
 if path.exists() and 'populated 0' not in (path/'cgroup.events').read_text().splitlines():raise RuntimeError('recovery descendants remain')


@contextlib.contextmanager
def execution_locks(worker=False):
 paths=[Path('/var/lib/siemcore-pod-boundary/work.lock')] if worker else [Path('/var/lib/siemcore-cascade-updater/state.json.cycle-lock'),Path('/var/lib/siemcore-pod-boundary/boundary.lock'),ROOT/'runner.lock']
 with contextlib.ExitStack() as stack:
  for path in paths:
   if path.name!='state.json.cycle-lock':
    trusted(path.parent)
    if os.path.lexists(path):trusted(path)
   fd=os.open(path,os.O_RDWR|os.O_NOFOLLOW|(os.O_CREAT if path==ROOT/'runner.lock' else 0),0o600)
   stream=stack.enter_context(os.fdopen(fd,'r+'))
   if not stat.S_ISREG(os.fstat(fd).st_mode):raise ValueError('invalid lock file')
   fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
   if os.fstat(fd).st_ino!=path.stat(follow_symlinks=False).st_ino:raise ValueError('lock replaced')
  yield


def worker(unit):
 if os.geteuid()!=0 or not UNIT.fullmatch(unit):raise ValueError('invalid recovery worker')
 trusted(ROOT,True)
 with execution_locks(worker=True):
  runner=Runner(load_legacy());state=parse(read(runner.journal))
  if state.get('pending_unit')!=unit or state['phase'] not in ('stage','verify-staged'):raise ValueError('superseded recovery worker')
  intent=parse(read(ROOT/'intent.json'))
  if intent!=state['intent']:raise ValueError('protected intent changed')
  validate_intent(intent,Path('/etc/machine-id').read_text().strip(),runner.old.protected(Path('/etc/siemcore/greenfield.json')))
  validate_release(state['release'],intent)
  with runner.bundle(state) as (bundle,runtime):
   if runtime!=state['runtime_sha256']:raise ValueError('runtime changed')
   env={'PATH':'/usr/sbin:/usr/bin:/sbin:/bin','HOME':'/root','LANG':'C.UTF-8','PRODUCT':'siemcore','VERSION':intent['version'],'CURRENT_DIR':str(bundle),'VERIFIED_ARTIFACT_SHA256':intent['artifact_sha256'],'RECOVERY_INTENT':str(ROOT/'intent.json')}
   subprocess.run([str(bundle/'updater/recover'),state['phase']],env=env,check=True)


def main():
 if os.geteuid()!=0 or len(sys.argv)!=2 or sys.argv[1] not in ('stage','verify-staged'):raise ValueError('root recovery phase required')
 os.umask(0o077)
 # This guard prevents an ordinary update after an outer runner crash.
 wrapper=Path('/usr/local/sbin/siemcore-apply-update');trusted(wrapper)
 expected='#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 '+str(BOUNDARY)+' "$@"\n'
 if wrapper.read_text()!=expected:raise ValueError('guarded normal boundary required')
 trusted(BOUNDARY)
 if sha(BOUNDARY.read_bytes())!=BOUNDARY_SHA:raise ValueError('normal boundary drift')
 ROOT.mkdir(mode=0o700,exist_ok=True);trusted(ROOT,True)
 with execution_locks():
  normal=parse(read(Path('/var/lib/siemcore-pod-boundary/transaction.json')))
  if normal.get('phase') not in ('healthy','rolled-back','preflight-refused') or normal.get('pending_unit') or normal.get('draining_unit'):raise ValueError('normal update transaction unresolved')
  old=load_legacy();intent=parse(read(INTENT));ref=parse(read(RELEASE))
  validate_intent(intent,Path('/etc/machine-id').read_text().strip(),old.protected(Path('/etc/siemcore/greenfield.json')));validate_release(ref,intent)
  runner=Runner(old);state=runner.prepare(intent,ref)
  if sys.argv[1]=='stage':runner.run(state,'stage')
  receipt=runner.run(state,'verify-staged')
  print(json.dumps({'status':'recovery-staged','application_health_verified':False,'receipt':receipt},sort_keys=True))

if __name__=='__main__':
 try:
  if len(sys.argv)==3 and sys.argv[1]=='_worker':worker(sys.argv[2])
  else:main()
 except Exception:
  # Private systemd files retain product errors; no raw traceback/credentials in reporting.
  print('Updates staged recovery refused',file=sys.stderr);sys.exit(1)
