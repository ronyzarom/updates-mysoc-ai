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
import readiness
import management
import selective_sync
import application_runner

p=argparse.ArgumentParser();p.add_argument('--root',required=True);p.add_argument('--readiness',action='store_true');p.add_argument('--management',action='store_true');p.add_argument('--application',action='store_true');p.add_argument('--application-v2',action='store_true');p.add_argument('--resume-application',action='store_true');p.add_argument('--resume-stage',type=int,choices=(9,10,11,12),default=9);p.add_argument('--installed-retry',action='store_true');p.add_argument('--lost-completion-test',action='store_true');args=p.parse_args()
if args.application_v2 and not args.application:p.error('--application-v2 requires --application')
if args.lost_completion_test and (not args.application or args.installed_retry or args.resume_application):p.error('--lost-completion-test requires --application and excludes other retry flags')
if args.installed_retry and (not args.application or args.resume_application):p.error('--installed-retry requires --application and excludes --resume-application')
if args.resume_application and not args.application:p.error('--resume-application requires --application')
if args.application and not args.management:p.error('--application requires --management')
if args.management and not args.readiness:p.error('--management requires --readiness')
root=Path(args.root)
d=protocol.strict_json(client.protected(root/'handoff.json'))
if d['fixture_only'] is not True or d['root']!=str(root):raise ValueError('exact disposable fixture required')
registry=d['registry'];pin=bytes.fromhex(client.protected(d['release_public_key_file']).decode().strip())
client.validate_registry(registry,pin,'arm64')
if handoff.registry_hash(registry)!=d['registry_sha256']:raise ValueError('registry fingerprint mismatch')
docker='/usr/local/bin/docker' if Path('/usr/local/bin/docker').exists() else '/usr/bin/docker'


def bundle(module_name='pod-data-runtime-v1',extracted=None):
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
        member=archive.getmember('bundle/'+runtime_worker.MODULES[module_name])
        if not member.isfile() or member.size>1024*1024:raise ValueError('invalid module member')
        module=archive.extractfile(member).read()
        if extracted is not None:
            files={}
            for entry in entries:
                if not entry.name.startswith('bundle/') and entry.name!='bundle':raise ValueError('unexpected archive root')
                if entry.isfile():files[entry.name[len('bundle/'):]]=archive.extractfile(entry).read()
            application_runner.verify_tree(extracted,files)
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


def data_factory(node_id,node,config,authorize,readiness_mode=False):
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
    target=data_runner.docker_json(docker,['container','inspect',node['data_container']])[0]
    pinned=data_runner.docker_json(docker,['image','inspect',node['postgres_image']])[0]
    peer_namespace=dict(container_id=target['Id'],image_id=pinned['Id'],network_id=d['network_id'],
                        ip_address='172.30.97.'+str(10+int(node_id)))
    return data_runner.Runner(docker,d['image_id'],d['network_id'],
        {'/run/bootstrap':node['bootstrap_root'],'/run/tls':node['tls_root'],'/run/siemcore-postgres':node['socket_root']},
        artifact,authorize,dependency,timeout=300,peer_namespace=peer_namespace,readiness=readiness_mode)


