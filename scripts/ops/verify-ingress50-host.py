import hashlib,json,subprocess
from pathlib import Path as P
import yaml
root=P('/var/lib/siemcore-recovery/ingress50-prerequisite');plan=json.loads((root/'signed-plan.json').read_text())['plan'];intent=json.loads((root/'install-intent.json').read_text());c=yaml.safe_load(P('/etc/siemcore-cascade-updater/config.yaml').read_text())
assert c['instance']['id']==plan['updater_id'] and c['self_update']['channel']=='stable'
assert next(p for p in c['products'] if p['name']=='siemcore')['server_type']=='normal'
assert next(p for p in c['products'] if p['name']=='siemcore')['channel']=='obs-test-20260918'
assert all(hashlib.sha256(P(p).read_bytes()).hexdigest()==h for p,h in intent['protected'].items())
state=json.loads(P('/var/lib/siemcore-cascade-updater/state.json').read_text());assert state['siemcore_installation']=={'server_type':'normal'}
assert state['product_versions']['siemcore']=='3.3.152.50'
a=state['last_update_attempt'];assert a['success'] is True and a['from_version']=='3.3.152.47' and a['target_version']=='3.3.152.50' and a['artifact_digest']==plan['target_sha256']
health=json.loads(subprocess.check_output(['curl','-fsS','http://127.0.0.1:8443/health']));assert health['version']=='3.3.152.50' and health['status']=='healthy'
containers=json.loads(subprocess.check_output(['docker','inspect','siemcore-app-a','siemcore-archiver-app-a','siemcore-db-a-etcd','siemcore-db-a-postgres','siemcore-redis-a','siemcore-lb-a']))
assert all(x['State']['Health']['Status']=='healthy' for x in containers)
assert containers[0]['NetworkSettings']['Ports']==plan['published_bindings']
tx=json.loads(P('/var/lib/siemcore-recovery/transaction.json').read_text());assert tx['stage']=='applied' and tx['target']=='3.3.152.50' and tx['port_normalization']['published_bindings']==plan['published_bindings']
assert subprocess.check_output(['systemctl','is-active','siemcore-cascade-updater'],text=True).strip()=='active'
print(json.dumps(dict(instance_id=c['instance']['id'],updater_version=subprocess.check_output(['/usr/local/bin/siemcore-cascade-updater','version'],text=True).strip(),self_update_channel='stable',product_channel='obs-test-20260918',installation=state['siemcore_installation'],last_update_attempt=a,health=health,protected_receipts_unchanged=True,exact_endpoints_preserved=True,containers=[dict(name=x['Name'],health=x['State']['Health']['Status'],restart_count=x['RestartCount']) for x in containers],journal_stage=tx['stage'])))
