import base64,copy,hashlib,importlib.util,json,unittest,tempfile,os
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
spec=importlib.util.spec_from_file_location('repair',Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1/updater_repair.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class RepairTests(unittest.TestCase):
 def test_exact_identity_signature_terminal_recovery_and_replay(self):
  blob=b'fixture';config=b'unchanged';key=Ed25519PrivateKey.generate();sha=hashlib.sha256(blob).hexdigest()
  operation=dict(operation_id='operation',operation_sha256='digest')
  p=dict(identity=dict(vm_id='vm',machine_id='machine',installation_id='node'),previous_operation=operation,config_sha256=hashlib.sha256(config).hexdigest(),previous_binary_sha256='old',release=dict(version='1.16.1.33',sha256=sha,size=len(blob),signature=base64.b64encode(key.sign(('mysoc-release-v1\nupdater-linux-amd64\n1.16.1.33\n'+sha).encode())).decode()),public_key=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex())
  app=dict(machine_id='machine',installation_id='node');state=dict(node_standalone_operation=dict(operation,phase='restored'))
  args=[p,app,'machine','vm',state,config,'old',blob]
  m.validate(*args);args[6]=sha;m.validate(*args)
  for index,value in [(1,dict(app,installation_id='other')),(2,'other'),(3,'other'),(4,dict(node_standalone_operation=dict(operation,phase='uncertain'))),(5,b'changed'),(6,'unknown'),(7,b'tampered')]:
   bad=copy.deepcopy(args);bad[index]=value
   with self.subTest(index=index),self.assertRaises(Exception):m.validate(*bad)
  bad=copy.deepcopy(args);bad[0]['release']['signature']=base64.b64encode(b'x'*64).decode()
  with self.assertRaises(Exception):m.validate(*bad)

 def test_receipt_exact_retry_conflict_and_symlink_refusal(self):
  with tempfile.TemporaryDirectory() as directory:
   # macOS /tmp is itself a symlink; use the canonical fixture location.
   root=Path(directory).resolve();receipt=root/'receipt.json';expected={'version':'1.16.1.33'}
   m.save_receipt(receipt,expected);m.save_receipt(receipt,expected)
   before=receipt.read_bytes()
   with self.assertRaises(ValueError):m.save_receipt(receipt,{'version':'other'})
   self.assertEqual(receipt.read_bytes(),before)
   link=root/'link';link.symlink_to(root,target_is_directory=True)
   with self.assertRaises(ValueError):m.protected_ancestors(link/'release'/'file')
