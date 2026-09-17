"""Source-only bounded application installation; never completes or activates a POD."""
import hashlib
import json
import os
from pathlib import Path
import selectors
import signal
import stat
import subprocess
import sys
import time
import types
import client
import protocol
import runtime_worker

NAME='pod-application-install-v1'

def load(manifest,raw,name):
    code=runtime_worker.verify_module(manifest,raw,sys.version_info,name)
    module=types.ModuleType('verified_'+name.replace('-','_'))
    exec(code,module.__dict__)
    return module


def supervised(args,*,env,check,capture_output,text,timeout):
    """Keep children in the worker group so its outer deadline kills descendants."""
    if check is not True or capture_output is not True or text is not True or not 0<timeout<=240:
        raise ValueError('bounded subprocess contract required')
    process=subprocess.Popen(args,env=env,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    selector=selectors.DefaultSelector();out=bytearray();total=0;deadline=time.monotonic()+timeout
    try:
        for stream in (process.stdout,process.stderr):selector.register(stream,selectors.EVENT_READ)
        while selector.get_map():
            remaining=deadline-time.monotonic()
            if remaining<=0:raise TimeoutError('application subprocess deadline exceeded')
            for key,_ in selector.select(min(remaining,.2)):
                chunk=os.read(key.fileobj.fileno(),4096)
                if not chunk:selector.unregister(key.fileobj);continue
                total+=len(chunk)
                if total>1024*1024:raise ValueError('application subprocess output limit exceeded')
                if key.fileobj is process.stdout:out.extend(chunk)
        code=process.wait(timeout=max(.001,deadline-time.monotonic()))
        if code:raise ValueError('application subprocess failed; retain partial state')
        return subprocess.CompletedProcess(args,code,out.decode(),'')
    finally:
        selector.close()
        if process.poll() is None:process.kill()
        process.wait();process.stdout.close();process.stderr.close()


def verify_tree(root,members):
    """members are exact relative regular-file bytes from authenticated archive."""
    root=Path(root)
    for parent in (root,*root.parents):
        info=parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid or info.st_mode&0o022:
            raise ValueError('unprotected extracted bundle')
    actual=set()
    for path in root.rglob('*'):
        info=path.lstat()
        if info.st_uid or info.st_mode&0o022 or not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)):
            raise ValueError('unsafe extracted bundle member')
        if stat.S_ISDIR(info.st_mode):continue
        name=path.relative_to(root).as_posix();actual.add(name)
        if name not in members or info.st_size!=len(members[name]):raise ValueError('unexpected extracted file')
        fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW)
        with os.fdopen(fd,'rb') as stream:
            if stream.read(len(members[name])+1)!=members[name]:raise ValueError('extracted file changed')
    if actual!=set(members):raise ValueError('missing extracted bundle file')


def capture(directory,runtime_input,receipt,manifest,raw,inspect):
    """Called immediately after authenticated runtime, never at app dispatch."""
    module=load(manifest,raw,'pod-data-runtime-v1')
    config=runtime_input['config'];binding=runtime_input['binding']
    digest=hashlib.sha256(json.dumps({'config':config,'tls':runtime_input['tls_material']},sort_keys=True,separators=(',',':')).encode()).hexdigest()
    expected=dict(protocol='pod-data-stage-v1',identity=dict(protocol='pod-independent-data-v1',pod_id=config['pod_id'],node_id=config['node_id'],database=config['database']),binding=binding,configuration_sha256=digest,phase='data-services-ready',processing_enabled=False,installation_complete=False)
    if receipt!=expected:raise ValueError('prior runtime receipt mismatch')
    plan=module.plan(config)['compose']
    planned=json.dumps(plan,sort_keys=True).encode()
    actual=client.protected(Path(directory)/'compose.json')
    if actual!=planned:raise ValueError('runtime compose differs from authenticated inputs')
    name='siemcore-pod-local-'+config['node_id'];network=inspect(['network','inspect',name])[0]
    evidence=dict(network_id=network['Id'],internal=network['Internal'])
    for role in ('postgres','redis'):
        container=inspect(['container','inspect',plan['services'][role]['container_name']])[0]
        image=inspect(['image','inspect',config[role+'_image']])[0]
        if container['Image']!=image['Id'] or config[role+'_image'] not in (image.get('RepoDigests') or []) or not container['State']['Running'] or container['State'].get('Health',{}).get('Status')!='healthy':
            raise ValueError('prior data service is not verified healthy image')
        if container['NetworkSettings']['Networks'][name]['NetworkID']!=network['Id'] or container['Id'] not in network.get('Containers',{}):raise ValueError('prior network membership mismatch')
        evidence[role+'_id']=container['Id']
    return dict(binding=binding,runtime_receipt=receipt,data_runtime_directory=str(directory),data_configuration_sha256=digest,data_compose_sha256=hashlib.sha256(actual).hexdigest(),data_evidence=evidence)


