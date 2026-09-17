import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import client
import observer_stage as o

F=json.loads((Path(__file__).parent/'registry-v1.fixture.json').read_text())

class ObserverTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.root=Path(self.temp.name)
        for name in ('drain','update'):(self.root/name).write_text('{}')
        self.config=dict(protocol='pod-bootstrap-observer-v1',registry=F['registry'],node=F['registry']['nodes'][2],
            generation=7,input_sha256='a'*64,host_machine_id_file='/etc/machine-id',invitation_file='/protected/invitation')
        reader=patch.object(client,'protected',side_effect=lambda path:Path(path).read_bytes());reader.start();self.addCleanup(reader.stop)
    def plan(self):return o.prepare(self.config,F['registry'],F['registry_sha256'],7,'a'*64,
         '/verified/siemcore',str(self.root/'drain'),str(self.root/'update'),'/protected/receipt.json')
    def receipt(self,plan):return dict(plan['binding'],phase='observer-management-verified',installation_complete=False,processing_allowed=False)
    def test_health_required_and_receipt_remains_partial(self):
        plan=self.plan();self.assertIn('--health',plan['argv']);self.assertIn('--receipt-config',plan['argv'])
        result=o.invoke_fixture(plan,self.root/'journal',lambda argv:(0,json.dumps(self.receipt(plan))))
        self.assertIs(result['installation_complete'],False);self.assertIs(result['processing_allowed'],False)
    def test_non_witness_and_stale_generation_rejected(self):
        self.config['node']=F['registry']['nodes'][0]
        with self.assertRaises(Exception):self.plan()
        self.config['node']=F['registry']['nodes'][2];self.config['generation']=8
        with self.assertRaises(ValueError):self.plan()
    def test_data_receipt_activation_and_wrong_binding_rejected(self):
        plan=self.plan()
        for field,value in [('phase','schema-prepared'),('generation',8),('artifact_sha256','b'*64),
                            ('installation_complete',True),('processing_allowed',True)]:
            receipt=self.receipt(plan);receipt[field]=value
            with self.subTest(field=field),self.assertRaises(Exception):o.validate_receipt(plan,json.dumps(receipt))
    def test_timeout_retains_incomplete_and_exact_retry(self):
        plan=self.plan()
        def lost(argv):raise TimeoutError()
        with self.assertRaises(TimeoutError):o.invoke_fixture(plan,self.root/'journal',lost)
        self.assertEqual(json.loads((self.root/'journal').read_text())['status'],'incomplete')
        o.invoke_fixture(plan,self.root/'journal',lambda argv:(0,json.dumps(self.receipt(plan))))
        (self.root/'drain').write_text('{"changed":true}')
        with self.assertRaises(ValueError):o.invoke_fixture(self.plan(),self.root/'journal',lost)
