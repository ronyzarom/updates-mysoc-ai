import json
from pathlib import Path
import unittest
import authorization as a

FIXTURE=json.loads((Path(__file__).parent/'registry-v1.fixture.json').read_text())
EXPIRY='2026-09-17T16:30:00.123456789Z'

class AuthorizationTests(unittest.TestCase):
    def sent(self,action='authorization-status',invitation=None):
        return a.request(FIXTURE['registry'],FIXTURE['registry_sha256'],'1',action,7,'a'*64,invitation)
    def ack(self):
        sent=self.sent()
        return dict(protocol=sent['protocol'],operation_id=sent['operation_id'],registry_sha256=sent['registry_sha256'],
                    generation=7,processing_allowed=False,phase='authorized',authorization_id='fixture-auth-1',expires_at=EXPIRY)
    def check(self,ack,action='authorization-status'):
        return a.validate_ack(self.sent(),action,json.dumps(ack),'fixture-auth-1',EXPIRY)
    def test_exact_status_can_reconcile_but_never_grants_processing(self):
        self.assertIs(self.check(self.ack())['processing_allowed'],False)
    def test_expired_status_is_not_authorize_success(self):
        ack=self.ack();ack['phase']='authorization-expired'
        self.assertEqual(self.check(ack)['phase'],'authorization-expired')
        with self.assertRaises(ValueError):self.check(ack,'authorize')
    def test_newer_invitation_or_changed_binding_is_conflict(self):
        for field,value in [('authorization_id','fixture-auth-2'),('expires_at','2026-09-17T16:30:00.123456788Z'),
                ('generation',8),('operation_id','other'),('registry_sha256','b'*64),('processing_allowed',True)]:
            ack=self.ack();ack[field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):self.check(ack)
    def test_request_shape_preserves_exact_generation_and_input(self):
        sent=self.sent('authorize',{'payload_base64':'e30=','signature':'fixture'})
        self.assertEqual(sent['generation'],7);self.assertEqual(sent['input_sha256'],'a'*64)
        with self.assertRaises(ValueError):self.sent('authorize')
        with self.assertRaises(ValueError):self.sent('authorization-status',{'payload_base64':'e30=','signature':'fixture'})
    def test_timestamp_preserves_nanosecond_precision_and_requires_utc(self):
        self.assertEqual(a.timestamp('2026-09-17T16:30:00Z'),a.timestamp('2026-09-17T16:30:00.000000000Z'))
        for value in ('2026-09-17T16:30:00+00:00','2026-09-17T16:30:00.1234567890Z','2026-02-30T00:00:00Z'):
            with self.assertRaises(ValueError):a.timestamp(value)
    def test_extra_and_duplicate_fields_rejected(self):
        ack=self.ack();ack['execute']=True
        with self.assertRaises(ValueError):self.check(ack)
        raw=json.dumps(self.ack())[:-1]+',"authorization_id":"fixture-auth-1"}'
        with self.assertRaises(ValueError):a.validate_ack(self.sent(),'authorization-status',raw,'fixture-auth-1',EXPIRY)

class IntegrationTests(unittest.TestCase):
    def setUp(self):
        import base64
        import tempfile
        from unittest.mock import patch
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        from cryptography.hazmat.primitives import serialization
        import client
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.path=Path(self.temp.name)/'auth.json'
        self.reader=patch.object(client,'protected',side_effect=lambda path:Path(path).read_bytes())
        self.reader.start();self.addCleanup(self.reader.stop)
        self.key=Ed25519PrivateKey.generate()
        self.pin=self.key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw)
        self.claims=dict(protocol='pod-bootstrap-invitation-v1',authorization_id='fixture-auth-1',
            operation_id=FIXTURE['registry']['operation_id'],registry_sha256=FIXTURE['registry_sha256'],
            node_id='1',input_sha256='a'*64,issued_at='2026-09-17T16:00:00Z',expires_at='2026-09-17T16:30:00Z')
        self.now=int(a.timestamp('2026-09-17T16:10:00Z')[0].timestamp())*10**9
        self.calls=[];self.lose=False;self.wrong=False
        class Transport:
            def call(_,action,sent,timeout):
                self.calls.append(action)
                if action=='authorize' and self.lose:raise ConnectionResetError()
                return json.dumps(dict(protocol=sent['protocol'],operation_id=sent['operation_id'],registry_sha256=sent['registry_sha256'],
                    generation=7,phase='authorized',processing_allowed=False,
                    authorization_id='newer-id' if self.wrong else self.claims['authorization_id'],expires_at=self.claims['expires_at']))
        self.registration=client.Coordinator(FIXTURE['registry'],FIXTURE['registry_sha256'],'1','a'*64,
            Transport(),Path(self.temp.name)/'registration.json',{},timeout=10)
    def invitation(self):
        import base64
        raw=json.dumps(self.claims,separators=(',',':')).encode()
        return dict(payload_base64=base64.b64encode(raw).decode(),
            signature=base64.b64encode(self.key.sign(b'mysoc-pod-bootstrap-invitation-v1\n'+raw)).decode())
    def run_client(self):
        return a.Coordinator(self.registration,self.invitation(),self.pin,self.path,lambda:self.now).run_locked(7)
    def test_lost_authorize_response_reconciles_status_without_resubmission(self):
        self.lose=True;result=self.run_client()
        self.assertEqual(self.calls,['authorize','authorization-status'])
        self.assertIs(result['installation_complete'],False)
    def test_restart_uses_status_only(self):
        self.run_client();self.calls=[];self.run_client()
        self.assertEqual(self.calls,['authorization-status'])
    def test_newer_server_invitation_is_not_success(self):
        self.lose=True;self.wrong=True
        with self.assertRaises(ValueError):self.run_client()
        self.assertEqual(json.loads(self.path.read_text())['phase'],'intent')
    def test_expired_invitation_never_mutates(self):
        self.now+=3600*10**9
        with self.assertRaises(ValueError):self.run_client()
        self.assertEqual(self.calls,[])
    def test_tampered_signature_never_mutates(self):
        invite=self.invitation();invite['signature']='AA=='
        with self.assertRaises(Exception):
            a.Coordinator(self.registration,invite,self.pin,self.path,lambda:self.now).run_locked(7)
        self.assertEqual(self.calls,[])

    def test_expired_existing_operation_can_inspect_status_without_mutation(self):
        self.run_client();self.calls=[];self.now+=3600*10**9
        self.run_client()
        self.assertEqual(self.calls,['authorization-status'])
