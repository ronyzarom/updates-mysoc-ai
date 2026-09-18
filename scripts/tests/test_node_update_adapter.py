import base64
import copy
from datetime import datetime, timezone
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
import uuid

ROOT=Path(__file__).resolve().parents[2]
SOURCE=ROOT/'deploy/node-update-v1'
# Unique module imports avoid collision with existing qualification protocols.
def load(name, filename):
    spec=importlib.util.spec_from_file_location(name,SOURCE/filename)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module
protocol=load('node_update_protocol','protocol.py')
saved=sys.modules.get('protocol');sys.modules['protocol']=protocol
adapter=load('node_update_adapter','adapter.py')
if saved is None:del sys.modules['protocol']
else:sys.modules['protocol']=saved


def binding():
    artifact=dict(product='siemcore',version='3.3.152.37',architecture='linux/amd64',
        artifact_sha256='a'*64,artifact_signature=base64.b64encode(b's'*64).decode(),binary_sha256='b'*64)
    return dict(protocol=protocol.PROTOCOL,operation_id=str(uuid.uuid4()),machine_id='c'*32,
        installation_id='node-1',updater_instance_id='updater-1',server_type='pod-node',node_id='1',
        bootstrap_policy_sha256='d'*64,predecessor=artifact,target=dict(artifact,version='3.3.152.38',artifact_sha256='e'*64),
        signing_public_key_sha256='f'*64,ui_protection_required=True)


class AdapterTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.binding=binding();self.directory=Path(self.temp.name)/self.binding['operation_id'];self.directory.mkdir()
        self.calls=[];self.worker_phase='accepted';self.health_fails=False;self.stopped=True;self.preserved=True;self.capable=False
        def worker(action,b,bundles,path):
            self.calls.append(action)
            if self.worker_phase=='timeout':raise TimeoutError()
            return dict(protocol=protocol.PROTOCOL,operation_id=b['operation_id'],operation_sha256=protocol.digest(b),phase=self.worker_phase,
                observed_at=datetime.now(timezone.utc).isoformat(),error_code='predecessor_ui_unprotected' if self.worker_phase=='blocked' else 'test')
        def health(b,which):
            if which=='target' and self.health_fails:raise OSError('health failed')
            return dict(version=b[which]['version'],binary_sha256=b[which]['binary_sha256'],installation_id=b['installation_id'],updater_id=b['updater_instance_id'],
                installation_state='installed-unlinked',node_id=b['node_id'],data_ready=True,installation_complete=True,link_ready=False,management_ready=True,pod_ready=False,authority_enabled=False,processing_enabled=False,ui_closed=which=='target' or self.capable,operator_login_usable=False)
        self.adapter=adapter.Adapter(self.directory,lambda b,new:None,lambda b:{'predecessor_ui_capable':self.capable},worker,health,
            lambda:self.stopped,lambda p:None,lambda prior:{'fixture':'unchanged'} if prior is None else self.preserved)

    def test_initial_security_upgrade_and_read_only_accepted_replay(self):
        result=self.adapter.run('apply',self.binding);self.assertEqual(result['phase'],'accepted')
        before=(self.directory/'adapter-journal.json').read_bytes()
        self.assertEqual(self.adapter.run('apply',self.binding),result)
        self.assertEqual(self.calls,['apply']);self.assertEqual((self.directory/'adapter-journal.json').read_bytes(),before)

    def test_health_failure_preserves_binding_then_first_hop_blocks(self):
        self.health_fails=True
        result=self.adapter.run('apply',self.binding);self.assertEqual(result['phase'],'recovery_required')
        original=(self.directory/'binding.json').read_bytes()
        self.worker_phase='blocked'
        result=self.adapter.run('recover',self.binding);self.assertEqual(result['phase'],'blocked');self.assertEqual(result['error_code'],'predecessor_ui_unprotected')
        self.assertEqual((self.directory/'binding.json').read_bytes(),original)
        self.adapter.run('apply',self.binding);self.assertEqual(self.calls,['apply','recover'])

    def test_uncertain_stop_never_claims_stopped(self):
        self.worker_phase='blocked';self.stopped=False
        result=self.adapter.run('apply',self.binding)
        self.assertEqual(result['phase'],'blocked');self.assertEqual(result['error_code'],'stop_unconfirmed')

    def test_worker_cannot_claim_unsafe_predecessor_restored(self):
        self.worker_phase='restored'
        self.assertEqual(self.adapter.run('apply',self.binding)['error_code'],'unsafe_predecessor_restore')

    def test_protected_future_predecessor_recovery_is_not_target_acceptance(self):
        self.capable=True;self.worker_phase='restored'
        result=self.adapter.run('apply',self.binding);self.assertEqual(result['phase'],'restored');self.assertEqual(result['error_code'],'target_update_failed')

    def test_timeout_reconciles_same_binding_not_second_apply(self):
        self.worker_phase='timeout';self.assertEqual(self.adapter.run('apply',self.binding)['phase'],'recovery_required')
        self.worker_phase='verifying';self.adapter.run('apply',self.binding)
        self.assertEqual(self.calls,['apply','status'])

    def test_changed_request_cannot_clear_security_floor(self):
        self.adapter.run('apply',self.binding)
        changed=copy.deepcopy(self.binding);changed['ui_protection_required']=False
        with self.assertRaises(ValueError):self.adapter.run('recover',changed)
        changed=copy.deepcopy(self.binding);changed['target']['artifact_sha256']='0'*64
        with self.assertRaises(ValueError):self.adapter.run('recover',changed)
        self.assertEqual(self.calls,['apply'])

    def test_protected_configuration_change_blocks_replay(self):
        self.adapter.run('apply',self.binding);self.preserved=False
        with self.assertRaises(ValueError):self.adapter.run('status',self.binding)
        self.assertEqual(self.calls,['apply'])

    def test_signature_and_unknown_or_duplicate_fields_refused(self):
        with self.assertRaises(ValueError):protocol.strict_json(b'{"x":1,"x":2}')
        for key,value in [('server_type','normal'),('machine_id','wrong'),('untrusted_path','/tmp')]:
            changed=copy.deepcopy(self.binding);changed[key]=value
            with self.assertRaises(ValueError):protocol.validate_binding(changed)
        changed=copy.deepcopy(self.binding);changed['target']['artifact_signature']=''
        with self.assertRaises(ValueError):protocol.validate_binding(changed)

    def test_stale_or_wrong_worker_result_refused(self):
        response=dict(protocol=protocol.PROTOCOL,operation_id=self.binding['operation_id'],operation_sha256=protocol.digest(self.binding),phase='accepted',observed_at='2000-01-01T00:00:00Z')
        with self.assertRaises(ValueError):protocol.validate_response(response,self.binding)
        response['observed_at']=datetime.now(timezone.utc).isoformat();response['operation_sha256']='0'*64
        with self.assertRaises(ValueError):protocol.validate_response(response,self.binding)

if __name__=='__main__':unittest.main()
