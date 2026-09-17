import hashlib
import json
from pathlib import Path
import unittest
import selective_sync as s

F=json.loads((Path(__file__).parent/'registry-v1.fixture.json').read_text())
class SelectiveSyncTests(unittest.TestCase):
    def setUp(self):
        self.app=dict(schema=4,topology='pod',cluster_id=F['registry']['pod_id'],
            selective_sync=dict(source_node_id='1',target_node_id='2',allowlist_version=2))
    def check(self,node='2',seed=None):
        raw=json.dumps({'application':self.app}).encode()
        return s.validate(raw,F['registry'],hashlib.sha256(raw).hexdigest(),node,seed)
    def test_explicit_direction_has_no_active_role(self):
        result=self.check(seed={'seed':{'source_node_id':'1'}})
        self.assertEqual(set(result),{'source_node_id','target_node_id','allowlist_version'})
    def test_missing_or_unknown_fields_and_versions_rejected(self):
        for field,value in [('allowlist_version',3),('allowlist_version',True),('target_node_id','witness'),('source_node_id','2')]:
            original=self.app['selective_sync'][field];self.app['selective_sync'][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):self.check()
            self.app['selective_sync'][field]=original
        self.app.pop('selective_sync')
        with self.assertRaises(ValueError):self.check()
    def test_seed_cannot_run_on_source_or_from_wrong_peer(self):
        with self.assertRaises(ValueError):self.check(node='1',seed={'seed':{'source_node_id':'1'}})
        with self.assertRaises(ValueError):self.check(seed={'seed':{'source_node_id':'2'}})
    def test_original_bytes_are_hashed_before_interpretation(self):
        raw=json.dumps({'application':self.app}).encode()
        with self.assertRaises(ValueError):s.validate(raw+b' ',F['registry'],hashlib.sha256(raw).hexdigest(),'2',None)