def validate_prior(config,binding,prior):
    if prior['binding']!=binding or prior['runtime_receipt']['binding']!=binding or prior['runtime_receipt']['phase']!='data-services-ready':raise ValueError('original data-stage binding required')
    for field in ('data_runtime_directory','data_configuration_sha256','data_compose_sha256','data_evidence'):
        if config[field]!=prior[field]:raise ValueError('application changed prior runtime evidence')
    identity=prior['runtime_receipt']['identity']
    if identity['pod_id']!=config['pod_id'] or identity['node_id']!=config['node_id'] or identity['database']!=config['database_name']:raise ValueError('application data identity changed')


def validate_v2_original(config,binding,original):
    if not isinstance(original,bytes) or hashlib.sha256(original).hexdigest()!=binding['input_sha256']:
        raise ValueError('exact original registration bytes required for v2')
    document=protocol.strict_json(original);app=document['application'];registry=document['registry']
    expected={key:config[key] for key in ('protocol','archive_readiness_file','archive_readiness_sha256')}
    coordinator=app.get('bootstrap_coordinator',{})
    if (app.get('schema')!=4 or app.get('topology')!='pod' or app.get('cluster_id')!=config['pod_id'] or
        app.get('bootstrap_installation')!=expected or
        set(coordinator)!={'registry','observer','invitation_file','authorization_key_file'} or
        coordinator['registry']!=registry or registry['operation_id']!=binding['operation_id'] or
        registry['release']['sha256']!=binding['artifact_sha256']):
        raise ValueError('v2 must be predeclared in the original coordinator input')
    nodes=[n for n in registry['nodes'] if n['node_id']==config['node_id']]
    if (len(nodes)!=1 or nodes[0]['updater_id']!=app.get('updater_instance_id') or
        {'a':'1','b':'2','witness':'witness'}.get(app.get('pod_role'))!=config['node_id']):
        raise ValueError('original v2 updater identity mismatch')


def readiness_bytes(config):
    raw=client.protected(config['archive_readiness_file'],limit=32768)
    if hashlib.sha256(raw).hexdigest()!=config['archive_readiness_sha256']:
        raise ValueError('original archive readiness bytes changed')
    protocol.strict_json(raw)
    return raw


