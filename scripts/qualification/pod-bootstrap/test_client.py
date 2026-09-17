import copy
import hashlib
import http.client
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import client as c
import protocol as p

FIXTURE=json.loads((Path(__file__).parent/'registry-v1.fixture.json').read_text())

class FakeTransport:
    def __init__(self, failures=(), pending=0):
        self.failures=list(failures); self.pending=pending; self.calls=[]
    def call(self, action, sent, timeout):
        self.calls.append((action,copy.deepcopy(sent)))
        if self.failures and action==self.failures[0]:
            self.failures.pop(0); raise ConnectionResetError('lost response after commit')
        if action=='all-registered' and self.pending:
            self.pending-=1; raise c.RegistrationsPending()
        return json.dumps(dict(protocol=p.PROTOCOL,operation_id=sent['operation_id'],
            registry_sha256=sent['registry_sha256'],generation=7,
            phase=p.ACTIONS[action],processing_allowed=False))

class CoordinatorTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.path=Path(self.temp.name)/'journal.json';self.now=0
        # Only the root filesystem reader is substituted: real atomic persistence remains.
        self.reader=patch.object(c,'protected',side_effect=lambda path:Path(path).read_bytes())
        self.reader.start();self.addCleanup(self.reader.stop)
    def sleep(self, seconds):self.now+=seconds
    def coordinator(self, transport, binding=None):
        return c.Coordinator(FIXTURE['registry'],FIXTURE['registry_sha256'],'1','a'*64,
            transport,self.path, binding or {'input_sha256':'a'*64},timeout=5,
            clock=lambda:self.now,sleep=self.sleep)
    def test_success_is_registration_only(self):
        result=self.coordinator(FakeTransport()).run_locked()
        self.assertEqual(result['phase'],'all-registered')
        self.assertIs(result['installation_complete'],False)
        self.assertIs(result['processing_allowed'],False)
    def test_each_lost_response_reconciles_same_operation_generation(self):
        for action in ('begin-or-resume','register','all-registered'):
            with self.subTest(action=action):
                self.path.unlink(missing_ok=True);self.now=0
                transport=FakeTransport([action]);self.coordinator(transport).run_locked()
                actions=[a for a,_ in transport.calls]
                index=actions.index(action)
                self.assertEqual(actions[index+1],'status')
                self.assertEqual({s['operation_id'] for _,s in transport.calls},
                                 {FIXTURE['registry']['operation_id']})
                self.assertEqual({s['generation'] for a,s in transport.calls if a=='register'},{7})
                self.assertEqual({s['input_sha256'] for a,s in transport.calls if a=='register'},{'a'*64})
    def test_restart_revalidates_server_before_claiming_registration(self):
        self.coordinator(FakeTransport()).run_locked()
        transport=FakeTransport();self.coordinator(transport).run_locked()
        self.assertEqual([a for a,_ in transport.calls],['status','all-registered'])
    def test_changed_binding_rejected_before_network(self):
        self.coordinator(FakeTransport()).run_locked();transport=FakeTransport()
        with self.assertRaises(ValueError):self.coordinator(transport,{'input_sha256':'b'*64}).run_locked()
        self.assertEqual(transport.calls,[])
    def test_intent_disk_failure_prevents_network(self):
        transport=FakeTransport()
        with patch.object(c,'durable',side_effect=OSError('disk full')),self.assertRaises(OSError):
            self.coordinator(transport).run_locked()
        self.assertEqual(transport.calls,[])
    def test_generation_disk_failure_prevents_registration(self):
        transport=FakeTransport();original=c.durable
        def write(path,state):
            if state['generation']:raise OSError('disk full')
            original(path,state)
        with patch.object(c,'durable',side_effect=write),self.assertRaises(OSError):
            self.coordinator(transport).run_locked()
        self.assertEqual([a for a,_ in transport.calls],['begin-or-resume'])
        self.assertEqual(json.loads(self.path.read_text())['generation'],0)
    def test_pending_deadline_preserves_registered_phase(self):
        with self.assertRaises(TimeoutError):self.coordinator(FakeTransport(pending=99)).run_locked()
        self.assertEqual(json.loads(self.path.read_text())['phase'],'registered')
    def test_changed_server_generation_fails_closed(self):
        transport=FakeTransport();base=transport.call
        def call(action,sent,timeout):
            ack=json.loads(base(action,sent,timeout))
            if action=='register':ack['generation']=8
            return json.dumps(ack)
        transport.call=call
        with self.assertRaises(ValueError):self.coordinator(transport).run_locked()
        self.assertEqual(json.loads(self.path.read_text())['phase'],'barrier')

class RegistryTests(unittest.TestCase):
    def validate(self,registry):
        c.validate_registry(registry,bytes.fromhex(FIXTURE['pinned_public_key_hex']),'amd64')
    def test_shared_signed_registry_valid(self):self.validate(FIXTURE['registry'])
    def test_identity_signature_architecture_and_duplicates_rejected(self):
        for field,value in [('architecture','arm64'),('version','0.0.0.2'),('sha256','f'*64),('product','mysoc')]:
            registry=copy.deepcopy(FIXTURE['registry']);registry['release'][field]=value
            with self.subTest(field=field),self.assertRaises(Exception):self.validate(registry)
        registry=copy.deepcopy(FIXTURE['registry']);registry['nodes'][1]['machine_id']=registry['nodes'][0]['machine_id']
        with self.assertRaises(ValueError):self.validate(registry)
        registry=copy.deepcopy(FIXTURE['registry']);registry['installation_id']='00000000-0000-0000-0000-000000000000'
        with self.assertRaises(ValueError):self.validate(registry)

if __name__=='__main__':unittest.main()
