import hashlib,json,subprocess,sys
from pathlib import Path as P
root=P('/etc/normal-qualification');sys.path.insert(0,'/usr/local/lib/siemcore-cascade/recovery');import recovery as r
kit=json.loads((root/'kit-result.json').read_text());state=json.loads(P('/var/lib/siemcore-cascade-updater/state.json').read_text());records=json.loads((root/'cascade50-records.json').read_text());auth=json.loads(P('/etc/siemcore-cascade-updater/port-normalization.json').read_text())['plan']
assert state['siemcore_installation']==kit['installed_identity']=={'server_type':'normal'}
assert records['reports'][0]['success'] is True and records['reports'][0]['from_version']=='3.3.152.47' and records['reports'][0]['to_version']=='3.3.152.50'
assert all(hashlib.sha256(P(p).read_bytes()).hexdigest()==d for p,d in auth['identity_files'].items())
data=json.loads((root/'envelope.json').read_text());app=dict(data['application'],machine_id=P('/etc/machine-id').read_text().strip())
assert json.loads(P('/etc/siemcore/greenfield.json').read_text())==app
expected=dict(input_sha256=hashlib.sha256((root/'envelope.json').read_bytes()).hexdigest(),schema=1,protocol='normal-prerequisites-v1',application_canonical_sha256=hashlib.sha256(json.dumps(app,sort_keys=True,separators=(',',':'),ensure_ascii=False,allow_nan=False).encode()).hexdigest(),prerequisite_manifest_sha256=app['normal_prerequisites']['sha256'])
assert json.loads(P('/etc/siemcore/updater-bootstrap.json').read_text())==expected
assert json.loads(P('/etc/siemcore/greenfield-release.json').read_text())==data['release']
health=json.loads(subprocess.check_output(['curl','-fsS','http://127.0.0.1:8443/health']));assert health['version']=='3.3.152.50' and health['status']=='healthy'
actual=json.loads(subprocess.check_output(['docker','inspect','siemcore-app-a']))[0]['NetworkSettings']['Ports'];assert actual==auth['published_bindings']
tx=json.loads(P('/var/lib/siemcore-recovery/transaction.json').read_text());assert tx['stage']=='applied' and tx['target']=='3.3.152.50' and 'port_normalization' in tx
assert state['product_versions']['siemcore']=='3.3.152.50'
result=dict(synthetic_only=True,actual_systemd_cascade=True,from_version='3.3.152.47',target_version='3.3.152.50',artifact_sha256=auth['target_sha256'],signature_verified_by_updater=True,exact_endpoints_preserved=True,normal_identity_preserved=True,baseline_installation=kit['installed_identity'],bootstrap_receipts_match_original_inputs=True,app_config_original_hashes_preserved=True,recovery_stage='applied',health=health,runtime_sha256=auth['runtime_files'],hook_sha256=hashlib.sha256(P('/usr/local/lib/siemcore-cascade/greenfield-hook.py').read_bytes()).hexdigest(),qualification_note='Original final assertion confused persisted server_type with heartbeat kind; read-only verification compares exact baseline and original bootstrap inputs.')
r.save_json(root/'cascade50-result.json',result);print(json.dumps(result))
