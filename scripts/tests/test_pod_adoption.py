import base64
import copy
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import uuid
from unittest.mock import patch
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

ROOT = Path(__file__).resolve().parents[2] / 'deploy/pod-adoption-v1'
def load(name):
    spec = importlib.util.spec_from_file_location('adoption_' + name, ROOT / (name + '.py'))
    m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m); return m
c = load('contract')
with patch.dict(sys.modules, {'contract': c}):
    j = load('journal')


def binding():
    return dict(protocol='pod-adoption-v1', operation_id=str(uuid.uuid4()),
      source=dict(installation_class='normal', machine_id='a'*32, installation_id='normal-b',
        updater_instance_id='updater-b', version='3.3.152.47', artifact_sha256='b'*64,
        bootstrap_receipt_sha256='c'*64),
      target=dict(product='siemcore', version='3.3.152.49', architecture='linux/amd64',
        source_commit='d'*40, artifact_sha256='e'*64, binary_sha256='f'*64),
      membership=dict(pod_id='pod-test', node_id='2'),
      observer=dict(installation_id='observer-test', endpoint='https://observer.test',
        public_key_sha256='1'*64, registry_revision=1, registry_sha256='2'*64),
      desired_settings=dict(revision=1, sha256='3'*64), approved_node_key_sha256='4'*64,
      adoption_plan_sha256='5'*64, issued_at=1000, expires_at=1300, expected_state='linked-paused')


