import contextlib,copy,json,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import runner as r
import normal_boundary as n


def intent():return {'schema':1,'product':'siemcore','operation_id':'recovery-1','pod_id':'pod','node_id':'b','machine_id':'machine','version':'3.3.152.34','artifact_sha256':'a'*64,'maintenance':{'operation_id':'maintenance-1','generation':42},'expected_owner':'','activation_allowed':False}
POLICY={'machine_id':'machine','topology':'pod','pod_role':'b','cluster_id':'pod'}

def receipt(value=None):
 d=dict(intent() if value is None else value,status='staged-paused',runtime_sha256='b'*64,prerequisites_verified=True,unchanged_state_verified=True,application_health_verified=False)
 return r.PREFIX+json.dumps(d).encode()+b'\n'

class Wire(unittest.TestCase):
 def test_valid_intent_and_receipt(self):
  r.validate_intent(intent(),'machine',POLICY);self.assertEqual(r.validate_receipt(receipt(),intent(),'b'*64)['status'],'staged-paused')
 def test_unsafe_intents(self):
  for key,value in [('schema',True),('activation_allowed',0),('activation_allowed',True),('expected_owner','a'),('machine_id','other'),('product','swf'),('node_id','witness'),('artifact_sha256','bad'),('extra','ignored')]:
   with self.subTest(key=key,value=value):
    d=intent();d[key]=value
    with self.assertRaises(ValueError):r.validate_intent(d,'machine',POLICY)
 def test_generation_bool_refused(self):
  d=intent();d['maintenance']['generation']=True
  with self.assertRaises(ValueError):r.validate_intent(d,'machine',POLICY)
 def test_wrong_receipt_identity_and_health(self):
  for key,value in [('status','healthy'),('application_health_verified',True),('prerequisites_verified',1),('unchanged_state_verified',False),('runtime_sha256','c'*64),('version','3.3.152.33'),('extra','secret')]:
   d=json.loads(receipt()[len(r.PREFIX):]);d[key]=value
   with self.subTest(key=key),self.assertRaises(ValueError):r.validate_receipt(r.PREFIX+json.dumps(d).encode(),intent(),'b'*64)
 def test_duplicate_extra_lines_oversize(self):
  for raw in [receipt()+b'log\n',b'log\n'+receipt(),r.PREFIX+b'{"status":1,"status":2}',r.PREFIX+b'x'*8192]:
   with self.subTest(raw=raw[:40]),self.assertRaises(ValueError):r.validate_receipt(raw,intent(),'b'*64)
 def test_non_finite_json_refused(self):
  with self.assertRaises(ValueError):r.parse('{"value":NaN}')
 def test_release_identity_arch_commit(self):
  ref={'product':'siemcore','version':'3.3.152.34','sha256':'a'*64,'signature':'signed','public_key':r.KEY,'source_commit':'c'*40,'architecture':'amd64','artifact':'/root/candidate.tar.gz'}
  with patch.object(r.platform,'machine',return_value='x86_64'):
   r.validate_release(ref,intent())
   for key,value in [('sha256','d'*64),('public_key','0'*64),('architecture','arm64'),('source_commit','short')]:
    with self.subTest(key=key),self.assertRaises(ValueError):r.validate_release(dict(ref,**{key:value}),intent())

