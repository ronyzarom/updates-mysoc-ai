import importlib.util
from pathlib import Path
import stat
import tempfile
import unittest
from unittest.mock import patch

path = Path(__file__).resolve().parents[1] / 'ops/install_pod_observer.py'
spec = importlib.util.spec_from_file_location('observer_install', path)
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class ObserverInstallTests(unittest.TestCase):
    def test_atomic_binary_and_permissions(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'siemcore'
            installer.atomic_bytes(target, b'verified fixture', 0o755)
            self.assertEqual(target.read_bytes(), b'verified fixture')
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o755)

    def test_interrupted_replace_preserves_previous_target(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'siemcore'
            target.write_bytes(b'previous fixture')
            with patch.object(installer.os, 'replace', side_effect=OSError('injected failure')):
                with self.assertRaises(OSError):
                    installer.atomic_bytes(target, b'next fixture', 0o755)
            self.assertEqual(target.read_bytes(), b'previous fixture')
            self.assertEqual(list(Path(directory).iterdir()), [target])


if __name__ == '__main__':
    unittest.main()
