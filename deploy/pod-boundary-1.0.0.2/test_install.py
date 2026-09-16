import contextlib
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import boundary as b
import reconcile as r
import install as installer

class InstallTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name);self.package=self.root/'package';self.package.mkdir()
        self.old=self.root/'old.py';self.old.write_text('old boundary')
        self.wrapper=self.root/'wrapper';self.wrapper.write_bytes(installer.OLD_WRAPPER)
        for name in installer.FILES|{'install.py'}:(self.package/name).write_text(name)
        manifest={name:r.digest((self.package/name).read_bytes()) for name in installer.FILES|{'install.py'}}
        b.atomic(self.package/'files.json',manifest)
        self.state={'phase':'compensating'}
        class Old:
            CURRENT=self.root/'current'
            def receipt(inner,path,policy):return r.PREVIOUS[0],{'sha256':r.PREVIOUS[1]}
        class Boundary:
            old=Old()
            def read(inner):return self.state
            @contextlib.contextmanager
            def bundle(inner,*a):yield self.root
        self.boundary=Boundary()
        self.state.update(candidate={},previous={})
        for p in [patch.object(b,'trusted',lambda *a:None),patch.object(b,'INSTALLED',self.root/'installed/boundary.py'),
                  patch.object(installer,'OLD',self.old),patch.object(installer,'OLD_SHA',r.digest(self.old.read_bytes())),
                  patch.object(installer,'WRAPPER',self.wrapper),patch.object(r,'identity'),patch.object(r,'validate_terminal'),patch.object(r,'verify_evidence')]:
            p.start();self.addCleanup(p.stop)
        self.reconcile=patch.object(r,'reconcile',side_effect=lambda *a:self.state.update(phase='preflight-refused'))
        self.reconcile_mock=self.reconcile.start();self.addCleanup(self.reconcile.stop)

    def activate(self):return installer.activate(self.package,self.boundary,{}, {},r.MACHINE)

    def test_installs_without_product_execution(self):
        result=self.activate()
        self.assertEqual(self.wrapper.read_bytes(),installer.NEW_WRAPPER)
        self.assertEqual(self.wrapper.stat().st_mode&0o777,0o755)
        self.assertFalse(result['health_verified'])
        self.assertEqual(self.state['phase'],'preflight-refused')

    def test_crash_after_reconciliation_resumes(self):
        real=installer.write_file
        def fail(path,*args):
            if path==self.wrapper:raise OSError('injected crash')
            return real(path,*args)
        with patch.object(installer,'write_file',side_effect=fail):
            with self.assertRaises(OSError):self.activate()
        self.assertEqual(self.wrapper.read_bytes(),installer.OLD_WRAPPER)
        self.assertEqual(self.state['phase'],'preflight-refused')
        self.activate()
        self.assertEqual(self.reconcile_mock.call_count,1)
        self.assertEqual(self.wrapper.read_bytes(),installer.NEW_WRAPPER)

    def test_tamper_never_reconciles(self):
        (self.package/'incident.json').write_text('tampered')
        with self.assertRaises(ValueError):self.activate()
        self.reconcile_mock.assert_not_called()
        self.assertEqual(self.wrapper.read_bytes(),installer.OLD_WRAPPER)

    def test_preexisting_version_drift_rejected(self):
        target=b.INSTALLED;target.parent.mkdir();target.write_text('wrong')
        with self.assertRaises(ValueError):self.activate()
        self.reconcile_mock.assert_not_called()

    def test_reconciliation_failure_leaves_wrapper_old(self):
        self.reconcile_mock.side_effect=ValueError('evidence mismatch')
        with self.assertRaises(ValueError):self.activate()
        self.assertEqual(self.wrapper.read_bytes(),installer.OLD_WRAPPER)
