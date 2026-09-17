"""Ephemeral data observation; never an activation or installation receipt."""
import json
from pathlib import Path
import time
import authorization
import client
import data_stage
import protocol
import selective_sync

COMMAND=('/app/siemcore','pod-bootstrap-data','--config','/run/bootstrap/data.json','--verify-readiness')


def nanos(value):
    seconds,fraction=authorization.timestamp(value)
    return int(seconds.timestamp())*10**9+fraction


def prepare(config,registry,fingerprint,node_id,generation,input_sha256,original_input):
    sync=selective_sync.validate(original_input,registry,input_sha256,node_id,config if 'seed' in config else None)
    if config.get('initial_sync')!=sync or type(config['initial_sync']['allowlist_version']) is not int:
        raise ValueError('immutable initial synchronization required')
    if (node_id=='2')!=('seed' in config):raise ValueError('target observation requires exact seed input')
    plan=data_stage.prepare(config,registry,fingerprint,node_id,generation,input_sha256)
    plan['binding']['protocol']='pod-bootstrap-readiness-v1'
    plan['argv']=COMMAND
    plan['mirror_identity']=dict(PodID=registry['pod_id'],InstallationID=registry['installation_id'],Generation=generation)
    return plan


def validate_receipt(plan,raw,now_ns):
    from jsonschema import Draft202012Validator, ValidationError
    if len(raw)>32768:raise ValueError('oversized readiness receipt')
    result=protocol.strict_json(raw)
    schema=json.loads((Path(__file__).parent/'readiness.receipt.schema.json').read_text())
    try:Draft202012Validator(schema).validate(result)
    except ValidationError:raise ValueError('invalid readiness receipt schema') from None
    expected_tables=sorted(schema['properties']['operational_content']['properties']['tables']['items']['properties']['name']['enum'])
    if any(result[k]!=v for k,v in plan['binding'].items()) or type(result['generation']) is not int:
        raise ValueError('readiness binding mismatch')
    if any(result[k] is not False for k in ('installation_complete','processing_allowed','activation_ready')):
        raise ValueError('observation cannot authorize activation')
    def fresh(value):
        age=now_ns-nanos(value)
        if not 0<=age<=5*10**9:raise ValueError('stale or future readiness observation')
        return age
    fresh(result['observed_at'])
    if type(result['policy_version']) is not int or result['policy_version']!=2:raise ValueError('policy mismatch')
    def content(value):
        fresh(value['observed_at'])
        if type(value['policy_version']) is not int or value['policy_version']!=2:raise ValueError('content policy mismatch')
        names=[row['name'] for row in value['tables']]
        if names!=expected_tables or any(type(row['rows']) is not int for row in value['tables']):
            raise ValueError('duplicate tables or invalid row counts')
        return value['sha256'],sorted(value['tables'],key=lambda row:row['name'])
    observed=content(result['operational_content'])
    if 'mirror' in result:
        mirror=result['mirror']
        if mirror['identity']!=plan['mirror_identity'] or type(mirror['identity']['Generation']) is not int:
            raise ValueError('mirror identity mismatch')
        age=fresh(mirror['observed_at'])
        if type(mirror['applied_age_ns']) is not int or not 0<=mirror['applied_age_ns']+age<=300*10**9:
            raise ValueError('applied marker stale')
        if content(mirror['source'])!=content(mirror['target']) or content(mirror['target'])!=observed:
            raise ValueError('source/target operational content differs')
    return result


def observe(plan,journal,verified_runner,clock=time.time_ns):
    journal=Path(journal)
    binding=dict(plan['binding'],config_sha256=plan['config_sha256'])
    if journal.exists() and protocol.strict_json(client.protected(journal)).get('binding')!=binding:
        raise ValueError('observation operation changed')
    client.durable(journal,dict(binding=binding,status='incomplete',activation_ready=False))
    code,raw=verified_runner(list(COMMAND))
    if type(code) is not int or code!=0:raise ValueError('readiness command failed')
    receipt=validate_receipt(plan,raw,clock())
    client.durable(journal,dict(binding=binding,status='observation-only',receipt=receipt,
                               expires_at_ns=nanos(receipt['observed_at'])+5*10**9,activation_ready=False))
    return receipt
