#!/usr/bin/env python3
"""Exact signed policy authorization; no network, service changes or database operations."""
import base64, copy, datetime, fcntl, hashlib, importlib, json, os, pathlib, re, socket, stat, subprocess, sys, tempfile, time, signal
P=pathlib.Path
VERSION='1.0.0.1'
RUNTIME=P('/usr/local/lib/siemcore-recovery/1.0.0.4')
POLICY=P('/etc/siemcore-cascade-updater/recovery-policy.json')
REQUEST=P('/var/lib/siemcore-cascade-updater/policy-request.json')
STORE=P('/var/lib/siemcore-recovery/policy-authorizations')
DOMAIN=b'mysoc-policy-authorization-v1\n'
MODULES={'artifact.py':'7ea03e78aa32789cb86e6b3d538315518f006bd384c3d305d1e977293e454dbc','recovery.py':'95b5c64cb8ccf32a449bca2bf7851118abb1d55742bd4513846487010d0eccea','supervise.py':'7a6355b55b4a064adcd1a5a8cac679d5db60d8c6f162b13348de631ac5d52c3d'}
def require(ok,msg):
    if not ok: raise ValueError(msg)
def digest(raw): return hashlib.sha256(raw).hexdigest()
def canonical(value): return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=True).encode()
def verify(public, payload, signature):
    sig=base64.b64decode(signature,validate=True);require(len(sig)==64,'signature length')
    with tempfile.TemporaryDirectory() as tmp:
        d=P(tmp);(d/'key').write_bytes(bytes.fromhex('302a300506032b6570032100'+public));(d/'msg').write_bytes(DOMAIN+canonical(payload));(d/'sig').write_bytes(sig)
        r=subprocess.run(['openssl','pkeyutl','-verify','-pubin','-keyform','DER','-inkey',str(d/'key'),'-rawin','-in',str(d/'msg'),'-sigfile',str(d/'sig')],capture_output=True,timeout=15)
        require(r.returncode==0,'policy signature rejected')
def candidate(old, old_raw, payload, hostname, now):
    fields={'protocol','hostname','product','old_policy_sha256','policy_revision','from_version','target_version','artifact_sha256','artifact_signature','source_commit','issued_at','expires_at'}
    require(set(payload)==fields,'unknown or missing authorization fields')
    require(payload['protocol']=='mysoc-policy-authorization-v1' and payload['product']=='siemcore','wrong protocol/product')
    require(payload['hostname']==hostname==old['hostname'],'wrong host')
    require(payload['old_policy_sha256']==digest(old_raw),'old policy mismatch')
    require(type(payload['policy_revision']) is int and payload['policy_revision']==old.get('policy_revision',0)+1,'revision mismatch')
    issued=payload['issued_at'];expiry=payload['expires_at']
    require(type(issued) is int and type(expiry) is int and issued<=now<expiry and 0<expiry-issued<=3600,'expired/future/overlong authorization')
    before=payload['from_version'];target=payload['target_version']
    require(all(isinstance(v,str) and re.fullmatch(r'\d+\.\d+\.\d+\.\d+',v) for v in (before,target)),'invalid version')
    require(tuple(map(int,target.split('.')))>tuple(map(int,before.split('.'))),'not forward')
    require(before in old['releases'] and target not in old['releases'],'unknown predecessor/reused target')
    require(not any(t[0]==before for t in old['allowed_transitions']),'ambiguous transition')
    require(isinstance(payload['artifact_sha256'],str) and re.fullmatch('[0-9a-f]{64}',payload['artifact_sha256']),'invalid artifact checksum')
    require(isinstance(payload['source_commit'],str) and re.fullmatch('[0-9a-f]{40}',payload['source_commit']),'full source commit required')
    require(len(base64.b64decode(payload['artifact_signature'],validate=True))==64,'invalid artifact signature')
    new=copy.deepcopy(old);new['policy_revision']=payload['policy_revision']
    new['allowed_transitions'].append([before,target])
    new['releases'][target]={'artifact':'/var/lib/siemcore-cascade-updater/artifacts/'+target+'.artifact','sha256':payload['artifact_sha256'],'signature':payload['artifact_signature']}
    mounts=old.get('expected_mounts_by_version',{}).get(before,old.get('expected_mounts'))
    require(mounts is not None,'missing baseline mount policy')
    new.setdefault('expected_mounts_by_version',{})[target]=copy.deepcopy(mounts)
    return new

