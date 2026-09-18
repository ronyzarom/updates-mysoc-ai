#!/usr/bin/env python3
"""One-time signed repair for an updater stranded by terminal product recovery.

Requires a separately authorized host-bound startup package. Never edits updater
state, product policy, database, or retained operation receipts.
"""
import base64,hashlib,json,os,pwd,subprocess,sys,urllib.request,tempfile
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
KIT=Path(__file__).resolve().parent

def validate(p,application,machine,vm,state,config,previous,blob):
 if vm!=p['identity']['vm_id'] or machine!=p['identity']['machine_id'] or any(application.get(k)!=v for k,v in p['identity'].items() if k!='vm_id'):raise ValueError('repair_host_identity')
 op=state.get('node_standalone_operation') or {}
 if op.get('operation_id')!=p['previous_operation']['operation_id'] or op.get('operation_sha256')!=p['previous_operation']['operation_sha256'] or op.get('phase')!='restored':raise ValueError('repair_requires_terminal_restored_operation')
 if hashlib.sha256(config).hexdigest()!=p['config_sha256']:raise ValueError('repair_configuration_changed')
 if previous not in (p['previous_binary_sha256'],p['release']['sha256']):raise ValueError('repair_previous_binary_changed')
 r=p['release']
 if hashlib.sha256(blob).hexdigest()!=r['sha256'] or len(blob)!=r['size']:raise ValueError('repair_artifact_checksum')
 message=('mysoc-release-v1\nupdater-linux-amd64\n'+r['version']+'\n'+r['sha256']).encode()
 Ed25519PublicKey.from_public_bytes(bytes.fromhex(p['public_key'])).verify(base64.b64decode(r['signature'],validate=True),message)

def protected_ancestors(path):
 for item in (path,*path.parents):
  if item.is_symlink():raise ValueError('repair_symlink_path')
  if item.exists() and item.stat().st_mode&0o022:raise ValueError('repair_writable_path')

def check_receipt(path,expected):
 protected_ancestors(path)
 if path.exists() and json.loads(path.read_text())!=expected:raise ValueError('repair_receipt_conflict')

def save_receipt(path,expected):
 check_receipt(path,expected)
 fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.updater-repair-')
 try:
  with os.fdopen(fd,'wb') as stream:
   os.fchmod(stream.fileno(),0o600);stream.write(json.dumps(expected,sort_keys=True).encode());stream.flush();os.fsync(stream.fileno())
  os.replace(tmp,path)
  fd=os.open(path.parent,os.O_DIRECTORY)
  try:os.fsync(fd)
  finally:os.close(fd)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)

def main():
 os.umask(0o077)
 if os.geteuid()!=0 or sys.argv[1:]!=['repair']:raise ValueError('root_repair_required')
 p=json.loads((KIT/'PACKAGE.json').read_text());blob=(KIT/'updater').read_bytes()
 service='siemcore-cascade-updater';root=Path('/var/lib/siemcore-cascade-updater');layout=root/'self-update'
 # Stop the sole writer before inspecting and preserving durable updater state.
 subprocess.run(['systemctl','stop',service],check=True,timeout=60)
 try:
  protected_ancestors(layout/'releases')
  statepath=root/'state.json';protected_ancestors(statepath);state=statepath.read_bytes();config=Path('/etc/siemcore-cascade-updater/config.yaml').read_bytes()
  req=urllib.request.Request('http://169.254.169.254/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
  with urllib.request.urlopen(req,timeout=5) as response:vm=response.read(128).decode().strip()
  binary=Path('/usr/local/bin/siemcore-cascade-updater').resolve();previous=hashlib.sha256(binary.read_bytes()).hexdigest()
  # Replay after a completed replacement is valid only with an exact receipt.
  receipt=root/'signed-updater-repair.json';expected=dict(version=p['release']['version'],sha256=p['release']['sha256'],previous_operation=p['previous_operation'])
  check_receipt(receipt,expected)
  validate(p,json.loads(Path('/etc/siemcore/greenfield.json').read_text()),Path('/etc/machine-id').read_text().strip(),vm,json.loads(state),config,previous,blob)
  if binary not in (layout/'releases'/p['previous_version']/'siemcore-cascade-updater',layout/'releases'/p['release']['version']/'siemcore-cascade-updater'):raise ValueError('managed_binary_required')
  owner=pwd.getpwnam(service);destination=layout/'releases'/p['release']['version'];protected_ancestors(destination);destination.mkdir(mode=0o755,exist_ok=True)
  if destination.is_symlink():raise ValueError('invalid_destination')
  target=destination/'siemcore-cascade-updater'
  if target.exists() and (target.is_symlink() or target.read_bytes()!=blob):raise ValueError('repair_destination_conflict')
  if not target.exists():
   with target.open('xb') as f:f.write(blob);f.flush();os.fsync(f.fileno())
  target.chmod(0o755);os.chown(destination,owner.pw_uid,owner.pw_gid);os.chown(target,owner.pw_uid,owner.pw_gid)
  output=subprocess.check_output([str(target),'version'],text=True,timeout=15)
  if output.splitlines()[0]!='updater-simulator '+p['release']['version']:raise ValueError('repair_binary_version')
  for command in ('run','relay'):subprocess.run([str(target),command,'--help'],check=True,stdout=subprocess.DEVNULL,timeout=15)
  if statepath.read_bytes()!=state:raise ValueError('updater_state_changed')
  temporary=layout/'repair-current'
  if temporary.is_symlink():temporary.unlink()
  os.symlink(destination,temporary);os.lchown(temporary,owner.pw_uid,owner.pw_gid);os.replace(temporary,layout/'current')
  save_receipt(receipt,expected)
  fd=os.open(layout,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
  print(json.dumps(dict(expected,state_unchanged=True)))
 finally:subprocess.run(['systemctl','start',service],check=True,timeout=60)
if __name__=='__main__':main()
