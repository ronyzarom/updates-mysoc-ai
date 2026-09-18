#!/usr/bin/env python3
"""Install signed successor authorization after verified terminal recovery only."""
import fcntl,hashlib,json,os,subprocess,sys,urllib.request
from pathlib import Path
KIT=Path(__file__).resolve().parent
sys.path.insert(0,str(KIT/'component'))
from source_loader import SourceLoader
from transaction import digest,validate_binding
from adapter import atomic_json
from configuration_inventory import configuration_digest
from host import canonical_data_identity
ROOT=Path('/var/lib/siemcore-node-standalone')
POLICY=Path('/etc/siemcore-cascade-updater/node-standalone-policy.json')
INPUTS=Path('/etc/siemcore-cascade-updater/standalone-inputs')
COMPONENT=Path('/usr/local/libexec/siemcore-node-standalone')
WRAPPER=Path('/usr/local/sbin/siemcore-node-standalone')

def put(path,raw,mode=0o600):
 import tempfile
 fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.successor-')
 try:
  with os.fdopen(fd,'wb') as stream:
   os.fchmod(stream.fileno(),mode);stream.write(raw);stream.flush();os.fsync(stream.fileno())
  os.replace(tmp,path)
  fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
  try:os.fsync(fd)
  finally:os.close(fd)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)

def snapshot(node,hostname):
 names=['siemcore-unlinked-'+node+'-'+r for r in ('postgres','redis')]+['siemcore-standalone-'+node+'-'+r for r in ('app','archiver')]
 records=json.loads(subprocess.check_output(['docker','inspect',*names],timeout=20));data={}
 for role,r in zip(('postgres','redis'),records[:2]):
  if not r['State']['Running']:raise ValueError('retained_data_not_running')
  data[role]={'id':r['Id'],'image':r['Image'],'mounts':[{k:m[k] for k in ('Type','Source','Destination','RW')} for m in r['Mounts']]}
 if len(records)!=4 or any(r['State']['Running'] or r['State']['Restarting'] for r in records[2:]):raise ValueError('previous_processing_not_stopped')
 health=json.loads(subprocess.check_output(['curl','--fail','--silent','--show-error','--max-time','10','--resolve',hostname+':443:127.0.0.1','https://'+hostname+'/health/live'],timeout=15))
 return data,health

