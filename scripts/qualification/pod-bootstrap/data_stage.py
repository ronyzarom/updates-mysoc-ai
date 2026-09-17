"""Source-only data-stage adapter. No runtime launcher and no kit enablement."""
import hashlib
import json
from pathlib import Path
import client
import protocol

ARGV=('/app/cyfox-siemcore','pod-bootstrap-data','--config','/run/bootstrap/data.json')
LIMIT=8192


def schema_validate(value, name):
    from jsonschema import Draft202012Validator
    schema=json.loads((Path(__file__).parent/('data-stage.'+name+'.schema.json')).read_text())
    Draft202012Validator(schema).validate(value)


def prepare(config, registry, fingerprint, node_id, generation, input_sha256):
    schema_validate(config,'config')
    if type(generation) is not int or generation<=0 or type(config['generation']) is not int:
        raise ValueError('positive exact generation required')
    nodes=[n for n in registry['nodes'] if n['node_id']==node_id]
    if node_id not in ('1','2') or len(nodes)!=1:
        raise ValueError('data stage only supports registered data nodes')
    if (config['registry']!=registry or config['node']!=nodes[0] or config['generation']!=generation or
            config['input_sha256']!=input_sha256):
        raise ValueError('stage configuration differs from original operation')
    if 'seed' in config:
        source=config['seed']['source_node_id']
        if source not in ('1','2') or source==node_id:
            raise ValueError('seed source must be the other registered data node')
    # Paths refer to protected container mounts, not arbitrary relative working paths.
    paths=[config[k] for k in ('database_connection_file','host_machine_id_file',
                              'release_public_key','authorization_key','invitation_file')]
    paths+=list(config['observer']['tls'].values())
    if 'seed' in config:
        paths += [config['seed'][k] for k in ('publisher_connection_file','apply_connection_file')]
    if any(not Path(p).is_absolute() or '..' in Path(p).parts or p=='/var/run/docker.sock' for p in paths):
        raise ValueError('protected absolute mount paths required')
    raw=json.dumps(config,sort_keys=True,separators=(',',':')).encode()
    binding=dict(protocol='pod-bootstrap-data-v1',operation_id=registry['operation_id'],
        generation=generation,node_id=node_id,input_sha256=input_sha256,registry_sha256=fingerprint,
        artifact_sha256=registry['release']['sha256'],installed_version=registry['release']['version'])
    return dict(argv=ARGV,config_sha256=hashlib.sha256(raw).hexdigest(),binding=binding,
                expected_phase='selected-synchronization-configured' if 'seed' in config else 'schema-prepared')


def validate_receipt(plan, raw):
    if len(raw)>LIMIT:raise ValueError('oversized data-stage receipt')
    receipt=protocol.strict_json(raw)
    schema_validate(receipt,'receipt')
    for field,value in plan['binding'].items():
        if receipt[field]!=value:raise ValueError('data-stage receipt binding mismatch')
    if (type(receipt['generation']) is not int or receipt['phase']!=plan['expected_phase'] or
            receipt['processing_allowed'] is not False or receipt['installation_complete'] is not False):
        raise ValueError('data-stage receipt cannot grant readiness or processing')
    return receipt


def invoke_fixture(plan, journal, runner):
    """Injected isolated runner only. Production runner intentionally not supplied.

    Caller owns protected locked journal directory, verified image, read-only mounts,
    fixed internal network, current Observer authorization and bounded process I/O.
    runner returns (exit_code, bounded_stdout); stderr must never reach receipts.
    """
    journal=Path(journal)
    binding=dict(plan['binding'],config_sha256=plan['config_sha256'],expected_phase=plan['expected_phase'])
    if journal.exists():
        previous=protocol.strict_json(client.protected(journal))
        if previous.get('binding')!=binding:
            raise ValueError('changed stage operation/config requires reconciliation')
    # Durable before invoking a potentially partially committing process.
    client.durable(journal,dict(binding=binding,status='incomplete',potential_partial_commit=True))
    try:
        code,raw=runner(list(ARGV))
        if type(code) is not int or code!=0:raise ValueError('data stage exited unsuccessfully')
        receipt=validate_receipt(plan,raw)
        client.durable(journal,dict(binding=binding,status='partial-stage-complete',
            potential_partial_commit=True,receipt=receipt))
        return receipt
    except Exception:
        # Retain intent and all product-owned state. Never reset a DB/slot/barrier.
        raise
