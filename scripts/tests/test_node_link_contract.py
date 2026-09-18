import copy
import json
from pathlib import Path
import sys
import unittest
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'deploy/node-link-v1'))
import product_contract as contract

def request():
    identity = dict(machine_id='a'*32, installation_id='install-a', updater_instance_id='updater-a', node_id='1')
    b = dict(protocol=contract.PROTOCOL, operation_id='5b3e531d-0bbb-44bd-b639-5b872c735f06', source=identity,
        source_version='3.3.152.41', source_artifact_sha256='a'*64, bootstrap_receipt_sha256='b'*64,
        target=dict(product='siemcore', version='3.3.152.42', architecture='linux/amd64', source_commit='c'*40, artifact_sha256='d'*64,binary_sha256='e'*64),
        pod=dict(pod_id='test', instance_id='siemcore-test',customer_url='https://pod.example.com'),
        observer=dict(installation_id='observer',endpoint='https://observer.example.com',registry_sha256='f'*64,generation=1),
        peer=dict(machine_id='b'*32,installation_id='install-b',updater_instance_id='updater-b',node_id='2'),
        adoption_plan_sha256='1'*64,expected_state='linked-paused')
    directory=contract.ROOT+b['operation_id']
    return dict(protocol=contract.PROTOCOL,action='readiness',binding=b,operation_sha256=contract.binding_digest(b),operation_directory=directory,target_bundle='/verified/target',adoption_plan=directory+'/adoption-plan.json')

class Admission(unittest.TestCase):
    def test_valid(self):
        for action in ('readiness','apply','status','recover'):
            q=request();q['action']=action
            self.assertEqual(contract.parse_request(json.dumps(q)), q)
    def reject(self, mutate):
        q=request();mutate(q);q['operation_sha256']=contract.binding_digest(q['binding'])
        with self.assertRaises(ValueError):contract.parse_request(json.dumps(q))
    def test_identity_conflicts(self):
        for key in request()['binding']['source']:
            self.reject(lambda q:q['binding']['peer'].update({key:q['binding']['source'][key]}))
    def test_observer_is_separate_installation(self):
        for who in ('source','peer'):
            self.reject(lambda q:q['binding']['observer'].update(installation_id=q['binding'][who]['installation_id']))
    def test_processing_cannot_be_granted(self):
        self.reject(lambda q:q['binding'].update(expected_state='active'))
        self.reject(lambda q:q['binding'].update(processing_allowed=True))
    def test_unknown_fields_and_generation(self):
        self.reject(lambda q:q.update(credential='secret'))
        for value in (True,0,-1,2**63,'1'):
            self.reject(lambda q:q['binding']['observer'].update(generation=value))
    def test_endpoint_and_path_injection(self):
        for url in ('http://observer.example.com','https://user:password@observer.example.com','https://observer.example.com/?token=secret','https://pod.example.com','https://POD.example.com','https://pod.example.com:443/','https://pod.example.com./'):
            self.reject(lambda q:q['binding']['observer'].update(endpoint=url))
        self.reject(lambda q:q.update(adoption_plan='/tmp/foreign'))
        self.reject(lambda q:q.update(target_bundle='/verified/../foreign'))
    def test_replay_binding_and_duplicates(self):
        q=request();q['binding']['observer']['generation']=2
        with self.assertRaises(ValueError):contract.parse_request(json.dumps(q))
        raw=json.dumps(request()).replace('"action": "readiness"','"action": "readiness", "action": "apply"')
        with self.assertRaises(ValueError):contract.parse_request(raw)
    def test_downgrade(self):
        self.reject(lambda q:q['binding']['target'].update(version='3.3.152.40'))

if __name__=='__main__':unittest.main()
