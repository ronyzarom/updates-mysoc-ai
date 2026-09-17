import base64
import hashlib
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import client
import handoff
import test_data_stage
FIXTURE=test_data_stage.FIXTURE

class HandoffTests(unittest.TestCase):
    def setUp(self):
        helper=test_data_stage.DataStageTests();helper.setUp();self.addCleanup(helper.doCleanups)
        self.root=Path(helper.temp.name);self.config=helper.config
        (self.root/'bootstrap').mkdir();(self.root/'receipts').mkdir()
        original=b'{"fixture":true}\n'
        digest=hashlib.sha256(original).hexdigest();self.config['input_sha256']=digest
        (self.root/'original-input.json').write_bytes(original)
        (self.root/'release-trust.hex').write_text(FIXTURE['pinned_public_key_hex'])
        self.request=dict(sequence=2,registry=FIXTURE['registry'],registry_sha256=FIXTURE['registry_sha256'],
            generation=7,node_id='1',input_sha256=digest,bootstrap_root=str(self.root/'bootstrap'),image_id='sha256:'+'b'*64)
        self.calls=[]
    def write(self):
        (self.root/'request-2.json').write_text(json.dumps(self.request))
        (self.root/'bootstrap'/'data.json').write_text(json.dumps(self.config))
    def factory(self,request,config,plan):
        self.calls.append(request['sequence'])
        return lambda argv:(0,json.dumps(dict(plan['binding'],phase=plan['expected_phase'],
            installation_complete=False,processing_allowed=False)))
    def test_exact_request_returns_atomic_partial_receipt(self):
        self.write();result=handoff.consume(self.root,2,self.factory)
        self.assertEqual(result['exit_code'],0)
        self.assertIs(json.loads(base64.b64decode(result['stdout']))['installation_complete'],False)
        self.assertEqual(json.loads((self.root/'response-2.json').read_text()),result)
        with self.assertRaises(ValueError):handoff.consume(self.root,2,self.factory)
        self.assertEqual(self.calls,[2])
    def test_changed_generation_rejected_before_runner(self):
        self.config['generation']=8;self.write()
        self.assertEqual(handoff.consume(self.root,2,self.factory),dict(exit_code=1,stdout=''))
        self.assertEqual(self.calls,[])
    def test_original_input_tamper_rejected(self):
        self.write();(self.root/'original-input.json').write_text('changed')
        self.assertEqual(handoff.consume(self.root,2,self.factory)['exit_code'],1)
        self.assertEqual(self.calls,[])
    def test_verifier_error_does_not_expose_secrets(self):
        self.write()
        def reject(*args):raise ValueError('private credential must not escape')
        self.assertEqual(handoff.consume(self.root,2,reject),dict(exit_code=1,stdout=''))
