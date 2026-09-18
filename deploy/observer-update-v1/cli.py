#!/usr/bin/env python3
"""Disabled-until-provisioned root entrypoint; no existing kit invokes this file."""
import hashlib
import json
import os
from pathlib import Path
import re
import sys

BASE=Path(__file__).resolve().parent
# Wrapper uses Python isolated mode; explicitly import only protected component.
sys.path.insert(0,str(BASE))
from protocol import PROTOCOL,strict_json,digest,validate_binding
from adapter import Adapter,atomic_json
from host import Host,protected,private_json,APPLICATION,BOOTSTRAP_RELEASE,BOOTSTRAP_JOURNAL,BLOCK
from worker import invoke
import fcntl
import uuid
from datetime import datetime,timezone

ROOT=Path('/var/lib/siemcore-observer-update')
POLICY=Path('/etc/siemcore-cascade-updater/observer-update-policy.json')
LOCK=Path('/var/lib/siemcore-greenfield/hook.lock')


def admission():
    policy=private_json(POLICY)
    if set(policy)!={'protocol','enabled','component_manifest_sha256','bootstrap_policy_sha256','bootstrap_binary_sha256','product_channel'} or policy['protocol']!=PROTOCOL or policy['enabled'] is not True:
        raise ValueError('Observer update component is not enabled')
    manifest_path=protected(BASE/'COMPONENT.json');raw=manifest_path.read_bytes()
    if hashlib.sha256(raw).hexdigest()!=policy['component_manifest_sha256']:
        raise ValueError('component manifest mismatch')
    manifest=strict_json(raw)
    files=manifest.get('files')
    expected={'cli.py','protocol.py','adapter.py','host.py','worker.py','supervisor.py'}
    if manifest.get('protocol')!=PROTOCOL or not isinstance(files,dict) or set(files)!=expected:
        raise ValueError('exact protected adapter component required')
    for name,checksum in files.items():
        if hashlib.sha256(protected(BASE/name).read_bytes()).hexdigest()!=checksum:
            raise ValueError('component file integrity mismatch')
    link=Path('/etc/siemcore-pod-observer/link.json')
    if link.exists() or link.is_symlink():raise ValueError('linked Observer refused')
    app=private_json(APPLICATION)
    if (app.get('schema')!=4 or app.get('topology')!='observer-unlinked' or
            hashlib.sha256(json.dumps(app,sort_keys=True).encode()).hexdigest()!=policy['bootstrap_policy_sha256']):
        raise ValueError('component activation identity mismatch')
    config=protected(Path('/etc/siemcore-cascade-updater/config.yaml')).read_text()
    products=re.search(r'(?ms)^products:\n(.*?)(?=^\S|\Z)',config)
    if not products or len(re.findall(r'^  - name:',products[1],re.M))!=1 or not re.search(r'^  - name: siemcore\s*$',products[1],re.M):
        raise ValueError('exact SiemCore configuration required')
    types=re.findall(r'^    server_type:\s*(.+)$',products[1],re.M)
    channels=re.findall(r'^    channel:\s*(\S+)',products[1],re.M)
    if len(types)!=1 or types[0].strip('"')!='observer-unlinked' or channels!=[policy['product_channel']]:
        raise ValueError('protected updater role/channel mismatch')
    return policy,app


def create_binding(host,operation,target):
    if set(target)!={'version','sha256','signature'}:
        raise ValueError('exact signed target receipt required')
    if not re.fullmatch(r'\d+\.\d+\.\d+\.\d+',target['version']) or not re.fullmatch('[0-9a-f]{64}',target['sha256']):
        raise ValueError('invalid target receipt')
    previous=host.bootstrap
    def artifact(version,sha,signature):return dict(product='siemcore',version=version,architecture='linux/amd64',artifact_sha256=sha,artifact_signature=signature,binary_sha256='')
    predecessor=artifact(previous['version'],previous['sha256'],previous['signature'])
    desired=artifact(target['version'],target['sha256'],target['signature'])
    for name,item in [('predecessor',predecessor),('target',desired)]:
        bundle,_=host.stage_one(item,'admission-'+name,derive_binary=True)
        item['binary_sha256']=hashlib.sha256((bundle/'pod/bin/siemcore').read_bytes()).hexdigest()
    app=host.application
    result=dict(protocol=PROTOCOL,operation_id=operation,machine_id=app['machine_id'],installation_id=app['installation_id'],updater_instance_id=app['updater_instance_id'],server_type='observer-unlinked',bootstrap_policy_sha256=hashlib.sha256(json.dumps(app,sort_keys=True).encode()).hexdigest(),predecessor=predecessor,target=desired,signing_public_key_sha256=hashlib.sha256(host.public_key).hexdigest(),ui_protection_required=True)
    return validate_binding(result)


