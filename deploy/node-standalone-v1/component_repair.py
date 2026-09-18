#!/usr/bin/env python3
"""Signed, exact component revision only; never invokes application workers."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import urllib.request
KIT=Path(__file__).resolve().parent
sys.path.insert(0,str(KIT/'component'))
from source_loader import SourceLoader
from transaction import digest
from adapter import atomic_json
ROOT=Path('/var/lib/siemcore-node-standalone')
POLICY=Path('/etc/siemcore-cascade-updater/node-standalone-policy.json')
WRAPPER=Path('/usr/local/sbin/siemcore-node-standalone')
COMPONENT=Path('/usr/local/libexec/siemcore-node-standalone')

def put(path,raw,mode=0o600):
 import tempfile
 fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.component-')
 try:
  with os.fdopen(fd,'wb') as stream:
   os.fchmod(stream.fileno(),mode);stream.write(raw);stream.flush();os.fsync(stream.fileno())
  os.replace(tmp,path)
  fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
  try:os.fsync(fd)
  finally:os.close(fd)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)

def repair(loader,p):
 policy=json.loads(loader.protected(POLICY).read_text())
 if digest(policy['binding'])!=p['operation_sha256'] or policy['binding']['source']!={k:v for k,v in p['identity'].items() if k!='vm_id'}:raise ValueError('repair_operation_mismatch')
 if hashlib.sha256(POLICY.read_bytes()).hexdigest() not in (p['old_policy_sha256'],p['new_policy_sha256']):raise ValueError('repair_policy_mismatch')
 directory=ROOT/'operations'/policy['binding']['operation_id']
 if directory.exists() and list(directory.iterdir()):raise ValueError('operation_already_started')
 old=COMPONENT/p['old_revision'];new=COMPONENT/p['new_revision']
 raw=loader.protected(old/'COMPONENT.json').read_bytes()
 if hashlib.sha256(raw).hexdigest()!=p['old_component_sha256']:raise ValueError('old_component_mismatch')
 for name,sha in json.loads(raw)['files'].items():
  if hashlib.sha256(loader.protected(old/name).read_bytes()).hexdigest()!=sha:raise ValueError('old_component_file_changed')
 component_raw=loader.protected(KIT/'component/COMPONENT.json').read_bytes()
 if hashlib.sha256(component_raw).hexdigest()!=p['new_component_sha256']:raise ValueError('new_component_mismatch')
 names=set(json.loads(component_raw)['files'])|{'COMPONENT.json'}
 new.mkdir(mode=0o700,exist_ok=True);loader.protected(new)
 if set(x.name for x in new.iterdir())-names:raise ValueError('unexpected_repair_destination')
 for name in names:
  content=loader.protected(KIT/'component'/name).read_bytes();target=new/name
  if target.exists() and loader.protected(target).read_bytes()!=content:raise ValueError('repair_file_conflict')
  if not target.exists():put(target,content)
 old_wrapper=('#!/bin/sh\nexec /usr/bin/python3 -I '+str(old/'cli.py')+' "$@"\n').encode()
 new_wrapper=('#!/bin/sh\nexec /usr/bin/python3 -I -B '+str(new/'cli.py')+' "$@"\n').encode()
 if loader.protected(WRAPPER).read_bytes() not in (old_wrapper,new_wrapper):raise ValueError('repair_wrapper_conflict')
 policy['component_manifest_sha256']=p['new_component_sha256']
 new_policy=(json.dumps(policy,sort_keys=True,indent=2)+'\n').encode()
 # atomic_json's exact format is used for policy provenance.
 if hashlib.sha256(new_policy).hexdigest()!=p['new_policy_sha256']:raise ValueError('new_policy_encoding_mismatch')
 put(POLICY,new_policy);put(WRAPPER,new_wrapper,0o755)
 receipt=dict(protocol='pod-node-standalone-component-repair-v1',kit_version=p['new_revision'],operation_sha256=p['operation_sha256'],component_manifest_sha256=p['new_component_sha256'],product_execution=False)
 atomic_json(ROOT/'component-repair.json',receipt)
 return receipt

def main():
 os.umask(0o077)
 if os.geteuid()!=0 or sys.argv[1:]!=['repair']:raise ValueError('root_repair_required')
 loader=SourceLoader();p=json.loads(loader.protected(KIT/'PACKAGE.json').read_text())
 for name,sha in p['files'].items():
  if '..' in Path(name).parts or Path(name).is_absolute() or hashlib.sha256(loader.protected(KIT/name).read_bytes()).hexdigest()!=sha:raise ValueError('repair_package_integrity')
 a=json.loads(loader.read('/etc/siemcore/greenfield.json'))
 request=urllib.request.Request('http://169.254.169.254/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
 with urllib.request.urlopen(request,timeout=5) as response:vm=response.read(128).decode().strip()
 if vm!=p['identity']['vm_id'] or any(a.get(k)!=v for k,v in p['identity'].items() if k!='vm_id') or loader.read('/etc/machine-id').decode().strip()!=p['identity']['machine_id']:raise ValueError('repair_host_identity')
 service='siemcore-cascade-updater'
 active=subprocess.run(['systemctl','is-active','--quiet',service]).returncode==0
 if active:subprocess.run(['systemctl','stop',service],check=True,timeout=60)
 lock=Path('/var/lib/siemcore-greenfield/hook.lock');loader.protected(lock)
 fd=os.open(lock,os.O_RDWR|os.O_NOFOLLOW)
 try:
  fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
  print(json.dumps(repair(loader,p),sort_keys=True))
 finally:
  os.close(fd)
  if active:subprocess.run(['systemctl','start',service],check=True,timeout=60)
if __name__=='__main__':
 try:main()
 except Exception as error:
  print('Signed component repair refused: '+(str(error) if isinstance(error,ValueError) else type(error).__name__),file=sys.stderr);sys.exit(1)
