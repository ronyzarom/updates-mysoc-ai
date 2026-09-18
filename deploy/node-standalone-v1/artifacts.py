"""Local signed archive staging only; no downloads or lifecycle invocation."""
import base64
import hashlib
import os
from pathlib import Path, PurePosixPath
import shutil
import stat
import tarfile
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from protocol import strict_json
MAX_ARCHIVE = 10 * 1024**3

def file_digest(path):
    checksum = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1024*1024), b''): checksum.update(block)
    return checksum.hexdigest()

class ArtifactStager:
    def __init__(self, directory, cache, public_key, protected):
        self.directory = protected(Path(directory))
        self.cache = Path(cache)
        self.public_key = public_key
        self.protected = protected

    def stage_one(self, artifact, label, derive_binary=False):
        if label not in ('source', 'target') or derive_binary:
            raise ValueError('fixed verified artifact role required')
        destination = self.directory / label
        # Private staging is regenerated from signed retained cache for each
        # invocation; never trust an extracted target left by a failed worker.
        if destination.exists():
            self.protected(destination);shutil.rmtree(destination)
        destination.mkdir(mode=0o700)
        source = self.cache / ('siemcore-' + artifact['version'] + '.artifact')
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
        if manifest.get('build', {}).get('git_commit') != artifact['source_commit']:
            raise ValueError('signed source commit mismatch')
        return bundle,manifest
