#!/usr/bin/env python3
"""Exact .47/.50 isolated native recovery drill; synthetic fixture only.

Run inside the disposable Normal qualification fixture after its literal kit
bootstrap succeeds. Never run on an enrolled host or against an existing DB.
Artifacts must be original signed-release bytes; disposable keys sign the local
qualification policy only. This exercises product recovery, not fleet release.
"""
import base64
import copy
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import uuid
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization

if os.geteuid()!=0 or os.environ.get('NORMAL_CLEAN_FIXTURE')!='1' or platform.machine() not in ('x86_64','amd64'):
    raise SystemExit('explicit native root qualification fixture required')
root=Path('/etc/normal-qualification')
app=json.loads(Path('/etc/siemcore/greenfield.json').read_text())
if app.get('cluster_id')!='normal-fixture' or app.get('updater_instance_id')!='fixture-normal-updater':
    raise SystemExit('refusing non-synthetic installation')
source=Path('/usr/local/lib/siemcore-cascade/recovery')
sys.path.insert(0,str(source))
import recovery as r
import port_normalization as n
original_popen=subprocess.Popen
class CaptureFailedApply(original_popen):
    def communicate(self,*args,**kwargs):
        result=super().communicate(*args,**kwargs)
        if self.returncode and any(str(x).endswith('/updater/apply') for x in self.args):
            fd=os.open('/root/port50-failed-apply-output.log',os.O_WRONLY|os.O_CREAT|os.O_APPEND,0o600)
            with os.fdopen(fd,'ab') as f:
                for output in result:
                    if output:f.write(output if isinstance(output,bytes) else output.encode())
        return result
subprocess.Popen=CaptureFailedApply
subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True)
versions=('3.3.152.47','3.3.152.50')
hashes=('7cf8b7b0011344ca8f568bb023d6bd69e0f01fc9dbd3e14271727da0b8172812',
        '9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc')
direct='--direct-normalization' in sys.argv
artifacts=[Path(p) for p in sys.argv[1:] if p!='--direct-normalization']
assert len(artifacts)==2
key=Ed25519PrivateKey.generate()
public=key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
entries={}
for version,digest,path in zip(versions,hashes,artifacts):
    assert hashlib.sha256(path.read_bytes()).hexdigest()==digest
    entries[version]={'artifact':str(path),'sha256':digest,
        'signature':base64.b64encode(key.sign(('mysoc-release-v1\nsiemcore\n'+version+'\n'+digest).encode())).decode()}
install=Path('/opt/siemcore-app-app-a');state=root/'ingress-retention';state.mkdir(mode=0o700)
policy=dict(schema=1,install_dir=str(install),journal_dir=str(state),cluster_id=app['cluster_id'],node_id='app-a',
    project_name='siemcore-app-app-a',containers={'siemcore':'siemcore-app-a','siemcore-archiver':'siemcore-archiver-app-a'},
    allow_dynamic_archiver_debug_port=True,health_url='http://127.0.0.1:8443/health',dashboard_dir=None,
    public_key=public,allowed_transitions=[list(versions)],releases=entries,health_timeout=120,apply_timeout=480,
    cascade_root='/opt/siemcore-cascade')
l=r.Lifecycle(policy)
containers=l.inspect();c=containers['siemcore-app-a']
initial_bindings=copy.deepcopy(c['NetworkSettings']['Ports'])
plan=dict(protocol='normal-ingress-retention-v1',operation_id=str(uuid.uuid4()),machine_id=Path('/etc/machine-id').read_text().strip(),
    updater_id=app['updater_instance_id'],identity_files={p:hashlib.sha256(Path(p).read_bytes()).hexdigest() for p in
        ('/etc/siemcore/greenfield.json','/etc/siemcore-cascade-updater/config.yaml')},
    runtime_files={p:hashlib.sha256((source/p).read_bytes()).hexdigest() for p in n.RUNTIME_FILES},
    from_version=versions[0],from_sha256=hashes[0],target_version=versions[1],target_sha256=hashes[1],
    container_id=c['Id'],image_id=c['Image'],files={p:hashlib.sha256((install/p).read_bytes()).hexdigest() if (install/p).exists() else None for p in r.FILES},
    host_bindings=c['HostConfig']['PortBindings'],published_bindings=c['NetworkSettings']['Ports'])
