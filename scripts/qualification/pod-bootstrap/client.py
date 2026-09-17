#!/usr/bin/env python3
"""Qualification-only registration client. Not shipped in kits; never installs."""
import argparse
import base64
import fcntl
import hashlib
import http.client
import json
import os
import platform
import uuid
from pathlib import Path
import re
import ssl
import stat
import tempfile
import time
from urllib.parse import urlsplit

from protocol import request, strict_json, validate_ack

LIMIT = 8192
PENDING = b'bootstrap blocked: all three bootstrap registrations required'

class Blocked(Exception):
    pass

class AuthorizationNotRecorded(Blocked):
    def __init__(self, body):
        super().__init__('authorization not recorded')
        self.body=body


class RegistrationsPending(Blocked):
    pass


def protected(path, limit=65536, owner=0):
    path = Path(path)
    if not path.is_absolute() or '..' in path.parts:
        raise ValueError('absolute protected path required')
    for parent in path.parents:
        info = parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != owner or info.st_mode & 0o022:
            raise ValueError('unprotected parent')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != owner or info.st_mode & 0o077 or info.st_size > limit:
            raise ValueError('private bounded regular file required')
        raw = os.read(fd, limit + 1)
        if len(raw) > limit:
            raise ValueError('oversized input')
        return raw
    finally:
        os.close(fd)


