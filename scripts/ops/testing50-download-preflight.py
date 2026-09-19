import hashlib,json,os,pwd,subprocess
from pathlib import Path as P
import yaml
assert os.geteuid()==0
root=P('/var/lib/siemcore-recovery/setup50-preflight-inputs');cfg=P('/etc/siemcore-cascade-updater/config.yaml');before=cfg.read_bytes();c=yaml.safe_load(before)
assert c['instance']['id']=='siemcore-testing-01' and c.get('self_update',{}).get('channel','stable')=='stable'
assert next(p for p in c['products'] if p['name']=='siemcore')['channel']=='obs-test-20260918'
assert subprocess.run(['systemctl','is-active','siemcore-cascade-updater'],capture_output=True,text=True).stdout.strip()=='inactive'
u=subprocess.check_output(['systemctl','show','siemcore-cascade-updater','-p','User','--value'],text=True).strip()
with (root/'download.log').open('w') as out:
 os.chmod(out.name,0o600)
 result=subprocess.run(['runuser','-u',u,'--','/usr/local/bin/siemcore-cascade-updater','once','--download','--config',str(cfg)],stdout=out,stderr=subprocess.STDOUT,timeout=600)
assert result.returncode==0,'download failed; inspect protectedlog'
assert hashlib.sha256(P('/var/lib/siemcore-cascade-updater/artifacts/siemcore-3.3.152.50.artifact').read_bytes()).hexdigest()=='9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc'
subprocess.run(['python3',str(root/'testing50-policy.py'),'preflight',str(root/'ingress50-signed-receipt.json')],check=True,timeout=850)
assert cfg.read_bytes()==before
subprocess.run(['systemctl','start','siemcore-cascade-updater'],check=True)
print('signed download + preflight passed; normal cascade daemon started')
