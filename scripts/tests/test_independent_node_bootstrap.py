import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import stat
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from scripts.tests import test_greenfield_bootstrap as bootstrap_tests
module = bootstrap_tests.module

spec = importlib.util.spec_from_file_location('node_identity', Path(__file__).parents[2] / 'kits/siemcore/installation-type.py')
identity = importlib.util.module_from_spec(spec)
spec.loader.exec_module(identity)

class IndependentNodeTests(unittest.TestCase):
    def fixture(self, node='1'):
        data = bootstrap_tests.IndependentObserverTests().fixture()
        data['application'].update(schema=5, topology='node-unlinked', node_id=node,
            settings_file='/etc/siemcore/node-settings.json', settings_sha256=hashlib.sha256(b'{}').hexdigest())
        return data

    def test_both_nodes_without_registry_or_authority(self):
        for node in ('1', '2'):
            data = self.fixture(node)
            before = copy.deepcopy(data)
            module.validate(data)
            self.assertEqual(data, before)
            result = identity.from_application(data['application'])
            self.assertEqual(result, dict(server_type='pod-node', node_id=node, pod_id=''))
            self.assertEqual(identity.existing_identity(identity.render('products:\n  - name: siemcore\n    channel: stable\n', result)), result)
            with self.assertRaises(ValueError): identity.from_application(data['application'], 'normal')

    def test_reject_invalid_or_authority_fields(self):
        for field, value in [('schema', True), ('node_id', 1), ('node_id', 'witness'),
                             ('pod_id', 'fake'), ('peer', 'host'), ('authority_enabled', True),
                             ('settings_file', '../settings'), ('settings_sha256', 'bad')]:
            data = self.fixture(); data['application'][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError): module.validate(data)

    def test_local_settings_are_private_and_bound(self):
        app = self.fixture()['application']
        regular = SimpleNamespace(st_mode=stat.S_IFREG | 0o600, st_uid=0, st_size=2)
        with patch.object(module.Path, 'lstat', return_value=regular), patch.object(module.Path, 'stat', return_value=regular), patch.object(module.Path, 'read_bytes', return_value=b'{}'):
            module.validate_local_node(app)
            changed = dict(app, settings_sha256='0'*64)
            with self.assertRaisesRegex(ValueError, 'checksum'): module.validate_local_node(changed)
            regular.st_mode = stat.S_IFREG | 0o644
            with self.assertRaisesRegex(ValueError, '0600'): module.validate_local_node(app)
            regular.st_mode = stat.S_IFLNK | 0o600
            with self.assertRaises(ValueError): module.validate_local_node(app)

    def test_duplicate_settings_or_envelope_fields_refused(self):
        with self.assertRaises(ValueError): json.loads('{"node_id":"1","node_id":"2"}', object_pairs_hook=module.unique_object)

    def test_delivery_gate_before_any_mutation(self):
        with patch.object(module.os, 'geteuid', return_value=0), patch.object(module.sys, 'argv', ['bootstrap', '/root/input', '/kit']), patch.object(module, 'read_input', return_value=self.fixture()), patch.object(module.subprocess, 'run') as run:
            with self.assertRaisesRegex(ValueError, 'delivery disabled'): module.main()
            run.assert_not_called()

    def test_machine_mismatch_and_reclassification_refused(self):
        with patch.object(module.Path, 'read_text', return_value='b'*32):
            with self.assertRaisesRegex(ValueError, 'machine binding'): module.validate_local_observer(self.fixture()['application'])
        for pod, node in [('fake','1'), ('',''), ('','witness')]:
            with self.assertRaises(ValueError): identity.validate_identity('pod-node', pod, node)
        text=identity.render('products:\n  - name: siemcore\n', identity.validate_identity('pod-node', '', '1'))
        with self.assertRaises(ValueError): identity.render(text, identity.validate_identity('normal'))

    def test_installer_preflight_refuses_unqualified_node_before_host_changes(self):
        with patch.object(module.os, 'geteuid', return_value=0), patch.object(module.sys, 'argv', ['bootstrap', '--validate-install', '/root/input']), patch.object(module, 'read_input', return_value=self.fixture()), patch.object(module.subprocess, 'run') as run:
            with self.assertRaisesRegex(ValueError, 'delivery disabled'): module.main()
            run.assert_not_called()

    def test_kit_marker_binds_reviewed_hook_and_commit(self):
        with tempfile.TemporaryDirectory() as tmp:
            kit = Path(tmp)
            hook = kit / 'greenfield-hook.py'
            hook.write_bytes(b'qualified hook fixture')
            commit = kit / 'PROVISIONING_COMMIT'
            commit.write_text('a' * 40 + '\n')
            record = dict(schema=1, protocol='pod-node-bootstrap-v1', provisioning_commit='a'*40,
                hook_sha256=hashlib.sha256(hook.read_bytes()).hexdigest())
            marker = kit / 'INDEPENDENT-NODE-BOOTSTRAP.json'
            marker.write_text(json.dumps(record))
            module.require_delivery(self.fixture(), kit)
            for field, value in [('schema', True), ('protocol', 'other'), ('extra', 1)]:
                marker.write_text(json.dumps(dict(record, **{field:value})))
                with self.assertRaises(ValueError): module.require_delivery(self.fixture(), kit)
            marker.write_text(json.dumps(record))
            hook.write_bytes(b'changed')
            with self.assertRaisesRegex(ValueError, 'binding'): module.require_delivery(self.fixture(), kit)
            hook.write_bytes(b'qualified hook fixture')
            commit.write_text('b'*40)
            with self.assertRaisesRegex(ValueError, 'binding'): module.require_delivery(self.fixture(), kit)
            commit.write_text('a'*40)
            hook.rename(kit / 'real-hook')
            hook.symlink_to(kit / 'real-hook')
            with self.assertRaises(ValueError): module.require_delivery(self.fixture(), kit)

    def test_node_execution_is_explicit_and_normal_observer_unchanged(self):
        self.assertIn('independent_node_bootstrap: true', module.filesystem_block(self.fixture()['application']))
        for topology in ('standalone', 'observer-unlinked', 'pod'):
            application = dict(topology=topology)
            self.assertEqual(module.filesystem_block(application), module.FILESYSTEM_BLOCK)
            module.require_delivery(dict(application=application), '/nonexistent-kit')

    def test_explicit_node_requires_relay_certificate_before_host_changes(self):
        script = Path(__file__).parents[2] / 'kits/siemcore/install.sh'
        for flags in ([], ['--relay-cert-file', '/missing-cert'], ['--relay-key-file', '/missing-key']):
            result = subprocess.run(['bash', str(script), '--clean', '--server-type', 'pod-node', *flags], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('requires --relay-cert-file and --relay-key-file', result.stderr)
            self.assertNotIn('creating service user', result.stdout)
