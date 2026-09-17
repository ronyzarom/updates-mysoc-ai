"""Qualification-only isolated worker for an authenticated artifact host module."""
import ast
import hashlib
import json
import os
from pathlib import Path
import selectors
import signal
import sys
import time
import types
import client
import protocol

MODULE_PATH='updater/pod_data_runtime.py'
IMPORTS={'ipaddress','re','hashlib','json','os','pathlib','stat','tempfile','fcntl','subprocess'}


def verify_module(manifest,raw,python_version):
    if not (3,10)<=tuple(python_version[:2])<(3,13):raise ValueError('host module requires Python3.10-3.12')
    if manifest.get('product')!='siemcore':raise ValueError('wrong module product')
    entries=manifest.get('bootstrap_host_modules')
    if not isinstance(entries,list) or len(entries)!=1:raise ValueError('exact reviewed module collection required')
    entry=entries[0]
    if not isinstance(entry,dict) or set(entry)!={'name','path','sha256','size','python','dependencies'}:
        raise ValueError('exact module descriptor required')
    if (entry['name']!='pod-data-runtime-v1' or entry['path']!=MODULE_PATH or entry['python']!='>=3.10,<3.13' or
            entry['dependencies']!=[] or type(entry['size']) is not int or not 0<entry['size']<=1024*1024 or
            entry['size']!=len(raw) or hashlib.sha256(raw).hexdigest()!=entry['sha256']):
        raise ValueError('host module provenance mismatch')
    tree=ast.parse(raw,filename=MODULE_PATH)
    for item in ast.walk(tree):
        if isinstance(item,ast.Import):
            if any(alias.name.split('.')[0] not in IMPORTS for alias in item.names):raise ValueError('undeclared import')
        elif isinstance(item,ast.ImportFrom):
            if item.level or not item.module or item.module.split('.')[0] not in IMPORTS:raise ValueError('unbound helper')
        elif isinstance(item,ast.Call) and isinstance(item.func,ast.Name) and item.func.id in ('__import__','eval','exec'):
            raise ValueError('undeclared dynamic loading')
    return compile(tree,MODULE_PATH,'exec')


def bounded_child(action,timeout):
    """Fork isolated control process; daemon-owned partial runtime is retained."""
    if type(timeout) is not int or not 1<=timeout<=600:raise ValueError('bounded worker deadline required')
    readfd,writefd=os.pipe();pid=os.fork()
    if pid==0:
        os.close(readfd)
        try:
            os.setsid()
            null=os.open(os.devnull,os.O_RDWR)
            for fd in (0,1,2):os.dup2(null,fd)
            if null>2:os.close(null)
            try:result={'ok':True,'receipt':action()}
            except Exception:result={'ok':False}
            raw=json.dumps(result,separators=(',',':')).encode()
            if len(raw)>8192:raw=b'{"ok":false}'
            offset=0
            while offset<len(raw):offset+=os.write(writefd,raw[offset:])
        finally:os._exit(0)
    os.close(writefd);selector=selectors.DefaultSelector();selector.register(readfd,selectors.EVENT_READ)
    deadline=time.monotonic()+timeout;raw=bytearray()
    try:
        while True:
            remaining=deadline-time.monotonic()
            if remaining<=0:raise TimeoutError('host runtime worker interrupted; retain partial state')
            if not selector.select(min(remaining,.2)):continue
            chunk=os.read(readfd,8193)
            if not chunk:break
            raw.extend(chunk)
            if len(raw)>8192:raise ValueError('oversized runtime receipt')
        result=protocol.strict_json(raw)
        if set(result)!={'ok','receipt'} or result['ok'] is not True:raise ValueError('runtime worker failed; retain partial state')
        return result['receipt']
    finally:
        selector.close();os.close(readfd)
        # Stop only the worker/process group, never daemon containers/data/barrier.
        completed,_=os.waitpid(pid,os.WNOHANG)
        if not completed:
            try:os.killpg(pid,signal.SIGKILL)
            except (ProcessLookupError,PermissionError):
                try:os.kill(pid,signal.SIGKILL)
                except ProcessLookupError:pass
            os.waitpid(pid,0)


def invoke(directory,config,tls,binding,journal,verify_bundle,authorize,timeout=300):
    """verify_bundle must verify retained outer signature/checksum/manifest/member.

    Returns authenticated (manifest, exact_module_bytes, bundle_sha256). No
    default verifier exists. authorize performs fresh pinned exact-operation
    authorization on every call. Caller holds protected operation lock.
    """
    if sys.platform!='linux' or os.geteuid()!=0:raise ValueError('Linux root worker required')
    if not callable(verify_bundle) or not callable(authorize):raise ValueError('real verifier/authorizer required')
    manifest,raw,bundle_sha=verify_bundle()
    if bundle_sha!=binding['artifact_sha256']:raise ValueError('wrong authenticated artifact')
    code=verify_module(manifest,raw,sys.version_info)
    config_hash=hashlib.sha256(json.dumps({'config':config,'tls':tls},sort_keys=True,separators=(',',':')).encode()).hexdigest()
    expected=dict(protocol='pod-data-stage-v1',identity=dict(protocol='pod-independent-data-v1',
         pod_id=config['pod_id'],node_id=config['node_id'],database=config['database']),binding=binding,
         configuration_sha256=config_hash,phase='data-services-ready',processing_enabled=False,installation_complete=False)
    intent=dict(binding=binding,configuration_sha256=config_hash,directory=str(directory),
                module_sha256=hashlib.sha256(raw).hexdigest())
    journal=Path(journal)
    if journal.exists() and protocol.strict_json(client.protected(journal)).get('intent')!=intent:
        raise ValueError('runtime operation changed; explicit reconciliation required')
    client.durable(journal,dict(intent=intent,status='incomplete',potential_partial_commit=True))
    def work():
        # Load verified in-memory bytes, not a second mutable path or PATH module.
        # Preload reviewed stdlib dependencies before restricting search paths.
        for name in IMPORTS:__import__(name)
        sys.path[:]=[p for p in sys.path if p and (p.startswith(sys.base_prefix+'/lib/python') and 'site-packages' not in p)]
        os.environ.clear();os.environ.update(PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',LANG='C.UTF-8')
        module=types.ModuleType('verified_pod_data_runtime');module.__file__=MODULE_PATH
        exec(code,module.__dict__)
        authorize()
        return module.start(directory,config,tls,binding,authorize)
    result=bounded_child(work,timeout)
    if result!=expected or result.get('processing_enabled') is not False or result.get('installation_complete') is not False:
        raise ValueError('runtime receipt mismatch; retain partial state')
    client.durable(journal,dict(intent=intent,status='data-services-ready',receipt=result))
    return result
