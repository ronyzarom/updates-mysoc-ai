#!/usr/bin/env python3
"""Drive the actual CLI against a loopback fixture server in a clean signed root."""
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import pwd
import shutil
import ssl
import subprocess
import threading
import time
from urllib.parse import urlparse

if os.geteuid()!=0 or os.environ.get('INDEPENDENT_NODE_FIXTURE')!='1':
    raise SystemExit('explicit disposable root fixture required')
root=Path('/etc/node-qualification')
data=json.loads((root/'envelope.json').read_text())
release=data['release'];app=data['application']
if Path('/var/lib/siemcore-greenfield/journal.json').exists():
    raise SystemExit('first-install CLI fixture refuses existing product journal')
records={'heartbeats':[],'reports':[],'downloads':0,'self_checks':[]}
class Handler(BaseHTTPRequestHandler):
    def log_message(self,*args):pass
    def reply(self,value):
        encoded=json.dumps(value).encode();self.send_response(200)
        self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(encoded)))
        self.end_headers();self.wfile.write(encoded)
    def do_GET(self):
        path=urlparse(self.path).path
        if path=='/artifact':
            records['downloads']+=1
            self.send_response(200);self.send_header('Content-Length',str((root/'release.tar.gz').stat().st_size));self.end_headers()
            with (root/'release.tar.gz').open('rb') as source:shutil.copyfileobj(source,self.wfile)
        elif path=='/api/v1/updates/siemcore/check':
            self.reply(dict(update_available=not records['reports'],latest_version=release['version'],download_url='/artifact',
                sha256=release['sha256'],signature=release['signature'],channel='stable',update_group='alpha'))
        else:self.send_error(404)
    def do_POST(self):
        value=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        path=urlparse(self.path).path
        if path=='/api/v1/updates/siemcore/check':
            self.reply(dict(update_available=not records['reports'],latest_version=release['version'],download_url='/artifact',sha256=release['sha256'],signature=release['signature'],channel='stable',update_group='alpha'))
            return
        if path=='/api/v1/heartbeat':records['heartbeats'].append(value)
        elif path=='/api/v1/updates/siemcore/report':records['reports'].append(value)
        elif path.startswith('/api/v1/updates/updater-') and path.endswith('/check'):
            records['self_checks'].append(value)
            self.reply(dict(update_available=False));return
        else:self.send_error(404);return
        self.reply(dict(status='ok', relay_token='disposable-relay-token', update_group='alpha'))
server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.load_cert_chain(app['management']['certificate'],app['management']['key'])
server.socket=tls.wrap_socket(server.socket,server_side=True)
thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
service='siemcore-cascade-updater'
kit=Path('/opt/fixture-kit')
if Path('/var/lib/siemcore-greenfield/journal.json').exists() or any(Path('/opt').glob('siemcore-node-unlinked-*')):
    raise SystemExit('kit first install refuses existing product state')
# The product inputs-only fixture must not preinstall policies or staging.
assert not Path('/etc/siemcore/greenfield-release.json').exists()
assert not Path('/opt/siemcore-cascade').exists()
command=[str(kit/'install.sh'),'--clean','--license-key','disposable-fixture',
    '--parent-url','https://'+app['management']['hostname']+':'+str(server.server_port),
    '--instance-id',app['updater_instance_id'],'--parent-id','fixture-parent',
    '--customer-id','fixture-customer','--customer-name','Disposable fixture',
    '--signing-key',release['public_key'],'--server-type','pod-node','--node-id',app['node_id'],
    '--ca-file',str(root/'credentials/ca.crt'),'--greenfield-input',str(root/'envelope.json')]
try:
    result=subprocess.run(command,capture_output=True,text=True,timeout=90)
    (root/'kit-install.log').write_text(result.stdout+result.stderr)
    if result.returncode:raise RuntimeError('installer failed: '+result.stdout[-2000:]+result.stderr[-2000:])
    deadline=time.monotonic()+600
    while not records['reports'] and time.monotonic()<deadline:time.sleep(1)
    assert records['reports'], 'no apply report within timeout'
    assert records['reports'][0]['success'] is True, records['reports'][0]
    containers_before=subprocess.check_output(['docker','ps','-q'],text=True).splitlines()
    retry=subprocess.run(command,capture_output=True,text=True,timeout=90)
    (root/'kit-retry.log').write_text(retry.stdout+retry.stderr)
    assert retry.returncode==0, 'exact installer retry failed'
    assert subprocess.check_output(['docker','ps','-q'],text=True).splitlines()==containers_before
    subprocess.run(['systemctl','restart',service],check=True,timeout=90)
    heartbeats=len(records['heartbeats'])
    deadline=time.monotonic()+80
    while len(records['heartbeats'])<=heartbeats and time.monotonic()<deadline:time.sleep(1)
    assert len(records['heartbeats'])>heartbeats
    assert records['heartbeats'][-1]['installation']==dict(kind='pod-node',node_id=app['node_id'])
    assert subprocess.check_output(['systemctl','is-active',service],text=True).strip()=='active'
    config=Path('/etc/'+service+'/config.yaml').read_text()
    assert 'independent_node_bootstrap: true' in config
    assert 'self_update:\n  channel: stable\n' in config and '\n  disabled: true' not in config
    assert subprocess.check_output(['systemctl','show',service,'--property=ProtectHome','--value'],text=True).strip()=='yes'
    health=json.loads(subprocess.check_output(['curl','--fail','--silent','https://'+app['management']['hostname']+'/health/live'],text=True))
    (root/'kit-health.json').write_text(json.dumps(health,indent=2))
    saved=json.loads(Path('/var/lib/'+service+'/state.json').read_text())
    assert saved['product_versions']['siemcore']==release['version']
    receipt=dict(node_id=app['node_id'],first_install_by_kit=True,kit_version='1.16.1.28-r1',architecture='arm64',
        updater_version=subprocess.check_output(['/usr/local/bin/'+service,'version'],text=True).strip(),
        signature_required=True,archive_sha256=release['sha256'],success_report=records['reports'][0],
        installed_identity=saved['siemcore_installation'],restart_heartbeat_identity=records['heartbeats'][-1]['installation'],
        service_active=True,protect_home=True,self_update_channel='stable',self_update_enabled=True,
        exact_install_retry_preserved_containers=True,self_checks=records['self_checks'],alpha_fixture_only=True)
    (root/'kit-result.json').write_text(json.dumps(receipt,indent=2))
    print('PASS: literal kit installer, systemd relay, signed first apply, restart/identity/health; node '+app['node_id'])
finally:
    server.shutdown();server.server_close();thread.join()