def configuration_hash(config):
    secrets={k:client.protected(config[p]).decode().strip() for k,p in [('database','database_password_file'),('redis','redis_password_file'),('jwt','jwt_secret_file')]}
    assets={}
    for field,names in [('database_tls_directory',('ca.crt','client.crt','client.key')),('syslog_tls_directory',('tls.crt','tls.key'))]:
        for name in names:assets[field+'/'+name]=hashlib.sha256(client.protected(Path(config[field])/name)).hexdigest()
    archive=client.protected(config['archive_input_file']);parsed=protocol.strict_json(archive)
    auth=parsed['archive'].get('authentication',{})
    key=client.protected(auth['path']) if auth.get('mode')=='file' else b''
    if config.get('protocol')=='pod-application-install-v2':
        assets['archive-readiness.json']=hashlib.sha256(readiness_bytes(config)).hexdigest()
    return hashlib.sha256(json.dumps(dict(config=config,assets=assets,secret_hashes={k:hashlib.sha256(v.encode()).hexdigest() for k,v in secrets.items()},archive_sha256=hashlib.sha256(archive).hexdigest(),archive_credential_sha256=hashlib.sha256(key).hexdigest()),sort_keys=True,separators=(',',':')).encode()).hexdigest()


def invoke(bundle,directory,config,binding,prior,journal,verify_bundle,authorize,timeout=300,fixture_lose_completion=False,allow_v2=False,original_input=None):
    if type(fixture_lose_completion) is not bool:raise ValueError('explicit fixture fault flag required')
    if sys.platform!='linux' or os.geteuid()!=0:raise ValueError('Linux root application worker required')
    if type(allow_v2) is not bool:raise ValueError('explicit v2 opt-in required')
    selected=config.get('protocol')
    if selected not in (NAME,'pod-application-install-v2'):raise ValueError('unsupported application protocol')
    if selected=='pod-application-install-v2':
        if not allow_v2:raise ValueError('application v2 disabled')
        validate_v2_original(config,binding,original_input)
    elif any(k in config for k in ('archive_readiness_file','archive_readiness_sha256')):
        raise ValueError('v1 cannot adopt v2 profile fields')
    validate_prior(config,binding,prior)
    manifest,raw,digest=verify_bundle()
    if digest!=binding['artifact_sha256'] or manifest['version']!=config['version'] or manifest['runtime_image_id']!=config['runtime_image_id']:raise ValueError('application release mismatch')
    code=runtime_worker.verify_module(manifest,raw,sys.version_info,NAME)
    expected=dict(protocol=selected,pod_id=config['pod_id'],node_id=config['node_id'],version=config['version'],runtime_image_id=config['runtime_image_id'],binding=binding,configuration_sha256=configuration_hash(config),phase='management-installed-paused',installation_complete=False,processing_allowed=False,activation_ready=False)
    intent=dict(expected=expected,bundle=str(bundle),directory=str(directory),module_sha256=hashlib.sha256(raw).hexdigest())
    journal=Path(journal)
    if journal.exists() and protocol.strict_json(client.protected(journal)).get('intent')!=intent:raise ValueError('application operation changed')
    client.durable(journal,dict(intent=intent,status='incomplete',potential_partial_commit=True))
    def verify():
        current=verify_bundle()
        if current!=(manifest,raw,digest):raise ValueError('installer provenance changed')
        return digest
    def work():
        for name in runtime_worker.IMPORTS:__import__(name)
        sys.path[:]=[p for p in sys.path if p and p.startswith(sys.base_prefix+'/lib/python') and 'site-packages' not in p]
        os.environ.clear();os.environ.update(PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',LANG='C.UTF-8')
        module=types.ModuleType('verified_pod_application_install');exec(code,module.__dict__)
        authorize()
        result=module.install(bundle,directory,config,binding,authorize,verify,run=supervised)
        authorize();verify()
        if fixture_lose_completion:os.kill(os.getpid(),signal.SIGKILL)
        return result
    receipt=runtime_worker.bounded_child(work,timeout)
    if receipt!=expected:raise ValueError('application receipt mismatch; retain partial state')
    if selected=='pod-application-install-v2':
        path=Path(directory)/'archive-readiness.json'
        if stat.S_IMODE(path.lstat().st_mode)!=0o600 or client.protected(path,limit=32768)!=readiness_bytes(config):
            raise ValueError('installed readiness profile mismatch')
    client.durable(journal,dict(intent=intent,status='awaiting-management-observation',receipt=receipt))
    return receipt
