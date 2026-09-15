import copy
import importlib.util
from pathlib import Path
import unittest

path = Path(__file__).resolve().parents[1] / 'ops/reset_bezeq_witness_control_state.py'
spec = importlib.util.spec_from_file_location('reset', path)
reset = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reset)


class WitnessResetTests(unittest.TestCase):
    def test_unrelated_state_preserved(self):
        data = {'relay_token': 'fixture-token', 'product_versions': {'siemcore': 'old', 'other': 'keep'},
                'product_retries': {'siemcore': {'attempt': 1}, 'other': {'attempt': 2}},
                'last_update_attempt': {'product': 'other', 'success': False}, 'self_update': 'keep'}
        before = copy.deepcopy(data)
        result = reset.reset_updater_state(data)
        self.assertEqual(data, before)
        expected = copy.deepcopy(data)
        expected['product_versions']['siemcore'] = '0.0.0'
        del expected['product_retries']['siemcore']
        self.assertEqual(result, expected)

    def test_unattributed_attempt_preserved_for_multiple_products(self):
        d = {'product_versions': {'siemcore': 'old', 'other': 'keep'}, 'last_update_attempt': {'success': True}}
        self.assertEqual(reset.reset_updater_state(d)['last_update_attempt'], d['last_update_attempt'])

    def test_single_product_attempt_can_be_cleared(self):
        d = {'product_versions': {'siemcore': 'old'}, 'last_update_attempt': {'success': True}}
        self.assertIsNone(reset.reset_updater_state(d)['last_update_attempt'])

    def test_receipt_mismatch_rejected(self):
        expected = {'generation': 11, 'vm_ids': {'b': 'old-b'}, 'volumes': ['owned']}
        receipt = {'authorization': copy.deepcopy(expected), 'stage': 'authorized'}
        reset.validate_reset_receipt(receipt, expected)
        for field, value in [('generation', 12), ('vm_ids', {'b': 'new-b'}), ('volumes', ['foreign'])]:
            bad = copy.deepcopy(receipt)
            bad['authorization'][field] = value
            with self.assertRaises(ValueError):
                reset.validate_reset_receipt(bad, expected)

    def test_partial_failure_resumes_only_remaining_resources(self):
        present = {'owned-1', 'owned-2', 'unrelated'}
        removed = []
        def remove(name):
            if name == 'owned-2':
                raise RuntimeError('injected interruption')
            present.remove(name)
            removed.append(name)
        with self.assertRaises(RuntimeError):
            reset.remove_remaining(['owned-1', 'owned-2'], present.__contains__, remove)
        self.assertEqual(removed, ['owned-1'])
        reset.remove_remaining(['owned-1', 'owned-2'], present.__contains__, present.remove)
        self.assertEqual(present, {'unrelated'})

    def test_remote_payload_compiles(self):
        compile(reset.REMOTE, 'remote', 'exec')


if __name__ == '__main__':
    unittest.main()