def durable(path, data):
    raw = (json.dumps(data, sort_keys=True, separators=(',', ':')) + '\n').encode()
    fd, temp = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as out:
            out.write(raw); out.flush(); os.fsync(out.fileno())
        os.replace(temp, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try: os.fsync(directory)
        finally: os.close(directory)
    finally:
        if os.path.exists(temp): os.unlink(temp)


class Transport:
    def __init__(self, config, owner=0):
        if set(config) != {'endpoint','ca_file','client_cert_file','client_key_file','certificate_sha256'}:
            raise ValueError('exact Observer transport settings required')
        self.url = urlsplit(config['endpoint'])
        if (self.url.scheme != 'https' or not self.url.hostname or self.url.username or
                self.url.password or self.url.path not in ('','/') or self.url.query or self.url.fragment):
            raise ValueError('private HTTPS origin required')
        self.pin = config['certificate_sha256']
        if not re.fullmatch('[0-9a-f]{64}', self.pin): raise ValueError('Observer pin required')
        for key in ['ca_file','client_cert_file','client_key_file']:
            protected(config[key], owner=owner)
        self.context = ssl.create_default_context(cafile=config['ca_file'])
        self.context.minimum_version = ssl.TLSVersion.TLSv1_3
        self.context.load_cert_chain(config['client_cert_file'], config['client_key_file'])

    def call(self, action, sent, timeout):
        raw = json.dumps(sent, separators=(',', ':')).encode()
        if len(raw)>LIMIT: raise ValueError('oversized request')
        connection = http.client.HTTPSConnection(self.url.hostname,self.url.port or 443,
                                                timeout=timeout,context=self.context)
        try:
            connection.connect()  # CA/hostname verification precedes pin and request.
            if hashlib.sha256(connection.sock.getpeercert(binary_form=True)).hexdigest() != self.pin:
                raise ValueError('Observer certificate pin mismatch')
            connection.request('POST','/v1/bootstrap/'+action,raw,
                               {'Content-Type':'application/json','Cache-Control':'no-store'})
            response=connection.getresponse(); body=response.read(LIMIT+1)
            if len(body)>LIMIT: raise ValueError('oversized response')
            if response.status==409 and action=='all-registered' and body.strip()==PENDING:
                raise RegistrationsPending('waiting for all three registrations')
            if response.status==409 and action=='authorization-status':
                # The caller must validate every typed absence binding before retrying.
                raise AuthorizationNotRecorded(body)
            if response.status!=200:
                raise Blocked('Observer rejected action (HTTP %d)'%response.status)
            return body  # Redirects are never followed.
        finally:
            connection.close()


class Coordinator:
    def __init__(self, registry, fingerprint, node_id, input_hash, transport, journal,
                 binding, timeout=60, interval=2, clock=time.monotonic, sleep=time.sleep):
        self.registry,self.fingerprint,self.node_id=registry,fingerprint,node_id
        self.input_hash,self.transport,self.path=input_hash,transport,Path(journal)
        self.binding=binding
        self.deadline,self.interval,self.clock,self.sleep=clock()+timeout,interval,clock,sleep

    def call(self, action, generation):
        remaining=self.deadline-self.clock()
        if remaining<=0: raise TimeoutError('registration deadline reached; retain original journal')
        sent=request(self.registry,self.fingerprint,self.node_id,action,generation,
                     self.input_hash if action=='register' else None)
        return validate_ack(sent,action,self.transport.call(action,sent,min(10,remaining)))

    def save(self, state, phase, generation):
        state.update(phase=phase,generation=generation)
        durable(self.path,state)

    def run_locked(self):
        if self.path.exists():
            state=strict_json(protected(self.path))
            if set(state)!={'binding','generation','phase'} or state['binding']!=self.binding:
                raise ValueError('original bootstrap operation/input changed')
            if type(state['generation']) is not int or state['generation']<0:
                raise ValueError('invalid persisted generation')
            if state['phase'] not in ('intent','barrier','registered','all-registered'):
                raise ValueError('unknown journal phase')
            if (state['phase']=='intent') != (state['generation']==0):
                raise ValueError('journal phase/generation conflict')
        else:
            state={'binding':self.binding,'generation':0,'phase':'intent'}
            durable(self.path,state)  # Intent is durable before any Observer mutation.
        generation=state['generation']
        if generation:
            self.call('status',generation)  # Never trust a local success receipt alone.
        else:
            try: ack=self.call('begin-or-resume',0)
            except (OSError,http.client.HTTPException):
                ack=self.call('status',0)  # Same operation; never invent a replacement.
            generation=ack['generation'];self.save(state,'barrier',generation)
        if state['phase']=='barrier':
            try: self.call('register',generation)
            except (OSError,http.client.HTTPException):
                self.call('status',generation)
                self.call('register',generation)  # Exact idempotent registration.
            self.save(state,'registered',generation)
        while True:
            try:
                ack=self.call('all-registered',generation)
                self.save(state,'all-registered',generation)
                return dict(ack,installation_complete=False)
            except RegistrationsPending:
                remaining=self.deadline-self.clock()
                if remaining<=0: raise TimeoutError('registrations pending; original journal retained')
                self.sleep(min(self.interval,remaining))
            except (OSError,http.client.HTTPException):
                self.call('status',generation)
                remaining=self.deadline-self.clock()
                if remaining<=0: raise TimeoutError('reconciliation timeout')
                self.sleep(min(self.interval,remaining))


def validate_registry(registry, pin, architecture):
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    identifier = r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}'
    def matches(pattern, value):
        return isinstance(value, str) and re.fullmatch(pattern, value)
    if (registry['protocol'] != 'pod-bootstrap-v1' or
            not matches(identifier, registry['pod_id']) or
            not matches(identifier, registry['operation_id'])):
        raise ValueError('invalid registry identity')
    incarnation = uuid.UUID(registry['installation_id'])
    if incarnation.int == 0 or str(incarnation) != registry['installation_id']:
        raise ValueError('canonical installation UUID required')
    release = registry['release']
    if (release['product'] != 'siemcore' or not matches(r'[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+', release['version']) or
            not matches('[0-9a-f]{64}', release['sha256']) or
            architecture not in ('amd64', 'arm64') or release['architecture'] != architecture):
        raise ValueError('release identity/architecture mismatch')
    if len(pin) != 32 or base64.b64decode(release['public_key'], validate=True) != pin:
        raise ValueError('registry key differs from local trust')
    Ed25519PublicKey.from_public_bytes(pin).verify(base64.b64decode(release['signature'], validate=True),
        ('mysoc-release-v1\nsiemcore\n'+release['version']+'\n'+release['sha256']).encode())
    nodes = registry['nodes']
    if len(nodes) != 3 or {n['node_id'] for n in nodes} != {'1','2','witness'}:
        raise ValueError('exact node identities required')
    for field in ('node_id','updater_id','machine_id','infrastructure_id','certificate_sha256'):
        values = [n[field] for n in nodes]
        pattern = '[0-9a-f]{64}' if field == 'certificate_sha256' else identifier
        if not all(matches(pattern, value) for value in values) or len(set(values)) != 3:
            raise ValueError('invalid or duplicate node identity')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config',required=True)
    args=parser.parse_args()
    if os.geteuid()!=0: raise ValueError('root qualification fixture required')
    config=strict_json(protected(args.config))
    expected={'registry','observer','node_id','original_input_file','input_receipt_file',
              'signing_public_key_file','journal_directory','timeout_seconds'}
    if set(config)!=expected: raise ValueError('exact qualification config required')
    if type(config['timeout_seconds']) is not int or not 1<=config['timeout_seconds']<=600:
        raise ValueError('timeout must be 1..600 seconds')
    registry=config['registry']
    # Mirror Go typed order; reject unknown fields before hashing.
    order=['protocol','pod_id','installation_id','operation_id','release','nodes']
    release_order=['product','version','architecture','sha256','public_key','signature']
    node_order=['node_id','updater_id','machine_id','infrastructure_id','certificate_sha256']
    if set(registry)!=set(order) or set(registry['release'])!=set(release_order):
        raise ValueError('registry schema mismatch')
    if len(registry['nodes'])!=3 or any(set(n)!=set(node_order) for n in registry['nodes']):
        raise ValueError('exact three-node registry required')
    registry={k:registry[k] for k in order}
    registry['release']={k:registry['release'][k] for k in release_order}
    registry['nodes']=[{k:n[k] for k in node_order} for n in sorted(registry['nodes'],key=lambda n:n['node_id'])]
    if {n['node_id'] for n in registry['nodes']}!={'1','2','witness'}:raise ValueError('node identities required')
    fingerprint=hashlib.sha256(json.dumps(registry,separators=(',',':')).encode()).hexdigest()
    original=protected(config['original_input_file'])
    digest=hashlib.sha256(original).hexdigest()
    receipt=strict_json(protected(config['input_receipt_file']))
    if receipt.get('input_sha256')!=digest:raise ValueError('original input receipt mismatch')
    app=strict_json(original)['application']
    # Reviewed schema-4 envelope; registration does not consume invitations or execute stages.
    coordinator=app.get('bootstrap_coordinator',{})
    if set(coordinator)!={'registry','observer','invitation_file','authorization_key_file'}:
        raise ValueError('exact bootstrap coordinator envelope required')
    supplied=coordinator['registry'].copy()
    if isinstance(supplied.get('nodes'),list):
        supplied['nodes']=sorted(supplied['nodes'],key=lambda n:n['node_id'])
    if (app.get('schema')!=4 or app.get('topology')!='pod' or app.get('cluster_id')!=registry['pod_id'] or
            supplied!=registry or coordinator['observer']!=config['observer']):
        raise ValueError('original input registry/transport mismatch')
    # Require protected detached inputs, but never interpret their presence as stage authorization.
    protected(coordinator['invitation_file'])
    protected(coordinator['authorization_key_file'])
    node=next((n for n in registry['nodes'] if n['node_id']==config['node_id']),None)
    if (node is None or node['machine_id']!=Path('/etc/machine-id').read_text().strip() or
            node['updater_id']!=app.get('updater_instance_id') or
            config['node_id']!={'a':'1','b':'2','witness':'witness'}.get(app.get('pod_role'))):
        raise ValueError('local machine/updater/role differs from registry')
    pin=bytes.fromhex(protected(config['signing_public_key_file']).decode().strip())
    architecture={'x86_64':'amd64','aarch64':'arm64','arm64':'arm64'}.get(platform.machine())
    validate_registry(registry,pin,architecture)
    transport=Transport(config['observer'])
    from cryptography import x509
    from cryptography.hazmat.primitives.serialization import Encoding
    cert=x509.load_pem_x509_certificate(protected(config['observer']['client_cert_file'])).public_bytes(Encoding.DER)
    if hashlib.sha256(cert).hexdigest()!=node['certificate_sha256']:raise ValueError('local client certificate mismatch')
    directory=Path(config['journal_directory'])
    # Directory must be provisioned explicitly; no arbitrary path creation.
    for p in (directory,*directory.parents):
        info=p.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid!=0 or info.st_mode&0o022:
            raise ValueError('protected journal directory required')
    if directory.stat().st_mode&0o077:raise ValueError('private journal directory required')
    fd=os.open(directory/'lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    with os.fdopen(fd,'a') as lock:
        info=os.fstat(lock.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_mode&0o077:
            raise ValueError('private regular lock required')
        fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        binding={'registry_sha256':fingerprint,'node_id':config['node_id'],'input_sha256':digest,
                 'transport':config['observer'],'public_key_hex':pin.hex()}
        registration=Coordinator(registry,fingerprint,config['node_id'],digest,transport,
            directory/'registration.json',binding,config['timeout_seconds'])
        result=registration.run_locked()
        import authorization
        authorization_key=bytes.fromhex(protected(coordinator['authorization_key_file']).decode().strip())
        if len(authorization_key)!=32 or authorization_key==pin:
            raise ValueError('distinct independently pinned authorization key required')
        invitation=strict_json(protected(coordinator['invitation_file']))
        result=authorization.Coordinator(registration,invitation,authorization_key,
            directory/'authorization.json').run_locked(result['generation'])
        print(json.dumps(result))

if __name__=='__main__':
    try:main()
    except Exception as error:
        # Never echo credential/configuration contents or remote error bodies.
        print('Bootstrap registration blocked: '+type(error).__name__,file=__import__('sys').stderr)
        raise SystemExit(1)
