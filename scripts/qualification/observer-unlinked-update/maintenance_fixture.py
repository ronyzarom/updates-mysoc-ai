"""Run after the isolated signed fixture setup, before product mutation."""
import sys,json,hashlib,shutil,subprocess,os,pwd
from pathlib import Path
sys.path.insert(0,'/fixture/adapter')
from protocol import PROTOCOL

def qualify(namespace,failure=False):
 write=namespace['write'];run=namespace['run'];key=namespace['key'];public=namespace['public'];base64=namespace['base64']
 root=Path('/fixture/maintenance-kit');shutil.copytree('/fixture/adapter',root/'component')
 (root/'component').chmod(0o700)
 files={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (root/'component').glob('*.py')}
 write(root/'component/COMPONENT.json',json.dumps(dict(protocol=PROTOCOL,files=files)))
 shutil.copyfile('/fixture/install.py',root/'install.py')
 name='siemcore-cascade-updater'
 try:account=pwd.getpwnam(name)
 except KeyError:run(['useradd','--system','--no-create-home',name]);account=pwd.getpwnam(name)
 old=write('/var/lib/'+name+'/self-update/releases/1.16.1.26/'+name,'#!/bin/sh\necho "updater 1.16.1.26"\n',0o755)
 layout=old.parents[2];(layout/'current').symlink_to(old.parent)
 write('/usr/local/sbin/siemcore-apply-update','#!/bin/sh\nexit 97\n',0o755)
 binary=write(root/'updater-linux-amd64','#!/bin/sh\necho "updater 1.16.1.27"\n',0o755);checksum=hashlib.sha256(binary.read_bytes()).hexdigest()
 sig=base64.b64encode(key.sign(('mysoc-release-v1\nupdater-linux-amd64\n1.16.1.27\n'+checksum).encode())).decode()
 config=write('/etc/'+name+'/config.yaml','products:\n  - name: siemcore\n    server_type: "observer-unlinked"\n    channel: fixture\nsimulation:\n  filesystem:\n    install_root: /opt/siemcore-cascade\nself_update:\n  channel: stable\n',0o640)
 before=config.read_bytes()
 write('/etc/systemd/system/'+name+'.service','[Service]\nExecStart=/usr/bin/sleep infinity\n[Install]\nWantedBy=multi-user.target\n',0o644)
 run(['systemctl','daemon-reload']);run(['systemctl','start',name])
 package=dict(updater_version='1.16.1.27',updater_receipt=dict(version='1.16.1.27',sha256=checksum,signature=sig),allowed_predecessor_updater_sha256=hashlib.sha256(old.read_bytes()).hexdigest(),allowed_bootstrap_hook_sha256=hashlib.sha256(Path('/usr/local/sbin/siemcore-apply-update').read_bytes()).hexdigest())
 if failure:
  # Valid signed/packaged component that fails *installed* readiness, after
  # maintenance has quiesced the updater and installed root-owned files.
  p=root/'component/cli.py';p.write_text(p.read_text().replace('action=sys.argv[1];policy,app=admission()',"raise ValueError('fixture installed readiness failure')"))
  files['cli.py']=hashlib.sha256(p.read_bytes()).hexdigest();write(root/'component/COMPONENT.json',json.dumps(dict(protocol=PROTOCOL,files=files)))
 package['files']={str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in root.rglob('*') if p.is_file()}
 write(root/'PACKAGE.json',json.dumps(package));root.chmod(0o700)
 product_pid=run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID','--value'])
 result=subprocess.run(['python3',str(root/'install.py')],capture_output=True,text=True)
 assert (result.returncode!=0)==failure,(result.stdout,result.stderr)
 assert run(['systemctl','show','siemcore-pod-unlinked.service','-p','MainPID','--value'])==product_pid
 assert run(['systemctl','is-active',name]).strip()=='active'
 if failure:
  assert config.read_bytes()==before and (layout/'current').resolve()==old.parent
  assert not Path('/usr/local/sbin/siemcore-observer-update').exists()
  assert not Path('/etc/siemcore-cascade-updater/observer-update-policy.json').exists()
 else:
  assert b'observer_unlinked_update: true' in config.read_bytes()
  assert (layout/'current').resolve().name=='1.16.1.27'
  assert json.loads(result.stdout)['product_execution'] is False
 if not failure and Path('/fixture/updatersim.test').exists():
  target=namespace['target'];q=dict(protocol=PROTOCOL,operation_id=namespace['binding']['operation_id'],target=dict(version=target['version'],sha256=target['artifact_sha256'],signature=target['artifact_signature']))
  write('/fixture/native-request.json',json.dumps(q),0o644)
  print(run(['runuser','-u',name,'--','env','OBSERVER_NATIVE_TEST=1','/fixture/updatersim.test','-test.run=^TestObserverNativeCommandIntegration$','-test.v']))
 print(json.dumps(dict(fixture='maintenance-rollback' if failure else 'maintenance-success',product_pid_unchanged=True,updater_active=True,synthetic_updater=True)))
