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
