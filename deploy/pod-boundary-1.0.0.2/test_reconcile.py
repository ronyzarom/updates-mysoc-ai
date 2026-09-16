import contextlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import boundary as b
import reconcile as r


class ReconcileTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = {'role':'b','public_key':r.KEY,'phase':'compensating','pending_unit':r.COMPENSATE}
        for name, ref in [('candidate',r.CANDIDATE),('previous',r.PREVIOUS)]:
            self.state[name] = {'version':ref[0], 'sha256':ref[1], 'signature':'fixture',
                                'archive':r.TX+'/'+name+'/release.tar.gz'}
        self.policy = {'public_key':r.KEY}
        self.app = {'updater_instance_id':r.UPDATER,'topology':'pod','pod_role':'b','cluster_id':'bezeq-pod-test'}
        class Old:
            CURRENT = self.root/'managed/current'
            def completed_greenfield(inner):return True
            def receipt(inner, path, policy):return r.PREVIOUS[0], {'sha256':r.PREVIOUS[1]}
        self.boundary = b.Boundary(Old(), self.root)
        b.atomic(self.boundary.journal, self.state)
        self.original = self.boundary.journal.read_bytes()
        self.verified = []
        @contextlib.contextmanager
        def bundle(ref, key):
            self.verified.append(ref['version']); yield self.root
        for target, value in [('trusted',lambda *a:None)]:
            p=patch.object(b,target,value);p.start();self.addCleanup(p.stop)
        for target, value in [('no_product_transaction',lambda:None),('verify_evidence',lambda boundary:{'fixture':'digest'})]:
            p=patch.object(r,target,value);p.start();self.addCleanup(p.stop)
        p=patch.object(self.boundary,'bundle',bundle);p.start();self.addCleanup(p.stop)

    def run_reconcile(self):return r.reconcile(self.boundary,self.policy,self.app,r.MACHINE)

    def test_exact_incident_preserves_evidence_without_execution(self):
        with patch.object(self.boundary,'invoke') as invoke:
            result=self.run_reconcile();invoke.assert_not_called()
        self.assertEqual(self.verified,[r.CANDIDATE[0],r.PREVIOUS[0]])
        self.assertEqual(result['status'],'preflight-refused')
        self.assertFalse(result['rollback_dispatched']);self.assertFalse(result['health_verified'])
        saved=json.loads((self.root/(r.TX+'.preflight-evidence.json')).read_text())
        self.assertEqual(saved['original_journal'],json.loads(self.original))
        self.assertNotIn('pending_unit',self.boundary.read())
        self.assertLess(len(json.dumps(result)),4096)

    def test_mismatches_leave_journal_unchanged(self):
        for mutate in [lambda s:s.update(role='a'), lambda s:s.update(phase='healthy'),
                       lambda s:s.update(pending_unit=r.APPLY),
                       lambda s:s['candidate'].update(sha256='0'*64),
                       lambda s:s['previous'].update(archive='other/previous/release.tar.gz')]:
            with self.subTest(mutate=mutate):
                state=json.loads(self.original);mutate(state);b.atomic(self.boundary.journal,state)
                before=self.boundary.journal.read_bytes()
                with self.assertRaises(ValueError):self.run_reconcile()
                self.assertEqual(before,self.boundary.journal.read_bytes())

    def test_missing_transaction_not_sufficient(self):
        with patch.object(r,'verify_evidence',side_effect=ValueError('log mismatch')):
            with self.assertRaises(ValueError):self.run_reconcile()
        self.assertEqual(self.original,self.boundary.journal.read_bytes())

    def test_signature_failure_blocks_reconciliation(self):
        with patch.object(self.boundary,'bundle',side_effect=ValueError('signature')):
            with self.assertRaises(ValueError):self.run_reconcile()
        self.assertEqual(self.original,self.boundary.journal.read_bytes())

    def test_new_product_transaction_blocks(self):
        with patch.object(r,'no_product_transaction',side_effect=[None,ValueError('appeared')]):
            with self.assertRaises(ValueError):self.run_reconcile()
        self.assertEqual(self.original,self.boundary.journal.read_bytes())

    def test_retry_does_not_rewrite_terminal(self):
        self.run_reconcile();before=self.boundary.journal.read_bytes()
        with self.assertRaises(ValueError):self.run_reconcile()
        self.assertEqual(before,self.boundary.journal.read_bytes())

    def test_identity_replay_rejected(self):
        with self.assertRaises(ValueError):r.reconcile(self.boundary,self.policy,self.app,'other')
        self.app['updater_instance_id']='other'
        with self.assertRaises(ValueError):self.run_reconcile()

    def test_generic_failure_diagnostics_do_not_include_exception(self):
        # Exercise real invoke with only artifact/systemd adapters stubbed.
        p=self.root/'updater';p.mkdir();(p/'apply').write_text('fixture');(p/'apply').chmod(0o700)
        with patch.object(self.boundary.units,'run',side_effect=RuntimeError('SECRET_TOKEN_CANARY')):
            with self.assertRaises(RuntimeError):self.boundary.invoke(self.state,self.state['candidate'],'apply')
        saved=self.boundary.read()
        self.assertNotIn('SECRET_TOKEN_CANARY',json.dumps(saved))
        self.assertEqual(saved['phase_failures'][0]['reason_code'],'supervised_phase_failed')


