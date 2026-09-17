"""Qualification-only host Observer verification process. Never starts a service."""
import hashlib
import json
import os
from pathlib import Path
import stat
import client
import data_runner
import protocol


def binary_digest(path):
    path=Path(path)
    if not path.is_absolute() or '..' in path.parts:raise ValueError('absolute verified binary required')
    for parent in path.parents:
        info=parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid!=0 or info.st_mode&0o022:
            raise ValueError('protected binary ancestor required')
    fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW)
    try:
        info=os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_mode&0o022 or not info.st_mode&0o100:
            raise ValueError('protected executable required')
        digest=hashlib.sha256()
        while True:
            chunk=os.read(fd,1024*1024)
            if not chunk:break
            digest.update(chunk)
        return digest.hexdigest()
    finally:os.close(fd)


class Runner:
    def __init__(self,plan,verify_artifact,timeout=60):
        if not callable(verify_artifact):raise ValueError('independent artifact verifier required')
        if type(timeout) is not int or not 1<=timeout<=300:raise ValueError('bounded verification timeout required')
        self.plan,self.verify_artifact,self.timeout=plan,verify_artifact,timeout
    def __call__(self,argv):
        if argv!=self.plan['argv']:raise ValueError('Observer command differs from verified plan')
        expected=self.verify_artifact(argv[0])
        if binary_digest(argv[0])!=expected:raise ValueError('Observer executable differs from signed artifact')
        for option,key in (('--drain-config','drain_sha256'),('--update-config','update_sha256')):
            raw=client.protected(argv[argv.index(option)+1])
            if hashlib.sha256(raw).hexdigest()!=self.plan[key]:raise ValueError('Observer configuration changed')
        raw=client.protected(argv[argv.index('--receipt-config')+1])
        canonical=json.dumps(protocol.strict_json(raw),sort_keys=True,separators=(',',':')).encode()
        if hashlib.sha256(canonical).hexdigest()!=self.plan['config_sha256']:
            raise ValueError('Observer receipt input changed')
        result=data_runner.bounded(argv,self.timeout)
        if binary_digest(argv[0])!=expected:raise ValueError('Observer executable changed during verification')
        return result
