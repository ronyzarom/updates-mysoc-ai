import tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import provision_osconfig as p
class Files(unittest.TestCase):
 def test_symlink_outputs_rejected_without_target_write(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);target=root/'target';target.write_text('keep');link=root/'sig';link.symlink_to(target)
   with patch.object(p,'protected',lambda *a:None):
    with self.assertRaises(ValueError):p.write_private(link,b'new')
   self.assertEqual(target.read_text(),'keep')
 def test_directory_output_rejected(self):
  with tempfile.TemporaryDirectory() as d,patch.object(p,'protected',lambda *a:None):
   with self.assertRaises(ValueError):p.write_private(Path(d),b'new')
 def test_private_atomic_repeat(self):
  with tempfile.TemporaryDirectory() as d,patch.object(p,'protected',lambda *a:None):
   f=Path(d)/'message';p.write_private(f,b'one');p.write_private(f,b'two')
   self.assertEqual(f.read_bytes(),b'two');self.assertEqual(f.stat().st_mode&0o777,0o600)

 def test_busy_lock_retries_without_stopping_service(self):
  with patch.object(p.fcntl,'flock',side_effect=[BlockingIOError(),None]) as lock,patch.object(p.time,'sleep'),patch.object(p.subprocess,'run') as run:
   p.acquire_cycle_lock(123)
   self.assertEqual(lock.call_count,2);run.assert_not_called()
 def test_busy_lock_deadline_refuses(self):
  with patch.object(p.fcntl,'flock',side_effect=BlockingIOError()),patch.object(p.time,'monotonic',side_effect=[0,46]),patch.object(p.time,'sleep') as sleep:
   with self.assertRaises(BlockingIOError):p.acquire_cycle_lock(123)
   sleep.assert_not_called()
