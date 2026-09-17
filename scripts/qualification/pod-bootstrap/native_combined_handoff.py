"""Test-only combined operation consumer. No cloud or activation actions."""
import argparse
import base64
import hashlib
import io
import json
from pathlib import Path
import tarfile
import time
import authorization
import client
import data_runner
import data_stage
import handoff
import protocol
import runtime_worker
import selective_sync

p=argparse.ArgumentParser();p.add_argument('--root',required=True);args=p.parse_args()
root=Path(args.root)
d=protocol.strict_json(client.protected(root/'handoff.json'))
if d['fixture_only'] is not True or d['root']!=str(root):raise ValueError('exact disposable fixture required')
registry=d['registry'];pin=bytes.fromhex(client.protected(d['release_public_key_file']).decode().strip())
client.validate_registry(registry,pin,'arm64')
if handoff.registry_hash(registry)!=d['registry_sha256']:raise ValueError('registry fingerprint mismatch')
docker='/usr/local/bin/docker'


def bundle():
    artifact=d['artifact']
    for field in ('product','version','sha256','signature'):
        if artifact[field]!=registry['release'][field]:raise ValueError('artifact release mismatch')
    raw=client.protected(artifact['path'],limit=256*1024*1024)
    if hashlib.sha256(raw).hexdigest()!=artifact['sha256']:raise ValueError('signed bundle checksum mismatch')
    with tarfile.open(fileobj=io.BytesIO(raw),mode='r:gz') as archive:
        entries=archive.getmembers();names=[m.name for m in entries]
        if len(set(names))!=len(names) or any(not (m.isfile() or m.isdir()) or Path(m.name).is_absolute() or '..' in Path(m.name).parts for m in entries):
            raise ValueError('unsafe signed fixture archive')
        member=archive.getmember('bundle/MANIFEST.json')
        if not member.isfile() or member.size>65536:raise ValueError('invalid manifest')
        manifest=protocol.strict_json(archive.extractfile(member).read())
        for field in ('product','version','architecture'):
            if manifest[field]!=registry['release'][field]:raise ValueError('manifest release mismatch')
        if manifest['runtime_image_id']!=d['image_id']:raise ValueError('signed common image mismatch')
        member=archive.getmember('bundle/'+runtime_worker.MODULE_PATH)
        if not member.isfile() or member.size>1024*1024:raise ValueError('invalid module member')
        module=archive.extractfile(member).read()
    return manifest,module,artifact['sha256']


def authorizer(node_id,node):
    key=bytes.fromhex(client.protected(node['authorization_key_file']).decode().strip())
    if key==pin:raise ValueError('distinct authorization trust required')
    transport_config=protocol.strict_json(client.protected(node['transport']))
    transport=client.Transport(transport_config)
    def authorize():
        invitation=protocol.strict_json(client.protected(node['invitation_file']))
        claims=authorization.verify_invitation(invitation,key,dict(operation_id=registry['operation_id'],
            registry_sha256=d['registry_sha256'],node_id=node_id,input_sha256=node['input_sha256']),time.time_ns())
        sent=authorization.request(registry,d['registry_sha256'],node_id,'authorize',d['generation'],node['input_sha256'],invitation)
        authorization.validate_ack(sent,'authorize',transport.call('authorize',sent,10),claims['authorization_id'],claims['expires_at'])
    authorize()  # Preload dependencies and fail before preparing any process.
    return authorize