class AdoptionTests(unittest.TestCase):
    def setUp(self):
        self.key = Ed25519PrivateKey.generate()
        self.public = self.key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)
        self.b = binding()

    def request(self, b=None):
        b = b or self.b
        return c.canonical(dict(protocol=c.PROTOCOL, binding=b,
            authorization_signature=base64.b64encode(self.key.sign(c.DOMAIN+c.canonical(b))).decode()))

    def measurements(self):
        return copy.deepcopy({k:self.b[k] for k in ('source','target','membership','observer',
            'desired_settings','approved_node_key_sha256','adoption_plan_sha256')})

    def test_exact_signature_and_current_measurements(self):
        self.assertEqual(c.verify(self.request(), self.public, self.measurements(), 1100)['binding'],self.b)

    def test_every_measured_field_must_match(self):
        for field in self.measurements():
            with self.subTest(field=field):
                m=self.measurements();m[field]=None
                with self.assertRaisesRegex(ValueError,'measured_binding_mismatch'):
                    c.verify(self.request(),self.public,m,1100)

    def test_wrong_key_and_tampering(self):
        raw=json.loads(self.request());raw['binding']['membership']['pod_id']='other'
        with self.assertRaises(InvalidSignature):c.verify(c.canonical(raw),self.public,self.measurements(),1100)
        wrong=Ed25519PrivateKey.generate().public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)
        with self.assertRaises(InvalidSignature):c.verify(self.request(),wrong,self.measurements(),1100)

    def test_expiry_future_and_lifetime(self):
        for now in (999,1300,True):
            with self.assertRaises(ValueError):c.verify(self.request(),self.public,self.measurements(),now)
        self.b['expires_at']=2000
        with self.assertRaisesRegex(ValueError,'lifetime'):c.parse(self.request())

    def test_unknown_duplicate_oversize_activation(self):
        raw=self.request()
        with self.assertRaises(ValueError):c.parse(raw.replace(b'{',b'{"protocol":"pod-adoption-v1",',1))
        with self.assertRaises(ValueError):c.parse(b' '*32769)
        b=copy.deepcopy(self.b);b['extra']=True
        with self.assertRaises(ValueError):c.parse(self.request(b))
        b=copy.deepcopy(self.b);b['expected_state']='active'
        with self.assertRaisesRegex(ValueError,'activation'):c.parse(self.request(b))

    def test_roles_endpoint_and_revision(self):
        for field,value in [('node_id','witness'),('node_id','../2')]:
            b=copy.deepcopy(self.b);b['membership'][field]=value
            with self.assertRaises(ValueError):c.parse(self.request(b))
        for url in ('http://observer.test','https://u:p@observer.test','https://observer.test/path'):
            b=copy.deepcopy(self.b);b['observer']['endpoint']=url
            with self.assertRaises(ValueError):c.parse(self.request(b))
        self.b['desired_settings']['revision']=True
        with self.assertRaises(ValueError):c.parse(self.request())

    def test_observer_self_adoption_schema(self):
        self.b['source']['installation_class']='observer-unlinked'
        self.b['source']['installation_id']='observer-test';self.b['membership']['node_id']='witness'
        c.verify(self.request(),self.public,self.measurements(),1100)

    def test_journal_retry_remeasures_and_conflict(self):
        with tempfile.TemporaryDirectory() as d:
            fd=os.open(d,os.O_RDONLY|os.O_DIRECTORY)
            try:
                first=j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
                self.assertEqual(first,j.record(fd,self.request(),self.public,self.measurements,lambda:1101))
                self.assertFalse(first['processing_authorized'])
                with self.assertRaises(ValueError):j.record(fd,self.request(),self.public,lambda:{},lambda:1100)
                with self.assertRaises(ValueError):j.record(fd,self.request(),self.public,self.measurements,lambda:1300)
                self.b['operation_id']=str(uuid.uuid4())
                with self.assertRaisesRegex(ValueError,'conflict'):j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
            finally:os.close(fd)

    def test_interrupted_write_and_symlink(self):
        with tempfile.TemporaryDirectory() as d:
            fd=os.open(d,os.O_RDONLY|os.O_DIRECTORY)
            try:
                with patch.object(j.os,'replace',side_effect=OSError('interrupted')):
                    with self.assertRaises(OSError):j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
                self.assertFalse((Path(d)/'admission.json').exists())
                j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
                (Path(d)/'admission.json').unlink();(Path(d)/'admission.json').symlink_to('/dev/null')
                with self.assertRaises(OSError):j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
            finally:os.close(fd)

    def test_lock_contention_and_unsafe_journal(self):
        with tempfile.TemporaryDirectory() as d:
            fd=os.open(d,os.O_RDONLY|os.O_DIRECTORY)
            lock=os.open(Path(d)/'admission.lock',os.O_CREAT|os.O_RDWR,0o600)
            try:
                fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
                with self.assertRaises(BlockingIOError):
                    j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
                self.assertFalse((Path(d)/'admission.json').exists())
                fcntl.flock(lock,fcntl.LOCK_UN)
                j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
                os.link(Path(d)/'admission.json',Path(d)/'hardlink')
                with self.assertRaisesRegex(ValueError,'hardlink'):
                    j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
                (Path(d)/'hardlink').unlink()
                (Path(d)/'admission.json').chmod(0o644)
                with self.assertRaisesRegex(ValueError,'private_owned'):
                    j.record(fd,self.request(),self.public,self.measurements,lambda:1100)
            finally:
                os.close(lock);os.close(fd)

    def test_release_domain_and_bytes(self):
        blob=b'synthetic';sha=hashlib.sha256(blob).hexdigest()
        msg=('mysoc-release-v1\nsiemcore\n3.3.152.49\n'+sha).encode()
        sig=base64.b64encode(self.key.sign(msg)).decode()
        c.verify_release(blob,'siemcore','3.3.152.49',sha,sig,self.public)
        with self.assertRaises(ValueError):c.verify_release(blob+b'!','siemcore','3.3.152.49',sha,sig,self.public)
        with self.assertRaises(InvalidSignature):c.verify_release(blob,'siemcore','3.3.152.50',sha,sig,self.public)

    def test_cli_all_actions_unavailable(self):
        for action in ('readiness','apply','status','recover'):
            r=subprocess.run([sys.executable,'-B',str(ROOT/'cli.py'),action],capture_output=True,text=True)
            self.assertEqual(r.returncode,78);v=json.loads(r.stdout)
            self.assertFalse(v['capability_enabled']);self.assertEqual(v['mutation'],'none')
            self.assertEqual(v['operation_state'],'unknown')
