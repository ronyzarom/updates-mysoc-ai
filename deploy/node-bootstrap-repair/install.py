#!/usr/bin/env python3
"""Install a verified, host-bound bootstrap repair component. Never applies product.
Caller must verify the outer signed kit manifest/archive before executing this file.
"""
import base64
import fcntl
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

FILES = {'install.py','greenfield-hook.py','bootstrap-health-repair.py',
         'bootstrap-health-repair.json','bootstrap-health-repair.json.sig'}

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def unique(pairs):
    value = {}
    for key, item in pairs:
        if key in value: raise ValueError('duplicate JSON key')
        value[key] = item
    return value

def protected(path):
    info=path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
        raise ValueError('expected private root-owned input')
    return json.loads(path.read_text(), object_pairs_hook=unique)

def verify_authorization(raw, signature, public_key):
    Ed25519PublicKey.from_public_bytes(bytes.fromhex(public_key)).verify(
        base64.b64decode(signature, validate=True), b'mysoc-pod-bootstrap-repair-v1\n'+raw)
    auth=json.loads(raw, object_pairs_hook=unique)
    if auth.get('protocol')!='pod-node-bootstrap-health-repair-v1' or auth.get('action')!='docker-cap-prefix-v1':
        raise ValueError('unsupported repair authorization')
    return auth

def verify_package(kit, package):
    if set(package['files']) != FILES: raise ValueError('unexpected kit files')
    for name,digest in package['files'].items():
        path=kit/name
        if path.is_symlink() or not path.is_file() or sha(path)!=digest:
            raise ValueError('kit file checksum mismatch')

def atomic(path, data, mode):
    fd,tmp=tempfile.mkstemp(prefix='.repair-',dir=path.parent)
    try:
        os.fchmod(fd,mode);os.fchown(fd,0,0)
        with os.fdopen(fd,'wb') as stream:
            stream.write(data);stream.flush();os.fsync(stream.fileno())
        os.replace(tmp,path)
        directory=os.open(path.parent,os.O_RDONLY)
        try: os.fsync(directory)
        finally: os.close(directory)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)

def main(kit):
    if os.geteuid()!=0:raise ValueError('root required')
    package=json.loads((kit/'PACKAGE.json').read_text(),object_pairs_hook=unique)
    verify_package(kit,package)
    policy=protected(Path('/etc/siemcore/greenfield-release.json'))
    auth=verify_authorization((kit/'bootstrap-health-repair.json').read_bytes(),
        (kit/'bootstrap-health-repair.json.sig').read_text().strip(),policy['public_key'])
    if auth['product']!='siemcore' or auth['version']!=policy['version'] or auth['release']!={k:policy[k] for k in ('sha256','signature','public_key')}:
        raise ValueError('original release binding mismatch')
    app=protected(Path('/etc/siemcore/greenfield.json'))
    if app.get('schema')!=5 or app.get('topology')!='node-unlinked':raise ValueError('independent node required')
    if auth['identity']!={k:app[k] for k in ('machine_id','installation_id','updater_instance_id','node_id')}:
        raise ValueError('installation binding mismatch')
    if Path('/etc/machine-id').read_text().strip()!=app['machine_id']:raise ValueError('wrong machine')
    library=Path('/usr/local/lib/siemcore-cascade')
    if library.is_symlink() or library.stat().st_uid!=0 or library.stat().st_mode & 0o022:
        raise ValueError('unsafe component directory')
    hook=library/'greenfield-hook.py';backup=library/'greenfield-hook.pre-repair.py'
    targets={'greenfield-hook.py':hook,'bootstrap-health-repair.py':library/'bootstrap-health-repair.py',
        'bootstrap-health-repair.json':Path('/etc/siemcore/bootstrap-health-repair.json'),
        'bootstrap-health-repair.json.sig':Path('/etc/siemcore/bootstrap-health-repair.json.sig')}
    if any(p.is_symlink() for p in [backup,*targets.values()]):raise ValueError('symlink target refused')
    complete=all(p.is_file() and sha(p)==package['files'][name] for name,p in targets.items())
    if complete:
        if not backup.is_file() or sha(backup)!=auth['original_hook_sha256']:raise ValueError('original hook missing')
        subprocess.run(['systemctl','start','siemcore-cascade-updater'],check=True)
        print(json.dumps(dict(status='already-installed',repair_id=auth['repair_id'])))
        return
    if sha(hook)!=auth['original_hook_sha256']:raise ValueError('unexpected installed hook')
    if sha(Path('/usr/local/bin/siemcore-cascade-updater'))!=auth['updater_sha256']:raise ValueError('unexpected updater predecessor')
    if sha(kit/'bootstrap-health-repair.py')!=auth['repair_module_sha256']:raise ValueError('repair module binding mismatch')
    for name,path in targets.items():
        if name!='greenfield-hook.py' and path.exists() and sha(path)!=package['files'][name]:raise ValueError('different repair already staged')
    journal=Path('/var/lib/siemcore-greenfield/journal.json')
    if sha(journal)!=auth['original_journal_sha256'] or protected(journal).get('status')!='installing':raise ValueError('original incomplete journal changed')
    root=Path('/var/lib/siemcore-bootstrap-repair');root.mkdir(mode=0o700,exist_ok=True)
    if root.is_symlink() or root.stat().st_uid!=0 or root.stat().st_mode&0o077:raise ValueError('unsafe repair state root')
    subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True,timeout=120)
    try:
        descriptor=os.open('/var/lib/siemcore-greenfield/hook.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
        with os.fdopen(descriptor,'a') as lock:
            if os.fstat(lock.fileno()).st_uid!=0 or not stat.S_ISREG(os.fstat(lock.fileno()).st_mode):raise ValueError('unsafe hook lock')
            fcntl.flock(lock,fcntl.LOCK_EX)
            if sha(journal)!=auth['original_journal_sha256'] or sha(hook)!=auth['original_hook_sha256']:
                raise ValueError('predecessor changed while stopping updater')
            for name,expected in auth['containers'].items():
                actual=json.loads(subprocess.check_output(['docker','inspect','--type','container','siemcore-unlinked-'+app['node_id']+'-'+name]))[0]
                if actual['Id']!=expected or not actual['State']['Running']:raise ValueError('retained container changed')
            if backup.exists():
                if sha(backup)!=auth['original_hook_sha256']:raise ValueError('different original hook backup')
            else:atomic(backup,hook.read_bytes(),0o600)
            # Entry point is installed last, with the updater stopped and hook lock held.
            for name in ('bootstrap-health-repair.py','bootstrap-health-repair.json','bootstrap-health-repair.json.sig','greenfield-hook.py'):
                atomic(targets[name],(kit/name).read_bytes(),0o600 if name!='greenfield-hook.py' else 0o644)
            atomic(root/'component-install.json',json.dumps(dict(repair_id=auth['repair_id'],files=package['files'],product_applied=False),sort_keys=True).encode(),0o600)
    finally:
        subprocess.run(['systemctl','start','siemcore-cascade-updater'],check=True)
    print(json.dumps(dict(status='component-installed',repair_id=auth['repair_id'],product_applied=False,updater_version_unchanged=True)))

if __name__=='__main__':
    main(Path(sys.argv[1]).resolve())
