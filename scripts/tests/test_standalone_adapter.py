from datetime import datetime, timezone
from pathlib import Path
import tempfile
import unittest
from scripts.tests import test_standalone_transaction as fixtures
from adapter import Adapter
from transaction import digest


class CoordinatorTests(unittest.TestCase):
    setUp = fixtures.StandaloneAdmissionTests.setUp
    admission = fixtures.StandaloneAdmissionTests.admission
    def test_ambiguous_apply_recovers_same_operation_and_never_accepts_alive(self):
        test = self
        with tempfile.TemporaryDirectory() as root:
            directory = Path(root)/self.binding['operation_id']; directory.mkdir(mode=0o700)
            calls = []
            class Host:
                protected = staticmethod(lambda p:p)
                admit = staticmethod(lambda binding:test.admission())
                stage = staticmethod(lambda binding:{})
                def verify_accepted(self, binding): raise ValueError('full pipeline not ready')
                def persist_effective_mode(self, binding, health): raise AssertionError('must not commit')
            def worker(action, binding, bundles):
                calls.append(action)
                if len(calls) == 1: raise TimeoutError('lost response')
                return dict(protocol=binding['protocol'],operation_id=binding['operation_id'],operation_sha256=digest(binding),phase='accepted',mutation='confirmed',observed_at=datetime.now(timezone.utc).isoformat(),health={'status':'alive'})
            adapter=Adapter(directory,Host(),worker)
            with self.assertRaises(TimeoutError): adapter.run('apply',self.binding)
            with self.assertRaisesRegex(ValueError,'pipeline'): adapter.run('apply',self.binding)
            self.assertEqual(calls,['apply','recover'])
            self.assertIn('recovery_required',(directory/'adapter-journal.json').read_text())

    def test_health_precedes_effective_mode_commit(self):
        test=self
        with tempfile.TemporaryDirectory() as root:
            directory=Path(root)/self.binding['operation_id'];directory.mkdir(mode=0o700)
            calls=[]
            class Host:
                protected=staticmethod(lambda p:p)
                admit=staticmethod(lambda binding:test.admission())
                stage=staticmethod(lambda binding:{})
                def verify_accepted(self,binding):calls.append('verified');return {'fixture_qualified':True}
                def persist_effective_mode(self,binding,health):calls.append('committed')
            def worker(action,binding,bundles):
                return dict(protocol=binding['protocol'],operation_id=binding['operation_id'],operation_sha256=digest(binding),phase='accepted',mutation='confirmed',observed_at=datetime.now(timezone.utc).isoformat())
            result=Adapter(directory,Host(),worker).run('apply',self.binding)
            self.assertEqual(calls,['verified','committed'])
            self.assertEqual(result['phase'],'accepted')


if __name__=='__main__':unittest.main()
