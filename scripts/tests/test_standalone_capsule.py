import base64
from datetime import datetime, timedelta, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1'))
from capsule import decrypt_capsule, CAPSULE_PROTOCOL
from transaction import canonical


class CapsuleTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp=tempfile.TemporaryDirectory();cls.root=Path(cls.tmp.name)
        cls.key=cls.root/'key.pem';cls.cert=cls.root/'cert.pem'
        subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(cls.key),'-out',str(cls.cert),'-days','1','-subj','/CN=Disposable Encryption Recipient'],check=True,capture_output=True)

    @classmethod
    def tearDownClass(cls):cls.tmp.cleanup()

    def setUp(self):
        self.signer=Ed25519PrivateKey.generate()
        self.public=self.signer.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw)
        self.identity={'vm_id':'1','machine_id':'a'*32,'installation_id':'fixture','updater_instance_id':'updater','node_id':'1'}
        payload={'protocol':CAPSULE_PROTOCOL,'identity':self.identity,'files':{'mysoc-bootstrap.json':base64.b64encode(b'{"fixture":true}').decode(),'archive-credentials/gcp-archiver.json':base64.b64encode(b'{"synthetic":true}').decode()},'policy':{'fixture':True}}
        self.cipher=subprocess.run(['openssl','cms','-encrypt','-aes256','-binary','-outform','DER',str(self.cert)],input=canonical(payload),check=True,capture_output=True).stdout
        self.manifest=dict(protocol=CAPSULE_PROTOCOL,identity=self.identity,recipient_sha256=hashlib.sha256(self.cert.read_bytes()).hexdigest(),ciphertext_sha256=hashlib.sha256(self.cipher).hexdigest(),expires_at=(datetime.now(timezone.utc)+timedelta(hours=1)).isoformat())

    def decrypt(self,manifest=None,cipher=None,identity=None):
        manifest=manifest or self.manifest
        signature=base64.b64encode(self.signer.sign(b'mysoc-standalone-inputs-v1\n'+canonical(manifest))).decode()
        return decrypt_capsule(manifest,cipher or self.cipher,signature,self.public,identity or self.identity,self.cert.read_bytes(),self.key)

    def test_real_cms_round_trip(self):
        files,policy=self.decrypt()
        self.assertEqual(files['mysoc-bootstrap.json'],b'{"fixture":true}')
        self.assertEqual(policy,{'fixture':True})

    def test_wrong_machine_and_tampered_cipher(self):
        with self.assertRaisesRegex(ValueError,'recipient_mismatch'):self.decrypt(identity=dict(self.identity,vm_id='2'))
        with self.assertRaisesRegex(ValueError,'ciphertext_mismatch'):self.decrypt(cipher=self.cipher+b'bad')

    def test_expired_and_extra_manifest(self):
        with self.assertRaisesRegex(ValueError,'expired'):self.decrypt(manifest=dict(self.manifest,expires_at=(datetime.now(timezone.utc)-timedelta(seconds=1)).isoformat()))
        with self.assertRaisesRegex(ValueError,'exact_capsule_manifest'):self.decrypt(manifest=dict(self.manifest,extra=True))


if __name__=='__main__':unittest.main()
