import base64
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

spec=importlib.util.spec_from_file_location('repair_install',Path(__file__).parents[2]/'deploy/node-bootstrap-repair/install.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class RepairInstallerTests(unittest.TestCase):
    def test_signed_authorization_tamper_and_wrong_key(self):
        key=Ed25519PrivateKey.generate();pub=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex()
        raw=json.dumps(dict(protocol='pod-node-bootstrap-health-repair-v1',action='docker-cap-prefix-v1')).encode()
        sig=base64.b64encode(key.sign(b'mysoc-pod-bootstrap-repair-v1\n'+raw)).decode()
        self.assertEqual(m.verify_authorization(raw,sig,pub)['action'],'docker-cap-prefix-v1')
        for content in (raw+b' ',raw.replace(b'prefix',b'other')):
            with self.assertRaises(Exception):m.verify_authorization(content,sig,pub)
        with self.assertRaises(Exception):m.verify_authorization(raw,sig,'00'*32)
    def test_duplicate_keys(self):
        with self.assertRaises(ValueError):json.loads('{"a":1,"a":2}',object_pairs_hook=m.unique)
    def test_package_tamper_extra_and_symlink(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            for name in m.FILES:(root/name).write_text(name)
            package={'files':{name:m.sha(root/name) for name in m.FILES}}
            m.verify_package(root,package)
            package['files']['../outside']='bad'
            with self.assertRaises(ValueError):m.verify_package(root,package)
            del package['files']['../outside']
            (root/'install.py').write_text('changed')
            with self.assertRaises(ValueError):m.verify_package(root,package)
            (root/'install.py').unlink();(root/'install.py').symlink_to(root/'greenfield-hook.py')
            with self.assertRaises(ValueError):m.verify_package(root,package)

if __name__=='__main__':unittest.main()