def management_observation(node_id,node,journal):
    def expectations():
        bundle()  # Authenticate manifest/image/module under the retained test signature.
        raw=client.protected(d['artifact']['path'],limit=256*1024*1024)
        if hashlib.sha256(raw).hexdigest()!=registry['release']['sha256']:raise ValueError('bundle changed')
        with tarfile.open(fileobj=io.BytesIO(raw),mode='r:gz') as archive:
            member=archive.getmember('bundle/pod/bin/siemcore')
            if not member.isfile() or member.size>128*1024*1024:raise ValueError('invalid host binary member')
            binary_hash=hashlib.sha256(archive.extractfile(member).read()).hexdigest()
        image=data_runner.docker_json(docker,['image','inspect',d['image_id']])[0]
        if image['Id']!=d['image_id'] or image['Architecture']!=registry['release']['architecture']:
            raise ValueError('immutable image defaults mismatch')
        overrides=protocol.strict_json(client.protected(node['management_environment_overrides']))
        hashes=management.reviewed_environment_hashes(image['Config'].get('Env') or [],overrides)
        return dict(binary_sha256=binary_hash,runtime_image_id=d['image_id'],environment_sha256=hashes)
    config=protocol.strict_json(client.protected(node['management_config']))
    original=protocol.strict_json(client.protected(node['schema_config']))
    host=protocol.strict_json(client.protected(config['data_binding_file']))
    management.verify_materialization(original,host,node['bootstrap_root'],node['management_materialization'])
    expected=expectations()
    plan=management.prepare(d['host_binary'],node['management_config'],registry,d['registry_sha256'],node_id,
        d['generation'],node['input_sha256'],expected['runtime_image_id'],expected['environment_sha256'])
    return management.observe(plan,journal/'management.json',management.Runner(plan,expectations))

order=[('1','runtime'),('2','runtime'),('1','schema'),('2','schema'),('2','seed'),('1','runtime')]
if args.readiness:order += [('1','readiness'),('2','readiness')]
if args.application:order += [('1','application'),('1','management'),('2','application'),('2','management')]
elif args.management:order += [('1','management'),('2','management')]
if args.lost_completion_test:
    order += [('1','application'),('1','management'),('1','application'),('1','application'),('1','management')]
    for sequence in range(1,15):
        path=root/('response-%d.json'%sequence)
        retry=root/('response-%d-retry.json'%sequence)
        if retry.exists():path=retry
        if protocol.strict_json(client.protected(path))['exit_code']!=0:raise ValueError('fault test requires successful earlier stages')
    for sequence in (15,16,17):
        if (root/('response-%d.json'%sequence)).exists():raise ValueError('fault evidence already exists')
if args.installed_retry:
    order += [('1','application'),('1','management')]
    for sequence in range(1,13):
        path=root/('response-%d.json'%sequence)
        retry=root/('response-%d-retry.json'%sequence)
        if retry.exists():path=retry
        if protocol.strict_json(client.protected(path))['exit_code']!=0:raise ValueError('installed retry requires successful earlier stages')
    for sequence in (13,14):
        if (root/('response-%d.json'%sequence)).exists():raise ValueError('installed retry evidence already exists')
if args.resume_application:
    for sequence in range(1,args.resume_stage):
        path=root/('response-%d.json'%sequence)
        retry=root/('response-%d-retry.json'%sequence)
        if retry.exists():path=retry
        if protocol.strict_json(client.protected(path))['exit_code']!=0:raise ValueError('earlier stage not successful')
    if protocol.strict_json(client.protected(root/('response-%d.json'%args.resume_stage)))['exit_code']!=1:raise ValueError('explicit failed stage required')
    if (root/('response-%d-retry.json'%args.resume_stage)).exists():raise ValueError('retry evidence already exists')
