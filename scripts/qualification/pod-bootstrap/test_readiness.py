import copy
import json
import unittest
from pathlib import Path
from unittest.mock import patch
import readiness as r
import test_data_orchestration
import test_data_stage
import data_runner

class ReadinessTests(unittest.TestCase):
    def setUp(self):
        helper=test_data_orchestration.OrchestrationTests();helper.setUp();self.addCleanup(helper.doCleanups)
        self.helper=helper;f=test_data_stage.FIXTURE
        self.plan=r.prepare(helper.seed,f['registry'],f['registry_sha256'],'2',7,helper.digest,helper.original)
        self.when='2026-09-17T18:00:00Z';self.now=r.nanos(self.when)
    def receipt(self):
        schema=json.loads((Path(__file__).parent/'readiness.receipt.schema.json').read_text())
        tables=sorted(schema['properties']['operational_content']['properties']['tables']['items']['properties']['name']['enum'])
        snapshot=dict(policy_version=2,tables=[dict(name=name,rows=1,sha256='a'*64) for name in tables],sha256='b'*64,observed_at=self.when)
        return dict(self.plan['binding'],phase='selected-mirror-verified',installation_complete=False,
            processing_allowed=False,activation_ready=False,observed_at=self.when,policy_version=2,
            operational_content=copy.deepcopy(snapshot),mirror=dict(identity=self.plan['mirror_identity'],
                source=copy.deepcopy(snapshot),target=copy.deepcopy(snapshot),applied_age_ns=10**9,observed_at=self.when))
    def check(self,value):return r.validate_receipt(self.plan,json.dumps(value),self.now)
    def test_bound_fresh_observation_stays_incomplete(self):
        value=self.check(self.receipt())
        self.assertIs(value['activation_ready'],False)
        self.assertEqual(self.plan['argv'][-1],'--verify-readiness')
    def test_stale_or_future_observation_rejected(self):
        for delta in (-1,5*10**9+1):
            with self.subTest(delta=delta),self.assertRaises(ValueError):r.validate_receipt(self.plan,json.dumps(self.receipt()),self.now+delta)
    def test_marker_effective_age_and_identity_rejected(self):
        value=self.receipt();value['mirror']['applied_age_ns']=300*10**9
        with self.assertRaises(ValueError):r.validate_receipt(self.plan,json.dumps(value),self.now+1)
        value=self.receipt();value['mirror']['identity']=dict(value['mirror']['identity'],Generation=8)
        with self.assertRaises(ValueError):self.check(value)
    def test_same_count_content_drift_and_duplicate_tables_rejected(self):
        value=self.receipt();value['mirror']['target']['tables'][0]['sha256']='f'*64
        with self.assertRaises(ValueError):self.check(value)
        value=self.receipt();value['operational_content']['tables']*=2
        with self.assertRaises(ValueError):self.check(value)
    def test_activation_flag_or_wrong_stage_receipt_rejected(self):
        for field,value in [('activation_ready',True),('processing_allowed',True),('installation_complete',True),('protocol','pod-bootstrap-data-v1')]:
            receipt=self.receipt();receipt[field]=value
            with self.subTest(field=field),self.assertRaises(Exception):self.check(receipt)
    def test_fresh_execution_required_even_when_receipt_retained(self):
        calls=[]
        def runner(argv):calls.append(argv);return 0,json.dumps(self.receipt())
        for _ in range(2):r.observe(self.plan,self.helper.root/'observation.json',runner,clock=lambda:self.now)
        self.assertEqual(calls,[list(r.COMMAND),list(r.COMMAND)])
    def test_runner_never_substitutes_mutating_command(self):
        runner=data_runner.Runner('/docker','sha256:'+'a'*64,'b'*64,{},lambda image:None,lambda:None,lambda:999,readiness=True)
        with self.assertRaises(ValueError):runner(list(data_runner.COMMAND))

    def test_readiness_output_limit_does_not_change_data_stage_limit(self):
        arguments=('/docker','sha256:'+'a'*64,'b'*64,{},lambda image:None,lambda:None,lambda:999)
        self.assertEqual(data_runner.Runner(*arguments).output_limit,8192)
        self.assertEqual(data_runner.Runner(*arguments,readiness=True).output_limit,32768)
