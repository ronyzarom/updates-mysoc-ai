import fcntl
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest
from scripts.tests.test_node_update_adapter import binding,protocol,SOURCE,load

saved=sys.modules.get('protocol');sys.modules['protocol']=protocol
worker=load('node_update_worker','worker.py')
if saved is None:del sys.modules['protocol']
else:sys.modules['protocol']=saved

class TransportTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.root=Path(self.temp.name);self.bundle=self.root/'target';(self.bundle/'updater').mkdir(parents=True)
        self.binding=binding();self.lock=(self.root/'lock').open('w');self.addCleanup(self.lock.close)
        fcntl.flock(self.lock,fcntl.LOCK_EX)
        self.bundles=dict(target_bundle=str(self.bundle),predecessor_bundle=str(self.root/'previous'))

    def run_worker(self,code,timeout=3):
        (self.bundle/'updater/pod_node_update.py').write_text(code)
        return worker.invoke('status',self.binding,self.bundles,self.root,self.lock.fileno(),timeout)

    def test_exact_stdin_and_response(self):
        result=self.run_worker("import json,sys\nq=json.load(sys.stdin)\nassert q['protocol']=='pod-node-update-v1'\nassert q['action']=='status'\nassert q['application_policy']=='/etc/siemcore/greenfield.json'\nprint(json.dumps({'operation_sha256':q['operation_sha256']}))\n")
        self.assertEqual(result,{'operation_sha256':protocol.digest(self.binding)})

    def test_stderr_is_never_in_public_error(self):
        with self.assertRaisesRegex(ValueError,'uncertain') as caught:
            self.run_worker("import sys\nsys.stderr.write('SECRET-FIXTURE-NOT-A-CREDENTIAL')\nsys.exit(1)\n")
        self.assertNotIn('SECRET',str(caught.exception))

    def test_output_limit_and_duplicate_field(self):
        for code in ["print('x'*70000)","print('{\"a\":1,\"a\":2}')"]:
            with self.assertRaises(ValueError):self.run_worker(code)

    def test_timeout_stops_worker_group(self):
        marker=self.root/'should-not-exist'
        code="import subprocess,time\nsubprocess.Popen(['/usr/bin/python3','-c',"+repr("import time,pathlib;time.sleep(2);pathlib.Path("+repr(str(marker))+").write_text('leaked')")+"])\ntime.sleep(20)\n"
        with self.assertRaises(ValueError):self.run_worker(code,timeout=1)
        time.sleep(2)
        self.assertFalse(marker.exists(),'worker descendant escaped supervision')

    def test_child_cannot_retain_root_lock_after_success(self):
        result=self.run_worker("import json\nprint(json.dumps({'ok':True}))\n")
        self.assertTrue(result['ok'])
        fcntl.flock(self.lock,fcntl.LOCK_UN)
        with (self.root/'lock').open() as other:
            fcntl.flock(other,fcntl.LOCK_EX|fcntl.LOCK_NB)

if __name__=='__main__':unittest.main()
