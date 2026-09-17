import copy
import json
import unittest
from unittest.mock import patch
import management as m
import test_data_stage

class ManagementTests(unittest.TestCase):
    def setUp(self):
        helper=test_data_stage.DataStageTests();helper.setUp();self.addCleanup(helper.doCleanups)
        self.root=helper.path.parent;self.data=self.root/'data.json';self.data.write_text(json.dumps(helper.config))
        self.env={'app':m.environment_digest(['ROLE=app','SECRET=fixture']), 'archiver':m.environment_digest(['ROLE=archiver','SECRET=fixture'])}
        self.image='sha256:'+'a'*64;self.config=self.root/'management.json'
        self.config.write_text(json.dumps(dict(protocol='pod-bootstrap-management-v1',data_binding_file=str(self.data),runtime_image_id=self.image,environment_sha256=self.env)))
        f=test_data_stage.FIXTURE
        self.plan=m.prepare('/verified/siemcore',str(self.config),f['registry'],f['registry_sha256'],'1',7,'a'*64,self.image,self.env)
        self.when='2026-09-17T19:00:00Z';self.now=m.readiness.nanos(self.when)
    def receipt(self):
        status=dict(pod_id=self.plan['pod_id'],node_id='1',version=self.plan['binding']['installed_version'],management_ready=True,
            processing_enabled=False,quiescent=True,blocked=False,generation=0,state='Mirror-STBY')
        return dict(self.plan['binding'],phase='management-paused-verified',configuration_sha256=self.plan['configuration_sha256'],
            observed_at=self.when,installation_complete=False,processing_allowed=False,activation_ready=False,
            runtimes=[dict(role=role,container_id=char*64,image_id=self.image,environment_sha256=self.env[role],status=copy.deepcopy(status))
                      for role,char in [('app','b'),('archiver','c')]])
    def check(self,value):return m.validate_receipt(self.plan,json.dumps(value),self.now)
    def test_exact_both_paused_runtimes_stay_partial(self):
        self.assertIs(self.check(self.receipt())['activation_ready'],False)
    def test_generation_processing_and_environment_drift_rejected(self):
        for field,value in [('generation',1),('generation',False),('processing_enabled',True),('management_ready',1),('state','Active')]:
            result=self.receipt();result['runtimes'][0]['status'][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):self.check(result)
        result=self.receipt();result['runtimes'][1]['environment_sha256']='f'*64
        with self.assertRaises(ValueError):self.check(result)
    def test_missing_runtime_stale_or_wrong_config_rejected(self):
        result=self.receipt();result['runtimes'].pop()
        with self.assertRaises(ValueError):self.check(result)
        with self.assertRaises(ValueError):m.validate_receipt(self.plan,json.dumps(self.receipt()),self.now+5*10**9+1)
        result=self.receipt();result['configuration_sha256']='f'*64
        with self.assertRaises(ValueError):self.check(result)
    def test_environment_hash_rejects_ambiguous_or_multiline_values(self):
        self.assertEqual(m.environment_digest(['B=2','A=1']),m.environment_digest(['A=1','B=2']))
        for values in (['A=1','A=2'],['A=1\nB=2'],['=bad'],['malformed']):
            with self.assertRaises(ValueError):m.environment_digest(values)
    def test_runner_rechecks_data_binding_after_process(self):
        expectations=dict(binary_sha256='d'*64,runtime_image_id=self.image,environment_sha256=self.env)
        def change(*args):self.data.write_text('{}');return 0,b'{}'
        with patch.object(m.observer_runner,'binary_digest',return_value='d'*64),patch.object(m.data_runner,'bounded',side_effect=change),self.assertRaises(ValueError):
            m.Runner(self.plan,lambda:expectations)(self.plan['argv'])
    def test_observation_reexecutes_and_never_promotes(self):
        calls=[]
        def runner(argv):calls.append(argv);return 0,json.dumps(self.receipt())
        for _ in range(2):m.observe(self.plan,self.root/'observation.json',runner,clock=lambda:self.now)
        self.assertEqual(len(calls),2)
        self.assertIs(json.loads((self.root/'observation.json').read_text())['activation_ready'],False)
