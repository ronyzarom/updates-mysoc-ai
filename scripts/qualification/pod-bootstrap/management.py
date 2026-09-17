"""Qualification-only paused app/archiver observation; no service mutation."""
import hashlib
import json
from pathlib import Path
import re
import time
import client
import data_runner
import data_stage
import observer_runner
import protocol
import readiness


def environment_digest(entries):
    keys=set()
    for entry in entries:
        if not isinstance(entry,str) or '=' not in entry or any(c in entry for c in '\r\n\0'):
            raise ValueError('invalid reviewed environment')
        key=entry.split('=',1)[0]
        if not key or key in keys:raise ValueError('ambiguous reviewed environment')
        keys.add(key)
    return hashlib.sha256(('\n'.join(sorted(entries))+'\n').encode()).hexdigest()


def prepare(binary,config_path,registry,fingerprint,node_id,generation,input_sha256,expected_image,expected_environments):
    raw=client.protected(config_path);config=protocol.strict_json(raw)
    if set(config)!={'protocol','data_binding_file','runtime_image_id','environment_sha256'} or config['protocol']!='pod-bootstrap-management-v1':
        raise ValueError('exact management config required')
    if (config['runtime_image_id']!=expected_image or not re.fullmatch('sha256:[0-9a-f]{64}',expected_image) or
            config['environment_sha256']!=expected_environments or set(expected_environments)!={'app','archiver'} or
            any(not re.fullmatch('[0-9a-f]{64}',v) for v in expected_environments.values())):
        raise ValueError('management expectations differ from reviewed artifact/inputs')
    for path in (binary,config_path,config['data_binding_file']):
        if not Path(path).is_absolute() or '..' in Path(path).parts:raise ValueError('protected absolute paths required')
    data_raw=client.protected(config['data_binding_file']);data=protocol.strict_json(data_raw)
    plan=data_stage.prepare(data,registry,fingerprint,node_id,generation,input_sha256)
    plan['binding']['protocol']='pod-bootstrap-management-v1'
    return dict(binding=plan['binding'],pod_id=registry['pod_id'],argv=[binary,'pod-bootstrap-management','--config',config_path],
        configuration_sha256=hashlib.sha256(raw).hexdigest(),data_binding_file=config['data_binding_file'],
        data_binding_sha256=hashlib.sha256(data_raw).hexdigest(),runtime_image_id=expected_image,environment_sha256=expected_environments)


def validate_receipt(plan,raw,now_ns):
    if len(raw)>8192:raise ValueError('oversized management receipt')
    result=protocol.strict_json(raw)
    fields=set(plan['binding'])|{'phase','configuration_sha256','observed_at','runtimes','installation_complete','processing_allowed','activation_ready'}
    if not isinstance(result,dict) or set(result)!=fields:raise ValueError('unexpected management fields')
    if any(result[k]!=v for k,v in plan['binding'].items()) or type(result['generation']) is not int:
        raise ValueError('management operation mismatch')
    if result['configuration_sha256']!=plan['configuration_sha256'] or result['phase']!='management-paused-verified':
        raise ValueError('management config/phase mismatch')
    if any(result[k] is not False for k in ('installation_complete','processing_allowed','activation_ready')):
        raise ValueError('management receipt cannot grant activation')
    if not 0<=now_ns-readiness.nanos(result['observed_at'])<=5*10**9:raise ValueError('stale management observation')
    if not isinstance(result['runtimes'],list) or len(result['runtimes'])!=2:raise ValueError('both runtimes required')
    ids=set()
    for role,item in zip(('app','archiver'),result['runtimes']):
        if set(item)!={'role','container_id','image_id','environment_sha256','status'} or item['role']!=role:
            raise ValueError('exact ordered runtime evidence required')
        if (not re.fullmatch('[0-9a-f]{64}',item['container_id']) or item['container_id'] in ids or
                item['image_id']!=plan['runtime_image_id'] or item['environment_sha256']!=plan['environment_sha256'][role]):
            raise ValueError('runtime image/environment/identity mismatch')
        ids.add(item['container_id'])
        expected=dict(pod_id=plan['pod_id'],node_id=plan['binding']['node_id'],version=plan['binding']['installed_version'],
            management_ready=True,processing_enabled=False,quiescent=True,blocked=False,generation=0,state='Mirror-STBY')
        status=item['status']
        if status!=expected or type(status['generation']) is not int or any(type(status[k]) is not bool for k in ('management_ready','processing_enabled','quiescent','blocked')):
            raise ValueError('runtime is not fresh verified paused management')
    return result


class Runner:
    def __init__(self,plan,verify_artifact):
        if not callable(verify_artifact):raise ValueError('independent artifact expectation verifier required')
        self.plan,self.verify_artifact=plan,verify_artifact
    def __call__(self,argv):
        plan=self.plan
        if argv!=plan['argv']:raise ValueError('unexpected management command')
        expected=self.verify_artifact()
        if set(expected)!={'binary_sha256','runtime_image_id','environment_sha256'} or any(expected[k]!=plan[k] for k in ('runtime_image_id','environment_sha256')):
            raise ValueError('independently derived expectations differ')
        def check():
            if observer_runner.binary_digest(argv[0])!=expected['binary_sha256']:raise ValueError('host binary changed')
            if hashlib.sha256(client.protected(argv[-1])).hexdigest()!=plan['configuration_sha256']:
                raise ValueError('management input changed')
            if hashlib.sha256(client.protected(plan['data_binding_file'])).hexdigest()!=plan['data_binding_sha256']:
                raise ValueError('authenticated data binding changed')
        check();result=data_runner.bounded(argv,10,8192);check()
        return result


def observe(plan,journal,runner,clock=time.time_ns):
    journal=Path(journal)
    if journal.exists() and protocol.strict_json(client.protected(journal)).get('plan')!=plan:
        raise ValueError('management operation changed')
    client.durable(journal,dict(plan=plan,status='incomplete',activation_ready=False))
    code,raw=runner(plan['argv'])
    if type(code) is not int or code!=0:raise ValueError('management observation failed')
    receipt=validate_receipt(plan,raw,clock())
    client.durable(journal,dict(plan=plan,status='observation-only',receipt=receipt,
        expires_at_ns=readiness.nanos(receipt['observed_at'])+5*10**9,activation_ready=False))
    return receipt
