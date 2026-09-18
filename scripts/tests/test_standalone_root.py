import base64
import hashlib
import io
import json
import os
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'deploy/node-standalone-v1'))
from source_loader import SourceLoader
from artifacts import ArtifactStager


class RootSourceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.loader = SourceLoader(self.root, os.geteuid())
        self.write('/etc/machine-id', 'a'*32)
        self.write('/etc/siemcore/greenfield.json', dict(schema=5, topology='node-unlinked', machine_id='a'*32, installation_id='a', updater_instance_id='u', node_id='1'))
        self.write('/var/lib/siemcore-greenfield/journal.json', {'status':'complete'})
        self.write('/etc/siemcore/greenfield-release.json', {'version':'3.3.152.40','sha256':'b'*64,'signature':'fixture'})

    def write(self, name, value):
        path = self.loader.path(name)
        path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        path.write_text(value if isinstance(value, str) else json.dumps(value))
        path.chmod(0o600)

    def test_measures_original_identity(self):
        evidence, _, _ = self.loader.measure({})
        self.assertEqual(evidence['current_version'], '3.3.152.40')
        self.assertEqual(evidence['application']['installation_id'], 'a')

    def test_new_history_and_incomplete_operation_rejected(self):
        path = '/var/lib/siemcore-node-update/operations/5b3e531d-0bbb-44bd-b639-5b872c735f06/binding.json'
        self.write(path, {})
        with self.assertRaisesRegex(ValueError, 'historical_inventory_changed'): self.loader.measure({})
        with self.assertRaisesRegex(ValueError, 'incomplete_historical_operation'): self.loader.measure(self.loader.inventory())

    def test_link_history_blocks_without_link_file(self):
        self.write('/var/lib/siemcore-node-link/operations/old/receipt.json', {})
        with self.assertRaisesRegex(ValueError, 'linked_history_refused'): self.loader.measure(self.loader.inventory())

    def test_symlink_and_changed_machine_refused(self):
        self.write('/etc/machine-id', 'b'*32)
        with self.assertRaisesRegex(ValueError, 'machine_identity_mismatch'): self.loader.measure({})
        path = self.loader.path('/etc/siemcore/greenfield.json')
        path.unlink(); path.symlink_to('/etc/passwd')
        with self.assertRaisesRegex(ValueError, 'unprotected_source_path'): self.loader.measure({})


class SignedStagingTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        (self.root/'cache').mkdir(); (self.root/'staging').mkdir()
        self.key = Ed25519PrivateKey.generate()
        public = self.key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
        self.stager = ArtifactStager(self.root/'staging', self.root/'cache', public, lambda p:p)
        self.version = '3.3.152.42'
        prefix = 'siemcore-universal-'+self.version
        self.archive = self.root/'cache'/('siemcore-'+self.version+'.artifact')
        with tarfile.open(self.archive, 'w:gz') as archive:
            for name, data in {'MANIFEST.json':json.dumps(dict(product='siemcore', version=self.version, architecture='amd64', build={'git_commit':'c'*40})).encode(), 'pod/bin/siemcore':b'fixture binary'}.items():
                info = tarfile.TarInfo(prefix+'/'+name); info.size=len(data)
                archive.addfile(info, io.BytesIO(data))
        checksum=hashlib.sha256(self.archive.read_bytes()).hexdigest()
        signature=self.key.sign(f'mysoc-release-v1\nsiemcore\n{self.version}\n{checksum}'.encode())
        self.artifact=dict(version=self.version,artifact_sha256=checksum,artifact_signature=base64.b64encode(signature).decode(),binary_sha256=hashlib.sha256(b'fixture binary').hexdigest(),source_commit='c'*40)

    def test_verified_local_archive(self):
        bundle, _ = self.stager.stage_one(self.artifact, 'target')
        self.assertEqual((bundle/'pod/bin/siemcore').read_bytes(), b'fixture binary')

    def test_commit_and_binary_mismatch(self):
        self.artifact['source_commit']='d'*40
        with self.assertRaisesRegex(ValueError, 'source commit mismatch'): self.stager.stage_one(self.artifact, 'target')
        self.artifact['source_commit']='c'*40; self.artifact['binary_sha256']='0'*64
        with self.assertRaisesRegex(ValueError, 'binary mismatch'): self.stager.stage_one(self.artifact, 'target')

    def test_tampered_archive(self):
        with self.archive.open('ab') as stream: stream.write(b'tampered')
        with self.assertRaisesRegex(ValueError, 'checksum mismatch'): self.stager.stage_one(self.artifact, 'target')


if __name__ == '__main__': unittest.main()
