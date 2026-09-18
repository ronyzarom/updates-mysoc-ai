import copy
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'deploy/node-standalone-v1'))
from transaction import admit, digest, persist_intent


class StandaloneAdmissionTests(unittest.TestCase):
    def setUp(self):
        identity = dict(machine_id='a'*32, installation_id='node-a', updater_instance_id='updater-a', node_id='1')
        app = dict(identity, schema=5, topology='node-unlinked')
        self.evidence = dict(application=app, journals=[], current_version='3.3.152.41', current_artifact_sha256='b'*64, linked_evidence=[], inventory_complete=True)
        self.bootstrap = json.dumps(dict(status='complete', installation_state='installed-unlinked', policy_sha256=hashlib.sha256(json.dumps(app, sort_keys=True).encode()).hexdigest())).encode()
        self.binding = dict(protocol='pod-node-standalone-v1', operation_id='5b3e531d-0bbb-44bd-b639-5b872c735f06', source=identity, source_version='3.3.152.41', source_artifact_sha256='b'*64, bootstrap_receipt_sha256=hashlib.sha256(self.bootstrap).hexdigest(), source_evidence_sha256=digest(self.evidence), target=dict(product='siemcore', version='3.3.152.42', architecture='linux/amd64', source_commit='c'*40, artifact_sha256='d'*64, binary_sha256='e'*64), configuration_sha256='f'*64, instance_id='customer-a', source_mode='independent-management', target_mode='independent-standalone')

    def admission(self):
        return admit(self.binding, copy.deepcopy(self.binding), self.evidence, self.bootstrap, 'f'*64)

    def test_mode_intent_does_not_enable_processing_or_updates(self):
        intent = self.admission()
        self.assertEqual(intent['effective_mode'], 'independent-management')
        self.assertEqual(intent['target_mode'], 'independent-standalone')
        self.assertFalse(intent['routine_updates_allowed'])
        with tempfile.TemporaryDirectory() as directory:
            persist_intent(directory, intent)
            persist_intent(directory, intent)
            self.assertEqual(json.loads((Path(directory)/'standalone-intent.json').read_text()), intent)
            self.assertFalse((Path(directory)/'mode.json').exists())

    def test_unreviewed_inventory_rejected(self):
        self.evidence['journals'].append({})
        with self.assertRaisesRegex(ValueError, 'reviewed_source_inventory_mismatch'): self.admission()

    def test_pending_and_linked_history_rejected_even_if_reviewed(self):
        for protocol, phase in (('pod-node-update-v1', 'switching'), ('pod-node-link-v1', 'accepted'), ('unknown', 'accepted')):
            with self.subTest(protocol=protocol):
                self.evidence['journals'] = [dict(protocol=protocol, phase=phase, identity=self.binding['source'], receipt_sha256='a'*64)]
                self.binding['source_evidence_sha256'] = digest(self.evidence)
                with self.assertRaisesRegex(ValueError, 'prior_operation_unreconciled'): self.admission()

    def test_original_receipt_preserved(self):
        self.bootstrap += b' '
        with self.assertRaisesRegex(ValueError, 'original_bootstrap_changed'): self.admission()

    def test_no_silent_identity_or_mode_change(self):
        self.binding['target_mode'] = 'linked'
        with self.assertRaisesRegex(ValueError, 'unsupported_mode_transition'): self.admission()

    def test_journal_cannot_mark_standalone_effective(self):
        intent = self.admission()
        intent['effective_mode'] = 'independent-standalone'
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(ValueError, 'admitted_intent_only'): persist_intent(directory, intent)

    def test_conflicting_replay_refused(self):
        intent = self.admission()
        with tempfile.TemporaryDirectory() as directory:
            persist_intent(directory, intent)
            self.binding['instance_id'] = 'different-customer'
            with self.assertRaisesRegex(ValueError, 'retained_operation_conflict'): persist_intent(directory, self.admission())


if __name__ == '__main__': unittest.main()
