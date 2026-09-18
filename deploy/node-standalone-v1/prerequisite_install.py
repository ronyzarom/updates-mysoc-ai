#!/usr/bin/env python3
"""Signed GCP-startup prerequisite provisioning; never executes product code.

The startup verifier must authenticate the outer kit before invoking this file.
enroll emits public encryption material and hashes only; install consumes a
signed encrypted capsule. No application/database commands or generic sudo rule.
"""
import base64
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import urllib.request

KIT=Path(__file__).resolve().parent
sys.path.insert(0,str(KIT/'component'))
from adapter import atomic_json
from capsule import decrypt_capsule
from configuration_inventory import configuration_digest
from protocol import strict_json
from source_loader import SourceLoader
from transaction import canonical, digest, validate_binding

ROOT=Path('/var/lib/siemcore-node-standalone')
INPUTS=Path('/etc/siemcore-cascade-updater/standalone-inputs')
CONFIG=Path('/etc/siemcore-cascade-updater/config.yaml')
POLICY=CONFIG.parent/'node-standalone-policy.json'
WRAPPER=Path('/usr/local/sbin/siemcore-node-standalone')
SUDO=Path('/etc/sudoers.d/siemcore-node-standalone')
SERVICE='siemcore-cascade-updater'
COMPONENT_ROOT=Path('/usr/local/libexec/siemcore-node-standalone')


def put(path,raw,mode=0o600,uid=0,gid=0):
    import tempfile
    fd,tmp=tempfile.mkstemp(prefix='.standalone-',dir=path.parent)
    try:
        with os.fdopen(fd,'wb') as out:
            os.fchmod(out.fileno(),mode);os.fchown(out.fileno(),uid,gid)
            out.write(raw);out.flush();os.fsync(out.fileno())
        os.replace(tmp,path)
        parent=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
        try:os.fsync(parent)
        finally:os.close(parent)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)


def run(args):
    result=subprocess.run(args,check=True,capture_output=True,timeout=60)
    return result.stdout.decode().strip()


