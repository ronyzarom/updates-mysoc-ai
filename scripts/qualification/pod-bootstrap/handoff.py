"""Qualification handoff consumer. Verifier/runner factory is supplied by harness."""
import base64
import hashlib
import json
from pathlib import Path
import client
import data_stage
import protocol

FIELDS={'sequence','registry','registry_sha256','generation','node_id','input_sha256','bootstrap_root','image_id'}


def registry_hash(registry):
    order=('protocol','pod_id','installation_id','operation_id','release','nodes')
    release_order=('product','version','architecture','sha256','public_key','signature')
    node_order=('node_id','updater_id','machine_id','infrastructure_id','certificate_sha256')
    if set(registry)!=set(order) or set(registry['release'])!=set(release_order):
        raise ValueError('exact registry schema required')
    typed={key:registry[key] for key in order}
    typed['release']={key:registry['release'][key] for key in release_order}
    if any(set(node)!=set(node_order) for node in registry['nodes']):raise ValueError('unexpected node fields')
    typed['nodes']=[{key:node[key] for key in node_order} for node in sorted(registry['nodes'],key=lambda node:node['node_id'])]
    return hashlib.sha256(json.dumps(typed,separators=(',',':')).encode()).hexdigest()


def consume(root,sequence,make_verified_runner):
    """Caller provisions root/receipt directories and serializes consumption.

    Factory verifies descriptor, test-only manifest/signature/image, protected
    mounts, current invitation and live runtime; returns data_runner.Runner.
    No default trust/runner is supplied. Exceptions return only generic failure.
    """
    if type(sequence) is not int or not 1<=sequence<=4:
        raise ValueError('expected qualification sequence 1..4')
    root=Path(root)
    response=root/('response-%d.json'%sequence)
    if response.exists():raise ValueError('response already exists; do not re-execute completed handoff')
    try:
        request=protocol.strict_json(client.protected(root/('request-%d.json'%sequence)))
        if set(request)!=FIELDS or type(request['sequence']) is not int or request['sequence']!=sequence:
            raise ValueError('handoff sequence/schema mismatch')
        if request['bootstrap_root']!=str(root/'bootstrap'):
            raise ValueError('unexpected bootstrap root')
        original=client.protected(root/'original-input.json')
        if hashlib.sha256(original).hexdigest()!=request['input_sha256']:
            raise ValueError('original input hash mismatch')
        registry=request['registry']
        if registry_hash(registry)!=request['registry_sha256']:
            raise ValueError('registry fingerprint mismatch')
        # Trust is a separately provisioned fixture key, never taken from registry.
        key=bytes.fromhex(client.protected(root/'release-trust.hex').decode().strip())
        client.validate_registry(registry,key,registry['release']['architecture'])
        config=protocol.strict_json(client.protected(root/'bootstrap'/'data.json'))
        plan=data_stage.prepare(config,registry,request['registry_sha256'],request['node_id'],
                                request['generation'],request['input_sha256'])
        runner=make_verified_runner(request,config,plan)
        receipt=data_stage.invoke_fixture(plan,root/'receipts'/'data-stage.json',runner)
        raw=json.dumps(receipt,separators=(',',':')).encode()
        result=dict(exit_code=0,stdout=base64.b64encode(raw).decode())
    except Exception:
        # Failure may follow partial DB commit; no rollback/reset or state deletion.
        result=dict(exit_code=1,stdout='')
    client.durable(response,result)
    return result
