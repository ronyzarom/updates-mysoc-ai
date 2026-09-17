from pathlib import Path
exec(Path('/source/native_combined_handoff.py').read_text().split('order=[')[0])
node=d['nodes']['2'];config=protocol.strict_json(client.protected(node['seed_config']))
authorize=authorizer('2',node)
original=client.protected(node['original_input']);sync=selective_sync.validate(original,registry,node['input_sha256'],'2',config)
if config['initial_sync']!=sync:raise ValueError('immutable sync mismatch')
plan=data_stage.prepare(config,registry,d['registry_sha256'],'2',d['generation'],node['input_sha256'])
try:
    receipt=data_stage.invoke_fixture(plan,Path(node['journal_directory'])/'seed.json',data_factory('2',node,config,authorize))
    result=dict(exit_code=0,stdout=base64.b64encode(json.dumps(receipt).encode()).decode())
except Exception as error:
    print(type(error).__name__,flush=True);result=dict(exit_code=1,stdout='')
client.durable(root/'response-5-retry.json',result)
print(json.dumps({'sequence':'5-retry','exit_code':result['exit_code']}),flush=True)
if result['exit_code']:raise SystemExit(1)
deadline=time.monotonic()+570
while not (root/'request-6.json').exists():
    if time.monotonic()>deadline:raise TimeoutError('request6missing')
    time.sleep(.1)
request=protocol.strict_json(client.protected(root/'request-6.json'));node=d['nodes']['1']
if request!={'sequence':6,'node_id':'1','stage':'runtime','input_file':node['runtime_input']}:raise ValueError('request6mismatch')
config=protocol.strict_json(client.protected(node['runtime_input']));authorize=authorizer('1',node)
receipt=runtime_worker.invoke(node['runtime_directory'],config['config'],config['tls_material'],config['binding'],Path(node['journal_directory'])/'runtime.json',bundle,authorize,timeout=300)
client.durable(root/'response-6.json',dict(exit_code=0,stdout=base64.b64encode(json.dumps(receipt).encode()).decode()))
print(json.dumps({'sequence':6,'exit_code':0}),flush=True)
