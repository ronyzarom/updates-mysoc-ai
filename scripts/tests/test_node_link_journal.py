import base64
import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from scripts.tests.test_node_link_contract import request, contract
from journal import record_admission
from release_verification import verify_archives


class LinkJournalTests(unittest.TestCase):
    def test_retry_retains_binding_and_rechecks_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            q = request()
            calls = []
            def verify():
                calls.append(True)
                return True
            first = record_admission(directory, q, verify)
            self.assertEqual(record_admission(directory, q, verify), first)
            self.assertEqual(len(calls), 2)
            self.assertEqual(first['phase'], 'admitted-awaiting-product')
            q['binding']['observer']['generation'] += 1
            q['operation_sha256'] = contract.binding_digest(q['binding'])
            with self.assertRaisesRegex(ValueError, 'retained_operation_conflict'):
                record_admission(directory, q, verify)

    def test_failed_admission_never_writes_receipt(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(ValueError, 'current_inputs_not_verified'):
                record_admission(directory, request(), lambda: False)
            self.assertFalse((Path(directory) / 'admission.json').exists())

    def test_interrupted_preparation_retry_and_symlink_refusal(self):
        with tempfile.TemporaryDirectory() as directory:
            # Crash before atomic rename can leave an unrelated temporary file.
            orphan = Path(directory) / '.admission-interrupted'
            orphan.write_text('partial')
            result = record_admission(directory, request(), lambda: True)
            self.assertEqual(result['mutation'], 'none')
            journal = Path(directory) / 'admission.json'
            journal.unlink()
            journal.symlink_to(orphan)
            with self.assertRaisesRegex(ValueError, 'private_owned_path_required'):
                record_admission(directory, request(), lambda: True)
            self.assertEqual(orphan.read_text(), 'partial')


class LinkReleaseTests(unittest.TestCase):
    def setUp(self):
        self.key = Ed25519PrivateKey.generate()
        self.public = self.key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
        self.b = copy.deepcopy(request()['binding'])
        self.source, self.target = b'source fixture', b'target fixture'
        self.b['source_artifact_sha256'] = hashlib.sha256(self.source).hexdigest()
        self.b['target']['artifact_sha256'] = hashlib.sha256(self.target).hexdigest()
        self.signatures = {}
        for role, version, digest in (
            ('source', self.b['source_version'], self.b['source_artifact_sha256']),
            ('target', self.b['target']['version'], self.b['target']['artifact_sha256']),
        ):
            message = f'mysoc-release-v1\nsiemcore\n{version}\n{digest}'.encode()
            self.signatures[role] = base64.b64encode(self.key.sign(message)).decode()

    def test_both_signed_archives(self):
        verify_archives(self.b, self.source, self.target, self.public, self.signatures)

    def test_tampered_archive(self):
        with self.assertRaisesRegex(ValueError, 'artifact_checksum_mismatch'):
            verify_archives(self.b, self.source, self.target + b'!', self.public, self.signatures)

    def test_wrong_key_and_rebound_version(self):
        wrong = Ed25519PrivateKey.generate().public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
        with self.assertRaises(InvalidSignature):
            verify_archives(self.b, self.source, self.target, wrong, self.signatures)
        self.b['target']['version'] = '3.3.152.43'
        with self.assertRaises(InvalidSignature):
            verify_archives(self.b, self.source, self.target, self.public, self.signatures)


if __name__ == '__main__':
    unittest.main()
