#!/usr/bin/env python3
"""Disposable root/systemd integration only: synthetic signed service executables.

Uses the exact product Python worker and actual root adapter/transport. No real
release, cloud connection or customer credential is used. Run only in the isolated
no-network/no-host-mount fixture container prepared by run-integration.sh.
"""
import base64
import fcntl
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tarfile
import time
import uuid
sys.path.insert(0,'/fixture/adapter')
from adapter import Adapter,atomic_json
from protocol import PROTOCOL,digest
from host import Host,protected
from worker import invoke
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat

mode=sys.argv[1];assert mode in ('success','health-failure','interrupted','installed-cli')
assert os.geteuid()==0 and Path('/fixture/ISOLATED_CONTAINER').exists()

def write(path,data,mode=0o600):
 p=Path(path);p.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
 p.write_bytes(data if isinstance(data,bytes) else data.encode());p.chmod(mode);return p

def run(args):return subprocess.check_output(args,stderr=subprocess.STDOUT,text=True)

run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-keyout','/tmp/fixture.key','-out','/tmp/fixture.crt','-subj','/CN=observer.fixture.test','-addext','subjectAltName=DNS:observer.fixture.test'])
write('/etc/ssl/siemcore/privkey.pem',Path('/tmp/fixture.key').read_bytes())
write('/etc/ssl/siemcore/fullchain.pem',Path('/tmp/fixture.crt').read_bytes(),0o644)
write('/usr/local/share/ca-certificates/observer-fixture.crt',Path('/tmp/fixture.crt').read_bytes(),0o644)
run(['update-ca-certificates'])
with open('/etc/hosts','a') as f:f.write('\n127.0.0.1 observer.fixture.test\n')
machine=Path('/etc/machine-id').read_text().strip();assert len(machine)==32
policy=dict(schema=4,topology='observer-unlinked',machine_id=machine,installation_id='fixture-observer',updater_instance_id='fixture-updater',management=dict(listen='0.0.0.0:443',hostname='observer.fixture.test',certificate='/etc/ssl/siemcore/fullchain.pem',key='/etc/ssl/siemcore/privkey.pem'))
policy_sha=hashlib.sha256(json.dumps(policy,sort_keys=True).encode()).hexdigest()
write('/etc/siemcore/greenfield.json',json.dumps(policy,sort_keys=True))
write('/etc/siemcore-pod-observer/unlinked.json',json.dumps(dict(policy['management'],protocol='observer-unlinked-v1',installation_id=policy['installation_id'],updater_id=policy['updater_instance_id'])))
write('/var/lib/siemcore-greenfield/journal.json',json.dumps(dict(status='complete',version='3.3.152.37',policy_sha256=policy_sha,installation_state='installed-unlinked')))

binary_template='''#!/usr/bin/python3
import hashlib,http.server,json,ssl,time
from pathlib import Path
VERSION=VERSION_VALUE
FAILED=FAILED_VALUE
DELAY=DELAY_VALUE
class Handler(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  if self.path=='/health/live':
   if FAILED:self.send_error(503);return
   d=dict(version=VERSION,binary_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),installation_id='fixture-observer',updater_id='fixture-updater',installation_state='installed-unlinked',management_ready=True,pod_ready=False,authority_enabled=False,processing_enabled=False)
   data=json.dumps(d).encode();self.send_response(200);self.end_headers();self.wfile.write(data)
  else:self.send_error(200 if VERSION=='3.3.152.37' else 401)
 def log_message(self,*args):pass
time.sleep(DELAY)
server=http.server.HTTPServer(('0.0.0.0',443),Handler)
ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);ctx.load_cert_chain('/etc/ssl/siemcore/fullchain.pem','/etc/ssl/siemcore/privkey.pem')
server.socket=ctx.wrap_socket(server.socket,server_side=True);server.serve_forever()
'''
key=Ed25519PrivateKey.generate();public=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)

