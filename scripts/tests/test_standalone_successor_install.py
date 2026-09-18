import hashlib,importlib.util,json,os,sys,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
base=Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1';sys.path.insert(0,str(base))
spec=importlib.util.spec_from_file_location('successor_install',base/'successor_install.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
from source_loader import SourceLoader
from transaction import canonical,digest
from scripts.tests import test_standalone_transaction as fixtures
class SuccessorInstallerTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);root=Path(self.tmp.name);self.loader=SourceLoader(root,os.geteuid())
  self.addCleanup(patch.stopall)
  for name,relative in {'ROOT':'state','POLICY':'policy.json','INPUTS':'inputs','COMPONENT':'components','WRAPPER':'wrapper','KIT':'kit'}.items():patch.object(m,name,root/relative).start()
  m.ROOT.mkdir();m.COMPONENT.mkdir();(m.KIT/'component').mkdir(parents=True)
  f=fixtures.StandaloneAdmissionTests();f.setUp();self.b=f.binding;previous={'operation_id':self.b['operation_id'],'operation_sha256':digest(self.b)}
  self.b=dict(self.b,operation_id='8c997647-b5f6-4dba-ad2a-15454a7a276d',previous_operation=previous)
  self.b['target']=dict(self.b['target'],version='3.3.152.44')
  app=dict(f.evidence['application'],management={'hostname':'fixture.invalid'})
  for path,value in [('/etc/siemcore/greenfield.json',app),('/opt/siemcore-node-standalone-1/operation.json',{'operation_sha256':previous['operation_sha256']})]:
   p=self.loader.path(path);p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(canonical(value))
  self.loader.measure=lambda expected:(f.evidence,f.bootstrap,{})
  patch.object(m,'configuration_digest',return_value=self.b['configuration_sha256']).start()
  data={r:dict(id=r,image=r,mounts=[]) for r in ('postgres','redis')}
  health=dict(version=self.b['source_version'],installation_id=self.b['source']['installation_id'],updater_id=self.b['source']['updater_instance_id'],node_id='1',installation_state='installed-unlinked',processing_enabled=False,authority_enabled=False,data_ready=True)
  self.measure=lambda *_:(data,health)
  (m.KIT/'component/cli.py').write_bytes(b'fixture');raw=canonical({'files':{'cli.py':hashlib.sha256(b'fixture').hexdigest()}});(m.KIT/'component/COMPONENT.json').write_bytes(raw)
  policy=dict(binding=self.b,previous_standalone={},historical_inventory={},configuration_metadata={},data_identity=data,component_manifest_sha256=hashlib.sha256(raw).hexdigest())
  (m.KIT/'POLICY.json').write_bytes(canonical(policy));m.POLICY.write_bytes(b'{"old":true}')
  self.p=dict(previous_operation=previous,operation_sha256=digest(self.b),kit_version='r32',old_revision='r31',old_policy_sha256=hashlib.sha256(m.POLICY.read_bytes()).hexdigest())
  m.WRAPPER.write_text('#!/bin/sh\nexec /usr/bin/python3 -I -B '+str(m.COMPONENT/'r31/cli.py')+' "$@"\n')
 def test_interrupted_policy_wrapper_boundary_retry_preserves_operation(self):
  original=m.put
  def interrupted(path,*args,**kwargs):
   if path==m.WRAPPER:raise OSError('fixture interruption')
   original(path,*args,**kwargs)
  with patch.object(m,'put',side_effect=interrupted):
   with self.assertRaises(OSError):m.install(self.loader,self.p,self.measure)
  self.assertFalse((m.ROOT/'successor-install.json').exists())
  first=m.install(self.loader,self.p,self.measure);self.assertEqual(first,m.install(self.loader,self.p,self.measure));self.assertFalse(first['product_execution'])
  self.assertEqual(json.loads(m.POLICY.read_text())['binding'],self.b)
 def test_running_or_unhealthy_predecessor_refuses_before_policy_write(self):
  before=m.POLICY.read_bytes()
  def stopped_guard(*_):raise ValueError('previous_processing_not_stopped')
  with self.assertRaisesRegex(ValueError,'not_stopped'):m.install(self.loader,self.p,stopped_guard)
  self.assertEqual(m.POLICY.read_bytes(),before);self.assertFalse((m.ROOT/'successor-install.json').exists())
