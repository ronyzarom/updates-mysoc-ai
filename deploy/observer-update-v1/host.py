"""Native root adapters for verified staging and independent observations.

Not installed by existing kits. No download or package-manager fallback exists.
"""
import base64
from datetime import datetime, timezone
import errno
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import socket
import ssl
import stat
import subprocess
import tarfile
import urllib.error
import urllib.request
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from protocol import digest, strict_json

APPLICATION = Path('/etc/siemcore/greenfield.json')
BOOTSTRAP_RELEASE = Path('/etc/siemcore/greenfield-release.json')
BOOTSTRAP_JOURNAL = Path('/var/lib/siemcore-greenfield/journal.json')
CONFIG = Path('/etc/siemcore-pod-observer/unlinked.json')
PASSWORD = Path('/etc/siemcore-pod-observer/ui.htpasswd')
CACHE = Path('/var/lib/siemcore-cascade-updater/artifacts')
BLOCK = Path('/var/lib/siemcore-observer-update/management-blocked')
DROPIN = Path('/etc/systemd/system/siemcore-pod-unlinked.service.d/10-update-barrier.conf')
UNIT = 'siemcore-pod-unlinked.service'
MAX_ARCHIVE = 10 * 1024**3


def protected(path):
    path = Path(path)
    for item in (path, *path.parents):
        info = item.lstat()
        if info.st_uid != 0 or info.st_mode & 0o022 or stat.S_ISLNK(info.st_mode):
            raise ValueError('unprotected root path')
        if item != path and not stat.S_ISDIR(info.st_mode):
            raise ValueError('invalid root parent')
    if not (path.is_file() or path.is_dir()):
        raise ValueError('invalid root path type')
    return path


def private_json(path):
    protected(path)
    if path.stat().st_mode & 0o077 or not path.is_file():
        raise ValueError('private root JSON required')
    return strict_json(path.read_bytes())


def file_digest(path):
    checksum = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda:stream.read(1024*1024), b''):checksum.update(block)
    return checksum.hexdigest()


