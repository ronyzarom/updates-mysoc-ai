import copy,hashlib,json,os,subprocess,sys,uuid
from pathlib import Path as P
import yaml
assert os.geteuid()==0
assert subprocess.run(['systemctl','is-active','siemcore-cascade-updater'],capture_output=True,text=True).stdout.strip()=='inactive'
cfg=P('/etc/siemcore-cascade-updater/config.yaml');appfile=P('/etc/siemcore/greenfield.json')
c=yaml.safe_load(cfg.read_bytes());app=json.loads(appfile.read_text())
assert app['topology']=='single' and app['updater_instance_id']==c['instance']['id'] in ['siemcore-normal-db91d16e-97a5-452d-ae54-6db5c6d8f3bf','siemcore-normal-b-90881d89-d9b0-4e41-8643-3cf758903cc1']
assert c['self_update']['channel']=='stable'
p=next(x for x in c['products'] if x['name']=='siemcore');assert p['server_type']=='normal' and p['channel']=='obs-test-20260918'
source=P('/usr/local/lib/siemcore-cascade/recovery');sys.path.insert(0,str(source));import recovery as r;import port_normalization as n
assert r.VERSION=='1.0.0.5'
container=json.loads(subprocess.check_output(['docker','inspect','siemcore-app-a']))[0]
assert container['State']['Health']['Status']=='healthy'
health=json.loads(subprocess.check_output(['curl','-fsS','http://127.0.0.1:8443/health']));assert health['version']=='3.3.152.47' and health['status']=='healthy'
install=P('/opt/siemcore-app-app-a')
plan=dict(protocol='normal-ingress-retention-v1',operation_id=str(uuid.uuid4()),machine_id=P('/etc/machine-id').read_text().strip(),updater_id=app['updater_instance_id'],identity_files={str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in (appfile,cfg)},runtime_files={p:hashlib.sha256((source/p).read_bytes()).hexdigest() for p in n.RUNTIME_FILES},from_version='3.3.152.47',from_sha256='7cf8b7b0011344ca8f568bb023d6bd69e0f01fc9dbd3e14271727da0b8172812',target_version='3.3.152.50',target_sha256='9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc',container_id=container['Id'],image_id=container['Image'],files={p:hashlib.sha256((install/p).read_bytes()).hexdigest() if (install/p).exists() else None for p in r.FILES},host_bindings=container['HostConfig']['PortBindings'],published_bindings=container['NetworkSettings']['Ports'])
n.pins(plan,container)
print(json.dumps(plan,sort_keys=True))
