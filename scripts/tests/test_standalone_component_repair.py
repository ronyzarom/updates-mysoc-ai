import hashlib,importlib.util,json,os,sys,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
root=Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1';sys.path.insert(0,str(root))
spec=importlib.util.spec_from_file_location('repair',root/'component_repair.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
from source_loader import SourceLoader
from transaction import canonical,digest
class RepairTests(unittest.TestCase):
 def test_exact_revision_retry_and_started_operation_refusal(self):
  with tempfile.TemporaryDirectory() as tmp:
   base=Path(tmp);loader=SourceLoader(base,os.geteuid())
   with patch.multiple(m,ROOT=base/'state',POLICY=base/'policy.json',WRAPPER=base/'wrapper',COMPONENT=base/'components',KIT=base/'kit'):
    m.ROOT.mkdir();m.COMPONENT.mkdir();(m.KIT/'component').mkdir(parents=True)
    binding={'source':{'node_id':'1'},'operation_id':'fixture'};old=m.COMPONENT/'r1';old.mkdir()
    for directory,content in [(old,b'old'),(m.KIT/'component',b'new')]:
     (directory/'cli.py').write_bytes(content);(directory/'COMPONENT.json').write_bytes(canonical({'files':{'cli.py':hashlib.sha256(content).hexdigest()}}))
    policy={'binding':binding,'component_manifest_sha256':hashlib.sha256((old/'COMPONENT.json').read_bytes()).hexdigest()};m.POLICY.write_bytes(canonical(policy))
    oldsha=hashlib.sha256(m.POLICY.read_bytes()).hexdigest();newsha=hashlib.sha256((m.KIT/'component/COMPONENT.json').read_bytes()).hexdigest()
    updated=dict(policy,component_manifest_sha256=newsha)
    p=dict(identity={'vm_id':'fixture','node_id':'1'},operation_sha256=digest(binding),old_policy_sha256=oldsha,new_policy_sha256=hashlib.sha256((json.dumps(updated,sort_keys=True,indent=2)+'\n').encode()).hexdigest(),old_revision='r1',new_revision='r2',old_component_sha256=policy['component_manifest_sha256'],new_component_sha256=newsha)
    m.WRAPPER.write_text('#!/bin/sh\nexec /usr/bin/python3 -I '+str(old/'cli.py')+' "$@"\n')
    first=m.repair(loader,p);self.assertEqual(first,m.repair(loader,p));self.assertFalse(first['product_execution'])
    d=m.ROOT/'operations/fixture';d.mkdir(parents=True);(d/'adapter-journal.json').write_text('{}')
    with self.assertRaisesRegex(ValueError,'already_started'):m.repair(loader,p)
