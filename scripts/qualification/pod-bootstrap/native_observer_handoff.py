"""Test-only Observer handoff inside retained disposable systemd fixture."""
import base64
import hashlib
import json
import os
from pathlib import Path
import stat
import tarfile
import time
import client
import handoff
import observer_stage
import observer_runner
import protocol

root=Path('/opt/observer-fixture')
descriptor=protocol.strict_json(client.protected(root/'handoff.json'))
if descriptor['fixture_only'] is not True:raise ValueError('fixture trust only')
registry=descriptor['registry']
key=bytes.fromhex(client.protected(descriptor['release_public_key_file']).decode().strip())
client.validate_registry(registry,key,'arm64')
if handoff.registry_hash(registry)!=descriptor['registry_sha256']:raise ValueError('registry digest mismatch')
original=client.protected(descriptor['original_input'])
if hashlib.sha256(original).hexdigest()!=descriptor['input_sha256'] or protocol.strict_json(original)!={'fixture_only':True,'registry':registry}:
    raise ValueError('original input mismatch')


def verify_artifact(binary):
    artifact=descriptor['artifact']
    for field in ('product','version','sha256','signature'):
        if artifact[field]!=registry['release'][field]:raise ValueError('artifact binding mismatch')
    fd=os.open(artifact['path'],os.O_RDONLY|os.O_NOFOLLOW)
    with os.fdopen(fd,'rb') as source:
        info=os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_mode&0o022:raise ValueError('protected artifact required')
        digest=hashlib.file_digest(source,'sha256').hexdigest()
        if digest!=artifact['sha256']:raise ValueError('signed artifact checksum mismatch')
        source.seek(0)
        with tarfile.open(fileobj=source,mode='r:gz') as archive:
            members=archive.getmembers()
            if {m.name for m in members}!={'bundle/MANIFEST.json','bundle/pod/bin/siemcore'} or len(members)!=2 or not all(m.isfile() for m in members):
                raise ValueError('unexpected fixture archive members')
            manifest=protocol.strict_json(archive.extractfile('bundle/MANIFEST.json').read())
            for field in ('product','version','architecture'):
                if manifest[field]!=registry['release'][field]:raise ValueError('manifest identity mismatch')
            expected=hashlib.file_digest(archive.extractfile('bundle/pod/bin/siemcore'),'sha256').hexdigest()
    if binary!=descriptor['binary'] or expected!=descriptor['binary_sha256']:raise ValueError('host binary differs from signed archive')
    return expected

for sequence in range(1,6):
    deadline=time.monotonic()+240;request_path=root/('request-%d.json'%sequence)
    while not request_path.exists():
        if time.monotonic()>deadline:raise TimeoutError('Observer fixture request missing')
        time.sleep(.1)
    try:
        request=protocol.strict_json(client.protected(request_path))
        if set(request)!={'sequence','descriptor','receipt_config'} or request['sequence']!=sequence or request['descriptor']!=str(root/'handoff.json'):
            raise ValueError('request binding mismatch')
        config=protocol.strict_json(client.protected(descriptor['receipt_config']))
        if config!=request['receipt_config']:raise ValueError('request config drift')
        plan=observer_stage.prepare(config,registry,descriptor['registry_sha256'],descriptor['generation'],descriptor['input_sha256'],
            descriptor['binary'],descriptor['drain_config'],descriptor['update_config'],descriptor['receipt_config'])
        runner=observer_runner.Runner(plan,verify_artifact,timeout=60)
        receipt=observer_stage.invoke_fixture(plan,Path(descriptor['journal_directory'])/'observer-stage.json',runner)
        result=dict(exit_code=0,stdout=base64.b64encode(json.dumps(receipt,separators=(',',':')).encode()).decode())
    except Exception as error:
        print(json.dumps(dict(sequence=sequence,error_type=type(error).__name__)),flush=True)
        result=dict(exit_code=1,stdout='')
    client.durable(root/('response-%d.json'%sequence),result)
    print(json.dumps(dict(sequence=sequence,exit_code=result['exit_code'])),flush=True)
