"""Install only signed root prerequisite files on exact paused Normal alpha hosts."""
import base64,hashlib,io,json,os,pwd,stat,subprocess,sys,tarfile,tempfile
from pathlib import Path as P
import yaml
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
root=P(sys.argv[1]);receipt=json.loads((root/'ingress50-root-receipt.json').read_text());blob=(root/'ingress50-root-runtime.tar').read_bytes()
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def trusted(p):
 for x in (p,*p.parents):
  s=x.lstat();assert not stat.S_ISLNK(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022
assert os.geteuid()==0
assert subprocess.run(['systemctl','is-active','siemcore-cascade-updater'],capture_output=True,text=True).stdout.strip()=='inactive'
bootstrap=P('/etc/siemcore/greenfield-release.json');trusted(bootstrap);key=json.loads(bootstrap.read_text())['public_key']
assert key==receipt['public_key']=='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
assert receipt['product']=='siemcore-normal-recovery-runtime' and receipt['version']=='1.0.0.5' and hashlib.sha256(blob).hexdigest()==receipt['sha256']
Ed25519PublicKey.from_public_bytes(bytes.fromhex(key)).verify(base64.b64decode(receipt['signature']),('mysoc-release-v1\n'+receipt['product']+'\n'+receipt['version']+'\n'+receipt['sha256']).encode())
with tarfile.open(fileobj=io.BytesIO(blob)) as t:
 assert set(t.getnames())=={'manifest.json','recovery.py','artifact.py','supervise.py','port_normalization.py','greenfield-hook.py'} and len(t.getnames())==6 and all(x.isfile() for x in t)
 manifest=json.load(t.extractfile('manifest.json'));files={n:t.extractfile(n).read() for n in manifest['files']}
 assert set(manifest['files'])=={'recovery.py','artifact.py','supervise.py','port_normalization.py','greenfield-hook.py'}
 assert all(hashlib.sha256(v).hexdigest()==manifest['files'][n] for n,v in files.items())
cfg=P('/etc/siemcore-cascade-updater/config.yaml');trusted(cfg);c=yaml.safe_load(cfg.read_text());app=P('/etc/siemcore/greenfield.json');a=json.loads(app.read_text());trusted(app)
assert c['instance']['id'] in manifest['instances'] and a['updater_instance_id']==c['instance']['id'] and a['topology']=='single'
assert c['self_update']['channel']=='stable'
product=next(p for p in c['products'] if p['name']=='siemcore');assert product['server_type']=='normal' and product['channel']=='obs-test-20260918'
state_path=P(c['simulation']['state_file']);assert state_path==P('/var/lib/siemcore-cascade-updater/state.json')
service_user=subprocess.check_output(['systemctl','show','siemcore-cascade-updater','-p','User','--value'],text=True).strip();assert service_user=='siemcore-cascade-updater'
uid=pwd.getpwnam(service_user).pw_uid;trusted(state_path.parent.parent)
for x in (state_path,state_path.parent):
 st=x.lstat();assert not stat.S_ISLNK(st.st_mode) and st.st_uid==uid and not st.st_mode&0o022
assert stat.S_ISREG(state_path.lstat().st_mode)
state=json.loads(state_path.read_text());assert state['product_versions']['siemcore']=='3.3.152.47' and not state.get('operation_state')
assert manifest['target_version']=='3.3.152.50' and manifest['target_sha256']=='9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc'
runtime=P('/usr/local/lib/siemcore-cascade/recovery');hook=runtime.parent/'greenfield-hook.py'
allowed_old={'recovery.py':{'89107cacaa08daad28b30e4e62ddd6214dd1e1d312d7f7b16cf49c60c1c0622e'},'greenfield-hook.py':{'1c925859674e1f47253e8cbdb1a559724baa7863c13148031956ddccbd4e5ec3','329e855866399e59e62219fcbd7d8b59087b472f60253c6ae95fe43e8ccedea8'},'port_normalization.py':{None},'artifact.py':{manifest['files']['artifact.py']},'supervise.py':{manifest['files']['supervise.py']}}
for n in files:
 target=hook if n=='greenfield-hook.py' else runtime/n
 trusted(target if target.exists() else target.parent)
 assert (digest(target) if target.exists() else None) in allowed_old[n]|{manifest['files'][n]}
protected=[cfg,app,bootstrap,P('/etc/siemcore/updater-bootstrap.json')];before={str(p):digest(p) for p in protected}
for p in protected:trusted(p)
trusted(root)
intent=root/'install-intent.json'
expected=dict(receipt_sha256=digest(root/'ingress50-root-receipt.json'),instance_id=c['instance']['id'],protected=before)
if intent.exists():
 trusted(intent);assert json.loads(intent.read_text())==expected
else:
 with intent.open('x') as f:
  os.fchmod(f.fileno(),0o600);json.dump(expected,f);f.flush();os.fsync(f.fileno())
 fd=os.open(root,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
backup=root/'original-root-components';backup.mkdir(mode=0o700,exist_ok=True);trusted(backup)
for n,data in files.items():
 target=hook if n=='greenfield-hook.py' else runtime/n
 trusted(target if target.exists() else target.parent)
 original=backup/n
 if original.exists():
  trusted(original);assert digest(original) in allowed_old[n]
 elif target.exists() and None not in allowed_old[n]:
  assert digest(target) in allowed_old[n], 'missing original component for interrupted install'
  with original.open('xb') as f:
   os.fchmod(f.fileno(),0o600);f.write(target.read_bytes());f.flush();os.fsync(f.fileno())
  fd=os.open(backup,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 fd,temp=tempfile.mkstemp(dir=target.parent,prefix='.ingress50-')
 with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno());os.fchmod(f.fileno(),0o755)
 os.replace(temp,target)
 fd=os.open(target.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
assert before=={str(p):digest(p) for p in protected}
assert all(digest(hook if n=='greenfield-hook.py' else runtime/n)==h for n,h in manifest['files'].items())
print(json.dumps(dict(instance_id=c['instance']['id'],root_runtime='1.0.0.5',files=manifest['files'],protected_receipts_unchanged=True)))
