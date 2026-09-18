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
from urllib.parse import urlparse

if os.geteuid()!=0 or os.environ.get('INDEPENDENT_NODE_FIXTURE')!='1':
    raise SystemExit('explicit disposable root fixture required')
root=Path('/root/node-qualification')
data=json.loads((root/'envelope.json').read_text())
release=data['release'];app=data['application']
if Path('/var/lib/siemcore-greenfield/journal.json').exists():
    raise SystemExit('first-install CLI fixture refuses existing product journal')
records={'heartbeats':[],'reports':[],'downloads':0}
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
        else:self.send_error(404);return
        self.reply(dict(status='ok'))
server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.load_cert_chain(app['management']['certificate'],app['management']['key'])
server.socket=tls.wrap_socket(server.socket,server_side=True)
thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
state=Path('/var/lib/node-updater-cli-fixture');state.mkdir(mode=0o700,exist_ok=True)
user=pwd.getpwnam('node-updater-fixture');os.chown(state,user.pw_uid,user.pw_gid)
command=['sudo','-n','/usr/local/sbin/siemcore-apply-update']
config=dict(server=dict(url='https://'+app['management']['hostname']+':'+str(server.server_port),api_key='disposable-fixture',timeout='60s'),
    instance=dict(id=app['updater_instance_id'],type='server',machine_id=app['machine_id'],product_tier='siemcore',parent_id='fixture-parent',customer_id='fixture-customer',customer_name='Disposable fixture'),
    signing=dict(require=True,public_key=release['public_key']),self_update=dict(disabled=True,channel='stable'),
    simulation=dict(mode='real',executor='filesystem',artifact_dir='/var/lib/siemcore-cascade-updater/artifacts',state_file=str(state/'state.json'),max_download_bytes=1073741824,
        filesystem=dict(independent_node_bootstrap=True,install_root='/opt/siemcore-cascade',restart_command=command,health_command=command,command_timeout='10m',keep_releases=3)),
    products=[dict(name='siemcore',server_type='pod-node',node_id=app['node_id'],current_version='0.0.0',channel='stable')])
path=Path('/fixture/cli-config.yaml');path.write_text(json.dumps(config));path.chmod(0o644)
try:
    for cycle in (1,2):
        result=subprocess.run(['runuser','-u',user.pw_name,'--','/fixture/updater','--config',str(path),'once'],capture_output=True,text=True,timeout=800)
        log=root/('updater-cli-'+str(cycle)+'.log');log.write_text(result.stdout+result.stderr);log.chmod(0o600)
        if result.returncode:
            raise RuntimeError('CLI cycle failed: '+result.stderr[-3000:])
    assert records['downloads']==1
    assert len(records['reports'])==1 and records['reports'][0]['success'] is True
    assert records['reports'][0]['to_version']==release['version']
    saved=json.loads((state/'state.json').read_text())
    assert saved['product_versions']['siemcore']==release['version']
    assert saved['siemcore_installation']['server_type']=='pod-node'
    assert saved['siemcore_installation']['node_id']==app['node_id']
    assert len(records['heartbeats'])==2
    assert records['heartbeats'][-1]['installation']==dict(kind='pod-node',node_id=app['node_id'])
    receipt=dict(node_id=app['node_id'],first_install_by_cli=True,signature_required=True,archive_sha256=release['sha256'],
        updater_binary_sha256=hashlib.sha256(Path('/fixture/updater').read_bytes()).hexdigest(),
        success_report=records['reports'][0],installed_identity=saved['siemcore_installation'],restart_heartbeat_identity=records['heartbeats'][-1]['installation'])
    (root/'updater-cli-result.json').write_text(json.dumps(receipt,indent=2))
    print('PASS: real updater CLI first install, signed download, protected root execution, report, restart and identity heartbeat; node '+app['node_id'])
finally:
    server.shutdown();server.server_close();thread.join()
