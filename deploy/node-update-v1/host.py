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
CACHE = Path('/var/lib/siemcore-cascade-updater/artifacts')
BLOCK = Path('/var/lib/siemcore-node-update/management-blocked')
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
        if self.application.get('schema') != 5 or self.application.get('topology') != 'node-unlinked':
            raise ValueError('independent Node required')
        self.node_root = Path('/opt/siemcore-node-unlinked-'+self.application['node_id'])
        self.public_key = bytes.fromhex(self.bootstrap['public_key'])
        self.url = 'https://' + self.application['management']['hostname']

    def verify_binding(self, binding, initial):
        application = private_json(APPLICATION)
        # Match existing product Installer.fingerprint, not raw file bytes or
        # the compact canonical operation JSON serialization.
        policy_digest = hashlib.sha256(json.dumps(application,sort_keys=True).encode()).hexdigest()
        if (application != self.application or binding['machine_id'] != Path('/etc/machine-id').read_text().strip()
                or binding['machine_id'] != application['machine_id']
                or binding['node_id'] != application['node_id']
                or binding['installation_id'] != application['installation_id']
                or binding['updater_instance_id'] != application['updater_instance_id']
                or binding['bootstrap_policy_sha256'] != policy_digest
                or binding['signing_public_key_sha256'] != hashlib.sha256(self.public_key).hexdigest()):
            raise ValueError('protected installation binding mismatch')
        if self.bootstrap_journal.get('status') != 'complete' or self.bootstrap_journal.get('policy_sha256') != policy_digest:
            raise ValueError('completed bootstrap evidence required')
        links=[self.node_root/'link.json',Path('/etc/siemcore-pod-node/link.json'),Path('/etc/siemcore-pod-controller/controller.json'),Path('/var/lib/siemcore-greenfield/pod-runtime.json')]
        if any(path.exists() or path.is_symlink() for path in links):raise ValueError('linked Node refused')
        if initial:
            previous = binding['predecessor']
            ancestor=self.directory/'previous-operation.json'
            if ancestor.exists():
                import uuid
                operation=private_json(ancestor)['operation_id']
                if str(uuid.UUID(operation))!=operation:raise ValueError('invalid predecessor operation')
                directory=self.directory.parent/operation
                journal=private_json(directory/'adapter-journal.json')
                retained=private_json(directory/'binding.json')
                if journal.get('phase')!='accepted' or journal.get('operation_sha256')!=digest(retained) or retained['target']!=previous:
                    raise ValueError('predecessor not accepted')
                for field in ('machine_id','installation_id','updater_instance_id','server_type','node_id','bootstrap_policy_sha256','signing_public_key_sha256'):
                    if retained[field]!=binding[field]:raise ValueError('predecessor identity changed')
            elif (previous['version'] != self.bootstrap['version'] or
                    previous['artifact_sha256'] != self.bootstrap['sha256'] or
                    previous['artifact_signature'] != self.bootstrap['signature']):
                raise ValueError('predecessor is not a qualified installation')

    def preservation(self, previous):
        paths = [APPLICATION, BOOTSTRAP_RELEASE, BOOTSTRAP_JOURNAL]
        names=['settings.json','ui.htpasswd','management.crt','management.key','stage.json','runtime.json','compose.json']
        names+=['credentials/'+k for k in ('postgres','application','redis')]
        names+=['tls/'+k for k in ('ca.crt','server.crt','server.key','client.crt','client.key')]
        names+=['management-inputs/'+k for k in ('management.json','management.crt','management.key','redis','ui.htpasswd','connection.txt')]
        paths += [self.node_root/name for name in names]
        current = {}
        config=private_json(self.node_root/'settings.json')
        for path in paths:
            relative=path.relative_to(self.node_root) if path.is_relative_to(self.node_root) else None
            postgres_owned=relative is not None and (str(relative).startswith('tls/') or str(relative) in ('credentials/postgres','credentials/application'))
            expected_uid=config['postgres_uid'] if postgres_owned else 0
            for parent in path.parents:
                if parent == self.node_root/'tls':
                    info=parent.lstat()
                    if not stat.S_ISDIR(info.st_mode) or info.st_uid!=config['postgres_uid'] or info.st_mode&0o077:raise ValueError('invalid retained TLS directory')
                else:protected(parent)
            info=path.lstat()
            if not stat.S_ISREG(info.st_mode) or info.st_uid!=expected_uid or info.st_mode&0o022:
                raise ValueError('retained file ownership or type mismatch')
            current[str(path)] = dict(sha256=file_digest(path), uid=info.st_uid, gid=info.st_gid, mode=stat.S_IMODE(info.st_mode))
        for role in ('postgres','redis'):
            value=json.loads(subprocess.check_output(['docker','inspect','--type','container','siemcore-unlinked-'+self.application['node_id']+'-'+role]))[0]
            if not value['State']['Running']:raise ValueError('data service unavailable')
            current['container:'+role]=dict(id=value['Id'],image=value['Image'])
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
        capabilities=manifest.get('pod_node_capabilities',[])
        if not isinstance(capabilities,list) or not {'pod-node-update-v1'} <= set(capabilities):
            raise ValueError('signed target Node capabilities required')
        return dict(target_bundle=str(target),predecessor_bundle=str(predecessor),
                    predecessor_ui_capable=bool(set(previous_manifest.get('pod_node_capabilities',[])) & {'pod-node-bootstrap-v1','pod-node-update-v1'}))

    def probe(self,binding,which):
        context=ssl.create_default_context()
        with urllib.request.urlopen(self.url+'/health/live',context=context,timeout=5) as response:
            data=response.read(65537)
        health=strict_json(data)
        try:
            with urllib.request.urlopen(self.url+'/',context=context,timeout=5) as response:code=response.status
        except urllib.error.HTTPError as error:code=error.code
        health={key:health.get(key) for key in ('version','binary_sha256','installation_id','updater_id','installation_state','management_ready','pod_ready','authority_enabled','processing_enabled','node_id','data_ready','installation_complete','link_ready')}
        health['ui_closed']=code in (401,503)
        health['operator_login_verified']=False
        if code==503:health['operator_login_usable']=False
        return health

    def stopped(self):
        return False  # No generic service stop may be claimed as node recovery.
