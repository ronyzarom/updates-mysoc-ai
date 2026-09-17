"""Disposable test-only signed-fixture handoff. Never deployment trust."""
import argparse
import hashlib
import json
import time
from pathlib import Path
import authorization
import client
import data_runner
import handoff
import protocol

parser=argparse.ArgumentParser();parser.add_argument('--descriptor',required=True)
args=parser.parse_args();descriptor=json.loads(Path(args.descriptor).read_text())
root=Path(descriptor['handoff_root']);(root/'receipts').mkdir(mode=0o700,exist_ok=True)
docker='/usr/local/bin/docker'

def factory(request,config,plan):
    def artifact(image):
        if image!=descriptor['image_id'] or image!=request['image_id']:raise ValueError('image drift')
        raw=client.protected(root/'artifact.fixture.json')
        manifest=protocol.strict_json(raw)
        release=request['registry']['release']
        expected=dict(fixture_only=True,product='siemcore',version=release['version'],architecture=release['architecture'],image_id=image)
        if manifest!=expected or hashlib.sha256(raw).hexdigest()!=release['sha256']:
            raise ValueError('test-only signed manifest mismatch')
        original=protocol.strict_json(client.protected(root/'original-input.json'))
        if original!={'fixture_only':True,'registry':request['registry']}:
            raise ValueError('fixture original input mismatch')
        details=data_runner.docker_json(docker,['image','inspect',image])[0]
        if details['Architecture']!=release['architecture']:raise ValueError('image architecture mismatch')
    def dependency():
        pinned=data_runner.docker_json(docker,['image','inspect',descriptor['postgres_image']])[0]
        runtime=data_runner.docker_json(docker,['container','inspect',descriptor['data_container']])[0]
        if runtime['Image']!=pinned['Id'] or not runtime['State']['Running'] or runtime['State'].get('Health',{}).get('Status')!='healthy':
            raise ValueError('pinned healthy PostgreSQL runtime required')
        code,out=data_runner.bounded([docker,'exec',descriptor['data_container'],'id','-u','postgres'],10)
        if code or int(out.strip())!=descriptor['postgres_uid']:raise ValueError('PostgreSQL image UID mismatch')
        # Compare the read-only command TLS copy to the actual PG runtime TLS mount.
        mounts=[m for m in runtime['Mounts'] if m['Destination']=='/run/tls']
        if len(mounts)!=1:raise ValueError('exact PostgreSQL TLS mount required')
        for path in Path(descriptor['tls_root']).iterdir():
            if path.is_symlink() or not path.is_file():raise ValueError('invalid TLS copy')
            if path.read_bytes()!=(Path(mounts[0]['Source'])/path.name).read_bytes():raise ValueError('TLS copy bytes differ')
        return descriptor['postgres_uid']
    def runtime():
        def local(path):
            p=Path(path)
            if p.parent!=Path('/run/bootstrap'):raise ValueError('unexpected protected credential path')
            return root/'bootstrap'/p.name
        invitation=protocol.strict_json(client.protected(local(config['invitation_file'])))
        key=bytes.fromhex(client.protected(local(config['authorization_key'])).decode().strip())
        expected=dict(operation_id=request['registry']['operation_id'],registry_sha256=request['registry_sha256'],
                      node_id=request['node_id'],input_sha256=request['input_sha256'])
        claims=authorization.verify_invitation(invitation,key,expected,time.time_ns())
        transport=client.Transport(dict(endpoint=config['observer']['endpoint'],
            ca_file=str(local(config['observer']['tls']['ca'])),
            client_cert_file=str(local(config['observer']['tls']['certificate'])),
            client_key_file=str(local(config['observer']['tls']['key'])),
            certificate_sha256=config['observer']['certificate_sha256']))
        sent=authorization.request(request['registry'],request['registry_sha256'],request['node_id'],
             'authorize',request['generation'],request['input_sha256'],invitation)
        authorization.validate_ack(sent,'authorize',transport.call('authorize',sent,10),claims['authorization_id'],claims['expires_at'])
    return data_runner.Runner(docker,descriptor['image_id'],descriptor['network_id'],
         {'/run/bootstrap':str(root/'bootstrap'),'/run/tls':descriptor['tls_root'],
          '/run/siemcore-postgres':descriptor['socket_root']},artifact,runtime,dependency,timeout=60)

for sequence in range(1,5):
    deadline=time.monotonic()+240
    while not (root/('request-%d.json'%sequence)).exists():
        if time.monotonic()>deadline:raise TimeoutError('fixture request missing')
        time.sleep(.1)
    result=handoff.consume(root,sequence,factory)
    print(json.dumps(dict(sequence=sequence,exit_code=result['exit_code'])),flush=True)