def data_factory(node_id,node,config,authorize):
    def artifact(image):
        manifest,module,_=bundle()
        if image!=d['image_id']:raise ValueError('image drift')
        detail=data_runner.docker_json(docker,['image','inspect',image])[0]
        if detail['Architecture']!=registry['release']['architecture']:raise ValueError('image architecture mismatch')
    def dependency():
        image=data_runner.docker_json(docker,['image','inspect',node['postgres_image']])[0]
        runtime=data_runner.docker_json(docker,['container','inspect',node['data_container']])[0]
        if runtime['Image']!=image['Id'] or not runtime['State']['Running'] or runtime['State'].get('Health',{}).get('Status')!='healthy':
            raise ValueError('pinned healthy PostgreSQL required')
        code,out=data_runner.bounded([docker,'exec',node['data_container'],'id','-u','postgres'],10)
        if code or int(out.strip())!=node['postgres_uid']:raise ValueError('PostgreSQL UID mismatch')
        mounts=[m for m in runtime['Mounts'] if m['Destination']=='/run/tls']
        if len(mounts)!=1:raise ValueError('PostgreSQL TLS mount required')
        for path in Path(node['tls_root']).iterdir():
            if path.is_symlink() or not path.is_file() or path.read_bytes()!=(Path(mounts[0]['Source'])/path.name).read_bytes():
                raise ValueError('TLS copy drift')
        return node['postgres_uid']
    return data_runner.Runner(docker,d['image_id'],d['network_id'],
        {'/run/bootstrap':node['bootstrap_root'],'/run/tls':node['tls_root'],'/run/siemcore-postgres':node['socket_root']},
        artifact,authorize,dependency,timeout=300)

order=[('1','runtime'),('2','runtime'),('1','schema'),('2','schema'),('2','seed'),('1','runtime')]
for sequence,(node_id,stage) in enumerate(order,1):
    deadline=time.monotonic()+570;request_path=root/('request-%d.json'%sequence)
    while not request_path.exists():
        if time.monotonic()>deadline:raise TimeoutError('combined request missing')
        time.sleep(.1)
    try:
        request=protocol.strict_json(client.protected(request_path));node=d['nodes'][node_id]
        field={'runtime':'runtime_input','schema':'schema_config','seed':'seed_config'}[stage]
        if request!={'sequence':sequence,'node_id':node_id,'stage':stage,'input_file':node[field]}:
            raise ValueError('unexpected stage order or path')
        original=client.protected(node['original_input'])
        document=protocol.strict_json(original)
        if document.get('registry')!=registry or document.get('node_id')!=node_id:raise ValueError('original registry/node mismatch')
        config=protocol.strict_json(client.protected(node[field]))
        sync=selective_sync.validate(original,registry,node['input_sha256'],node_id,config if stage=='seed' else None)
        authorize=authorizer(node_id,node)
        journal=Path(node['journal_directory'])
        if stage=='runtime':
            if set(config)!={'config','tls_material','binding'}:raise ValueError('runtime input shape')
            expected=dict(operation_id=registry['operation_id'],generation=d['generation'],input_sha256=node['input_sha256'],artifact_sha256=registry['release']['sha256'])
            if config['binding']!=expected or config['config']['node_id']!=node_id or config['config']['pod_id']!=registry['pod_id']:
                raise ValueError('runtime binding mismatch')
            receipt=runtime_worker.invoke(node['runtime_directory'],config['config'],config['tls_material'],expected,
                journal/'runtime.json',bundle,authorize,timeout=300)
        else:
            if config.get('initial_sync')!=sync:raise ValueError('initial sync mismatch')
            plan=data_stage.prepare(config,registry,d['registry_sha256'],node_id,d['generation'],node['input_sha256'])
            # Exact protected per-stage input mounted at the artifact's fixed path.
            client.durable(Path(node['bootstrap_root'])/'data.json',config)
            receipt=data_stage.invoke_fixture(plan,journal/(stage+'.json'),data_factory(node_id,node,config,authorize))
        result=dict(exit_code=0,stdout=base64.b64encode(json.dumps(receipt,separators=(',',':')).encode()).decode())
    except Exception as error:
        print(json.dumps(dict(sequence=sequence,error_type=type(error).__name__)),flush=True)
        result=dict(exit_code=1,stdout='')
    client.durable(root/('response-%d.json'%sequence),result)
    print(json.dumps(dict(sequence=sequence,node_id=node_id,stage=stage,exit_code=result['exit_code'])),flush=True)
    if result['exit_code']:break
