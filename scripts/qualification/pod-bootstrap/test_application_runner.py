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
        self.config=dict(protocol=a.NAME,pod_id='pod',node_id='1',database_name='siemcore',version='3.3.152.99',runtime_image_id='sha256:'+'c'*64,data_runtime_directory='/root/data',data_configuration_sha256='d'*64,data_compose_sha256='e'*64,data_evidence={'network_id':'f'*64})
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

    def invoke(self,root,receipt,lose=False,allow_v2=False,original=None):
        raw=('def install(*args, **kwargs):\n    return '+repr(receipt)+'\n').encode()
        entry=dict(name=a.NAME,path=a.runtime_worker.MODULES[a.NAME],sha256=hashlib.sha256(raw).hexdigest(),size=len(raw),python='>=3.10,<3.13',dependencies=[])
        manifest=dict(product='siemcore',version=self.config['version'],runtime_image_id=self.config['runtime_image_id'],bootstrap_host_modules=[dict(entry,name='pod-data-runtime-v1',path=a.runtime_worker.MODULE_PATH),entry])
        with patch.object(a.sys,'platform','linux'),patch.object(a.os,'geteuid',return_value=0),patch.object(a.sys,'version_info',(3,12)),patch.object(a,'configuration_hash',return_value='f'*64),patch.object(a.client,'protected',side_effect=lambda path,**kwargs:Path(path).read_bytes()):
            return a.invoke('/root/bundle',str(root/'app'),self.config,self.binding,self.prior,root/'journal',lambda:(manifest,raw,'b'*64),lambda:None,timeout=3,fixture_lose_completion=lose,allow_v2=allow_v2,original_input=original)
    def receipt(self):
        return dict(protocol=self.config['protocol'],pod_id='pod',node_id='1',version=self.config['version'],runtime_image_id=self.config['runtime_image_id'],binding=self.binding,configuration_sha256='f'*64,phase='management-installed-paused',installation_complete=False,processing_allowed=False,activation_ready=False)
    def test_partial_receipt_and_retry_intent(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);expected=self.receipt()
            self.assertEqual(self.invoke(root,expected),expected)
            state=json.loads((root/'journal').read_text());self.assertEqual(state['status'],'awaiting-management-observation')
            self.config['version']='3.3.152.100'
            with self.assertRaises(ValueError):self.invoke(root,self.receipt())
    def original_v2(self):
        registry=dict(operation_id=self.binding['operation_id'],release=dict(sha256=self.binding['artifact_sha256']),nodes=[dict(node_id='1',updater_id='updater-original')])
        app=dict(schema=4,topology='pod',cluster_id='pod',pod_role='a',updater_instance_id='updater-original',
            bootstrap_installation={k:self.config[k] for k in ('protocol','archive_readiness_file','archive_readiness_sha256')},
            bootstrap_coordinator=dict(registry=registry,observer={},invitation_file='/root/invitation',authorization_key_file='/root/key'))
        raw=json.dumps(dict(application=app,registry=registry)).encode()
        self.binding['input_sha256']=hashlib.sha256(raw).hexdigest()
        return raw
    def test_v2_requires_opt_in_original_registration_and_identical_installed_copy(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);profile=b'{"protocol":"pod-archive-readiness-v1"}'
            (root/'profile').write_bytes(profile);(root/'app').mkdir()
            installed=root/'app/archive-readiness.json';installed.write_bytes(profile);installed.chmod(0o600)
            self.config.update(protocol='pod-application-install-v2',archive_readiness_file=str(root/'profile'),archive_readiness_sha256=hashlib.sha256(profile).hexdigest())
            original=self.original_v2()
            with self.assertRaises(ValueError):self.invoke(root,self.receipt(),original=original)
            self.assertFalse((root/'journal').exists())
            with self.assertRaises(ValueError):self.invoke(root,self.receipt(),allow_v2=True,original=original+b' ')
            self.assertEqual(self.invoke(root,self.receipt(),allow_v2=True,original=original),self.receipt())
            installed.write_bytes(b'changed')
            with self.assertRaises(ValueError):self.invoke(root,self.receipt(),allow_v2=True,original=original)
    def test_v2_cannot_be_added_after_registration(self):
        self.config.update(protocol='pod-application-install-v2',archive_readiness_file='/root/profile',archive_readiness_sha256='e'*64)
        original=self.original_v2()
        a.validate_v2_original(self.config,self.binding,original)
        for field,value in [('archive_readiness_sha256','f'*64),('archive_readiness_file','/root/replacement'),('node_id','2')]:
            with self.subTest(field=field),self.assertRaises(ValueError):a.validate_v2_original(dict(self.config,**{field:value}),self.binding,original)
    def test_configuration_hash_preserves_v1_and_binds_v2_profile_asset(self):
        config=dict(self.config,database_password_file='/pg',redis_password_file='/redis',jwt_secret_file='/jwt',database_tls_directory='/dbtls',syslog_tls_directory='/tls',archive_input_file='/archive')
        archive=b'{"archive":{"authentication":{"mode":"adc"}}}'
        files={'/pg':b'1'*64,'/redis':b'2'*64,'/jwt':b'3'*64,'/archive':archive}
        for directory,names in [('/dbtls',('ca.crt','client.crt','client.key')),('/tls',('tls.crt','tls.key'))]:
            for name in names:files[directory+'/'+name]=name.encode()
        assets={field+'/'+name:hashlib.sha256(files[directory+'/'+name]).hexdigest() for field,directory,names in [('database_tls_directory','/dbtls',('ca.crt','client.crt','client.key')),('syslog_tls_directory','/tls',('tls.crt','tls.key'))] for name in names}
        payload=dict(config=config,assets=assets,secret_hashes={k:hashlib.sha256(files[p]).hexdigest() for k,p in [('database','/pg'),('redis','/redis'),('jwt','/jwt')]},archive_sha256=hashlib.sha256(archive).hexdigest(),archive_credential_sha256=hashlib.sha256(b'').hexdigest())
        digest=lambda:hashlib.sha256(json.dumps(payload,sort_keys=True,separators=(',',':')).encode()).hexdigest()
        with patch.object(a.client,'protected',side_effect=lambda path,**kwargs:files[str(path)]):
            self.assertEqual(a.configuration_hash(config),digest())
            files['/profile']=b'{}'
            config.update(protocol='pod-application-install-v2',archive_readiness_file='/profile',archive_readiness_sha256=hashlib.sha256(b'{}').hexdigest())
            assets['archive-readiness.json']=config['archive_readiness_sha256']
            self.assertEqual(a.configuration_hash(config),digest())
            files['/profile']=b'{"changed":true}'
            with self.assertRaises(ValueError):a.configuration_hash(config)

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
