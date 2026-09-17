import copy
import hashlib
import time
import unittest
import runtime_worker as r

RAW=b'import json\ndef start(*args):\n    return {}\n'

class ModuleTests(unittest.TestCase):
    def manifest(self,raw=RAW):
        return dict(product='siemcore',bootstrap_host_modules=[dict(name='pod-data-runtime-v1',path=r.MODULE_PATH,
            sha256=hashlib.sha256(raw).hexdigest(),size=len(raw),python='>=3.10,<3.13',dependencies=[])])
    def test_exact_verified_module_and_supported_interpreter(self):
        self.assertIsNotNone(r.verify_module(self.manifest(),RAW,(3,12)))
        for version in ((3,9),(3,13)):
            with self.assertRaises(ValueError):r.verify_module(self.manifest(),RAW,version)
    def test_tamper_path_dependencies_and_duplicate_entries_rejected(self):
        for field,value in [('path','../runtime.py'),('sha256','f'*64),('size',True),('dependencies',['helper']),('python','>=3.10')]:
            manifest=self.manifest();manifest['bootstrap_host_modules'][0][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):r.verify_module(manifest,RAW,(3,12))
        manifest=self.manifest();manifest['bootstrap_host_modules']*=2
        with self.assertRaises(ValueError):r.verify_module(manifest,RAW,(3,12))
    def test_unreviewed_imports_and_dynamic_loading_rejected(self):
        for raw in (b'import requests',b'from . import helper',b'__import__("helper")',b'exec("pass")'):
            with self.subTest(raw=raw),self.assertRaises(ValueError):r.verify_module(self.manifest(raw),raw,(3,12))
    def test_real_worker_returns_receipt_without_stdout(self):
        def action():
            print('fixture credential must not be returned')
            return {'processing_enabled':False}
        self.assertEqual(r.bounded_child(action,2),{'processing_enabled':False})
    def test_real_worker_timeout_and_oversized_output_rejected(self):
        with self.assertRaises(TimeoutError):r.bounded_child(lambda:time.sleep(10),1)
        with self.assertRaises(ValueError):r.bounded_child(lambda:{'data':'x'*9000},2)
    def test_real_worker_error_does_not_expose_exception_contents(self):
        def action():raise ValueError('private fixture data')
        with self.assertRaises(ValueError) as caught:r.bounded_child(action,2)
        self.assertNotIn('private fixture data',str(caught.exception))

    @unittest.skipUnless(__import__('sys').platform=='linux' and __import__('os').geteuid()==0 and (3,10)<=__import__('sys').version_info[:2]<(3,13),'Linux root supported Python required')
    def test_linux_worker_invocation_reauthorizes_and_persists_exact_partial_receipt(self):
        import json
        import tempfile
        from pathlib import Path
        from unittest.mock import patch
        import client
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);config=dict(pod_id='fixture',node_id='1',database='siemcore');tls={'fixture':'bytes'}
            binding=dict(operation_id='fixture-operation',generation=7,input_sha256='a'*64,artifact_sha256='b'*64)
            expected=dict(protocol='pod-data-stage-v1',identity=dict(protocol='pod-independent-data-v1',pod_id='fixture',node_id='1',database='siemcore'),
                binding=binding,configuration_sha256=hashlib.sha256(json.dumps({'config':config,'tls':tls},sort_keys=True,separators=(',',':')).encode()).hexdigest(),
                phase='data-services-ready',processing_enabled=False,installation_complete=False)
            raw=('def start(directory, config, tls, binding, authorize):\n    authorize()\n    authorize()\n    return '+repr(expected)+'\n').encode()
            def authorize():
                with (root/'authorizations').open('a') as out:out.write('called\n')
            verifier=lambda:(self.manifest(raw),raw,'b'*64)
            with patch.object(client,'protected',side_effect=lambda path:Path(path).read_bytes()):
                result=r.invoke(str(root/'runtime'),config,tls,binding,root/'journal',verifier,authorize,timeout=5)
                self.assertEqual(result,expected)
                self.assertEqual(len((root/'authorizations').read_text().splitlines()),3)
                self.assertEqual(json.loads((root/'journal').read_text())['status'],'data-services-ready')
                changed=dict(binding,generation=8)
                with self.assertRaises(ValueError):r.invoke(str(root/'runtime'),config,tls,changed,root/'journal',verifier,authorize,timeout=5)

    def test_second_reviewed_module_keeps_runtime_compatible(self):
        manifest=self.manifest()
        app=dict(manifest['bootstrap_host_modules'][0],name='pod-application-install-v1',path='updater/pod_application_install.py')
        manifest['bootstrap_host_modules'].append(app)
        self.assertIsNotNone(r.verify_module(manifest,RAW,(3,12)))
        self.assertIsNotNone(r.verify_module(manifest,RAW,(3,12),'pod-application-install-v1'))
        for field,value in [('path',r.MODULE_PATH),('name','unknown-module'),('sha256','invalid')]:
            changed=copy.deepcopy(manifest);changed['bootstrap_host_modules'][1][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):r.verify_module(changed,RAW,(3,12))
        with self.assertRaises(ValueError):r.verify_module(dict(product='siemcore',bootstrap_host_modules=[app]),RAW,(3,12),'pod-application-install-v1')
        with self.assertRaises(ValueError):r.verify_module(self.manifest(),RAW,(3,12),'pod-application-install-v1')
