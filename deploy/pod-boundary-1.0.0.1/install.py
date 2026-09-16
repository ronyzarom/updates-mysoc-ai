#!/usr/bin/env python3
"""Qualified-package provisioning only. Does not stop/start services or reboot."""
import argparse,hashlib,json,os,pathlib,subprocess
import boundary as b
p=argparse.ArgumentParser();p.add_argument('--expected-machine-id',required=True);p.add_argument('--expected-updater-id',required=True);a=p.parse_args()
if os.geteuid()!=0:raise ValueError('root required')
base=pathlib.Path(__file__).resolve().parent;b.trusted(base)
if pathlib.Path('/etc/machine-id').read_text().strip()!=a.expected_machine_id:raise ValueError('wrong machine')
application=pathlib.Path('/etc/siemcore/greenfield.json');b.trusted(application,True);data=json.loads(application.read_text())
if data.get('topology')!='pod' or data.get('pod_role') not in ('a','b') or data.get('updater_instance_id')!=a.expected_updater_id:raise ValueError('wrong pod identity')
b.trusted(b.LEGACY)
if hashlib.sha256(b.LEGACY.read_bytes()).hexdigest()!=b.LEGACY_SHA:raise ValueError('legacy boundary drift')
state=subprocess.check_output(['systemctl','show','siemcore-cascade-updater.service','-p','ActiveState','--value'],text=True).strip()
if state!='inactive':raise ValueError('updater must already be held/inactive through approved management')
if (b.ROOT/'transaction.json').exists():raise ValueError('existing transaction requires explicit reconciliation')
manifest=json.loads((base/'files.json').read_text())
for name,digest in manifest.items():
 if '/' in name or hashlib.sha256((base/name).read_bytes()).hexdigest()!=digest:raise ValueError('package integrity mismatch')
wrapper=pathlib.Path('/usr/local/sbin/siemcore-apply-update');b.trusted(wrapper)
expected=b'#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 /usr/local/lib/siemcore-cascade/greenfield-hook.py "$@"\n'
if wrapper.read_bytes() not in (expected,(base/'wrapper').read_bytes()):raise ValueError('wrapper drift')
b.INSTALLED.parent.mkdir(parents=True,mode=0o755,exist_ok=True);b.trusted(b.INSTALLED.parent)
# Write a versioned implementation first; atomic wrapper replacement is last.
import tempfile
for destination,source in [(b.INSTALLED,base/'boundary.py'),(wrapper,base/'wrapper')]:
 fd,tmp=tempfile.mkstemp(dir=destination.parent)
 try:
  with os.fdopen(fd,'wb') as out:
   out.write(source.read_bytes());os.fchmod(out.fileno(),0o755);out.flush();os.fsync(out.fileno())
  os.replace(tmp,destination)
  fd=os.open(destination.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)
print('Boundary installed; services, sudo policy, product versions and fleet settings unchanged. Updater remains inactive.')
