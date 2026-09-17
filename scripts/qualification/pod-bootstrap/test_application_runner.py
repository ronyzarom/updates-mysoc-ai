import copy
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
import application_runner as a

class ApplicationRunnerTests(unittest.TestCase):
    def setUp(self):
        self.binding=dict(operation_id='original',generation=1,input_sha256='a'*64,artifact_sha256='b'*64)
        self.config=dict(pod_id='pod',node_id='1',database_name='siemcore',version='3.3.152.99',runtime_image_id='sha256:'+'c'*64,data_runtime_directory='/root/data',data_configuration_sha256='d'*64,data_compose_sha256='e'*64,data_evidence={'network_id':'f'*64})
        self.prior={k:self.config[k] for k in ('data_runtime_directory','data_configuration_sha256','data_compose_sha256','data_evidence')}
        self.prior.update(binding=self.binding,runtime_receipt=dict(binding=self.binding,phase='data-services-ready',identity=dict(pod_id='pod',node_id='1',database='siemcore')))
    def test_prior_evidence_cannot_be_replaced_at_application_dispatch(self):
        a.validate_prior(self.config,self.binding,self.prior)
        for key,value in [('data_compose_sha256','0'*64),('data_configuration_sha256','0'*64),('data_evidence',{}),('node_id','2')]:
            with self.subTest(key=key),self.assertRaises(ValueError):a.validate_prior(dict(self.config,**{key:value}),self.binding,self.prior)
        with self.assertRaises(ValueError):a.validate_prior(self.config,dict(self.binding,generation=2),self.prior)
    def test_supervisor_does_not_expose_stderr_and_bounds_output_and_timeout(self):
        options=dict(env={'PATH':os.environ.get('PATH','')},check=True,capture_output=True,text=True,timeout=2)
        result=a.supervised([sys.executable,'-c','import sys;print("ok");print("private",file=sys.stderr)'],**options)
        self.assertEqual(result.stdout,'ok\n');self.assertEqual(result.stderr,'')
        with self.assertRaises(ValueError) as error:a.supervised([sys.executable,'-c','import sys;print("secret",file=sys.stderr);sys.exit(1)'],**options)
        self.assertNotIn('secret',str(error.exception))
        with self.assertRaises(ValueError):a.supervised([sys.executable,'-c','print("x"*1100000)'],**options)
        with self.assertRaises(TimeoutError):a.supervised([sys.executable,'-c','import time;time.sleep(2)'],**dict(options,timeout=.05))
    @unittest.skipUnless(os.geteuid()==0,'root-owned extracted fixture required')
    def test_extracted_bundle_rejects_changed_extra_and_symlink_files(self):
        with tempfile.TemporaryDirectory(dir='/root') as temp:
            root=Path(temp);path=root/'helper.py';path.write_bytes(b'pass');path.chmod(0o600)
            a.verify_tree(root,{'helper.py':b'pass'})
            path.write_bytes(b'fail')
            with self.assertRaises(ValueError):a.verify_tree(root,{'helper.py':b'pass'})
            path.write_bytes(b'pass');extra=root/'extra';extra.write_bytes(b'unsigned')
            with self.assertRaises(ValueError):a.verify_tree(root,{'helper.py':b'pass'})
            extra.unlink();path.unlink();path.symlink_to('/etc/passwd')
            with self.assertRaises(ValueError):a.verify_tree(root,{'helper.py':b'pass'})

    def invoke(self,root,receipt,lose=False):
        raw=('def install(*args, **kwargs):\n    return '+repr(receipt)+'\n').encode()
        entry=dict(name=a.NAME,path=a.runtime_worker.MODULES[a.NAME],sha256=hashlib.sha256(raw).hexdigest(),size=len(raw),python='>=3.10,<3.13',dependencies=[])
        manifest=dict(product='siemcore',version=self.config['version'],runtime_image_id=self.config['runtime_image_id'],bootstrap_host_modules=[dict(entry,name='pod-data-runtime-v1',path=a.runtime_worker.MODULE_PATH),entry])
        with patch.object(a.sys,'platform','linux'),patch.object(a.os,'geteuid',return_value=0),patch.object(a.sys,'version_info',(3,12)),patch.object(a,'configuration_hash',return_value='f'*64),patch.object(a.client,'protected',side_effect=lambda path:Path(path).read_bytes()):
            return a.invoke('/root/bundle','/root/app',self.config,self.binding,self.prior,root/'journal',lambda:(manifest,raw,'b'*64),lambda:None,timeout=3,fixture_lose_completion=lose)
    def receipt(self):
        return dict(protocol=a.NAME,pod_id='pod',node_id='1',version=self.config['version'],runtime_image_id=self.config['runtime_image_id'],binding=self.binding,configuration_sha256='f'*64,phase='management-installed-paused',installation_complete=False,processing_allowed=False,activation_ready=False)
    def test_partial_receipt_and_retry_intent(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);expected=self.receipt()
            self.assertEqual(self.invoke(root,expected),expected)
            state=json.loads((root/'journal').read_text());self.assertEqual(state['status'],'awaiting-management-observation')
            self.config['version']='3.3.152.100'
            with self.assertRaises(ValueError):self.invoke(root,self.receipt())
    def test_lost_completion_retains_incomplete_and_same_operation_retries(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp)
            with self.assertRaises(ValueError):self.invoke(root,self.receipt(),lose=True)
            self.assertEqual(json.loads((root/'journal').read_text())['status'],'incomplete')
            self.assertEqual(self.invoke(root,self.receipt()),self.receipt())

    def test_activation_or_changed_receipt_rejected_and_incomplete_retained(self):
        for key,value in [('processing_allowed',True),('activation_ready',True),('installation_complete',True),('phase','complete')]:
            with self.subTest(key=key),tempfile.TemporaryDirectory() as temp:
                root=Path(temp)
                with self.assertRaises(ValueError):self.invoke(root,dict(self.receipt(),**{key:value}))
                self.assertEqual(json.loads((root/'journal').read_text())['status'],'incomplete')

if __name__=='__main__':unittest.main()
