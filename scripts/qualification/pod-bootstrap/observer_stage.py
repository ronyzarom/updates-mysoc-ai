"""Source-only Observer management receipt adapter; no service install/start."""
import hashlib
import json
from pathlib import Path
import client
import protocol


def validate(value,kind):
    from jsonschema import Draft202012Validator
    schema=json.loads((Path(__file__).parent/('observer-stage.'+kind+'.schema.json')).read_text())
    Draft202012Validator(schema).validate(value)


def prepare(config,registry,fingerprint,generation,input_sha256,verified_binary,drain_path,update_path,receipt_path):
    validate(config,'config')
    witnesses=[n for n in registry['nodes'] if n['node_id']=='witness']
    if (len(witnesses)!=1 or config['node']!=witnesses[0] or config['registry']!=registry or
            type(generation) is not int or generation<=0 or type(config['generation']) is not int or
            config['generation']!=generation or config['input_sha256']!=input_sha256):
        raise ValueError('Observer original operation binding mismatch')
    for path in (verified_binary,drain_path,update_path,receipt_path,config['host_machine_id_file'],config['invitation_file']):
        if not Path(path).is_absolute() or '..' in Path(path).parts:raise ValueError('protected absolute paths required')
    binding=dict(protocol='pod-bootstrap-observer-v1',operation_id=registry['operation_id'],generation=generation,
        node_id='witness',input_sha256=input_sha256,registry_sha256=fingerprint,
        artifact_sha256=registry['release']['sha256'],installed_version=registry['release']['version'])
    argv=[verified_binary,'pod-observer-bootstrap','--drain-config',drain_path,'--update-config',update_path,
          '--pod-id',registry['pod_id'],'--updater-id',witnesses[0]['updater_id'],'--health','--receipt-config',receipt_path]
    return dict(binding=binding,argv=argv,config_sha256=hashlib.sha256(json.dumps(config,sort_keys=True,separators=(',',':')).encode()).hexdigest(),
        drain_sha256=hashlib.sha256(client.protected(drain_path)).hexdigest(),
        update_sha256=hashlib.sha256(client.protected(update_path)).hexdigest())


def validate_receipt(plan,raw):
    if len(raw)>8192:raise ValueError('oversized Observer receipt')
    receipt=protocol.strict_json(raw);validate(receipt,'receipt')
    if any(receipt[k]!=v for k,v in plan['binding'].items()) or type(receipt['generation']) is not int:
        raise ValueError('Observer receipt identity mismatch')
    if receipt['installation_complete'] is not False or receipt['processing_allowed'] is not False:
        raise ValueError('Observer health cannot grant installation or processing')
    return receipt


def invoke_fixture(plan,journal,verified_runner):
    """Runner verifies signed host binary/config hashes/current authorization.

    It must bound process lifetime/output; no systemd launcher is supplied here.
    Caller serializes access using a protected operation lock/directory.
    """
    journal=Path(journal)
    if journal.exists():
        prior=protocol.strict_json(client.protected(journal))
        if prior.get('plan')!=plan:raise ValueError('Observer operation changed; reconcile explicitly')
    client.durable(journal,dict(plan=plan,status='incomplete',installation_complete=False,processing_allowed=False))
    code,raw=verified_runner(plan['argv'])
    if type(code) is not int or code!=0:raise ValueError('Observer verification failed')
    receipt=validate_receipt(plan,raw)
    client.durable(journal,dict(plan=plan,status='observer-management-verified',receipt=receipt,
                               installation_complete=False,processing_allowed=False))
    return receipt
