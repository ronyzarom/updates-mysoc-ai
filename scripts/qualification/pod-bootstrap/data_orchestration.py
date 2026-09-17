"""Source-only per-node stage coordination; not a schema-4 installer entrypoint."""
from pathlib import Path
import client
import data_stage
import protocol


def run_data_stages(registry,fingerprint,node_id,generation,input_sha256,schema_config,
                    seed_config,journal_directory,verified_runner_factory):
    """Caller has registered/authorized original operation and locked private dir.

    Every factory-created runner must reverify artifact, runtime and current
    Observer authorization. Initial seed source comes from reviewed protected
    product configuration, never inferred from desired active/standby status.
    No Observer runtime command, readiness, activation or role-IP flow is supplied.
    """
    if node_id not in ('1','2'):
        raise ValueError('Observer requires its own product bootstrap contract')
    if 'seed' in schema_config:
        raise ValueError('schema preparation config must omit seed')
    configs=[('schema',schema_config)]
    if seed_config is not None:
        if 'seed' not in seed_config or {k:v for k,v in seed_config.items() if k!='seed'}!=schema_config:
            raise ValueError('seed stage must preserve original schema configuration')
        configs.append(('seed',seed_config))
    plans=[(stage,config,data_stage.prepare(config,registry,fingerprint,node_id,generation,input_sha256))
           for stage,config in configs]
    directory=Path(journal_directory)
    # Parent runner must provision and lock this protected directory, never /tmp.
    binding=dict(operation_id=registry['operation_id'],registry_sha256=fingerprint,node_id=node_id,
                 generation=generation,input_sha256=input_sha256,
                 stages=[dict(stage=stage,config_sha256=plan['config_sha256']) for stage,_,plan in plans])
    operation=directory/'data-operation.json'
    if operation.exists():
        existing=protocol.strict_json(client.protected(operation))
        if existing.get('binding')!=binding:
            raise ValueError('data operation or planned stages changed; reconcile explicitly')
    client.durable(operation,dict(binding=binding,phase='incomplete',installation_complete=False,processing_allowed=False))
    receipts=[]
    for stage,config,plan in plans:
        # Do not trust/skip a local success alone. Product stage retry is idempotent,
        # and a fresh runner rechecks current authorization/runtime each time.
        runner=verified_runner_factory(stage,config,plan)
        receipts.append(data_stage.invoke_fixture(plan,directory/(stage+'-stage.json'),runner))
    result=dict(binding=binding,phase='awaiting-product-readiness',receipts=receipts,
                installation_complete=False,processing_allowed=False)
    client.durable(operation,result)
    return result
