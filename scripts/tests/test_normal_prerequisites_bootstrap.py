import base64,copy,hashlib,importlib.util,json,os,stat,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('normal_bootstrap',Path(__file__).resolve().parents[2]/'kits/siemcore/greenfield-bootstrap.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class NormalPrerequisiteTests(unittest.TestCase):
 def test_receipt_binds_final_application_and_manifest_without_changing_legacy(self):
  app=self.fixture()['application'];app['machine_id']='a'*32;app['label']='שלום'
  record=m.execution_receipt('e'*64,app)
  canonical=json.dumps(app,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode('utf-8')
  self.assertEqual(record['application_canonical_sha256'],hashlib.sha256(canonical).hexdigest())
  self.assertEqual(record['prerequisite_manifest_sha256'],'a'*64)
  self.assertEqual(record['input_sha256'],'e'*64)
  self.assertEqual(record,m.execution_receipt('e'*64,dict(reversed(list(app.items())))))
  changed=copy.deepcopy(app);changed['machine_id']='b'*32
  self.assertNotEqual(record,m.execution_receipt('e'*64,changed))
  changed=copy.deepcopy(app);changed['normal_prerequisites']['sha256']='b'*64
  self.assertNotEqual(record,m.execution_receipt('e'*64,changed))
  del app['normal_prerequisites']
  self.assertEqual(m.execution_receipt('e'*64,app),{'input_sha256':'e'*64})
 def fixture(self):
  return {'application':{'schema':3,'topology':'single','cluster_id':'normal','instance_id':'app','updater_instance_id':'fresh','database_name':'siemcore','normal_prerequisites':{'schema':1,'path':'/root/provisioning/normal.json','sha256':'a'*64}},'release':{'version':'3.3.152.99','sha256':'b'*64,'signature':base64.b64encode(b'x'*64).decode(),'public_key':'c'*64,'channel':'normal-a-20260919','required_capabilities':['normal-prerequisites-v1']}}
 def test_explicit_new_flow_preserved_and_old_shapes_unchanged(self):
  q=self.fixture();original=copy.deepcopy(q);m.validate(q);self.assertEqual(q,original)
  for schema in (1,3):
   legacy=copy.deepcopy(q);del legacy['application']['normal_prerequisites'];del legacy['release']['required_capabilities'];legacy['application']['schema']=schema
   if schema==3:legacy['application']['archive']={'product_owned':'opaque'}
   m.validate(legacy)
 def test_wrong_schema_topology_reference_or_capability_rejected(self):
  cases=[]
  for name,bad in [('schema',True),('schema',1),('topology','pod'),('topology','node-unlinked')]:
   q=self.fixture();q['application'][name]=bad;cases.append(q)
  for name,bad in [('schema',True),('schema',2),('path','relative'),('path','/root/../etc/a'),('path','/root//a'),('path','//root/a'),('path','/root/a/'),('path','/root/./a'),('sha256','A'*64)]:
   q=self.fixture();q['application']['normal_prerequisites'][name]=bad;cases.append(q)
  for caps in (None,[],['wrong'],['normal-prerequisites-v1','unknown'],['normal-prerequisites-v1']*2):
   q=self.fixture();q['release']['required_capabilities']=caps;cases.append(q)
  q=self.fixture();q['application']['normal_prerequisites']['extra']=True;cases.append(q)
  q=self.fixture();del q['application']['normal_prerequisites'];cases.append(q)
  for i,q in enumerate(cases):
   with self.subTest(case=i),self.assertRaises(ValueError):m.validate(q)
 def test_old_or_mismatched_kit_refused(self):
  with tempfile.TemporaryDirectory() as folder:
   root=Path(folder);q=self.fixture()
   with self.assertRaisesRegex(ValueError,'capability-aware'):m.require_delivery(q,root)
   (root/'greenfield-hook.py').write_bytes(b'fixture hook');(root/'PROVISIONING_COMMIT').write_text('a'*40+'\n')
   record=dict(schema=1,protocol='normal-prerequisites-v1',provisioning_commit='a'*40,hook_sha256=hashlib.sha256(b'fixture hook').hexdigest())
   (root/'NORMAL-PREREQUISITES.json').write_text(json.dumps(record));m.require_delivery(q,root)
   (root/'greenfield-hook.py').write_bytes(b'old/replaced hook')
   with self.assertRaisesRegex(ValueError,'binding'):m.require_delivery(q,root)
 def test_private_manifest_hash_type_and_duplicate_checks(self):
  with tempfile.TemporaryDirectory() as folder:
   path=Path(folder).resolve()/'manifest.json';path.write_bytes(b'{"images":{}}');path.chmod(0o600)
   q=self.fixture();value=q['application']['normal_prerequisites'];value['path']=str(path);value['sha256']=hashlib.sha256(path.read_bytes()).hexdigest()
   # Preserve real filesystem modes/type/inode; only root ownership is mapped
   # for unprivileged developer tests. Native root uses the real UID checks.
   orig=Path.lstat;fstat=os.fstat
   def root_stat(s):
    values=list(s);values[4]=0;return os.stat_result(values)
   with patch.object(Path,'lstat',lambda p:root_stat(orig(p))),patch.object(m.os,'fstat',lambda fd:root_stat(fstat(fd))):
    m.validate_local_normal(q['application'])
    path.chmod(0o644)
    with self.assertRaises(ValueError):m.validate_local_normal(q['application'])
    path.chmod(0o600);path.write_bytes(b'{"images":{},"images":{}}');value['sha256']=hashlib.sha256(path.read_bytes()).hexdigest()
    with self.assertRaisesRegex(ValueError,'duplicate'):m.validate_local_normal(q['application'])
    path.write_bytes(b'[]');value['sha256']=hashlib.sha256(path.read_bytes()).hexdigest()
    with self.assertRaisesRegex(ValueError,'object'):m.validate_local_normal(q['application'])
    path.write_bytes(b'{}');value['sha256']='a'*64
    with self.assertRaisesRegex(ValueError,'checksum'):m.validate_local_normal(q['application'])
    path.unlink();path.symlink_to('/etc/hosts')
    with self.assertRaises(ValueError):m.validate_local_normal(q['application'])
 def test_validation_failure_cannot_start_services(self):
  with patch.object(m.os,'geteuid',return_value=0),patch.object(m.sys,'argv',['bootstrap','--validate-install','/root/input']),patch.object(m,'read_input',return_value=self.fixture()),patch.object(m,'require_normal_delivery',side_effect=ValueError('missing capability')),patch.object(m.subprocess,'run') as run:
   with self.assertRaises(ValueError):m.main()
   run.assert_not_called()
 def test_retry_checks_stored_application_and_machine_before_service_start(self):
  q=self.fixture();original=copy.deepcopy(q);q['application']['machine_id']='a'*32
  fingerprint=hashlib.sha256(b'input envelope').hexdigest()
  receipt=m.execution_receipt(fingerprint,q['application'])
  for change in ('none','machine','configuration','manifest','receipt'):
   app=copy.deepcopy(q['application']);saved=copy.deepcopy(receipt)
   if change=='configuration':app['database_name']='other'
   if change=='manifest':app['normal_prerequisites']['sha256']='b'*64
   if change=='receipt':saved['application_canonical_sha256']='0'*64
   contents={'/etc/machine-id':('b' if change=='machine' else 'a')*32,
             '/etc/siemcore/updater-bootstrap.json':json.dumps(saved),
             '/etc/siemcore/greenfield.json':json.dumps(app)}
   with self.subTest(change=change),patch.object(m.os,'geteuid',return_value=0),patch.object(m.sys,'argv',['bootstrap','/root/input','/root/kit']),patch.object(m,'read_input',return_value=copy.deepcopy(original)),patch.object(m,'require_delivery'),patch.object(Path,'read_bytes',return_value=b'input envelope'),patch.object(Path,'read_text',lambda p:contents[str(p)]),patch.object(Path,'exists',return_value=True),patch.object(Path,'is_symlink',return_value=False),patch.object(m.subprocess,'run') as run:
    if change=='none':
     m.main();run.assert_called_once_with(['systemctl','start',m.NAME],check=True)
    else:
     with self.assertRaisesRegex(ValueError,'binding'):m.main()
     run.assert_not_called()

class NormalPackagingTests(unittest.TestCase):
 def test_legacy_hook_has_no_capability_marker(self):
  from scripts.packaging.normal_prerequisite_capability import marker_for_hook
  with tempfile.TemporaryDirectory() as directory:
   hook=Path(directory)/'hook.py';hook.write_text('def old_bootstrap(): pass\n')
   self.assertIsNone(marker_for_hook(hook,'a'*40))
 def test_new_marker_binds_exact_hook_and_commit(self):
  from scripts.packaging.normal_prerequisite_capability import marker_for_hook
  with tempfile.TemporaryDirectory() as directory:
   hook=Path(directory)/'hook.py';hook.write_text("NORMAL_PREREQUISITES_PROTOCOL = 'normal-prerequisites-v1'\ndef require_normal_capability(bundle, application): pass\n")
   marker=marker_for_hook(hook,'a'*40)
   self.assertEqual(marker['hook_sha256'],hashlib.sha256(hook.read_bytes()).hexdigest())
   self.assertEqual(marker['provisioning_commit'],'a'*40)
   for raw in ["NORMAL_PREREQUISITES_PROTOCOL = 'normal-prerequisites-v1'\n", "NORMAL_PREREQUISITES_PROTOCOL = 'unsupported'\ndef require_normal_capability(): pass\n",hook.read_text()+"NORMAL_PREREQUISITES_PROTOCOL = 'normal-prerequisites-v1'\n"]:
    hook.write_text(raw)
    with self.assertRaises(ValueError):marker_for_hook(hook,'a'*40)
