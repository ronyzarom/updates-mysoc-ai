import json
from pathlib import Path
import unittest
import protocol as p

FIXTURE = json.loads((Path(__file__).parent/'registry-v1.fixture.json').read_text())

class ProtocolTests(unittest.TestCase):
    def request(self, action, generation=0, digest=None):
        return p.request(FIXTURE['registry'],FIXTURE['registry_sha256'],'1',action,generation,digest)

    def ack(self, sent, action):
        return dict(protocol=p.PROTOCOL,operation_id=sent['operation_id'],
                    registry_sha256=sent['registry_sha256'],generation=7,
                    phase=p.ACTIONS[action],processing_allowed=False)

    def test_supported_phases_never_grant_processing(self):
        for action,generation,digest in [('begin-or-resume',0,None),('status',0,None),
                ('status',7,None),('register',7,'a'*64),('all-registered',7,None)]:
            sent=self.request(action,generation,digest)
            self.assertFalse(p.validate_ack(sent,action,json.dumps(self.ack(sent,action)))['processing_allowed'])

    def test_response_identity_epoch_and_permission_refused(self):
        sent=self.request('register',7,'a'*64)
        for field,value in [('protocol','pod-maintenance-v1'),('operation_id','other'),
                ('registry_sha256','b'*64),('generation',8),('generation',0),
                ('generation',True),('processing_allowed',True),('processing_allowed',0),
                ('phase','ready')]:
            ack=self.ack(sent,'register');ack[field]=value
            with self.subTest(field=field,value=value),self.assertRaises(ValueError):
                p.validate_ack(sent,'register',json.dumps(ack))

    def test_partial_registration_is_not_all_registered(self):
        sent=self.request('all-registered',7)
        ack=self.ack(sent,'all-registered');ack['phase']='registered'
        with self.assertRaises(ValueError):p.validate_ack(sent,'all-registered',json.dumps(ack))

    def test_invalid_mutations_are_not_requests(self):
        for args in [('activate',7,None),('clear',7,None),('register',0,'a'*64),
                     ('register',7,None),('begin-or-resume',7,None),('status',0,'a'*64)]:
            with self.assertRaises(ValueError):self.request(*args)

    def test_duplicate_and_extra_response_fields_rejected(self):
        sent=self.request('status');raw=json.dumps(self.ack(sent,'status'))
        with self.assertRaises(ValueError):p.validate_ack(sent,'status',raw[:-1]+',"generation":7}')
        ack=self.ack(sent,'status');ack['activation']=True
        with self.assertRaises(ValueError):p.validate_ack(sent,'status',json.dumps(ack))
