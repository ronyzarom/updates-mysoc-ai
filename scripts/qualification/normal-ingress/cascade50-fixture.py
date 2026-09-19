"""Isolated synthetic fixture: signed .47 -> .50 via actual systemd updater."""
import base64,copy,hashlib,json,os,ssl,subprocess,sys,threading,time,uuid
from pathlib import Path
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from urllib.parse import urlparse
import yaml
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization
assert os.geteuid()==0 and os.environ.get('NORMAL_CLEAN_FIXTURE')=='1'
root=Path('/etc/normal-qualification');cfg=Path('/etc/siemcore-cascade-updater/config.yaml')
app=json.loads(Path('/etc/siemcore/greenfield.json').read_text());c=yaml.safe_load(cfg.read_text())
assert app['cluster_id']=='normal-fixture' and app['updater_instance_id']==c['instance']['id']=='fixture-normal-updater'
kit_result=json.loads((root/'kit-result.json').read_text());assert kit_result['first_install_by_kit']
baseline_installation=kit_result['installed_identity'];assert baseline_installation=={'server_type':'normal'}
assert c['self_update']['channel']=='stable'
protected=[Path('/etc/siemcore/updater-bootstrap.json'),Path('/etc/siemcore/greenfield.json'),Path('/etc/siemcore/greenfield-release.json')]
protected_before={str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in protected}
hook=Path('/usr/local/lib/siemcore-cascade/greenfield-hook.py')
assert hashlib.sha256(hook.read_bytes()).hexdigest()=='329e855866399e59e62219fcbd7d8b59087b472f60253c6ae95fe43e8ccedea8'
service='siemcore-cascade-updater';subprocess.run(['systemctl','stop',service],check=True)
key=Ed25519PrivateKey.from_private_bytes((root/'fixture-signing-key.raw').read_bytes())
public=key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
bootstrap=json.loads(Path('/etc/siemcore/greenfield-release.json').read_text())
assert bootstrap['public_key']==public
version='3.3.152.50';digest='9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc'
archive=Path('/tmp/siemcore-universal-'+version+'.tar.gz')
assert hashlib.sha256(archive.read_bytes()).hexdigest()==digest
signature=base64.b64encode(key.sign(('mysoc-release-v1\nsiemcore\n'+version+'\n'+digest).encode())).decode()
source=Path('/usr/local/lib/siemcore-cascade/recovery');sys.path.insert(0,str(source))
import recovery as r
import port_normalization as n
assert r.VERSION=='1.0.0.5'
r.save_json(root/'cascade50-baseline.json',dict(installation=baseline_installation,protected_receipt_sha256=protected_before))
container=json.loads(subprocess.check_output(['docker','inspect','siemcore-app-a']))[0]
bindings=copy.deepcopy(container['NetworkSettings']['Ports'])
install=Path('/opt/siemcore-app-app-a')
plan=dict(protocol='normal-ingress-retention-v1',operation_id=str(uuid.uuid4()),machine_id=Path('/etc/machine-id').read_text().strip(),updater_id=app['updater_instance_id'],identity_files={str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in (Path('/etc/siemcore/greenfield.json'),cfg)},runtime_files={p:hashlib.sha256((source/p).read_bytes()).hexdigest() for p in n.RUNTIME_FILES},from_version='3.3.152.47',from_sha256='7cf8b7b0011344ca8f568bb023d6bd69e0f01fc9dbd3e14271727da0b8172812',target_version=version,target_sha256=digest,container_id=container['Id'],image_id=container['Image'],files={p:hashlib.sha256((install/p).read_bytes()).hexdigest() if (install/p).exists() else None for p in r.FILES},host_bindings=container['HostConfig']['PortBindings'],published_bindings=bindings)
r.save_json(n.AUTH,dict(plan=plan,signature=base64.b64encode(key.sign(n.DOMAIN+n.canonical(plan))).decode()))
records={'reports':[],'heartbeats':[],'downloads':0,'self_checks':0}
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def reply(self,value):
  body=json.dumps(value).encode();self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
 def offer(self):
  self.reply(dict(update_available=not records['reports'],latest_version=version,download_url='/artifact',sha256=digest,signature=signature,channel=c['products'][0]['channel'],update_group='alpha'))
 def do_GET(self):
  path=urlparse(self.path).path
  if path=='/artifact':
   records['downloads']+=1;body=archive.read_bytes();self.send_response(200);self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
  elif path=='/api/v1/updates/siemcore/check':self.offer()
  else:self.send_error(404)
 def do_POST(self):
  value=json.loads(self.rfile.read(int(self.headers['Content-Length'])));path=urlparse(self.path).path
  if path=='/api/v1/updates/siemcore/check':self.offer();return
  if path=='/api/v1/heartbeat':records['heartbeats'].append(value)
  elif path=='/api/v1/updates/siemcore/report':records['reports'].append(value)
  elif path.startswith('/api/v1/updates/updater-') and path.endswith('/check'):
   records['self_checks']+=1;self.reply(dict(update_available=False));return
  else:self.send_error(404);return
  self.reply(dict(status='ok',relay_token='disposable-relay-token',update_group='alpha'))
url=urlparse(c['server']['url']);assert url.hostname=='normal.fixture'
server=ThreadingHTTPServer(('127.0.0.1',url.port),Handler)
context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);context.load_cert_chain(str(root/'credentials/server.crt'),str(root/'credentials/server.key'));server.socket=context.wrap_socket(server.socket,server_side=True)
thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
try:
 subprocess.run(['systemctl','start',service],check=True)
 deadline=time.monotonic()+900
 while not records['reports'] and time.monotonic()<deadline:time.sleep(1)
 r.save_json(root/'cascade50-records.json',records)
 assert records['reports'],'no cascade report'
 assert records['reports'][0]['success'] is True,'cascade application failed; protected report retained'
 health=json.loads(subprocess.check_output(['curl','-fsS','http://127.0.0.1:8443/health']))
 assert health['version']==version and health['status']=='healthy'
 actual=json.loads(subprocess.check_output(['docker','inspect','siemcore-app-a']))[0]['NetworkSettings']['Ports'];assert actual==bindings
 state=json.loads(Path('/var/lib/siemcore-cascade-updater/state.json').read_text());assert state['product_versions']['siemcore']==version
 tx=json.loads(Path('/var/lib/siemcore-recovery/transaction.json').read_text());assert tx['stage']=='applied' and tx['target']==version and 'port_normalization' in tx
 assert hashlib.sha256(cfg.read_bytes()).hexdigest()==plan['identity_files'][str(cfg)]
 result=dict(synthetic_only=True,actual_systemd_cascade=True,from_version='3.3.152.47',target_version=version,artifact_sha256=digest,signature_verified_by_updater=True,exact_endpoints_preserved=True,normal_identity_preserved=state['siemcore_installation']==baseline_installation,recovery_stage=tx['stage'],health=health)
 assert result['normal_identity_preserved']
 assert protected_before=={str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in protected}
 result.update(protected_receipt_sha256=protected_before,runtime_sha256=plan['runtime_files'],hook_sha256=hashlib.sha256(hook.read_bytes()).hexdigest())
 r.save_json(root/'cascade50-result.json',result);print(json.dumps(result),flush=True)
finally:
 subprocess.run(['systemctl','stop',service],check=True)
 server.shutdown();server.server_close();thread.join()
