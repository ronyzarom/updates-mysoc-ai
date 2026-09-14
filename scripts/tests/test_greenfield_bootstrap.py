import base64
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('bootstrap', Path(__file__).parents[2] / 'kits/siemcore/greenfield-bootstrap.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class InputTests(unittest.TestCase):
    def test_pod_nodes_share_application_identity_but_not_enrollment(self):
        active = {'instance_id': 'siemcore-pod', 'updater_instance_id': 'pod-node-a'}
        standby = dict(active, updater_instance_id='pod-node-b')
        self.assertEqual(module.updater_identity(active), 'pod-node-a')
        self.assertEqual(module.updater_identity(standby), 'pod-node-b')
        self.assertEqual(active['instance_id'], standby['instance_id'])
        self.assertEqual(module.updater_identity({'instance_id':'legacy'}), 'legacy')
        for value in ('', '../host', 'node\nother', None):
            with self.subTest(value=value), self.assertRaises((ValueError, TypeError)):
                module.updater_identity(dict(active, updater_instance_id=value))

    def test_signed_receipt_and_identity_required(self):
        def fixture():
            return {'application': {'schema':1,'topology':'single','cluster_id':'lab','instance_id':'siemcore-lab','database_name':'siemcore'},
                    'release': {'channel':'pod-lab','version':'3.3.151.99','sha256':'a'*64,'public_key':'b'*64,'signature':base64.b64encode(b'x'*64).decode()}}
        module.validate(fixture())
        for section, field, bad in [('release','version','../x'),('release','signature',''),('release','sha256','bad'),('release','public_key','bad'),('application','instance_id','bad\nid'),('application','topology','ha')]:
            with self.subTest(field=field), self.assertRaises(ValueError):
                data=fixture();data[section][field]=bad;module.validate(data)

if __name__ == '__main__': unittest.main()
