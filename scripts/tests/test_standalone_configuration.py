import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'deploy/node-standalone-v1'))
from configuration_inventory import configuration_digest


class ConfigurationInventoryTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.inputs = {'application.env': b'fixture=true\n', 'installation/tls.crt': b'fixture cert', 'installation/tls.key': b'fixture key'}
        self.metadata = {}
        for name, data in self.inputs.items():
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
            path.chmod(0o600)
            self.metadata[name] = dict(uid=os.geteuid(), gid=os.getegid(), mode=0o600)

    def test_exact_canonical_bytes_no_mutation(self):
        expected = hashlib.sha256(json.dumps({k: hashlib.sha256(v).hexdigest() for k, v in self.inputs.items()}, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
        self.assertEqual(configuration_digest(self.root, self.metadata), expected)
        for name, data in self.inputs.items():
            self.assertEqual((self.root / name).read_bytes(), data)
            self.assertEqual((self.root / name).stat().st_mode & 0o777, 0o600)

    def test_extra_missing_and_mode_change(self):
        extra = self.root / 'extra'
        extra.write_text('unexpected')
        with self.assertRaises(ValueError): configuration_digest(self.root, self.metadata)
        extra.unlink()
        (self.root / 'application.env').chmod(0o644)
        with self.assertRaisesRegex(ValueError, 'metadata_mismatch'): configuration_digest(self.root, self.metadata)
        (self.root / 'application.env').unlink()
        with self.assertRaises(ValueError): configuration_digest(self.root, self.metadata)

    def test_symlinks_refused(self):
        path = self.root / 'application.env'
        path.unlink()
        path.symlink_to(self.root / 'installation/tls.key')
        with self.assertRaises((ValueError, OSError)): configuration_digest(self.root, self.metadata)

    def test_mode_receipt_cannot_enter_hash(self):
        self.metadata['installation/mode.json'] = dict(uid=os.geteuid(), gid=os.getegid(), mode=0o600)
        with self.assertRaisesRegex(ValueError, 'invalid_configuration_name'): configuration_digest(self.root, self.metadata)


if __name__ == '__main__': unittest.main()
