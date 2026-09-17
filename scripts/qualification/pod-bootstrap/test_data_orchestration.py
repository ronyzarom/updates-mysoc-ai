import copy
import hashlib
import json
from pathlib import Path
import unittest
import test_data_stage
import data_orchestration as o

class OrchestrationTests(unittest.TestCase):
    def setUp(self):
        helper=test_data_stage.DataStageTests();helper.setUp();self.addCleanup(helper.doCleanups)
        self.schema=helper.config;self.root=Path(helper.temp.name)
        self.schema['node']=test_data_stage.FIXTURE['registry']['nodes'][1]
        self.original=json.dumps(dict(application=dict(schema=4,topology='pod',cluster_id=test_data_stage.FIXTURE['registry']['pod_id'],
            selective_sync=dict(source_node_id='1',target_node_id='2',allowlist_version=2)))).encode()
        self.digest=hashlib.sha256(self.original).hexdigest();self.schema['input_sha256']=self.digest
        self.schema['initial_sync']=dict(source_node_id='1',target_node_id='2',allowlist_version=2)
        self.seed=copy.deepcopy(self.schema);self.seed['seed']=dict(source_node_id='1',source_address='10.0.0.2',
            publisher_connection_file='/run/bootstrap/publisher',apply_connection_file='/run/bootstrap/apply')
        self.calls=[];self.fail=None
    def factory(self,stage,config,plan):
        self.calls.append(stage)
        def runner(argv):
            if self.fail==stage:raise TimeoutError()
            return 0,json.dumps(dict(plan['binding'],phase=plan['expected_phase'],installation_complete=False,processing_allowed=False))
        return runner
    def run_stages(self,seed=True,node='2'):
        f=test_data_stage.FIXTURE
        return o.run_data_stages(f['registry'],f['registry_sha256'],node,7,self.digest,self.schema,
                                self.seed if seed else None,self.root,self.factory,original_input=self.original)
    def test_schema_then_seed_stops_before_readiness(self):
        result=self.run_stages()
        self.assertEqual(self.calls,['schema','seed'])
        self.assertEqual(result['phase'],'awaiting-product-readiness')
        self.assertIs(result['installation_complete'],False);self.assertIs(result['processing_allowed'],False)
        self.assertTrue((self.root/'schema-stage.json').exists());self.assertTrue((self.root/'seed-stage.json').exists())
    def test_interrupted_seed_retains_schema_and_exact_retry(self):
        self.fail='seed'
        with self.assertRaises(TimeoutError):self.run_stages()
        self.assertEqual(json.loads((self.root/'schema-stage.json').read_text())['status'],'partial-stage-complete')
        self.assertEqual(json.loads((self.root/'data-operation.json').read_text())['phase'],'incomplete')
        self.fail=None;self.calls=[];self.run_stages()
        self.assertEqual(self.calls,['schema','seed'])
    def test_changed_seed_or_stage_plan_rejected_before_runner(self):
        self.run_stages();self.calls=[]
        self.seed['seed']['source_address']='10.0.0.3'
        with self.assertRaises(ValueError):self.run_stages()
        self.assertEqual(self.calls,[])
        with self.assertRaises(ValueError):self.run_stages(seed=False)
    def test_observer_never_uses_data_command(self):
        with self.assertRaises(ValueError):self.run_stages(node='witness')
        self.assertEqual(self.calls,[])
    def test_schema_only_does_not_invent_seed_or_active_role(self):
        result=self.run_stages(seed=False)
        self.assertEqual(self.calls,['schema'])
        self.assertEqual(len(result['receipts']),1)
        self.assertIs(result['processing_allowed'],False)

    def test_initial_sync_required_and_exact_copy_of_original(self):
        for value in (None,dict(source_node_id='2',target_node_id='1',allowlist_version=2),
                      dict(source_node_id='1',target_node_id='2',allowlist_version=3),
                      dict(source_node_id='1',target_node_id='2',allowlist_version=2.0)):
            self.schema['initial_sync']=value
            with self.subTest(value=value),self.assertRaises(ValueError):self.run_stages(seed=False)
        self.assertEqual(self.calls,[])
