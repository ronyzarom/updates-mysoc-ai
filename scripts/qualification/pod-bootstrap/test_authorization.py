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
