import copy
from pathlib import Path
import sys
import unittest
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1'))
from host import canonical_data_identity
class DataIdentityTests(unittest.TestCase):
 def setUp(self):
  self.value={r:{'id':r,'image':'sha256:'+r,'mounts':[{'Destination':'/data','Type':'bind','Source':'/retained/'+r,'RW':True},{'Destination':'/secret','Type':'bind','Source':'/private/'+r,'RW':False}]} for r in ('postgres','redis')}
 def test_order_only_is_ignored(self):
  other=copy.deepcopy(self.value)
  for record in other.values():record['mounts'].reverse()
  self.assertEqual(canonical_data_identity(self.value),canonical_data_identity(other))
 def test_identity_and_mount_changes_remain_rejected(self):
  for key,new in [('id','replacement'),('image','changed')]:
   other=copy.deepcopy(self.value);other['postgres'][key]=new
   self.assertNotEqual(canonical_data_identity(self.value),canonical_data_identity(other))
  for key,new in [('Source','/different'),('Destination','/different'),('Type','volume'),('RW',False)]:
   other=copy.deepcopy(self.value);other['postgres']['mounts'][0][key]=new
   self.assertNotEqual(canonical_data_identity(self.value),canonical_data_identity(other))
  other=copy.deepcopy(self.value);other['postgres']['mounts'].pop()
  self.assertNotEqual(canonical_data_identity(self.value),canonical_data_identity(other))
 def test_duplicates_fail(self):
  self.value['postgres']['mounts'].append(self.value['postgres']['mounts'][0])
  with self.assertRaisesRegex(ValueError,'duplicate'):canonical_data_identity(self.value)
