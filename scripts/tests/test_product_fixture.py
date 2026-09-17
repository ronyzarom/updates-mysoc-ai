import copy
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('product_fixture', Path(__file__).resolve().parents[1] / 'qualification/pod-maintenance/product_fixture.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class ProductFixtureTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.binary = Path(self.temp.name)/'siemcore-fixture-binary'
        self.binary.write_bytes(b'validator unit fixture only, never executed')
        self.plan = dict(isolated=True,adapter_kind='product',evidence_tier='native-with-synthetic-host',
            product_binaries={str(self.binary):hashlib.sha256(self.binary.read_bytes()).hexdigest()},
            scenarios=[dict(name='case',reset_command=['/fixture','reset'],ready_command=['/fixture','ready'],
                verify_command=['/fixture','verify'],stop_command=['/fixture','stop'],runs=[dict(config='/fixture/case.json',expected='success')])])
        self.case=dict(mode='drain-recovery',directory='/fixture/journal',adapter_command=[str(self.binary),'pod-drain-adapter','--config','/fixture/protected.json'])

    def test_accepts_pinned_adapter_and_isolated_proxy(self):
        m.validate_plan(self.plan)
        self.assertEqual(m.validate_case(self.plan,self.case),self.plan['product_binaries'])
        proxy=dict(isolated=True,adapter_command=self.case['adapter_command'])
        self.assertEqual(m.validate_case(self.plan,self.case,proxy),self.plan['product_binaries'])

    def test_observer_mode_uses_same_product_provenance_checks(self):
        self.case["mode"]="observer-maintenance"
        self.assertEqual(m.validate_case(self.plan,self.case),self.plan["product_binaries"])
        self.case["adapter_command"]=["/fixture/unpinned-observer"]
        with self.assertRaises(ValueError):m.validate_case(self.plan,self.case)

    def test_changed_executable_rejected(self):
        self.binary.write_bytes(b'changed')
        with self.assertRaises(ValueError):m.validate_case(self.plan,self.case)

    def test_verifier_required_and_reference_refused(self):
        del self.plan['scenarios'][0]['verify_command']
        with self.assertRaises(ValueError):m.validate_plan(self.plan)
        self.case['adapter_command'].append('/fixture/reference_adapter.py')
        with self.assertRaises(ValueError):m.validate_case(self.plan,self.case)

    def test_unpinned_command_and_wrong_mode_refused(self):
        bad=copy.deepcopy(self.case);bad['adapter_command']=['/fixture/unpinned']
        with self.assertRaises(ValueError):m.validate_case(self.plan,bad)
        bad=copy.deepcopy(self.case);bad['mode']='activate'
        with self.assertRaises(ValueError):m.validate_case(self.plan,bad)

if __name__=='__main__':unittest.main()
