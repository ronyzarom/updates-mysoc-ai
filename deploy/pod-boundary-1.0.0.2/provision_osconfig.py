#!/usr/bin/env python3
"""Exact B signed prerequisite delivery; only the updater service is restarted.
Run by reviewed GCP OS Config assignment with receipt/archive/expected.json.
Requires central product hold independently verified by rollout coordinator.
"""
import base64,fcntl,hashlib,json,os,stat,subprocess,tarfile,tempfile,time,urllib.request
from pathlib import Path
VERSION='1.0.0.2'
PACKAGE_SHA='ba508630db07192ceda951957755c0fed076798823c61547af06943b0be2aa2e'
KEY='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
GCP_ID='5012156962629255074'
MACHINE='19e0966bf88842f08bc180c40044d2ed'
UPDATER='bezeq-pod-test-b-5012156962629255074'
BASE=Path(__file__).resolve().parent

def digest(path):return hashlib.sha256(path.read_bytes()).hexdigest()

def protected(path):
 for p in (path,*path.parents):
  st=p.lstat()
  if stat.S_ISLNK(st.st_mode) or st.st_uid!=0 or st.st_mode&0o022:raise ValueError('unprotected delivery input')

def regular(path):
 protected(path)
 if not stat.S_ISREG(path.lstat().st_mode):raise ValueError('regular delivery file required')

def write_private(path,raw):
 protected(path.parent)
 if os.path.lexists(path):regular(path)
 fd,tmp=tempfile.mkstemp(dir=path.parent)
 try:
  with os.fdopen(fd,'wb') as out:
   os.fchmod(out.fileno(),0o600);out.write(raw);out.flush();os.fsync(out.fileno())
  os.replace(tmp,path)
  directory=os.open(path.parent,os.O_DIRECTORY)
  try:os.fsync(directory)
  finally:os.close(directory)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)

def verify_package(base):
 protected(base)
 regular(base/'receipt.json')
 receipt=json.loads((base/'receipt.json').read_text())
 if receipt['version']!=VERSION or receipt['public_key']!=KEY or receipt['sha256']!=PACKAGE_SHA:raise ValueError('wrong signing identity')
 archive=base/('pod-boundary-'+VERSION+'.tar.gz')
 regular(archive)
 if digest(archive)!=receipt['sha256']:raise ValueError('package checksum mismatch')
 write_private(base/'key.der',bytes.fromhex('302a300506032b6570032100'+KEY))
 write_private(base/'sig',base64.b64decode(receipt['signature'],validate=True))
 write_private(base/'message',('mysoc-pod-boundary-v1\n'+VERSION+'\n'+receipt['sha256']).encode())
 subprocess.run(['openssl','pkeyutl','-verify','-pubin','-keyform','DER','-inkey',str(base/'key.der'),'-rawin','-in',str(base/'message'),'-sigfile',str(base/'sig')],check=True,capture_output=True,timeout=15)
 package=base/'package';package.mkdir(mode=0o700,exist_ok=True);protected(package)
 expected={'boundary.py','reconcile.py','incident.json','install.py','files.json'};seen=set()
 with tarfile.open(archive) as tar:
  for member in tar:
   if member.name not in expected or not member.isfile() or member.name in seen or member.size>65536:raise ValueError('invalid package member')
   seen.add(member.name);target=package/member.name
   write_private(target,tar.extractfile(member).read())
 if seen!=expected:raise ValueError('incomplete package')
 return package,receipt

def acquire_cycle_lock(fd, wait_seconds=45):
 deadline=time.monotonic()+wait_seconds
 while True:
  try:
   fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
   return
  except BlockingIOError:
   if time.monotonic()>=deadline:raise
   time.sleep(1)

def main():
 if os.geteuid()!=0:raise ValueError('root required')
 protected(BASE)
 req=urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
 if urllib.request.urlopen(req,timeout=10).read().decode()!=GCP_ID or Path('/etc/machine-id').read_text().strip()!=MACHINE:raise ValueError('wrong host')
 apppath=Path('/etc/siemcore/greenfield.json');protected(apppath);app=json.loads(apppath.read_text())
 if (app.get('topology'),app.get('pod_role'),app.get('updater_instance_id'))!=('pod','b',UPDATER):raise ValueError('wrong product identity')
 regular(BASE/'expected.json')
 expected=json.loads((BASE/'expected.json').read_text())
 # Preserve config, protected policy and sudo configuration exactly.
 for name,sha in expected['unchanged_hashes'].items():
  p=Path(name);regular(p)
  if digest(p)!=sha:raise ValueError('preexisting configuration drift')
 package,receipt=verify_package(BASE)
 service='siemcore-cascade-updater.service'
 active=subprocess.check_output(['systemctl','show',service,'-p','ActiveState','--value'],text=True,timeout=10).strip()
 if active not in ('active','inactive'):raise ValueError('unexpected updater state')
 lockpath=Path('/var/lib/siemcore-cascade-updater/state.json.cycle-lock')
 fd=os.open(lockpath,os.O_RDWR|os.O_NOFOLLOW)
 try:
  acquire_cycle_lock(fd)
  if os.fstat(fd).st_ino!=lockpath.stat(follow_symlinks=False).st_ino:raise ValueError('cycle lock changed')
  if active=='active':subprocess.run(['systemctl','stop',service],check=True,timeout=45)
 finally:
  os.close(fd)
 # Service is inactive; installer acquires the same cycle/boundary/work locks.
 try:
  completed=subprocess.run(['/usr/bin/python3',str(package/'install.py')],check=True,capture_output=True,text=True,timeout=180)
  result=json.loads(completed.stdout)
  for name,sha in expected['unchanged_hashes'].items():
   if digest(Path(name))!=sha:raise ValueError('configuration changed')
 finally:
  if active=='active':subprocess.run(['systemctl','start',service],check=True,timeout=30)
 result.update(package_sha256=receipt['sha256'],product_hold='preserved-by-coordinator',gcp_id=GCP_ID)
 # Persist only after complete package activation; repeated assignments validate
 # installed hashes and terminal evidence, never just marker existence.
 marker=Path('/var/lib/siemcore-pod-boundary-2-provisioned.json')
 import sys
 sys.path.insert(0,str(package))
 import boundary
 if os.path.lexists(marker):regular(marker)
 boundary.atomic(marker,result)
 print(json.dumps(result,sort_keys=True))

if __name__=='__main__':main()
