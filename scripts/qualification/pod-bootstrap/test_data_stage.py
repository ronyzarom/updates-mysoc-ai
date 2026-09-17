import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import client
import data_stage as d

FIXTURE=json.loads((Path(__file__).parent/'registry-v1.fixture.json').read_text())

class DataStageTests(unittest.TestCase):
    def setUp(self):
        r=copy.deepcopy(FIXTURE['registry'])
        self.config=dict(protocol='pod-bootstrap-data-v1',registry=r,node=r['nodes'][0],generation=7,
            input_sha256='a'*64,database_name='siemcore',database_connection_file='/run/bootstrap/owner',
            host_machine_id_file='/run/bootstrap/machine-id',release_public_key='/run/bootstrap/release-key',
            authorization_key='/run/bootstrap/auth-key',invitation_file='/run/bootstrap/invitation',
            observer=dict(endpoint='https://observer:443',certificate_sha256='b'*64,
                tls=dict(ca='/run/bootstrap/ca',certificate='/run/bootstrap/cert',key='/run/bootstrap/key')))
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.path=Path(self.temp.name)/'stage.json'
        reader=patch.object(client,'protected',side_effect=lambda path:Path(path).read_bytes())
        reader.start();self.addCleanup(reader.stop)
    def plan(self):
        return d.prepare(self.config,FIXTURE['registry'],FIXTURE['registry_sha256'],'1',7,'a'*64)
    def receipt(self,plan):
        return dict(plan['binding'],phase=plan['expected_phase'],installation_complete=False,processing_allowed=False)
    def test_fixed_argv_and_partial_receipt(self):
        plan=self.plan()
        def runner(argv):
            self.assertEqual(argv,list(d.ARGV))
            self.assertEqual(json.loads(self.path.read_text())['status'],'incomplete')
            return 0,json.dumps(self.receipt(plan))
        result=d.invoke_fixture(plan,self.path,runner)
        self.assertIs(result['installation_complete'],False)
        self.assertEqual(json.loads(self.path.read_text())['status'],'partial-stage-complete')
    def test_seed_requires_other_data_node_and_exact_phase(self):
        self.config['seed']=dict(source_node_id='2',source_address='10.0.0.2',
            publisher_connection_file='/run/bootstrap/publisher',apply_connection_file='/run/bootstrap/apply')
        plan=self.plan();self.assertEqual(plan['expected_phase'],'selected-synchronization-configured')
        receipt=self.receipt(plan);receipt['phase']='schema-prepared'
        with self.assertRaises(ValueError):d.validate_receipt(plan,json.dumps(receipt))
        self.config['seed']['source_node_id']='1'
        with self.assertRaises(ValueError):self.plan()
    def test_failure_or_lost_output_retains_potential_partial_commit(self):
        for code,raw in [(1,''),(0,'{'),(0,'{}'),(0,'x'*8193)]:
            with self.subTest(code=code,raw=raw[:10]),self.assertRaises(Exception):
                d.invoke_fixture(self.plan(),self.path,lambda argv:(code,raw))
            state=json.loads(self.path.read_text())
            self.assertEqual(state['status'],'incomplete');self.assertIs(state['potential_partial_commit'],True)
    def test_timeout_then_same_operation_retry_preserves_binding(self):
        def timeout(argv):raise TimeoutError()
        plan=self.plan()
        with self.assertRaises(TimeoutError):d.invoke_fixture(plan,self.path,timeout)
        binding=json.loads(self.path.read_text())['binding']
        d.invoke_fixture(plan,self.path,lambda argv:(0,json.dumps(self.receipt(plan))))
        self.assertEqual(json.loads(self.path.read_text())['binding'],binding)
    def test_changed_config_after_partial_commit_blocks_before_invocation(self):
        with self.assertRaises(ValueError):d.invoke_fixture(self.plan(),self.path,lambda argv:(1,''))
        self.config['database_name']='changed';called=[]
        with self.assertRaises(ValueError):d.invoke_fixture(self.plan(),self.path,lambda argv:called.append(argv))
        self.assertEqual(called,[])
    def test_receipt_identity_generation_and_completion_are_strict(self):
        plan=self.plan()
        for field,value in [('generation',8),('generation',True),('node_id','2'),('artifact_sha256','f'*64),
                ('registry_sha256','f'*64),('input_sha256','f'*64),('installed_version','0.0.0.2'),
                ('operation_id','other'),('installation_complete',True),('processing_allowed',True)]:
            receipt=self.receipt(plan);receipt[field]=value
            with self.subTest(field=field),self.assertRaises(Exception):d.validate_receipt(plan,json.dumps(receipt))
    def test_unknown_config_or_receipt_fields_rejected(self):
        plan=self.plan();receipt=self.receipt(plan);receipt['activate']=True
        with self.assertRaises(Exception):d.validate_receipt(plan,json.dumps(receipt))
        self.config['reset_database']=True
        with self.assertRaises(Exception):self.plan()
    def test_intent_failure_prevents_runner(self):
        calls=[]
        with patch.object(client,'durable',side_effect=OSError()),self.assertRaises(OSError):
            d.invoke_fixture(self.plan(),self.path,lambda argv:calls.append(argv))
        self.assertEqual(calls,[])
