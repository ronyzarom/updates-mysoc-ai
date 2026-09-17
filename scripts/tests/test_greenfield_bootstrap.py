import base64
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('bootstrap', Path(__file__).parents[2] / 'kits/siemcore/greenfield-bootstrap.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class InputTests(unittest.TestCase):
    def test_executor_appends_health_phase_exactly_once(self):
        self.assertIn('health_command: ["sudo", "-n", "/usr/local/sbin/siemcore-apply-update"]',
                      module.FILESYSTEM_BLOCK)
        self.assertNotIn('siemcore-apply-update", "health"', module.FILESYSTEM_BLOCK)

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
        for section, field, bad in [('release','channel','pod-qualified-20260915'),('release','version','../x'),('release','signature',''),('release','sha256','bad'),('release','public_key','bad'),('application','instance_id','bad\nid'),('application','topology','ha')]:
            with self.subTest(field=field), self.assertRaises(ValueError):
                data=fixture();data[section][field]=bad;module.validate(data)

    def test_pod_roles_use_distinct_updater_identity(self):
        release = {'channel':'pod-lab','version':'3.3.152.4','sha256':'a'*64,
                   'public_key':'b'*64,'signature':base64.b64encode(b'x'*64).decode()}
        common = {'schema':2,'topology':'pod','cluster_id':'bezeq-rehearsal',
                  'updater_instance_id':'bezeq-node-a'}
        for role in ('a','b'):
            application = dict(common, pod_role=role, instance_id='siemcore-bezeq', database_name='siemcore')
            module.validate({'application':application,'release':release})
        witness = dict(common, pod_role='witness', updater_instance_id='bezeq-witness')
        module.validate({'application':witness,'release':release})
        broken = dict(witness, pod_role='observer')
        with self.assertRaisesRegex(ValueError, 'pod bootstrap role'):
            module.validate({'application':broken,'release':release})

class SchemaThreeTests(unittest.TestCase):
    def test_forwarding_and_legacy_boundaries(self):
        import copy
        release = {'channel':'stable','version':'3.3.152.36','sha256':'a'*64,
                   'public_key':'b'*64,'signature':base64.b64encode(b'x'*64).decode()}
        for topology, role in [('single',None),('pod','a'),('pod','b'),('pod','witness')]:
            app = dict(schema=3, topology=topology, cluster_id='lab',
                       instance_id='app', database_name='siemcore', updater_instance_id='node')
            if role: app['pod_role'] = role
            feature = 'allocation_observer' if role == 'witness' else 'archive'
            app[feature] = {'opaque_product_owned_value':'preserve-me'}
            data = {'application':app,'release':release}
            original = copy.deepcopy(data)
            module.validate(data)
            self.assertEqual(data, original)
            app['schema'] = 1 if topology == 'single' else 2
            with self.assertRaises(ValueError): module.validate(data)
            del app[feature]
            module.validate(data)
            app['schema'] = 3
            with self.assertRaises(ValueError): module.validate(data)
        app.update(schema=True,topology='single')
        with self.assertRaises(ValueError): module.validate(data)

class InputFileTests(unittest.TestCase):
    def test_private_file_and_parent_checks(self):
        from unittest.mock import Mock, patch
        import json
        import stat
        from types import SimpleNamespace
        source=Mock()
        parent=Mock()
        source.parents=[parent]
        def file(mode=0o600, uid=0, size=100):
            return SimpleNamespace(st_mode=stat.S_IFREG | mode,st_uid=uid,st_size=size)
        source.lstat.return_value=file()
        parent.lstat.return_value=SimpleNamespace(st_mode=stat.S_IFDIR | 0o700,st_uid=0)
        source.read_text.return_value=json.dumps({'application':{},'release':{}})
        with patch.object(module,'validate') as validate:
            module.read_input(source)
            validate.assert_called_once()
        for info in [file(0o644),file(uid=501),file(size=65537),
                     SimpleNamespace(st_mode=stat.S_IFLNK | 0o600,st_uid=0,st_size=100)]:
            source.lstat.return_value=info
            with self.assertRaises(ValueError): module.read_input(source)
        source.lstat.return_value=file()
        parent.lstat.return_value=SimpleNamespace(st_mode=stat.S_IFDIR | 0o777,st_uid=0)
        with self.assertRaises(ValueError): module.read_input(source)

    def test_read_only_validation_does_not_invoke_services(self):
        from unittest.mock import patch
        with patch.object(module.os,'geteuid',return_value=0), \
             patch.object(module.sys,'argv',['bootstrap','--validate-input','/root/input.json']), \
             patch.object(module,'read_input',return_value={}) as read, \
             patch.object(module.subprocess,'run') as run:
            module.main()
            read.assert_called_once_with(Path('/root/input.json'))
            run.assert_not_called()

if __name__ == '__main__': unittest.main()
