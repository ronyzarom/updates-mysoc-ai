import hashlib
import json
import unittest
from unittest.mock import patch
import client
import observer_runner as r
import test_observer_stage

class ObserverRunnerTests(unittest.TestCase):
    def setUp(self):
        helper=test_observer_stage.ObserverTests();helper.setUp();self.addCleanup(helper.doCleanups)
        self.helper=helper;self.plan=helper.plan();self.calls=[]
        self.inputs={str(helper.root/'drain'):b'{}',str(helper.root/'update'):b'{}',
                     '/protected/receipt.json':json.dumps(helper.config).encode()}
        read=patch.object(client,'protected',side_effect=lambda path:self.inputs[str(path)]);read.start();self.addCleanup(read.stop)
    def test_exact_verified_inputs_call_only_health_command(self):
        with patch.object(r,'binary_digest',return_value='a'*64),patch.object(r.data_runner,'bounded',return_value=(0,b'{}')) as call:
            result=r.Runner(self.plan,lambda path:'a'*64)(self.plan['argv'])
            self.assertEqual(result,(0,b'{}'));call.assert_called_once_with(self.plan['argv'],60)
            self.assertIn('--health',call.call_args[0][0])
    def test_changed_binary_config_or_input_prevents_execution(self):
        cases=[('binary',None),('drain',str(self.helper.root/'drain')),('receipt','/protected/receipt.json')]
        for kind,path in cases:
            old=self.inputs.get(path)
            if path:self.inputs[path]=b'{"changed":true}'
            with self.subTest(kind=kind),patch.object(r,'binary_digest',return_value=('b'*64 if kind=='binary' else 'a'*64)),patch.object(r.data_runner,'bounded') as call,self.assertRaises(ValueError):
                r.Runner(self.plan,lambda path:'a'*64)(self.plan['argv'])
            call.assert_not_called()
            if path:self.inputs[path]=old
    def test_post_execution_binary_change_rejects_receipt(self):
        with patch.object(r,'binary_digest',side_effect=['a'*64,'b'*64]),patch.object(r.data_runner,'bounded',return_value=(0,b'{}')),self.assertRaises(ValueError):
            r.Runner(self.plan,lambda path:'a'*64)(self.plan['argv'])
    def test_verifier_rejection_or_argv_change_never_launches(self):
        def reject(path):raise ValueError('unverified artifact')
        with patch.object(r.data_runner,'bounded') as call:
            with self.assertRaises(ValueError):r.Runner(self.plan,reject)(self.plan['argv'])
            with self.assertRaises(ValueError):r.Runner(self.plan,reject)(['/bin/true'])
            call.assert_not_called()