r.save_json(n.AUTH,dict(plan=plan,signature=base64.b64encode(key.sign(n.DOMAIN+n.canonical(plan))).decode()))
original_env=(install/'.env').read_bytes()
# Real preflight verifies signatures and renders the pinned compose without
# changing the installed configuration or containers.
with tempfile.TemporaryDirectory(dir=state) as tmp:
    probe=r.Lifecycle(dict(policy,journal_dir=tmp));tx=probe.snapshot(versions[1])
    assert 'port_normalization' in tx
assert (install/'.env').read_bytes()==original_env and l.inspect()['siemcore-app-a']['Id']==c['Id']
print('PASS signed native preflight; no installed mutation',flush=True)
# Crash at the durable intent boundary: fresh runtime restores the explicitly
# pinned predecessor, never the old dynamic .env.
if not direct:
    tx=l.snapshot(versions[1]);l.stage(tx,'applying');l.verify_ingress_intent(tx);l.pin_ingress(tx)
    fresh=r.Lifecycle(policy);fresh.rollback()
    assert fresh.inspect()['siemcore-app-a']['NetworkSettings']['Ports']==initial_bindings
    assert json.loads(fresh.journal.read_text())['stage']=='restored'
    print('PASS simulated crash after pinning; pinned predecessor recovered',flush=True)
else:
    fresh=r.Lifecycle(policy)
# Once normalized, normal strict recovery runs with no dynamic authorization.
# Saved authorization no longer matches changed container or env; no code path
# may use it as an exemption.
fresh.apply(versions[1]);tx=json.loads(fresh.journal.read_text())
assert tx['stage']=='applied' and ('port_normalization' in tx)==direct
assert fresh.inspect()['siemcore-app-a']['NetworkSettings']['Ports']==initial_bindings
receipt=fresh.journal.read_bytes();fresh.apply(versions[1]);assert fresh.journal.read_bytes()==receipt
print('PASS exact .47 -> .50 signed apply and idempotent retry',flush=True)
fresh.rollback();fresh.rollback()
assert fresh.inspect()['siemcore-app-a']['NetworkSettings']['Ports']==initial_bindings
assert fresh.probe(policy['health_url'])['version']==versions[0]
print('PASS signed predecessor rollback twice; exact customer endpoints retained',flush=True)
# Force an actual Docker port collision only inside this isolated fixture.
# Hold the old TCP ingress while product apply recreates the app. Remove the
# synthetic holder when apply fails so the same transaction can auto-rollback.
original_run=fresh.run
collision='normal-fixture-ingress-collision'
injected=[False]
def collide(args,**kwargs):
    if not injected[0] and len(args)>1 and str(args[0]).endswith('/updater/apply') and args[1]=='apply':
        injected[0]=True
        original_run(['docker','stop','siemcore-app-a'])
        endpoint=initial_bindings['1514/tcp'][0]
        image=json.loads(original_run(['docker','inspect','siemcore-lb-a']))[0]['Image']
        original_run(['docker','run','-d','--pull','never','--name',collision,'-p',
                      endpoint['HostIp']+':'+endpoint['HostPort']+':80/tcp',image])
        try:return original_run(args,**kwargs)
        finally:original_run(['docker','rm','-f',collision])
    return original_run(args,**kwargs)
fresh.run=collide
try:
    fresh.apply(versions[1])
    raise AssertionError('real port collision unexpectedly accepted')
except r.a.RecoveryError:
    assert injected[0] and json.loads(fresh.journal.read_text())['stage']=='restored'
    assert fresh.inspect()['siemcore-app-a']['NetworkSettings']['Ports']==initial_bindings
    assert fresh.probe(policy['health_url'])['version']==versions[0]
finally:
    fresh.run=original_run
print('PASS real port collision refused; automatic pinned predecessor rollback verified',flush=True)
# Restore target for final health; existing rollback journal remains available.
fresh.apply(versions[1])
assert fresh.inspect()['siemcore-app-a']['NetworkSettings']['Ports']==initial_bindings
result=dict(native_architecture=platform.machine(),from_version=versions[0],target_version=versions[1],
    artifact_sha256=dict(zip(versions,hashes)),preflight_read_only=True,pinned_crash_recovery=not direct,
    direct_normalization_apply=direct,
    port_collision_rollback=True,
    exact_endpoints_preserved=True,apply_retry=True,rollback_retry=True,final_health=fresh.probe(policy['health_url']),
    synthetic_fixture_only=True,no_database_copy=True)
result_path=root/('ingress-retention-'+('direct' if direct else 'crash')+'-result.json')
result_path.write_text(json.dumps(result,indent=2)+'\n')
print('PASS final target healthy; result '+str(result_path),flush=True)
