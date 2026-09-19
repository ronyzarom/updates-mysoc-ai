import copy,json,os,subprocess,sys,tempfile
from pathlib import Path as P
import yaml
kind=sys.argv[1];expected={'A':'siemcore-normal-db91d16e-97a5-452d-ae54-6db5c6d8f3bf','B':'siemcore-normal-b-90881d89-d9b0-4e41-8643-3cf758903cc1'}
assert os.geteuid()==0 and kind in expected
cfg=P('/etc/siemcore-cascade-updater/config.yaml');raw=cfg.read_bytes();c=yaml.safe_load(raw);assert c['instance']['id']==expected[kind] and c['self_update']['channel']=='stable'
p=next(p for p in c['products'] if p['name']=='siemcore');assert p['server_type']=='normal' and p['channel']=='normal-a-20260919'
h=json.loads(subprocess.check_output(['curl','-fsS','http://127.0.0.1:8443/health']));assert h['status']=='healthy' and h['version']=='3.3.152.47'
root=P('/var/lib/siemcore-recovery/ingress50-prerequisite');root.mkdir(mode=0o700)
for n in ('ingress50-root-runtime.tar','ingress50-root-receipt.json','ingress50-signed-receipt.json','install-ingress50-runtime.py','measure-ingress50-plan.py','ingress50-native-preflight.py'):
 subprocess.run(['install','-o','root','-g','root','-m','0600','/tmp/'+n,str(root/n)],check=True)
subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True)
assert cfg.read_bytes()==raw
with (root/'original-config.yaml').open('xb') as f:os.fchmod(f.fileno(),0o600);f.write(raw);f.flush();os.fsync(f.fileno())
before=copy.deepcopy(c);p['channel']='obs-test-20260918'
check=copy.deepcopy(c);next(x for x in check['products'] if x['name']=='siemcore')['channel']='normal-a-20260919';assert check==before
st=cfg.stat();fd,tmp=tempfile.mkstemp(dir=cfg.parent,prefix='.ingress50-config-')
with os.fdopen(fd,'wb') as f:f.write(yaml.safe_dump(c,sort_keys=False).encode());f.flush();os.fsync(f.fileno());os.fchmod(f.fileno(),st.st_mode&0o777);os.fchown(f.fileno(),st.st_uid,st.st_gid)
os.replace(tmp,cfg);fd=os.open(cfg.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
subprocess.run(['python3',str(root/'install-ingress50-runtime.py'),str(root)],check=True)
plan=subprocess.check_output(['python3',str(root/'measure-ingress50-plan.py')])
with (root/'unsigned-plan.json').open('xb') as f:os.fchmod(f.fileno(),0o600);f.write(plan);f.flush();os.fsync(f.fileno())
print('MEASURED_PLAN '+plan.decode())
print('updater remains stopped for signed authorization + preflight')
