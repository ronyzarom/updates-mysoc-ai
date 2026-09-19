import base64,copy,fcntl,hashlib,json,os,socket,subprocess,sys,tempfile,time
from pathlib import Path as P
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
assert os.geteuid()==0 and socket.gethostname()=='ip-172-26-13-54'
assert subprocess.run(['systemctl','is-active','siemcore-cascade-updater'],capture_output=True,text=True).stdout.strip()=='inactive'
sys.path.insert(0,'/usr/local/lib/siemcore-recovery/1.0.0.4');import recovery as r
policy=P('/etc/siemcore-cascade-updater/recovery-policy.json');r.a.trusted(policy)
raw=policy.read_bytes();p=json.loads(raw);receipt=json.loads(P(sys.argv[2]).read_text())
assert receipt['product']=='siemcore' and receipt['version']=='3.3.152.50' and receipt['sha256']=='9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc'
assert receipt['public_key']==p['public_key']=='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57'
Ed25519PublicKey.from_public_bytes(bytes.fromhex(p['public_key'])).verify(base64.b64decode(receipt['signature']),('mysoc-release-v1\nsiemcore\n3.3.152.50\n'+receipt['sha256']).encode())
store=P('/var/lib/siemcore-recovery');r.a.trusted(store)
with open(store/'lock','a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB);r.LOCK_FD=lock.fileno();r.DEADLINE=time.monotonic()+780
 journal=store/'transaction.json';before=journal.read_bytes();assert json.loads(before)['stage']=='applied'
 if sys.argv[1]=='stage':
  assert hashlib.sha256(raw).hexdigest()=='abcfd4db04599a5d202a69b981bf8b2854cafcac922d17717aa528d84a112bb3'
  assert p['policy_revision']==9 and '3.3.152.50' not in p['releases']
  p['policy_revision']=10;p['allowed_transitions'].append(['3.3.152.49','3.3.152.50'])
  p['releases']['3.3.152.50']=dict(artifact='/var/lib/siemcore-cascade-updater/artifacts/siemcore-3.3.152.50.artifact',sha256=receipt['sha256'],signature=receipt['signature'])
  p['expected_mounts_by_version']['3.3.152.50']=copy.deepcopy(p['expected_mounts_by_version']['3.3.152.49'])
  encoded=json.dumps(p,sort_keys=True,indent=2).encode()+b'\n';history=store/'policy-provisioning';r.a.trusted(history)
  r.durable(history/(hashlib.sha256(raw).hexdigest()+'.json'),raw);r.durable(history/(hashlib.sha256(encoded).hexdigest()+'.json'),encoded);r.durable(policy,encoded)
 elif sys.argv[1]=='preflight':
  assert p['policy_revision']==10 and ['3.3.152.49','3.3.152.50'] in p['allowed_transitions']
  with tempfile.TemporaryDirectory(dir=store,prefix='setup50-preflight-') as tmp:
   tx=r.Lifecycle(dict(p,journal_dir=tmp)).snapshot('3.3.152.50');assert tx['from_version']=='3.3.152.49'
 else:raise ValueError('unknown operation')
 assert journal.read_bytes()==before
 print(json.dumps(dict(operation=sys.argv[1],policy_revision=p['policy_revision'],policy_sha256=hashlib.sha256(policy.read_bytes()).hexdigest(),journal_unchanged=True)))
