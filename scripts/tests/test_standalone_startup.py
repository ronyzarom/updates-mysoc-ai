import base64
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

spec=importlib.util.spec_from_file_location('startup',Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1/startup_bootstrap.py')
startup=importlib.util.module_from_spec(spec);spec.loader.exec_module(startup)

class StartupTransportTests(unittest.TestCase):
    def test_signed_ciphertext_retry_and_tamper(self):
        signer=Ed25519PrivateKey.generate();cipher=b'fixture ciphertext'
        manifest={'ciphertext_sha256':hashlib.sha256(cipher).hexdigest()}
        raw=json.dumps(manifest,sort_keys=True,separators=(',',':')).encode()
        files={'manifest.json':raw,'manifest.sig':base64.b64encode(signer.sign(b'mysoc-standalone-inputs-v1\n'+raw)),'inputs.cms':cipher}
        public=signer.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
        with tempfile.TemporaryDirectory() as tmp:
            kit=Path(tmp);original=Path.stat
            def stat(path,*args,**kwargs):
                value=original(path,*args,**kwargs)
                if path==kit/'capsule':return SimpleNamespace(st_uid=0,st_mode=value.st_mode)
                return value
            with patch.object(startup,'fetch',side_effect=lambda n,*a:files[n]),patch.object(startup,'PUBLIC_KEY',public),patch.object(startup,'CAPSULE_MANIFEST_SHA256',hashlib.sha256(raw).hexdigest()),patch.object(Path,'stat',stat):
                startup.stage_capsule(kit);startup.stage_capsule(kit)
                self.assertEqual((kit/'capsule/inputs.cms').read_bytes(),cipher)
                files['inputs.cms']=cipher+b'tampered'
                with self.assertRaisesRegex(ValueError,'ciphertext_mismatch'):startup.stage_capsule(kit)
                files['inputs.cms']=cipher
                (kit/'capsule/inputs.cms').write_bytes(b'conflict')
                with self.assertRaisesRegex(ValueError,'retry_conflict'):startup.stage_capsule(kit)

    def test_manifest_pin_before_signature_or_write(self):
        with tempfile.TemporaryDirectory() as tmp,patch.object(startup,'fetch',return_value=b'{}'):
            with self.assertRaisesRegex(ValueError,'pinned_capsule_manifest'):startup.stage_capsule(Path(tmp))
            self.assertEqual(list(Path(tmp).iterdir()),[])

if __name__=='__main__':unittest.main()
