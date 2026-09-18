import base64
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('node_kit', ROOT/'scripts/packaging/independent_node_kit.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class KitTests(unittest.TestCase):
    def test_signature_architecture_and_version_before_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)
            version = (ROOT/'VERSION').read_text().strip()
            binary = bytearray(64); binary[:6] = b'\x7fELF\x02\x01'; binary[18:20] = struct.pack('<H',183)
            (path/'binary').write_bytes(binary)
            digest = hashlib.sha256(binary).hexdigest()
            key = Ed25519PrivateKey.generate()
            receipt = dict(product='updater-linux-arm64',version=version,sha256=digest,
                signature=base64.b64encode(key.sign(('mysoc-release-v1\nupdater-linux-arm64\n'+version+'\n'+digest).encode())).decode())
            (path/'receipt').write_text(json.dumps(receipt))
            args = SimpleNamespace(provisioning_source=str(path), package_revision='r1', receipt=str(path/'receipt'),binary=str(path/'binary'),architecture='arm64',public_key=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex(),output=str(path/'out'))
            with patch.object(module, 'clean_commit', return_value='a'*40):
                # A valid signed input reaches the missing product modules check.
                with self.assertRaisesRegex(ValueError, 'provisioning module'): module.package(args, ROOT)
                self.assertFalse((path/'out').exists())
                args.architecture='amd64'
                with self.assertRaisesRegex(ValueError, 'mismatch'): module.package(args, ROOT)
                args.architecture='arm64'
                receipt['signature']=base64.b64encode(b'0'*64).decode()
                (path/'receipt').write_text(json.dumps(receipt))
                with self.assertRaises(Exception): module.package(args, ROOT)
                self.assertFalse((path/'out').exists())