def artifact(version,target):
 code=binary_template.replace('VERSION_VALUE',repr(version)).replace('FAILED_VALUE',str(target and mode=='health-failure')).replace('DELAY_VALUE',str(5 if target and mode=='interrupted' else 0)).encode()
 files={'MANIFEST.json':json.dumps(dict(product='siemcore',version=version,architecture='amd64',observer_unlinked_capabilities=['observer-ui-auth-v1','observer-unlinked-update-v1'] if target else [])).encode(),'pod/bin/siemcore':code}
 for p in Path('/fixture/product').glob('*.py'):files['updater/'+p.name]=p.read_bytes()
 out=io.BytesIO()
 with tarfile.open(fileobj=out,mode='w:gz') as archive:
  for name,data in files.items():
   m=tarfile.TarInfo('siemcore-universal-'+version+'/'+name);m.size=len(data);m.mode=0o700 if name=='pod/bin/siemcore' else 0o600;archive.addfile(m,io.BytesIO(data))
 raw=out.getvalue();sha=hashlib.sha256(raw).hexdigest();signature=base64.b64encode(key.sign(('mysoc-release-v1\nsiemcore\n'+version+'\n'+sha).encode())).decode()
 write('/var/lib/siemcore-cascade-updater/artifacts/siemcore-'+version+'.artifact',raw)
 return dict(product='siemcore',version=version,architecture='linux/amd64',artifact_sha256=sha,artifact_signature=signature,binary_sha256=hashlib.sha256(code).hexdigest()),code

previous,old_binary=artifact('3.3.152.37',False);target,_=artifact('3.3.152.38',True)
write('/etc/siemcore/greenfield-release.json',json.dumps(dict(version=previous['version'],channel='fixture',sha256=previous['artifact_sha256'],signature=previous['artifact_signature'],public_key=public.hex())))
old_path=write('/usr/local/libexec/siemcore-pod-unlinked/3.3.152.37/siemcore',old_binary,0o700)
write('/etc/systemd/system/siemcore-pod-unlinked.service','[Unit]\nDescription=ISOLATED synthetic Observer fixture\n[Service]\nExecStart='+str(old_path)+' pod-observer-unlinked\nRestart=on-failure\n[Install]\nWantedBy=multi-user.target\n',0o644)
run(['systemctl','daemon-reload']);run(['systemctl','enable','--now','siemcore-pod-unlinked.service'])
binding=dict(protocol=PROTOCOL,operation_id=str(uuid.uuid4()),machine_id=machine,installation_id=policy['installation_id'],updater_instance_id=policy['updater_instance_id'],server_type='observer-unlinked',bootstrap_policy_sha256=policy_sha,predecessor=previous,target=target,signing_public_key_sha256=hashlib.sha256(public).hexdigest(),ui_protection_required=True)
directory=Path('/var/lib/siemcore-observer-update/operations')/binding['operation_id'];directory.mkdir(parents=True,mode=0o700)
host=Host(directory)
for attempt in range(30):
 try:host.probe(binding,'predecessor');break
 except Exception:time.sleep(.1)
else:raise AssertionError('fixture predecessor health failed')

