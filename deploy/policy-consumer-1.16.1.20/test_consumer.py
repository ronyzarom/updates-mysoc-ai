import base64,copy,hashlib,importlib.util,json,pathlib,subprocess,tempfile,time,types,unittest
from unittest.mock import patch
P=pathlib.Path
spec=importlib.util.spec_from_file_location('consumer',P(__file__).with_name('consumer.py'));c=importlib.util.module_from_spec(spec);spec.loader.exec_module(c)
class ConsumerTests(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  cls.keys=tempfile.TemporaryDirectory();d=P(cls.keys.name);cls.key=d/'key'
  subprocess.run(['openssl','genpkey','-algorithm','ED25519','-out',str(cls.key)],check=True,capture_output=True)
  cls.public=subprocess.check_output(['openssl','pkey','-in',str(cls.key),'-pubout','-outform','DER'])[-32:].hex()
 @classmethod
 def tearDownClass(cls):cls.keys.cleanup()
 def setUp(self):
  self.old={'hostname':'fixture-host','public_key':self.public,'journal_dir':'/var/lib/siemcore-recovery','health_url':'http://fixture','policy_revision':3,'releases':{'3.3.152.19':{'retained':'yes'}},'allowed_transitions':[],'expected_mounts':{'app':[{'Name':'existing-volume'}]},'trust':'unchanged'}
  self.raw=json.dumps(self.old).encode();now=int(time.time())
  self.payload={'protocol':'mysoc-policy-authorization-v1','hostname':'fixture-host','product':'siemcore','old_policy_sha256':c.digest(self.raw),'policy_revision':4,'from_version':'3.3.152.19','target_version':'3.3.152.22','artifact_sha256':'a'*64,'artifact_signature':base64.b64encode(b'x'*64).decode(),'source_commit':'b'*40,'issued_at':now-1,'expires_at':now+300}
 def signed(self,payload):
  with tempfile.TemporaryDirectory() as tmp:
   d=P(tmp);(d/'msg').write_bytes(c.DOMAIN+c.canonical(payload))
   sig=subprocess.check_output(['openssl','pkeyutl','-sign','-inkey',str(self.key),'-rawin','-in',str(d/'msg')])
  return {'payload':payload,'signature':base64.b64encode(sig).decode()}
 def test_exact_additive_scope(self):
  new=c.candidate(self.old,self.raw,self.payload,'fixture-host',int(time.time()))
  self.assertEqual(new['trust'],self.old['trust']);self.assertEqual(new['releases']['3.3.152.19'],self.old['releases']['3.3.152.19'])
  self.assertEqual(new['expected_mounts_by_version']['3.3.152.22'],self.old['expected_mounts']);self.assertEqual(new['allowed_transitions'],[['3.3.152.19','3.3.152.22']])
 def test_negative_authorizations(self):
  cases={'hostname':'other','product':'mysoc','old_policy_sha256':'0'*64,'policy_revision':5,'from_version':'3.3.152.18','target_version':'../../etc/shadow','artifact_sha256':'latest','artifact_signature':'invalid','source_commit':'644c035','expires_at':0,'issued_at':int(time.time())+100}
  for field,value in cases.items():
   with self.subTest(field=field):
    payload=dict(self.payload);payload[field]=value
    with self.assertRaises((ValueError,TypeError)):c.candidate(self.old,self.raw,payload,'fixture-host',int(time.time()))
  payload=dict(self.payload,command='sh')
  with self.assertRaises(ValueError):c.candidate(self.old,self.raw,payload,'fixture-host',int(time.time()))
 def test_real_ed25519_tamper_and_domain(self):
  e=self.signed(self.payload);c.verify(self.public,e['payload'],e['signature'])
  e['payload']=dict(e['payload'],target_version='3.3.152.23')
  with self.assertRaises(ValueError):c.verify(self.public,e['payload'],e['signature'])
  with self.assertRaises(ValueError):c.verify('00'*32,self.payload,e['signature'])
 def test_request_refuses_symlink_oversize_duplicate(self):
  with tempfile.TemporaryDirectory() as tmp:
   d=P(tmp);p=d/'request';p.write_bytes(b'x'*65537)
   with self.assertRaises(ValueError):c.read_request(p)
   p.write_text('{"payload":{},"payload":{},"signature":"a"}')
   with self.assertRaises(ValueError):c.read_request(p)
   (d/'link').symlink_to(p)
   with self.assertRaises(OSError):c.read_request(d/'link')
 def exercise_commit(self,fail_at=None,pending=False):
  with tempfile.TemporaryDirectory() as tmp:
   d=P(tmp);policy=d/'policy';policy.write_bytes(self.raw);request=d/'request';request.write_text(json.dumps(self.signed(self.payload)));journal=d/'transaction.json';journal.write_text(json.dumps({'stage':'applying' if pending else 'applied'}));prior=journal.read_bytes();writes=[];failure=[fail_at]
   def durable(path,raw):
    if failure[0] is not None and len(writes)==failure[0]:failure[0]=None;raise OSError('injected interruption')
    path.write_bytes(raw);writes.append(path)
   snapshots=[]
   class Lifecycle:
    def __init__(self,policy):self.policy=policy
    def snapshot(self,target):snapshots.append(target)
   a=types.SimpleNamespace(trusted=lambda _:None,health=lambda _:{'version':'3.3.152.19'})
   recovery=types.SimpleNamespace(Lifecycle=Lifecycle,durable=durable)
   mapper=lambda x:d if str(x)=='/var/lib/siemcore-recovery' else P(x)
   with patch.multiple(c,P=mapper,POLICY=policy,REQUEST=request,STORE=d/'receipts'),patch.object(c.socket,'gethostname',return_value='fixture-host'):
    if pending:
     with self.assertRaises(ValueError):c.install_authorization(a,recovery)
     self.assertEqual(policy.read_bytes(),self.raw);return
    if fail_at is not None:
     with self.assertRaises(OSError):c.install_authorization(a,recovery)
    c.install_authorization(a,recovery)
    new=policy.read_bytes();c.install_authorization(a,recovery)
    self.assertEqual(policy.read_bytes(),new);self.assertEqual(journal.read_bytes(),prior)
    self.assertEqual(json.loads(new)['policy_revision'],4)
 def test_atomic_commit_retry(self):self.exercise_commit()
 def test_interruption_each_write(self):
  for n in range(4):
   with self.subTest(write=n):self.exercise_commit(n)
 def test_pending_transaction(self):self.exercise_commit(pending=True)
if __name__=='__main__':unittest.main()