for sequence,(node_id,stage) in enumerate(order,1):
    if args.lost_completion_test and sequence<15:continue
    if args.installed_retry and sequence<13:continue
    if args.resume_application and sequence<args.resume_stage:continue
    deadline=time.monotonic()+570;request_path=root/('request-%d.json'%sequence)
    while not request_path.exists():
        if time.monotonic()>deadline:raise TimeoutError('combined request missing')
        time.sleep(.1)
    try:
        request=protocol.strict_json(client.protected(request_path));node=d['nodes'][node_id]
        field=('schema_config' if node_id=='1' else 'seed_config') if stage=='readiness' else {'runtime':'runtime_input','schema':'schema_config','seed':'seed_config','management':'management_config','application':'application_input'}[stage]
        if request!={'sequence':sequence,'node_id':node_id,'stage':stage,'input_file':node[field]}:
            raise ValueError('unexpected stage order or path')
        original=client.protected(node['original_input'])
        document=protocol.strict_json(original)
        if document.get('registry')!=registry or document.get('node_id')!=node_id:raise ValueError('original registry/node mismatch')
        config=protocol.strict_json(client.protected(node[field]))
        sync=selective_sync.validate(original,registry,node['input_sha256'],node_id,config if 'seed' in config else None)
        authorize=authorizer(node_id,node)
        journal=Path(node['journal_directory'])
        if stage=='application':
            expected=dict(operation_id=registry['operation_id'],generation=d['generation'],input_sha256=node['input_sha256'],artifact_sha256=registry['release']['sha256'])
            if set(config)!={'config','binding'} or config['binding']!=expected or config['config']['node_id']!=node_id or config['config']['pod_id']!=registry['pod_id']:raise ValueError('application input binding mismatch')
            prior=protocol.strict_json(client.protected(journal/'application-data-evidence.json'))
            receipt=application_runner.invoke(d['application_bundle'],node['application_directory'],config['config'],expected,prior,journal/'application.json',lambda:bundle(application_runner.NAME,d['application_bundle']),authorize,fixture_lose_completion=args.lost_completion_test and sequence==15,allow_v2=args.application_v2,original_input=original)
        elif stage=='management':
            receipt=management_observation(node_id,node,journal)
        elif stage=='runtime':
            if set(config)!={'config','tls_material','binding'}:raise ValueError('runtime input shape')
            expected=dict(operation_id=registry['operation_id'],generation=d['generation'],input_sha256=node['input_sha256'],artifact_sha256=registry['release']['sha256'])
            if config['binding']!=expected or config['config']['node_id']!=node_id or config['config']['pod_id']!=registry['pod_id']:
                raise ValueError('runtime binding mismatch')
            receipt=runtime_worker.invoke(node['runtime_directory'],config['config'],config['tls_material'],expected,
                journal/'runtime.json',bundle,authorize,timeout=300)
            if args.application:
                manifest,module,_=bundle()
                evidence=application_runner.capture(node['runtime_directory'],config,receipt,manifest,module,lambda argv:data_runner.docker_json(docker,argv))
                path=journal/'application-data-evidence.json'
                if path.exists() and protocol.strict_json(client.protected(path))!=evidence:raise ValueError('prior runtime evidence changed on retry')
                client.durable(path,evidence)
        else:
            if config.get('initial_sync')!=sync:raise ValueError('initial sync mismatch')
            # Exact protected per-stage input mounted at the artifact's fixed path.
            client.durable(Path(node['bootstrap_root'])/'data.json',config)
            if stage=='readiness':
                plan=readiness.prepare(config,registry,d['registry_sha256'],node_id,d['generation'],node['input_sha256'],original)
                receipt=readiness.observe(plan,journal/'readiness.json',data_factory(node_id,node,config,authorize,readiness_mode=True))
            else:
                plan=data_stage.prepare(config,registry,d['registry_sha256'],node_id,d['generation'],node['input_sha256'])
                receipt=data_stage.invoke_fixture(plan,journal/(stage+'.json'),data_factory(node_id,node,config,authorize))
        result=dict(exit_code=0,stdout=base64.b64encode(json.dumps(receipt,separators=(',',':')).encode()).decode())
    except Exception as error:
        print(json.dumps(dict(sequence=sequence,error_type=type(error).__name__)),flush=True)
        result=dict(exit_code=1,stdout='')
    response=root/('response-%d-retry.json'%sequence if args.resume_application and sequence==args.resume_stage else 'response-%d.json'%sequence)
    if response.exists():raise ValueError('refusing to overwrite fixture response')
    client.durable(response,result)
    print(json.dumps(dict(sequence=sequence,node_id=node_id,stage=stage,exit_code=result['exit_code'])),flush=True)
    if args.lost_completion_test and sequence==15:
        state=protocol.strict_json(client.protected(journal/'application.json'))
        if result['exit_code']!=1 or state.get('status')!='incomplete':raise ValueError('lost completion did not retain incomplete state')
        continue
    if result['exit_code']:break
