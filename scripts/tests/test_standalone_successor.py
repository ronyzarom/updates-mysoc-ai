import copy,json,os,sys,tempfile,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1'))
from source_loader import SourceLoader
from transaction import canonical,digest,validate_binding
from scripts.tests import test_standalone_transaction as fixtures
class SuccessorTests(unittest.TestCase):
 def setUp(self):
  f=fixtures.StandaloneAdmissionTests();f.setUp();self.old=f.binding
  self.next=copy.deepcopy(self.old);self.next.update(operation_id='8c997647-b5f6-4dba-ad2a-15454a7a276d',previous_operation={'operation_id':self.old['operation_id'],'operation_sha256':digest(self.old)})
  self.next['target']['version']='3.3.152.44'
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
  self.base=self.root/'var/lib/siemcore-node-standalone/operations'/self.old['operation_id'];self.base.mkdir(parents=True)
  self.write('standalone-intent.json',{'binding':self.old,'operation_sha256':digest(self.old)})
  self.proof=dict(self.next['previous_operation'])
  for kind in ('adapter','product'):
   r=dict(protocol='pod-node-standalone-v1',phase='restored',mutation='confirmed',operation_id=self.old['operation_id'],operation_sha256=digest(self.old),observed_at='fixture')
   if kind=='product':r['binding']=self.old
   self.write(kind+'-journal.json',r);self.proof[kind+'_receipt_sha256']=digest({k:v for k,v in r.items() if k!='observed_at'})
  self.loader=SourceLoader(self.root,os.geteuid(),self.next['operation_id'],self.proof,self.next)
 def write(self,name,value):(self.base/name).write_bytes(canonical(value))
 def test_exact_restored_successor_and_timestamp_refresh(self):
  validate_binding(self.next);self.loader.measure_previous_standalone(self.old['source'])
  path=self.base/'adapter-journal.json';r=json.loads(path.read_text());r['observed_at']='later';self.write(path.name,r)
  self.loader.measure_previous_standalone(self.old['source'])
 def test_unfinished_and_receipt_changes_refused(self):
  path=self.base/'product-journal.json';r=json.loads(path.read_text());r['phase']='restoring';self.write(path.name,r)
  with self.assertRaisesRegex(ValueError,'not_restored'):self.loader.measure_previous_standalone(self.old['source'])
  r['phase']='restored';r['extra']='changed';self.write(path.name,r)
  with self.assertRaisesRegex(ValueError,'receipt_changed'):self.loader.measure_previous_standalone(self.old['source'])
 def test_changed_original_configuration_and_reused_target_refused(self):
  self.next['configuration_sha256']='e'*64
  with self.assertRaisesRegex(ValueError,'source_identity'):self.loader.measure_previous_standalone(self.old['source'])
  self.next['configuration_sha256']=self.old['configuration_sha256'];self.next['target']['version']=self.old['target']['version']
  with self.assertRaisesRegex(ValueError,'newer_target'):self.loader.measure_previous_standalone(self.old['source'])
 def test_unreviewed_prior_directory_blocks(self):
  (self.base.parent/'unexpected').mkdir()
  with self.assertRaisesRegex(ValueError,'reconciliation'):self.loader.measure({})