class QuiescenceTests(unittest.TestCase):
    def test_active_unit_refused(self):
        class Result:stdout='LoadState=loaded\nActiveState=active\nControlGroup=\n'
        with patch.object(r.subprocess,'run',return_value=Result()):
            with self.assertRaises(ValueError):r.quiescent(r.APPLY)

    def test_populated_failed_cgroup_refused(self):
        class Result:stdout='LoadState=loaded\nActiveState=failed\nControlGroup=/system.slice/'+r.APPLY+'\n'
        with patch.object(r.subprocess,'run',return_value=Result()),patch.object(Path,'exists',return_value=True),patch.object(Path,'read_text',return_value='populated 1\nfrozen 0\n'):
            with self.assertRaises(ValueError):r.quiescent(r.APPLY)

if __name__ == '__main__':unittest.main()

class EvidenceTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name)
        class Old:CURRENT=self.root/'managed/current'
        self.boundary=b.Boundary(Old(),self.root)
        logs={}
        for unit,action,entry in [(r.APPLY,'apply','apply'),(r.COMPENSATE,'rollback','compensate')]:
            bundle=str(self.root)+'/verified-fixture/unpacked/siemcore-universal-3.3.152.33'
            env={'PATH':'/usr/sbin:/usr/bin:/sbin:/bin','HOME':'/root','LANG':'C.UTF-8',
                 'UPDATER_PHASE':action,'PRODUCT':'siemcore','VERSION':r.CANDIDATE[0],
                 'SIEMCORE_POD_RECOVERY_PROTOCOL':'1','CURRENT_DIR':bundle,
                 'INSTALL_ROOT':str(Old.CURRENT.parents[1]),'SIEMCORE_INSTALL_DIR':'/opt/siemcore-app-app-b'}
            b.atomic(self.root/(unit+'.request.json'),{'unit':unit,'args':[bundle+'/updater/'+entry,action],'env':env})
            for suffix in ('.stdout','.stdout.stderr'):
                raw=b'' if suffix=='.stdout' else b'fixed reviewed failure fixture'
                (self.root/(unit+suffix)).write_bytes(raw);logs[unit+suffix]=r.digest(raw)
        b.atomic(self.root/'incident.json',{'logs':logs})
        for p in [patch.object(b,'trusted',lambda *a:None),patch.object(r,'__file__',str(self.root/'reconcile.py')),patch.object(r,'quiescent')]:
            p.start();self.addCleanup(p.stop)

    def test_complete_evidence_passes(self):
        self.assertEqual(len(r.verify_evidence(self.boundary)),6)

    def test_changed_log_rejected(self):
        (self.root/(r.APPLY+'.stdout.stderr')).write_text('different failure')
        with self.assertRaises(ValueError):r.verify_evidence(self.boundary)

    def test_extra_candidate_execution_rejected(self):
        data=json.loads((self.root/(r.APPLY+'.request.json')).read_text());data['unit']='other'
        b.atomic(self.root/'extra.request.json',data)
        with self.assertRaises(ValueError):r.verify_evidence(self.boundary)

    def test_injected_environment_rejected(self):
        p=self.root/(r.APPLY+'.request.json');data=json.loads(p.read_text());data['env']['PYTHONPATH']='/untrusted';b.atomic(p,data)
        with self.assertRaises(ValueError):r.verify_evidence(self.boundary)

    def test_oversized_log_rejected(self):
        (self.root/(r.APPLY+'.stdout')).write_bytes(b'x'*65537)
        with self.assertRaises(ValueError):r.verify_evidence(self.boundary)

    def test_quiescence_failure_rejected(self):
        with patch.object(r,'quiescent',side_effect=ValueError('active')):
            with self.assertRaises(ValueError):r.verify_evidence(self.boundary)
