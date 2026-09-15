import importlib.util
from pathlib import Path
import unittest

path = Path(__file__).resolve().parents[1] / 'ops/prepare_bezeq_b_redis_recovery.py'
spec = importlib.util.spec_from_file_location('bezeq_recovery', path)
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


class RedisRecoveryTests(unittest.TestCase):
    original = (b'# keep comments\nrequirepass test-only-secret\nreplicaof redis-a 6379\n'
                b'min-replicas-to-write 1\nappendonly yes\n')

    def test_only_approved_authority_and_write_policy_change(self):
        self.assertEqual(recovery.amended(self.original),
                         b'# keep comments\nrequirepass test-only-secret\n'
                         b'min-replicas-to-write 0\nappendonly yes\n')

    def test_unreviewed_configs_fail_closed(self):
        for bad in [self.original + b'replicaof redis-a 6379\n',
                    self.original.replace(b'redis-a', b'redis-c'),
                    self.original.replace(b'write 1', b'write 0'),
                    self.original.replace(b'min-replicas-to-write 1\n', b'')]:
            with self.subTest(config=bad), self.assertRaises(ValueError):
                recovery.amended(bad)


if __name__ == '__main__':
    unittest.main()
