import json,hashlib,subprocess
from pathlib import Path
import yaml
root=Path('/etc/normal-qualification');data=json.loads((root/'envelope.json').read_text());c=yaml.safe_load(Path('/etc/siemcore-cascade-updater/config.yaml').read_text());service='siemcore-cascade-updater'
cmd=['/opt/fixture-kit/install.sh','--clean','--server-type','normal','--license-key','disposable-fixture','--parent-url',c['server']['url'],'--instance-id',data['application']['updater_instance_id'],'--parent-id','fixture-parent','--customer-id','fixture-customer','--customer-name','Disposable fixture','--signing-key',data['release']['public_key'],'--ca-file',str(root/'credentials/ca.crt'),'--greenfield-input',str(root/'envelope.json')]
paths=[Path('/etc/siemcore/updater-bootstrap.json'),Path('/etc/siemcore/greenfield.json'),Path('/etc/siemcore/greenfield-release.json')]
before={str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}
r=subprocess.run(cmd,capture_output=True,text=True,timeout=60);assert r.returncode==0,r.stderr
assert before=={str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}
changed=dict(data);changed['application']=dict(data['application'],frontend_url='https://changed.fixture');other=root/'changed.json';other.write_text(json.dumps(changed));other.chmod(0o600);cmd[-1]=str(other)
r=subprocess.run(cmd,capture_output=True,text=True,timeout=60);assert r.returncode!=0
assert before=={str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}
state=json.loads(Path('/var/lib/'+service+'/state.json').read_text());assert state.get('product_versions',{}).get('siemcore','0.0.0')=='0.0.0'
report=dict(updater_version='1.16.1.33',kit_version='1.16.1.33-r1',native_architecture='amd64',normal_installation_identity=state.get('siemcore_installation'),exact_installer_retry_preserved_receipts=True,changed_input_refused=True,no_product_success_recorded=True,application_qualified=False)
(root/'retry-result.json').write_text(json.dumps(report,indent=2));print(json.dumps(report))