def main():
    if os.geteuid()!=0 or sys.platform!='linux' or len(sys.argv)!=2 or sys.argv[1] not in ('readiness','apply','status','recover'):
        raise ValueError('exact privileged operation required')
    action=sys.argv[1];policy,app=admission()
    request=strict_json(sys.stdin.buffer.read(65537))
    protected(LOCK.parent)
    if LOCK.is_symlink():raise ValueError('unsafe lifecycle lock')
    fd=os.open(LOCK,os.O_WRONLY|os.O_CREAT|os.O_NOFOLLOW,0o600)
    with os.fdopen(fd,'a') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        if action=='readiness':
            if request!={'protocol':PROTOCOL}:raise ValueError('invalid readiness request')
            if BLOCK.exists() or BLOCK.is_symlink():raise ValueError('retained management barrier')
            journal=private_json(BOOTSTRAP_JOURNAL)
            if journal.get('status')!='complete':raise ValueError('bootstrap incomplete')
            from protocol import validate_health
            host=Host(BASE)
            active=ROOT/'active-operation.json'
            eligible=True
            if active.exists():
                operation=private_json(active)['operation_id']
                directory=ROOT/'operations'/operation
                retained=private_json(directory/'adapter-journal.json')
                if retained.get('phase')!='accepted':raise ValueError('retained operation requires reconciliation')
                binding=private_json(directory/'binding.json');artifact=binding['target'];eligible=False
            else:
                artifact=dict(version=host.bootstrap['version'],binary_sha256=policy['bootstrap_binary_sha256'])
                binding=dict(installation_id=app['installation_id'],updater_instance_id=app['updater_instance_id'])
            health=validate_health(host.probe(binding,'target'),binding,artifact,require_closed=not eligible)
            print(json.dumps(dict(protocol=PROTOCOL,capabilities=[PROTOCOL] if eligible else [],adapter_manifest_sha256=policy['component_manifest_sha256'],server_type='observer-unlinked',observed_at=datetime.now(timezone.utc).isoformat(),health=health,eligible_for_security_upgrade=eligible,ui_security_compliant=health['ui_closed'])))
            return
        if set(request)!={'protocol','operation_id','target'} or request['protocol']!=PROTOCOL or str(uuid.UUID(request['operation_id']))!=request['operation_id']:
            raise ValueError('invalid root operation request')
        ROOT.mkdir(mode=0o700,exist_ok=True);protected(ROOT)
        if ROOT.stat().st_mode&0o077:raise ValueError('private operation root required')
        operations=ROOT/'operations';operations.mkdir(mode=0o700,exist_ok=True);protected(operations)
        active=ROOT/'active-operation.json'
        if active.exists() and private_json(active)!=dict(operation_id=request['operation_id']):
            raise ValueError('retained operation cannot be replaced')
        directory=operations/request['operation_id']
        if not directory.exists():
            if action!='apply':raise ValueError('unknown operation')
            directory.mkdir(mode=0o700)
        protected(directory)
        host=Host(directory);binding_file=directory/'binding.json'
        if binding_file.exists():binding=private_json(binding_file)
        else:
            if action!='apply':raise ValueError('operation not admitted')
            binding=create_binding(host,request['operation_id'],request['target'])
        target=request['target'];bound=binding['target']
        if target!={'version':bound['version'],'sha256':bound['artifact_sha256'],'signature':bound['artifact_signature']}:
            raise ValueError('target changed during retained operation')
        if not active.exists():atomic_json(active,dict(operation_id=request['operation_id']))
        coordinator=Adapter(directory,host.verify_binding,host.stage,lambda a,b,bundles,d:invoke(a,b,bundles,d,lock.fileno(),900 if a!='status' else 30),host.probe,host.stopped,protected,host.preservation)
        result=coordinator.run(action,binding)
        print(json.dumps(dict(result,observed_at=datetime.now(timezone.utc).isoformat(),target_version=bound['version'],artifact_sha256=bound['artifact_sha256']),sort_keys=True))

if __name__=='__main__':
    try:main()
    except Exception:
        print('Observer adapter refused or retained uncertain operation; inspect protected journal',file=sys.stderr)
        sys.exit(1)
