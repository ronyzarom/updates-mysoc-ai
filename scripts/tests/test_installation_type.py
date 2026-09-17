import importlib.util
from pathlib import Path
import unittest
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('installation_type', ROOT / 'kits/siemcore/installation-type.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class InstallationTypeTests(unittest.TestCase):
    def test_types_roundtrip_and_relay_unchanged(self):
        template = (ROOT / 'kits/siemcore/config.yaml').read_text()
        for kind, node in [('normal', ''), ('pod-active', '1'), ('pod-stby', '2'), ('pod-observer', 'witness')]:
            with self.subTest(kind=kind):
                identity = m.validate_identity(kind, 'pod' if node else '', node)
                rendered = m.render(template, identity)
                self.assertEqual(m.existing_identity(rendered), identity)
                self.assertEqual(m.render(rendered, identity), rendered)
                start, end = m.product_span(template)
                self.assertEqual(template[:start], rendered[:start])
                self.assertIn('relay:\n  enabled: true', rendered)
                self.assertNotIn('pod_maintenance:', rendered)

    def test_bootstrap_node_is_not_primary_authority(self):
        for role, node in [('a','1'), ('b','2')]:
            app = dict(schema=2, topology='pod', cluster_id='pod', pod_role=role)
            self.assertEqual(m.from_application(app), m.validate_identity('pod-stby','pod',node))
            self.assertEqual(m.from_application(app,'pod-active')['node_id'], node)
        self.assertEqual(m.from_application(dict(schema=1,topology='single'))['server_type'], 'normal')
        self.assertEqual(m.from_application(dict(schema=2,topology='pod',cluster_id='pod',pod_role='witness'))['server_type'],'pod-observer')

    def test_schema_three_preserves_role_isolation(self):
        self.assertEqual(m.from_application(dict(schema=3,topology='single'))['server_type'],'normal')
        for role,node in [('a','1'),('b','2'),('witness','witness')]:
            identity=m.from_application(dict(schema=3,topology='pod',cluster_id='pod',pod_role=role))
            self.assertEqual(identity['node_id'],node)
            self.assertEqual(identity['server_type'],'pod-observer' if role=='witness' else 'pod-stby')
        for schema,topology in [(True,'single'),(4,'pod'),(2,'single'),(1,'pod')]:
            with self.assertRaises(ValueError):m.from_application(dict(schema=schema,topology=topology))

    def test_conflicts_and_yaml_injection_refused(self):
        for args in [('normal','pod','1'),('pod-active','pod','witness'),('pod-observer','pod','1'),('pod-stby','pod\nrelay:','1'),('unknown','','')]:
            with self.subTest(args=args), self.assertRaises(ValueError):m.validate_identity(*args)
        text='products:\n  - name: siemcore\n    server_type: "normal"\n'
        with self.assertRaises(ValueError):m.render(text,m.validate_identity('pod-stby','pod','1'))
        with self.assertRaises(ValueError):m.from_application(dict(schema=1,topology='single'),'pod-active')

class InstallerCLITests(unittest.TestCase):
    def test_older_binary_cannot_silently_ignore_pod_role(self):
        with tempfile.TemporaryDirectory() as directory:
            directory = Path(directory)
            config = directory / 'config.yaml'
            original = 'products:\n  - name: siemcore\n    current_version: "1"\n'
            config.write_text(original)
            binary = directory / 'old-updater'
            binary.write_text('#!/bin/sh\nexit 1\n')
            binary.chmod(0o700)
            command = [sys.executable, str(ROOT / 'kits/siemcore/installation-type.py'),
                       '--config', str(config), '--binary', str(binary)]
            result = subprocess.run(command + ['--server-type','pod-stby','--pod-id','pod','--node-id','2'], capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(config.read_text(), original)
            result = subprocess.run(command + ['--server-type','normal'], capture_output=True)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(m.existing_identity(config.read_text())['server_type'], 'normal')

    def test_explicit_pod_configuration_is_rendered_for_capable_binary(self):
        with tempfile.TemporaryDirectory() as directory:
            directory = Path(directory)
            config = directory / 'config.yaml'
            config.write_text('products:\n  - name: siemcore\n    current_version: "1"\n')
            binary = directory / 'new-updater'
            binary.write_text("#!/bin/sh\ncat <<'EOF'\n{\"schema\":1,\"server_types\":[\"pod-stby\"]}\nEOF\n")
            binary.chmod(0o700)
            result = subprocess.run([sys.executable,str(ROOT / 'kits/siemcore/installation-type.py'),
                '--config',str(config),'--binary',str(binary),'--server-type','pod-stby',
                '--pod-id','pod','--node-id','2'], capture_output=True)
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(m.existing_identity(config.read_text()),m.validate_identity('pod-stby','pod','2'))

if __name__ == '__main__':unittest.main()
