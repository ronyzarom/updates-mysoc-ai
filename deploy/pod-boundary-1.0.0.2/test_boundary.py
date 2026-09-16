import contextlib, hashlib, importlib.util, json, pathlib, tempfile, unittest
from unittest.mock import patch
import boundary as b

class Legacy:
    def __init__(self,root):
        self.CURRENT=root/'managed'/'siemcore'/'current';self.CURRENT.parent.mkdir(parents=True)
        self.releases=self.CURRENT.parent/'releases';self.releases.mkdir()
        for v in ('3.3.152.30','3.3.152.32'):
            d=self.releases/v;d.mkdir();(d/'archive').write_text(v)
        self.CURRENT.symlink_to(self.releases/'3.3.152.32')
        (self.CURRENT.parent/'.previous').write_text(str(self.releases/'3.3.152.30'))
    def release_path(self,p):return p.resolve(strict=True)
    def receipt(self,p,policy):
        p=self.release_path(p)
        return p.name,{'artifact':str(p/'archive'),'sha256':hashlib.sha256((p/'archive').read_bytes()).hexdigest(),'signature':'fixture'}
    def verified_bundle(self,entry,version,key,d):
        raw=pathlib.Path(entry['artifact']).read_bytes()
        if hashlib.sha256(raw).hexdigest()!=entry['sha256']:raise ValueError('tampered retained archive')
        (d/'release.tar.gz').write_bytes(raw)
        p=d/('unpacked-'+version);(p/'updater').mkdir(parents=True)
        for name in ('apply','compensate'):
            e=p/'updater'/name;e.write_text('#!/bin/sh\n');e.chmod(0o700)
        return p

class Units:
    def __init__(self):self.calls=[];self.failure=None;self.badreceipt=False;self.quiescence=True
    def quiesce(self,unit):
        self.calls.append('quiesce')
        if not self.quiescence:raise RuntimeError('unit still running')
    def run(self,unit,args,env,output,timeout):
        kind='compensate' if args[0].name=='compensate' else args[1]
        self.calls.append((env['VERSION'],kind))
        if kind==self.failure:raise RuntimeError('injected '+kind)
        value={'status':'paused','product':'siemcore','version':env['VERSION']}
        return b'bad receipt' if self.badreceipt else (b.PREFIX+json.dumps(value)+'\n').encode()

class Tests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.root=pathlib.Path(self.tmp.name);self.store=self.root/'store';self.store.mkdir()
        self.old=Legacy(self.root);self.units=Units();self.boundary=b.Boundary(self.old,self.store,self.units)
        self.policy={'public_key':'fixture-key'};self.app={'pod_role':'a'}
        self.trust=patch.object(b,'trusted',lambda *args:None);self.trust.start();self.addCleanup(self.trust.stop)
    def call(self,phase):return self.boundary.dispatch(self.policy,self.app,phase)
    def previous(self):self.old.CURRENT.unlink();self.old.CURRENT.symlink_to(self.old.releases/'3.3.152.30')
    def test_post_apply_health_failure_compensates_before_previous(self):
        self.call('apply');self.units.failure='health'
        with self.assertRaises(RuntimeError):self.call('health')
        self.units.failure=None;self.previous();self.call('rollback')
        self.assertEqual(self.units.calls[-2:],[('3.3.152.32','compensate'),('3.3.152.30','rollback')])
    def test_product_apply_failure_also_compensates(self):
        self.units.failure='apply'
        with self.assertRaises(RuntimeError):self.call('apply')
        self.previous();self.units.failure=None;self.call('rollback')
        self.assertEqual(self.boundary.read()['phase'],'rolled-back')
    def test_bad_receipt_blocks_previous(self):
        self.call('apply');self.previous();self.units.badreceipt=True
        with self.assertRaises(ValueError):self.call('rollback')
        self.assertNotIn(('3.3.152.30','rollback'),self.units.calls)
    def test_retained_copy_survives_untrusted_cache_changes(self):
        self.call('apply');(self.old.releases/'3.3.152.32'/'archive').write_text('attacker replacement')
        self.previous();self.call('rollback')
        self.assertIn(('3.3.152.32','compensate'),self.units.calls)
    def test_pending_unit_must_stop_before_compensation(self):
        self.units.failure='apply'
        with self.assertRaises(RuntimeError):self.call('apply')
        self.previous();self.units.quiescence=False
        with self.assertRaises(RuntimeError):self.call('rollback')
        self.assertNotIn(('3.3.152.32','compensate'),self.units.calls)
    def test_compensation_command_failure_blocks_previous(self):
        self.call('apply');self.previous();self.units.failure='compensate'
        with self.assertRaises(RuntimeError):self.call('rollback')
        self.assertNotIn(('3.3.152.30','rollback'),self.units.calls)
    def test_repeated_quiescence_failure_stays_blocked(self):
        self.units.failure='apply'
        with self.assertRaises(RuntimeError):self.call('apply')
        self.previous();self.units.quiescence=False
        for _ in range(2):
            with self.assertRaises(RuntimeError):self.call('rollback')
        self.assertIn('draining_unit',self.boundary.read())
        self.assertNotIn(('3.3.152.30','rollback'),self.units.calls)
    def test_archive_tamper_refused(self):
        self.call('apply');s=self.boundary.read();archive=self.store/s['candidate']['archive'];archive.chmod(0o600);archive.write_text('corruption')
        self.previous()
        with self.assertRaises(ValueError):self.call('rollback')
        self.assertNotIn(('3.3.152.30','rollback'),self.units.calls)
    def test_success_keeps_candidate_for_later_rollback(self):
        self.call('apply');self.call('health');self.previous();self.call('rollback')
        self.assertEqual(self.boundary.read()['phase'],'rolled-back')
        count=len(self.units.calls);self.call('rollback');self.assertEqual(count,len(self.units.calls))
    def test_superseded_worker_refuses_execution(self):
        unit='siemcore-pod-boundary-'+'a'*32+'.service'
        b.atomic(self.store/'transaction.json',{'pending_unit':'different'})
        with patch.object(b,'ROOT',self.store),patch.object(b.os,'geteuid',return_value=0),patch.object(b.subprocess,'run') as run:
            with self.assertRaises(ValueError):b.worker(unit)
            run.assert_not_called()

if __name__=='__main__':unittest.main()
