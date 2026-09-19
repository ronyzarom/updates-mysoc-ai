import base64,hashlib,json,os,subprocess,sys
from pathlib import Path as P
import yaml
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
assert os.geteuid()==0
root=P('/var/lib/siemcore-recovery/ingress50-prerequisite');cfg=P('/etc/siemcore-cascade-updater/config.yaml');c=yaml.safe_load(cfg.read_bytes())
assert subprocess.run(['systemctl','is-active','siemcore-cascade-updater'],capture_output=True,text=True).stdout.strip()=='inactive'
envelope=json.loads((root/'signed-plan.json').read_text());plan=envelope['plan'];unsigned=json.loads((root/'unsigned-plan.json').read_text());assert plan==unsigned
assert plan['updater_id']==c['instance']['id']
receipt=json.loads((root/'ingress50-signed-receipt.json').read_text());key=receipt['public_key']
assert key=='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
sys.path.insert(0,'/usr/local/lib/siemcore-cascade/recovery');import recovery as r;import port_normalization as n
Ed25519PublicKey.from_public_bytes(bytes.fromhex(key)).verify(base64.b64decode(envelope['signature']),n.DOMAIN+n.canonical(plan))
if n.AUTH.exists():
 r.a.trusted(n.AUTH);assert json.loads(n.AUTH.read_text())==envelope
else:r.save_json(n.AUTH,envelope)
n.authorization(dict(public_key=key),r.command)
u=subprocess.check_output(['systemctl','show','siemcore-cascade-updater','-p','User','--value'],text=True).strip();assert u=='siemcore-cascade-updater'
with (root/'download.log').open('w') as out:
 os.chmod(out.name,0o600)
 result=subprocess.run(['runuser','-u',u,'--','/usr/local/bin/siemcore-cascade-updater','once','--download','--config',str(cfg)],stdout=out,stderr=subprocess.STDOUT,timeout=600)
assert result.returncode==0,'download failed; inspect protected log'
artifact=P('/var/lib/siemcore-cascade-updater/artifacts/siemcore-3.3.152.50.artifact');assert hashlib.sha256(artifact.read_bytes()).hexdigest()==plan['target_sha256']
result=subprocess.run(['python3',str(root/'ingress50-native-preflight.py'),str(root/'ingress50-signed-receipt.json')],capture_output=True,text=True,timeout=850)
with (root/'preflight.log').open('w') as out:os.chmod(out.name,0o600);out.write(result.stdout+result.stderr)
assert result.returncode==0,'preflight failed; inspect protectedlog'
print(result.stdout)
subprocess.run(['systemctl','start','siemcore-cascade-updater'],check=True)
print('signed authorization + actualdownload + read-onlypreflight passed; native auto cascade started')
