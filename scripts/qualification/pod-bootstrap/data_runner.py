"""Qualification-only bounded Docker runner; never imported by shipped kits."""
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import signal
import subprocess
import time
import uuid

COMMAND=('/app/siemcore','pod-bootstrap-data','--config','/run/bootstrap/data.json')
MOUNTS={'/run/bootstrap','/run/tls','/run/siemcore-postgres'}


def bounded(argv, timeout, output_limit=8192):
    """Bound combined output and wall time; errors never include command output."""
    process=subprocess.Popen(argv,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE,start_new_session=True)
    output=bytearray();total=0;deadline=time.monotonic()+timeout
    selector=selectors.DefaultSelector()
    try:
        for stream in (process.stdout,process.stderr):selector.register(stream,selectors.EVENT_READ)
        while selector.get_map():
            remaining=deadline-time.monotonic()
            if remaining<=0:raise TimeoutError('qualification process deadline exceeded')
            for key,_ in selector.select(min(remaining,0.2)):
                chunk=os.read(key.fileobj.fileno(),4096)
                if not chunk:selector.unregister(key.fileobj);continue
                total+=len(chunk)
                if total>output_limit:raise ValueError('qualification process output exceeded limit')
                if key.fileobj is process.stdout:output.extend(chunk)
        remaining=deadline-time.monotonic()
        if remaining<=0:raise TimeoutError('qualification process deadline exceeded')
        return process.wait(timeout=remaining),bytes(output)
    finally:
        selector.close()
        if process.poll() is None:
            os.killpg(process.pid,signal.SIGKILL)
        process.wait()
        process.stdout.close();process.stderr.close()


def docker_json(docker,args):
    code,raw=bounded([docker,*args],10,65536)
    if code:raise ValueError('Docker inspection failed')
    return json.loads(raw)


def mount_path(value):
    p=Path(value)
    if not p.is_absolute() or '..' in p.parts or ',' in str(p):
        raise ValueError('absolute unambiguous mount source required')
    for parent in (p,*p.parents):
        st=parent.lstat()
        if not parent.is_dir() or parent.is_symlink() or st.st_uid!=0 or st.st_mode&0o022:
            raise ValueError('protected root-owned mount directory required')
    return str(p)


def prepare(docker,image_id,network_id,mounts,verify_artifact_image,authorize_runtime):
    """Verifier callbacks are mandatory: image presence is not signature verification.

    verify_artifact_image must independently verify signed artifact, image identity,
    architecture, release and source. authorize_runtime must verify current exact
    Observer operation authorization, runtime readiness, mount contents and peers.
    Both must raise on failure. No permissive default implementation exists.
    """
    if not Path(docker).is_absolute() or not re.fullmatch(r'sha256:[0-9a-f]{64}',image_id):
        raise ValueError('absolute Docker binary and immutable image ID required')
    if not re.fullmatch(r'[0-9a-f]{64}',network_id) or set(mounts)!=MOUNTS:
        raise ValueError('fixed network ID and exact mount set required')
    if not callable(verify_artifact_image) or not callable(authorize_runtime):
        raise ValueError('independent artifact and runtime verifiers required')
    sources={target:mount_path(source) for target,source in mounts.items()}
    verify_artifact_image(image_id)
    image=docker_json(docker,['image','inspect',image_id])
    if len(image)!=1 or image[0]['Id']!=image_id or image[0].get('Os')!='linux':
        raise ValueError('verified local Linux image missing')
    if image[0].get('Config',{}).get('Volumes'):
        raise ValueError('implicit image volumes forbidden')
    network=docker_json(docker,['network','inspect',network_id])
    if len(network)!=1 or network[0]['Id']!=network_id or network[0].get('Internal') is not True or network[0].get('Driver')!='bridge':
        raise ValueError('fixed internal bridge network required')
    authorize_runtime()
    name='updates-pod-stage-'+uuid.uuid4().hex
    argv=[docker,'run','--name',name,'--pull=never','--network',network_id,
          '--read-only','--user','0:0','--cap-drop=ALL','--security-opt','no-new-privileges',
          '--pids-limit','128','--memory','512m','--cpus','1',
          '--log-driver','none','--entrypoint',COMMAND[0]]
    for target in sorted(sources):
        argv+=['--mount','type=bind,src='+sources[target]+',dst='+target+',readonly']
    argv += [image_id,*COMMAND[1:]]
    return name,argv


class Runner:
    def __init__(self,docker,image_id,network_id,mounts,verify_artifact_image,authorize_runtime,timeout=300):
        if type(timeout) is not int or not 1<=timeout<=600:
            raise ValueError('bounded qualification timeout required')
        self.settings=(docker,image_id,network_id,mounts,verify_artifact_image,authorize_runtime)
        self.timeout=timeout
        self.container_name=None
    def __call__(self,command):
        if tuple(command)!=COMMAND:raise ValueError('unexpected artifact command')
        docker=self.settings[0]
        name,argv=prepare(*self.settings)
        self.container_name=name
        try:
            return bounded(argv,self.timeout)
        finally:
            # CLI termination alone does not stop a detached daemon-side process.
            # Keep the stopped container for qualification evidence; never remove
            # mounts, database, slots, product journals, or Observer state.
            state=docker_json(docker,['container','inspect',name])
            if len(state)!=1:raise ValueError('cannot prove qualification container state')
            if state[0]['State']['Running']:
                code,_=bounded([docker,'kill',name],10)
                if code:raise ValueError('cannot stop qualification container')
                state=docker_json(docker,['container','inspect',name])
                if state[0]['State']['Running']:raise ValueError('qualification container remains running')