def read_request(path):
    fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
    with os.fdopen(fd,'rb') as f:
        require(stat.S_ISREG(os.fstat(f.fileno()).st_mode),'request not regular')
        raw=f.read(65537);require(len(raw)<=65536,'oversized request')
    def pairs(items):
        d={}
        for k,v in items: require(k not in d,'duplicate JSON field');d[k]=v
        return d
    envelope=json.loads(raw,object_pairs_hook=pairs)
    require(set(envelope)=={'payload','signature'},'invalid envelope')
    return raw,envelope

def install_authorization(a,recovery):
    raw,envelope=read_request(REQUEST)
    a.trusted(POLICY);old_raw=POLICY.read_bytes();old=json.loads(old_raw)
    require(old['journal_dir']=='/var/lib/siemcore-recovery','journal scope')
    store=P(old['journal_dir']);a.trusted(store)
    with open(store/'lock','a') as lock:
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB);recovery.LOCK_FD=lock.fileno()
        # Read again under the same lock used by application and rollback.
        old_raw=POLICY.read_bytes();old=json.loads(old_raw)
        verify(old['public_key'],envelope['payload'],envelope['signature'])
        STORE.mkdir(mode=0o700,exist_ok=True);a.trusted(STORE)
        receipt=STORE/(digest(raw)+'.json')
        if receipt.exists():
            a.trusted(receipt);prior=json.loads(receipt.read_text())
            if digest(old_raw)==prior['new_policy_sha256']:
                print('MYSOC_POLICY_RESULT_V1:'+json.dumps({'request_sha256':digest(raw),'policy_sha256':prior['new_policy_sha256'],'status':'applied'},sort_keys=True,separators=(',',':')));return
            require(digest(old_raw)==prior['old_policy_sha256'],'receipt/current policy mismatch')
        new=candidate(old,old_raw,envelope['payload'],socket.gethostname(),int(datetime.datetime.now(datetime.timezone.utc).timestamp()))
        journal=store/'transaction.json';journal_raw=journal.read_bytes() if journal.exists() else None
        if journal_raw:require(json.loads(journal_raw)['stage'] in ('applied','restored'),'pending application transaction')
        require(a.health(old['health_url'])['version']==envelope['payload']['from_version'],'runtime predecessor mismatch')
        # Existing 1.0.0.4 checks both signed archives, actual image IDs,
        # runtime identity, health, mounts, ports, env pins and retained storage.
        # Its temporary snapshot contains configuration only, never database data.
        with tempfile.TemporaryDirectory(dir=store,prefix='policy-preflight-') as tmp:
            check=recovery.Lifecycle(dict(new,journal_dir=tmp));check.snapshot(envelope['payload']['target_version'])
        require((journal.read_bytes() if journal.exists() else None)==journal_raw,'active journal changed')
        require(POLICY.read_bytes()==old_raw,'policy changed during validation')
        new_raw=(json.dumps(new,indent=2)+'\n').encode()
        # Prepare receipt before policy replace. Retry can finish an interrupted commit.
        result={'request_sha256':digest(raw),'old_policy_sha256':digest(old_raw),'new_policy_sha256':digest(new_raw),'version':VERSION}
        recovery.durable(STORE/(digest(old_raw)+'.policy.json'),old_raw)
        recovery.durable(STORE/(digest(new_raw)+'.policy.json'),new_raw)
        recovery.durable(receipt,canonical(result)+b'\n')
        recovery.durable(POLICY,new_raw)
        print('MYSOC_POLICY_RESULT_V1:'+json.dumps({'request_sha256':digest(raw),'policy_sha256':digest(new_raw),'status':'applied'},sort_keys=True,separators=(',',':')))

def main():
    require(os.geteuid()==0 and sys.argv[1:] in (['apply'],['rollback']),'root/exact phase required')
    os.environ.clear();os.environ.update(PATH='/usr/sbin:/usr/bin:/sbin:/bin',HOME='/root',LANG='C.UTF-8')
    for name,sha in MODULES.items():
        p=RUNTIME/name
        for x in [p,*p.parents]:
            s=x.lstat();require(not stat.S_ISLNK(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,'untrusted runtime')
        require(digest(p.read_bytes())==sha,'runtime checksum mismatch')
    if sys.argv[1]=='apply' and os.path.lexists(REQUEST):
        sys.path.insert(0,str(RUNTIME));a=importlib.import_module('artifact');recovery=importlib.import_module('recovery')
        recovery.DEADLINE=time.monotonic()+780
        for signum in (signal.SIGTERM,signal.SIGINT):signal.signal(signum,recovery.interrupted)
        install_authorization(a,recovery);return
    os.execv('/usr/bin/python3',['/usr/bin/python3',str(RUNTIME/'recovery.py'),sys.argv[1]])
if __name__=='__main__':main()