def metadata_id():
    request=urllib.request.Request('http://169.254.169.254/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
    with urllib.request.urlopen(request,timeout=5) as response:
        return response.read(128).decode().strip()


def verify_package(loader):
    package=strict_json(loader.protected(KIT/'PACKAGE.json').read_bytes())
    for name,wanted in package['files'].items():
        if not re.fullmatch(r'(?:component/)?[A-Za-z0-9_.-]+',name):raise ValueError('invalid_package_path')
        if hashlib.sha256(loader.protected(KIT/name).read_bytes()).hexdigest()!=wanted:raise ValueError('package_integrity_mismatch')
    app=strict_json(loader.read('/etc/siemcore/greenfield.json'))
    identity={k:app[k] for k in ('machine_id','installation_id','updater_instance_id','node_id')}
    identity['vm_id']=metadata_id()
    if identity!=package['identity'] or identity['machine_id']!=loader.read('/etc/machine-id').decode().strip() or app.get('schema')!=5 or app.get('topology')!='node-unlinked':
        raise ValueError('signed_host_identity_mismatch')
    return package,app,identity


def enroll(loader,package,app,identity):
    ROOT.mkdir(mode=0o700,exist_ok=True);loader.protected(ROOT)
    recipient=ROOT/'recipient';recipient.mkdir(mode=0o700,exist_ok=True);loader.protected(recipient)
    marker=recipient/'identity.json'
    if marker.exists() and strict_json(loader.protected(marker).read_bytes())!=identity:raise ValueError('recipient_identity_changed')
    if not marker.exists():atomic_json(marker,identity)
    key=recipient/'key.pem';cert=recipient/'certificate.pem'
    if not key.exists():
        if cert.exists():raise ValueError('orphan_recipient_certificate')
        run(['openssl','req','-x509','-newkey','rsa:3072','-nodes','-keyout',str(key),'-out',str(cert),'-days','2','-subj','/CN=Updater Input Recipient'])
        key.chmod(0o600);cert.chmod(0o600)
    loader.protected(key);loader.protected(cert)
    INPUTS.mkdir(mode=0o700,exist_ok=True);loader.protected(INPUTS)
    node=loader.path('/opt/siemcore-node-unlinked-'+identity['node_id'])
    settings=strict_json(loader.protected(node/'settings.json').read_bytes())
    # Retained credential files have known database ownership; their protected
    # parent and exact existing receipt are checked again by product admission.
    def retained(name):
        path=node/'credentials'/name
        st=path.lstat()
        if path.is_symlink() or not path.is_file() or st.st_mode&0o077 or st.st_uid not in (0,settings['postgres_uid']):raise ValueError('retained_credential_protection')
        return path.read_text().strip()
    jwt=recipient/'jwt.secret'
    if not jwt.exists():put(jwt,secrets.token_hex(32).encode())
    env=dict(package['public_environment'])
    if any(k in env for k in ('DB_PASSWORD','REDIS_URL','JWT_SECRET','DB_NAME')):raise ValueError('local_credentials_cannot_be_overridden')
    env.update(DB_NAME=settings['database'],DB_PASSWORD=retained('application'),REDIS_URL='redis://:'+retained('redis')+'@siemcore-unlinked-'+identity['node_id']+'-redis:6379/0',JWT_SECRET=loader.protected(jwt).read_text())
    if any(any(c in str(v) for c in ('\n','\r','\x00','$','"',"'")) for v in env.values()):raise ValueError('unsafe_environment_value')
    files={'application.env':''.join(k+'='+str(v)+'\n' for k,v in sorted(env.items())).encode(),
           'installation/tls.crt':loader.protected(node/'management.crt').read_bytes(),
           'installation/tls.key':loader.protected(node/'management.key').read_bytes()}
    for name,raw in files.items():
        path=INPUTS/name;path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
        if path.exists() and loader.protected(path).read_bytes()!=raw:raise ValueError('enrolled_configuration_changed')
        if not path.exists():put(path,raw)
    history=loader.inventory();evidence,bootstrap,_=loader.measure(history)
    if evidence['current_version']!=package['source_version']:raise ValueError('unexpected_source_version')
    data_identity={}
    for role in ('postgres','redis'):
        record=strict_json(subprocess.check_output(['docker','inspect','--type','container','siemcore-unlinked-'+identity['node_id']+'-'+role],timeout=15))[0]
        if record['State']['Running'] is not True:raise ValueError('data_service_not_running')
        data_identity[role]={'id':record['Id'],'image':record['Image'],
                             'mounts':[{k:m.get(k) for k in ('Type','Source','Destination','RW')} for m in record['Mounts']]}
    report=dict(protocol='pod-node-standalone-enrollment-v1',identity=identity,
                recipient_certificate_pem=cert.read_text(),recipient_sha256=hashlib.sha256(cert.read_bytes()).hexdigest(),
                local_input_sha256={name:hashlib.sha256(raw).hexdigest() for name,raw in files.items()},
                historical_inventory=history,source_evidence=evidence,source_evidence_sha256=digest(evidence),data_identity=data_identity,
                bootstrap_receipt_sha256=hashlib.sha256(bootstrap).hexdigest(),updater_config_sha256=hashlib.sha256(loader.protected(CONFIG).read_bytes()).hexdigest())
    atomic_json(ROOT/'enrollment.json',report)
    return report


def install(loader,package,identity):
    capsule=KIT/'capsule'
    manifest=strict_json(loader.protected(capsule/'manifest.json').read_bytes())
    release=strict_json(loader.read('/etc/siemcore/greenfield-release.json'))
    files,policy=decrypt_capsule(manifest,loader.protected(capsule/'inputs.cms').read_bytes(),
         loader.protected(capsule/'manifest.sig').read_text().strip(),bytes.fromhex(release['public_key']),identity,
         loader.protected(ROOT/'recipient/certificate.pem').read_bytes(),loader.protected(ROOT/'recipient/key.pem'))
    validate_binding(policy['binding'])
    if policy['binding']['source']!={k:v for k,v in identity.items() if k!='vm_id'} or policy['binding']['target']['version']!=package['target_version']:
        raise ValueError('capsule_transition_identity_mismatch')
    version=run(['/usr/local/bin/siemcore-cascade-updater','version']).splitlines()[0].split()[1]
    if tuple(map(int,version.split('.')))<tuple(map(int,package['minimum_updater_version'].split('.'))):raise ValueError('normal_signed_updater_self_update_required')
    current=loader.protected(CONFIG).read_bytes();config_stat=CONFIG.stat()
    original_path=ROOT/'prerequisite-original-config.yaml'
    if not original_path.exists():
        enrollment=strict_json(loader.protected(ROOT/'enrollment.json').read_bytes())
        if hashlib.sha256(current).hexdigest()!=enrollment['updater_config_sha256']:raise ValueError('updater_configuration_changed_since_enrollment')
        put(original_path,current)
    original=loader.protected(original_path).read_bytes()
    text=original.decode()
    if not re.search(r'(?ms)^self_update:.*?^  channel: stable\s*$',text) or re.search(r'^  disabled: true\s*$',text,re.M):raise ValueError('stable_automatic_self_updates_required')
    if re.findall(r'^    channel: (\S+)',text,re.M)!=[package['product_channel']] or len(re.findall(r'^    independent_node_update: true\s*$',text,re.M))!=1 or 'independent_node_standalone:' in text:
        raise ValueError('unexpected_original_updater_config')
    updated=re.sub(r'^    independent_node_update: true\s*$', '    independent_node_update: false\n    independent_node_standalone: true',text,count=1,flags=re.M).encode()
    if not re.fullmatch(r'[A-Za-z0-9_-]+',package['standalone_product_channel']):raise ValueError('invalid_standalone_channel')
    updated=re.sub(r'^    channel: '+re.escape(package['product_channel'])+r'\s*$',
                   '    channel: '+package['standalone_product_channel'],updated.decode(),count=1,flags=re.M).encode()
    if current not in (original,updated):raise ValueError('unrelated_updater_configuration_change')
    intent=dict(package_sha256=hashlib.sha256((KIT/'PACKAGE.json').read_bytes()).hexdigest(),
                capsule_manifest_sha256=digest(manifest),operation_sha256=digest(policy['binding']),
                original_config_sha256=hashlib.sha256(original).hexdigest())
    intent_path=ROOT/'prerequisite-intent.json'
    if intent_path.exists() and strict_json(loader.protected(intent_path).read_bytes())!=intent:raise ValueError('prerequisite_retry_binding_changed')
    if not intent_path.exists():atomic_json(intent_path,intent)
    for name,raw in files.items():
        path=INPUTS/name;path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
        if path.exists() and loader.protected(path).read_bytes()!=raw:raise ValueError('secret_retry_conflict')
        if not path.exists():put(path,raw)
    if configuration_digest(INPUTS,policy['configuration_metadata'])!=policy['binding']['configuration_sha256']:raise ValueError('capsule_configuration_mismatch')
    destination=COMPONENT_ROOT/package['kit_version']
    destination.parent.mkdir(mode=0o755,parents=True,exist_ok=True);loader.protected(destination.parent)
    destination.mkdir(mode=0o700,exist_ok=True);loader.protected(destination)
    names={path.name for path in (KIT/'component').iterdir()}
    if {path.name for path in destination.iterdir()}-names:raise ValueError('unexpected_component_files')
    # Exact same signed intent can finish interrupted installation. Never
    # overwrite foreign bytes or replace an unrelated installed component.
    for name in names:
        raw=loader.protected(KIT/'component'/name).read_bytes();path=destination/name
        if path.exists() and loader.protected(path).read_bytes()!=raw:raise ValueError('existing_component_conflict')
        if not path.exists():put(path,raw)
    if hashlib.sha256((destination/'COMPONENT.json').read_bytes()).hexdigest()!=policy['component_manifest_sha256']:raise ValueError('capsule_component_mismatch')
    fixed={WRAPPER:(('#!/bin/sh\nexec /usr/bin/python3 -I '+str(destination/'cli.py')+' "$@"\n').encode(),0o755),
           SUDO:((SERVICE+' ALL=(root) NOPASSWD: '+', '.join(str(WRAPPER)+' '+a for a in ('readiness','apply','status','recover'))+'\n').encode(),0o440)}
    for path,(raw,mode) in fixed.items():
        if path.exists() and loader.protected(path).read_bytes()!=raw:raise ValueError('existing_root_boundary_conflict')
        if not path.exists():put(path,raw,mode)
    run(['/usr/sbin/visudo','-cf',str(SUDO)])
    if POLICY.exists() and strict_json(loader.protected(POLICY).read_bytes())!=policy:raise ValueError('existing_root_policy_conflict')
    if not POLICY.exists():atomic_json(POLICY,policy)
    if current!=updated:put(CONFIG,updated,config_stat.st_mode&0o777,config_stat.st_uid,config_stat.st_gid)
    receipt=dict(protocol='pod-node-standalone-prerequisite-v1',kit_version=package['kit_version'],identity=identity,
                 updater_version=version,product_execution=False,configuration_sha256=policy['binding']['configuration_sha256'])
    receipt_path=ROOT/'prerequisite-install.json'
    if receipt_path.exists():
        saved=strict_json(loader.protected(receipt_path).read_bytes())
        if any(saved.get(k)!=v for k,v in receipt.items() if k!='updater_version'):raise ValueError('installed_receipt_conflict')
        return saved
    atomic_json(receipt_path,receipt)
    return receipt


def main():
    os.umask(0o077)
    if os.geteuid()!=0 or len(sys.argv)!=2 or sys.argv[1] not in ('enroll','install'):raise ValueError('exact_root_prerequisite_phase_required')
    loader=SourceLoader();package,app,identity=verify_package(loader)
    lock_path=Path('/var/lib/siemcore-greenfield/hook.lock');loader.protected(lock_path.parent)
    was_active=False
    if sys.argv[1]=='install':
        was_active=subprocess.run(['systemctl','is-active','--quiet',SERVICE],capture_output=True).returncode==0
        if was_active:run(['systemctl','stop',SERVICE])
    fd=os.open(lock_path,os.O_RDWR|os.O_CREAT|os.O_NOFOLLOW,0o600)
    try:
        fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
        result=enroll(loader,package,app,identity) if sys.argv[1]=='enroll' else install(loader,package,identity)
        print(json.dumps(result,sort_keys=True))
    finally:
        os.close(fd)
        if was_active:run(['systemctl','start',SERVICE])


if __name__=='__main__':
    try:main()
    except Exception:
        print('Signed prerequisite phase refused; inspect protected state',file=sys.stderr)
        sys.exit(1)