class Host:
    def __init__(self, operation_directory):
        self.directory = protected(Path(operation_directory))
        if self.directory.stat().st_mode & 0o077:
            raise ValueError('private operation directory required')
        self.application = private_json(APPLICATION)
        self.bootstrap = private_json(BOOTSTRAP_RELEASE)
        self.bootstrap_journal = private_json(BOOTSTRAP_JOURNAL)
        if self.application.get('schema') != 4 or self.application.get('topology') != 'observer-unlinked':
            raise ValueError('independent Observer required')
        self.public_key = bytes.fromhex(self.bootstrap['public_key'])
        self.url = 'https://' + self.application['management']['hostname']

    def verify_binding(self, binding, initial):
        application = private_json(APPLICATION)
        # Match existing product Installer.fingerprint, not raw file bytes or
        # the compact canonical operation JSON serialization.
        policy_digest = hashlib.sha256(json.dumps(application,sort_keys=True).encode()).hexdigest()
        if (application != self.application or binding['machine_id'] != Path('/etc/machine-id').read_text().strip()
                or binding['machine_id'] != application['machine_id']
                or binding['installation_id'] != application['installation_id']
                or binding['updater_instance_id'] != application['updater_instance_id']
                or binding['bootstrap_policy_sha256'] != policy_digest
                or binding['signing_public_key_sha256'] != hashlib.sha256(self.public_key).hexdigest()):
            raise ValueError('protected installation binding mismatch')
        if self.bootstrap_journal.get('status') != 'complete' or self.bootstrap_journal.get('policy_sha256') != policy_digest:
            raise ValueError('completed bootstrap evidence required')
        link = Path('/etc/siemcore-pod-observer/link.json')
        if link.exists() or link.is_symlink():raise ValueError('linked Observer refused')
        if initial:
            # Initial implementation intentionally admits only first security
            # hop. Future accepted-predecessor chaining requires qualification.
            previous = binding['predecessor']
            if (previous['version'] != self.bootstrap['version'] or
                    previous['artifact_sha256'] != self.bootstrap['sha256'] or
                    previous['artifact_signature'] != self.bootstrap['signature']):
                raise ValueError('predecessor is not the pinned initial installation')

    def preservation(self, previous):
        paths = [APPLICATION, BOOTSTRAP_RELEASE, BOOTSTRAP_JOURNAL, CONFIG,
                 Path(self.application['management']['certificate']),
                 Path(self.application['management']['key']), PASSWORD]
        current = {}
        for path in paths:
            if path == PASSWORD and not path.exists() and not path.is_symlink():
                protected(path.parent);current[str(path)] = None;continue
            protected(path)
            info = path.stat()
            if not path.is_file():raise ValueError('preserved file not regular')
            if path in (PASSWORD,Path(self.application['management']['key'])) and info.st_mode & 0o077:
                raise ValueError('private credential permissions required')
            current[str(path)] = dict(sha256=file_digest(path), uid=info.st_uid, gid=info.st_gid, mode=stat.S_IMODE(info.st_mode))
        return current if previous is None else previous == current

    def stage_one(self, artifact, label, derive_binary=False):
        destination = self.directory / label
        # Private staging is regenerated from signed retained cache for each
        # invocation; never trust an extracted target left by a failed worker.
        if destination.exists():
            protected(destination);shutil.rmtree(destination)
        destination.mkdir(mode=0o700)
        source = CACHE / ('siemcore-' + artifact['version'] + '.artifact')
        archive = destination / 'release.tar.gz'
        fd = os.open(source,os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        try:
            info=os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_size>MAX_ARCHIVE:raise ValueError('invalid cached artifact')
            checksum=hashlib.sha256();total=0
            with os.fdopen(fd,'rb',closefd=False) as src, archive.open('xb') as dst:
                archive.chmod(0o600)
                for block in iter(lambda:src.read(1024*1024),b''):
                    total+=len(block)
                    if total>MAX_ARCHIVE:raise ValueError('cached artifact exceeds limit')
                    checksum.update(block);dst.write(block)
        finally:os.close(fd)
        if checksum.hexdigest()!=artifact['artifact_sha256']:raise ValueError('artifact checksum mismatch')
        message=('mysoc-release-v1\nsiemcore\n'+artifact['version']+'\n'+artifact['artifact_sha256']).encode()
        Ed25519PublicKey.from_public_bytes(self.public_key).verify(base64.b64decode(artifact['artifact_signature'],validate=True),message)
        prefix='siemcore-universal-'+artifact['version'];seen=set();total=0
        with tarfile.open(archive,'r:gz') as source_tar:
            for member in source_tar:
                name=member.name.removeprefix('./').rstrip('/');parts=PurePosixPath(name).parts
                if not parts or parts[0]!=prefix or '..' in parts or name.startswith('/') or name in seen or not(member.isdir() or member.isfile()):
                    raise ValueError('unsafe signed archive tree')
                seen.add(name);total+=member.size
                if len(seen)>100000 or total>30*1024**3:raise ValueError('archive expansion limit')
                target=destination.joinpath(*parts)
                if member.isdir():target.mkdir(mode=0o700,parents=True,exist_ok=True)
                else:
                    target.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
                    with source_tar.extractfile(member) as src,target.open('xb') as dst:shutil.copyfileobj(src,dst)
                    target.chmod(0o700 if member.mode&0o111 else 0o600)
        bundle=destination/prefix
        manifest=strict_json((bundle/'MANIFEST.json').read_bytes())
        if manifest.get('product')!='siemcore' or manifest.get('version')!=artifact['version'] or manifest.get('architecture')!='amd64':
            raise ValueError('signed manifest identity mismatch')
        if not derive_binary and file_digest(bundle/'pod/bin/siemcore')!=artifact['binary_sha256']:raise ValueError('packaged binary mismatch')
        return bundle,manifest

    def stage(self,binding):
        predecessor,previous_manifest=self.stage_one(binding['predecessor'],'predecessor')
        target,manifest=self.stage_one(binding['target'],'target')
        capabilities=manifest.get('observer_unlinked_capabilities',[])
        if not isinstance(capabilities,list) or not {'observer-ui-auth-v1','observer-unlinked-update-v1'} <= set(capabilities):
            raise ValueError('signed target Observer capabilities required')
        return dict(target_bundle=str(target),predecessor_bundle=str(predecessor),
                    predecessor_ui_capable='observer-ui-auth-v1' in previous_manifest.get('observer_unlinked_capabilities',[]))

    def probe(self,binding,which):
        context=ssl.create_default_context()
        with urllib.request.urlopen(self.url+'/health/live',context=context,timeout=5) as response:
            data=response.read(65537)
        health=strict_json(data)
        try:
            with urllib.request.urlopen(self.url+'/',context=context,timeout=5) as response:code=response.status
        except urllib.error.HTTPError as error:code=error.code
        health={key:health.get(key) for key in ('version','binary_sha256','installation_id','updater_id','installation_state','management_ready','pod_ready','authority_enabled','processing_enabled')}
        health['ui_closed']=code in (401,503)
        health['operator_login_verified']=False
        if code==503:health['operator_login_usable']=False
        return health

    def stopped(self):
        try:
            protected(BLOCK);protected(DROPIN)
            if BLOCK.read_bytes()!=b'Observer update recovery barrier\n':return False
            expected=('[Unit]\nConditionPathExists=!'+str(BLOCK)+'\n[Service]\nKillMode=control-group\nRestart=no\n').encode()
            if DROPIN.read_bytes()!=expected:return False
            raw=subprocess.check_output(['systemctl','show',UNIT,'--property=ActiveState,MainPID,ControlPID,ControlGroup,Restart,KillMode'],timeout=10,text=True)
            values=dict(line.split('=',1) for line in raw.splitlines() if '=' in line)
            if values.get('ActiveState') not in ('inactive','failed') or values.get('MainPID')!='0' or values.get('ControlPID')!='0' or values.get('Restart')!='no' or values.get('KillMode')!='control-group':return False
            if 'ControlGroup' not in values:return False
            group=values['ControlGroup']
            if group=='/':return False
            if group:
                relative=PurePosixPath(group.lstrip('/'))
                if not group.startswith('/') or '..' in relative.parts:return False
                for processes in (Path('/sys/fs/cgroup')/str(relative)).rglob('cgroup.procs'):
                    if processes.read_text().strip():return False
            try:
                with socket.create_connection(('127.0.0.1',443),timeout=2):return False
            except OSError as error:return error.errno==errno.ECONNREFUSED
        except (OSError,ValueError,subprocess.SubprocessError):return False
