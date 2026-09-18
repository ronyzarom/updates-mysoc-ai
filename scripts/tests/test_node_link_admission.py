import copy
import hashlib
import json
import unittest

from scripts.tests.test_node_link_contract import request, contract
from admission import validate_candidate


class ProtectedBindingTests(unittest.TestCase):
    def setUp(self):
        self.plan = b'{"fixture":"no execution"}'
        self.q = request()
        self.q['binding']['adoption_plan_sha256'] = hashlib.sha256(self.plan).hexdigest()
        self.protected = copy.deepcopy(self.q['binding'])

    def raw(self):
        self.q['operation_sha256'] = contract.binding_digest(self.q['binding'])
        return json.dumps(self.q).encode()

    def test_exact_protected_candidate(self):
        self.assertEqual(validate_candidate(self.raw(), self.protected, self.plan), self.q)

    def test_recomputed_hash_does_not_authorize_identity_change(self):
        self.q['binding']['source']['installation_id'] = 'foreign'
        with self.assertRaisesRegex(ValueError, 'protected_binding_mismatch'):
            validate_candidate(self.raw(), self.protected, self.plan)

    def test_changed_plan_rejected(self):
        with self.assertRaisesRegex(ValueError, 'adoption_plan_digest_mismatch'):
            validate_candidate(self.raw(), self.protected, self.plan + b' ')

    def test_equivalent_observer_origin_rejected(self):
        for endpoint in ('https://POD.example.com', 'https://pod.example.com:443/', 'https://pod.example.com.'):
            with self.subTest(endpoint=endpoint):
                self.q['binding']['observer']['endpoint'] = endpoint
                with self.assertRaisesRegex(ValueError, 'observer_must_use_fixed_endpoint'):
                    validate_candidate(self.raw(), copy.deepcopy(self.q['binding']), self.plan)


if __name__ == '__main__':
    unittest.main()