def install(loader,p,measure=snapshot):
 new=json.loads(loader.protected(KIT/'POLICY.json').read_text());b=new['binding'];validate_binding(b)
 if b.get('previous_operation')!=p['previous_operation'] or digest(b)!=p['operation_sha256']:raise ValueError('successor_package_binding')
 current=loader.protected(POLICY).read_bytes();new_raw=json.dumps(new,sort_keys=True,separators=(',',':')).encode()
 if hashlib.sha256(current).hexdigest() not in (p['old_policy_sha256'],hashlib.sha256(new_raw).hexdigest()):raise ValueError('successor_policy_conflict')
 receipt_path=ROOT/'successor-install.json'
 expected=dict(protocol='pod-node-standalone-successor-install-v1',kit_version=p['kit_version'],operation_id=b['operation_id'],operation_sha256=digest(b),previous_operation=b['previous_operation'],component_manifest_sha256=new['component_manifest_sha256'],product_execution=False)
 destination=COMPONENT/p['kit_version']
 new_wrapper=('#!/bin/sh\nexec /usr/bin/python3 -I -B '+str(destination/'cli.py')+' "$@"\n').encode()
 if receipt_path.exists():
  if json.loads(loader.protected(receipt_path).read_text())!=expected or current!=new_raw or loader.protected(WRAPPER).read_bytes()!=new_wrapper:raise ValueError('successor_receipt_conflict')
  return expected
 operation=ROOT/'operations'/b['operation_id']
 if operation.exists() and list(operation.iterdir()):raise ValueError('successor_already_started')
 original=json.loads(loader.read('/etc/siemcore/greenfield.json'))
 loader.expected_operation=b['operation_id'];loader.previous_standalone=new['previous_standalone'];loader.successor_binding=b
 evidence,bootstrap,_=loader.measure(new['historical_inventory'])
 if digest(evidence)!=b['source_evidence_sha256'] or hashlib.sha256(bootstrap).hexdigest()!=b['bootstrap_receipt_sha256']:raise ValueError('successor_source_changed')
 if configuration_digest(INPUTS,new['configuration_metadata'])!=b['configuration_sha256']:raise ValueError('successor_configuration_changed')
 data,health=measure(b['source']['node_id'],original['management']['hostname'])
 if canonical_data_identity(data)!=canonical_data_identity(new['data_identity']):raise ValueError('successor_data_changed')
 wanted=dict(version=b['source_version'],installation_id=b['source']['installation_id'],updater_id=b['source']['updater_instance_id'],node_id=b['source']['node_id'],installation_state='installed-unlinked',processing_enabled=False,authority_enabled=False,data_ready=True)
 if any(type(health.get(k)) is not type(v) or health[k]!=v for k,v in wanted.items()):raise ValueError('restored_management_not_healthy')
 marker=json.loads(loader.read('/opt/siemcore-node-standalone-'+b['source']['node_id']+'/operation.json'))
 if marker!={'operation_sha256':b['previous_operation']['operation_sha256']}:raise ValueError('previous_runtime_owner_changed')
 old_wrapper=('#!/bin/sh\nexec /usr/bin/python3 -I -B '+str(COMPONENT/p['old_revision']/'cli.py')+' "$@"\n').encode()
 if loader.protected(WRAPPER).read_bytes() not in (old_wrapper,new_wrapper):raise ValueError('successor_wrapper_conflict')
 manifest=loader.protected(KIT/'component/COMPONENT.json').read_bytes()
 if hashlib.sha256(manifest).hexdigest()!=new['component_manifest_sha256']:raise ValueError('successor_component_mismatch')
 names=set(json.loads(manifest)['files'])|{'COMPONENT.json'}
 destination.mkdir(mode=0o700,exist_ok=True);loader.protected(destination)
 if {x.name for x in destination.iterdir()}-names:raise ValueError('unexpected_component_destination')
 for name in names:
  raw=loader.protected(KIT/'component'/name).read_bytes();target=destination/name
  if target.exists() and loader.protected(target).read_bytes()!=raw:raise ValueError('successor_component_conflict')
  if not target.exists():put(target,raw)
 put(POLICY,new_raw);put(WRAPPER,new_wrapper,0o755);atomic_json(receipt_path,expected)
 return expected

def main():
 os.umask(0o077)
 if os.geteuid()!=0 or sys.argv[1:]!=['repair']:raise ValueError('root_successor_install_required')
 loader=SourceLoader();p=json.loads(loader.protected(KIT/'PACKAGE.json').read_text())
 for name,sha in p['files'].items():
  if '..' in Path(name).parts or Path(name).is_absolute() or hashlib.sha256(loader.protected(KIT/name).read_bytes()).hexdigest()!=sha:raise ValueError('successor_package_integrity')
 a=json.loads(loader.read('/etc/siemcore/greenfield.json'))
 req=urllib.request.Request('http://169.254.169.254/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
 with urllib.request.urlopen(req,timeout=5) as response:vm=response.read(128).decode().strip()
 if vm!=p['identity']['vm_id'] or any(a.get(k)!=v for k,v in p['identity'].items() if k!='vm_id') or loader.read('/etc/machine-id').decode().strip()!=p['identity']['machine_id']:raise ValueError('successor_host_identity')
 version=subprocess.check_output(['/usr/local/bin/siemcore-cascade-updater','version'],text=True,timeout=15).splitlines()[0].split()[1]
 if tuple(map(int,version.split('.')))<(1,16,1,32):raise ValueError('signed_updater_32_required')
 service='siemcore-cascade-updater';active=subprocess.run(['systemctl','is-active','--quiet',service]).returncode==0
 if active:subprocess.run(['systemctl','stop',service],check=True,timeout=60)
 lock=Path('/var/lib/siemcore-greenfield/hook.lock');loader.protected(lock);fd=os.open(lock,os.O_RDWR|os.O_NOFOLLOW)
 try:
  fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB);print(json.dumps(install(loader,p),sort_keys=True))
 finally:
  os.close(fd)
  if active:subprocess.run(['systemctl','start',service],check=True,timeout=60)
if __name__=='__main__':
 try:main()
 except Exception as error:
  print('Signed successor installation refused: '+(str(error) if isinstance(error,ValueError) else type(error).__name__),file=sys.stderr);sys.exit(1)
