import base64,hashlib,io,json,os,pathlib,subprocess,tarfile,tempfile,yaml
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
P=pathlib.Path
root=P('/var/lib/siemcore-recovery/setup49-preflight-inputs')
blob=(root/'setup49-root-repair.tar').read_bytes();r=json.loads((root/'setup49-root-repair-receipt.json').read_text())
key=json.loads(P('/etc/siemcore/greenfield-release.json').read_text())['public_key']
assert key==r['public_key']=='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
assert r['product']=='siemcore-normal-recovery-policy' and r['version']=='1.0.0.1' and hashlib.sha256(blob).hexdigest()==r['sha256']
Ed25519PublicKey.from_public_bytes(bytes.fromhex(key)).verify(base64.b64decode(r['signature']),('mysoc-release-v1\n'+r['product']+'\n'+r['version']+'\n'+r['sha256']).encode())
with tarfile.open(fileobj=io.BytesIO(blob)) as t:
 assert t.getnames()==['manifest.json','greenfield-hook.py'] and all(x.isfile() for x in t)
 m=json.load(t.extractfile('manifest.json'));new=t.extractfile('greenfield-hook.py').read()
cfg=P('/etc/siemcore-cascade-updater/config.yaml');c=yaml.safe_load(cfg.read_bytes())
assert c['instance']['id'] in m['instances'] and c['products'][0]['server_type']=='normal'
assert c['products'][0]['channel']=='normal-a-20260919' and c.get('self_update',{}).get('channel')=='stable'
target=P('/usr/local/lib/siemcore-cascade/greenfield-hook.py')
for x in (target,*target.parents):
 st=x.lstat();assert not x.is_symlink() and st.st_uid==0 and not st.st_mode&0o022
old=target.read_bytes();assert hashlib.sha256(old).hexdigest()==m['old_sha256']
assert hashlib.sha256(new).hexdigest()==m['new_sha256']
addition=b"        # The shipped compose leaves the archiver debug host port dynamic by\n        # default. Recovery permits only this named container's 8444/tcp;\n        # configured bind addresses and every customer-facing port stay gated.\n        'allow_dynamic_archiver_debug_port': True,\n"
assert new.count(addition)==1 and new.replace(addition,b'')==old
unchanged={p:p.read_bytes() for p in [cfg,P('/etc/siemcore/updater-bootstrap.json'),P('/etc/siemcore/greenfield-release.json'),P('/etc/siemcore/greenfield.json')]}
subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True)
try:
 assert target.read_bytes()==old
 state=json.loads(P(c['simulation']['state_file']).read_text());assert state['product_versions']['siemcore']=='3.3.152.47'
 assert not state.get('operation_state')
 backup=root/'greenfield-hook.before';assert not backup.exists();backup.write_bytes(old);backup.chmod(0o600)
 st=target.stat();fd,temp=tempfile.mkstemp(dir=target.parent,prefix='.setup49-hook-')
 with os.fdopen(fd,'wb') as f:
  f.write(new);f.flush();os.fsync(f.fileno());os.fchmod(f.fileno(),st.st_mode&0o777);os.fchown(f.fileno(),st.st_uid,st.st_gid)
 os.replace(temp,target)
 d=os.open(target.parent,os.O_DIRECTORY);os.fsync(d);os.close(d)
 assert all(p.read_bytes()==v for p,v in unchanged.items())
 print(json.dumps({'instance_id':c['instance']['id'],'root_component':'1.0.0.1','new_hook_sha256':m['new_sha256'],'config_and_bootstrap_receipts_unchanged':True}))
finally:subprocess.run(['systemctl','start','siemcore-cascade-updater'],check=True)