class Execution(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
  self.runner=r.Runner(None,self.root)
  self.state={'schema':1,'intent':intent(),'release':{},'phase':'prepared','runtime_sha256':'b'*64}
  p=patch.object(r,'trusted',lambda *a:None);p.start();self.addCleanup(p.stop)
  @contextlib.contextmanager
  def bundle(state):yield self.root,'b'*64
  p=patch.object(self.runner,'bundle',bundle);p.start();self.addCleanup(p.stop)
 def systemd(self,command,**kwargs):
  unit=next(x.split('=',1)[1] for x in command if x.startswith('--unit='))
  (self.root/(unit+'.stdout')).write_bytes(receipt())
  class Result:returncode=0
  return Result()
 def test_stage_then_verify_not_health(self):
  with patch.object(r.subprocess,'run',side_effect=self.systemd),patch.object(r,'quiesce'):
   self.runner.run(self.state,'stage');self.assertEqual(self.state['phase'],'stage-returned')
   self.runner.run(self.state,'verify-staged');self.assertEqual(self.state['phase'],'recovery-staged')
  self.assertFalse(self.state['receipt']['application_health_verified'])
 def test_stage_failure_no_verify_or_health(self):
  with patch.object(r.subprocess,'run',side_effect=RuntimeError('SECRET_TOKEN')),patch.object(r,'quiesce'):
   with self.assertRaises(RuntimeError):self.runner.run(self.state,'stage')
  self.assertEqual(self.state['phase'],'failed');self.assertNotIn('SECRET_TOKEN',self.runner.journal.read_text())
 def test_bad_receipt_fails_closed(self):
  def bad(command,**kwargs):
   result=self.systemd(command,**kwargs)
   unit=next(x.split('=',1)[1] for x in command if x.startswith('--unit='))
   (self.root/(unit+'.stdout')).write_bytes(b'healthy')
   return result
  with patch.object(r.subprocess,'run',side_effect=bad),patch.object(r,'quiesce'):
   with self.assertRaises(ValueError):self.runner.run(self.state,'stage')
  self.assertEqual(self.state['phase'],'failed')
 def test_interrupted_unit_revoked_before_drain(self):
  unit='updates-pod-recovery-'+'a'*32+'.service';self.state['pending_unit']=unit
  def quiesce(actual):
   d=r.parse(self.runner.journal.read_bytes());self.assertNotIn('pending_unit',d);self.assertEqual(d['draining_unit'],unit)
  with patch.object(r,'quiesce',side_effect=quiesce):self.runner.drain(self.state)
  self.assertNotIn('draining_unit',self.state)
 def test_drain_failure_retains_unit_and_prevents_run(self):
  self.state['pending_unit']='updates-pod-recovery-'+'a'*32+'.service'
  with patch.object(r,'quiesce',side_effect=RuntimeError('still running')),patch.object(r.subprocess,'run') as run:
   with self.assertRaises(RuntimeError):self.runner.run(self.state,'stage')
   run.assert_not_called()
  self.assertIn('draining_unit',self.state)
 def test_mismatched_retry_intent_refused(self):
  self.runner.save(self.state);other=intent();other['operation_id']='other'
  with self.assertRaises(ValueError):self.runner.prepare(other,{})
 def test_delayed_worker_refused(self):
  self.runner.save(self.state)
  with patch.object(r,'ROOT',self.root),patch.object(r.os,'geteuid',return_value=0),patch.object(r,'execution_locks',return_value=contextlib.nullcontext()),patch.object(r,'load_legacy',return_value=None),patch.object(r.subprocess,'run') as run:
   with self.assertRaises(ValueError):r.worker('updates-pod-recovery-'+'a'*32+'.service')
   run.assert_not_called()
 def test_normal_boundary_blocks_every_phase_during_recovery(self):
  with patch.object(n.os.path,'lexists',return_value=True):
   for phase in ('apply','health','rollback'):
    with self.subTest(phase=phase),self.assertRaisesRegex(RuntimeError,'dedicated pod recovery'):n.Boundary(None,self.root).dispatch({}, {},phase)

if __name__=='__main__':unittest.main()

class VerifiedBundle(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
  (self.root/'retained.tar.gz').write_bytes(b'signed archive fixture')
  self.manifest={'product':'siemcore','version':'3.3.152.34','architecture':'amd64','build':{'git_commit':'c'*40}}
  self.calls=0;outer=self
  class Old:
   def verified_bundle(self,entry,version,key,root):
    outer.calls+=1
    if key!=r.KEY or entry['signature']!='valid' or Path(entry['artifact']).read_bytes()!=b'signed archive fixture':raise ValueError('verifier refused')
    bundle=root/'bundle';(bundle/'updater').mkdir(parents=True);(bundle/'pod/bin').mkdir(parents=True)
    (bundle/'MANIFEST.json').write_text(json.dumps(outer.manifest))
    e=bundle/'updater/recover';e.write_text('#!/bin/sh\nexit 0');e.chmod(0o700)
    (bundle/'pod/bin/siemcore').write_bytes(b'runtime')
    return bundle
  self.runner=r.Runner(Old(),self.root)
  self.state={'release':{'version':'3.3.152.34','sha256':'a'*64,'signature':'valid','architecture':'amd64','source_commit':'c'*40}}
  p=patch.object(r,'trusted',lambda *a:None);p.start();self.addCleanup(p.stop)
 def test_manifest_and_runtime_verified_each_time(self):
  for _ in range(2):
   with self.runner.bundle(self.state) as (bundle,digest):self.assertEqual(digest,r.sha(b'runtime'))
  self.assertEqual(self.calls,2)
 def test_signed_manifest_mismatch(self):
  for key,value in [('product','mysoc'),('version','3.3.152.33'),('architecture','arm64')]:
   original=self.manifest[key];self.manifest[key]=value
   with self.subTest(key=key),self.assertRaises(ValueError):
    with self.runner.bundle(self.state):pass
   self.manifest[key]=original
 def test_commit_mismatch(self):
  self.manifest['build']['git_commit']='d'*40
  with self.assertRaises(ValueError):
   with self.runner.bundle(self.state):pass
 def test_bad_signature_refused_before_entrypoint(self):
  self.state['release']['signature']='bad'
  with self.assertRaises(ValueError):
   with self.runner.bundle(self.state):self.fail('entered invalid bundle')
 def test_retained_tamper_refused(self):
  (self.root/'retained.tar.gz').write_bytes(b'tampered')
  with self.assertRaises(ValueError):
   with self.runner.bundle(self.state):self.fail('entered invalid bundle')