if mode=='installed-cli':
 Path('/var/lib/siemcore-observer-update').chmod(0o700)
 Path('/var/lib/siemcore-observer-update/operations').chmod(0o700)
 component=Path('/usr/local/libexec/siemcore-observer-update/v1')
 shutil.copytree('/fixture/adapter',component);component.chmod(0o700)
 files={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in component.glob('*.py')}
 manifest=json.dumps(dict(protocol=PROTOCOL,files=files),sort_keys=True).encode()
 write(component/'COMPONENT.json',manifest)
 write('/etc/siemcore-cascade-updater/observer-update-policy.json',json.dumps(dict(protocol=PROTOCOL,enabled=True,component_manifest_sha256=hashlib.sha256(manifest).hexdigest(),bootstrap_policy_sha256=policy_sha,bootstrap_binary_sha256=previous['binary_sha256'],product_channel='fixture')))
 write('/etc/siemcore-cascade-updater/config.yaml','products:\n  - name: siemcore\n    server_type: observer-unlinked\n    channel: fixture\n')
 def cli(action,request):
  result=subprocess.run(['/usr/bin/python3','-I',str(component/'cli.py'),action],input=json.dumps(request),text=True,capture_output=True)
  assert result.returncode==0,(action,result.stderr)
  return json.loads(result.stdout)
 ready=cli('readiness',dict(protocol=PROTOCOL));assert ready['eligible_for_security_upgrade'] and not ready['ui_security_compliant']
 def refused():
  p=subprocess.run(['/usr/bin/python3','-I',str(component/'cli.py'),'readiness'],input=json.dumps(dict(protocol=PROTOCOL)),text=True,capture_output=True)
  assert p.returncode!=0,'unqualified readiness advertised'
 # Installed boundary rejects component tampering, wrong role and linking,
 # without invoking a product operation or changing the running service.
 initial_pid=run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID'])
 module=component/'worker.py';original=module.read_bytes();module.write_bytes(original+b'\n# fixture tamper\n');refused();module.write_bytes(original)
 config=Path('/etc/siemcore-cascade-updater/config.yaml');original_config=config.read_bytes();config.write_bytes(original_config.replace(b'observer-unlinked',b'normal'));refused();config.write_bytes(original_config)
 link=write('/etc/siemcore-pod-observer/link.json','{}');refused();link.unlink()
 assert run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID'])==initial_pid
 def execute(action):
  return cli(action,dict(protocol=PROTOCOL,operation_id=binding['operation_id'],target=dict(version=target['version'],sha256=target['artifact_sha256'],signature=target['artifact_signature'])))
else:
 def execute(action):
  with open('/var/lib/siemcore-greenfield/hook.lock','a') as lock:
   fcntl.flock(lock,fcntl.LOCK_EX)
   coordinator=Adapter(directory,host.verify_binding,host.stage,lambda a,b,bundles,d:invoke(a,b,bundles,d,lock.fileno(),60),host.probe,host.stopped,protected,host.preservation)
   return coordinator.run(action,binding)

if mode=='interrupted':
 pid=os.fork()
 if pid==0:
  try:execute('apply')
  finally:os._exit(0)
 deadline=time.monotonic()+20
 while time.monotonic()<deadline:
  p=directory/'product-journal.json'
  if p.exists() and json.loads(p.read_text()).get('phase')=='verifying':break
  time.sleep(.01)
 else:raise AssertionError('interruption checkpoint not reached')
 os.kill(pid,signal.SIGKILL);os.waitpid(pid,0);time.sleep(6)
 result=execute('status');assert result['phase']=='accepted',result
else:
 result=execute('apply')
 if mode=='health-failure':
  assert result['phase']=='recovery_required',result
  result=execute('recover');assert result['phase']=='blocked' and result['error_code']=='predecessor_ui_unprotected',result
  assert host.stopped();assert execute('status')['phase']=='blocked'
 else:assert result['phase']=='accepted',result
if mode!='health-failure':
 before=(directory/'adapter-journal.json').read_bytes();pid=run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID'])
 assert execute('apply')['phase']=='accepted'
 assert (directory/'adapter-journal.json').read_bytes()==before and run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID'])==pid
if mode=='installed-cli':
 ready=cli('readiness',dict(protocol=PROTOCOL));assert ready['ui_security_compliant'] and not ready['eligible_for_security_upgrade'] and ready['capabilities']==[]
print(json.dumps({'fixture':mode,'phase':result['phase'],'root_and_exact_product_worker':True,'real_systemd':True,'synthetic_signed_executable':True,'network':'none','public_artifact_or_live_host':False}),flush=True)
