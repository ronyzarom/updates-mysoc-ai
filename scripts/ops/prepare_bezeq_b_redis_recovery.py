#!/usr/bin/env python3
"""Reviewed, incident-scoped prerequisite; never starts services or clears maintenance.

Requires an already acquired durable maintenance marker and explicit approval
for degraded single-node Redis writes. No database files are read or changed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import urllib.request

CONFIG = Path('/opt/siemcore-redis-redis-b/redis.conf')
EXPECTED = '65e89b027b9d423a0de4632052bf5f0b46ecbee653f2567d15df0da629bd3974'
OPERATION = 'recovery-20260915-b-authority'
PREFIX = '/siemcore/pods/bezeq-pod-test/'


def amended(data):
    lines = data.decode().splitlines(keepends=True)
    replicas = [s for s in lines if s.strip().split()[:1] in (['replicaof'], ['slaveof'])]
    limits = [s for s in lines if s.strip().split()[:1] == ['min-replicas-to-write']]
    if len(replicas) != 1 or replicas[0].split() != ['replicaof', 'redis-a', '6379']:
        raise ValueError('unexpected replica configuration')
    if len(limits) != 1 or limits[0].split() != ['min-replicas-to-write', '1']:
        raise ValueError('unexpected replica write policy')
    return ''.join('min-replicas-to-write 0\n' if s == limits[0] else s
                   for s in lines if s != replicas[0]).encode()


def metadata(path):
    req = urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/' + path,
                                 headers={'Metadata-Flavor': 'Google'})
    with urllib.request.urlopen(req, timeout=10) as r:
        return r.read().decode()


def guard(generation):
    if metadata('instance/id') != '6272051641424551474':
        raise ValueError('wrong VM')
    if metadata('project/project-id') != 'osherad-graylog':
        raise ValueError('wrong project')
    token = json.loads(metadata('instance/service-accounts/default/token'))['access_token']
    req = urllib.request.Request(
        'https://compute.googleapis.com/compute/v1/projects/osherad-graylog/zones/me-west1-a/instances/bezeq-pod-test-a',
        headers={'Authorization': 'Bearer ' + token})
    with urllib.request.urlopen(req, timeout=15) as r:
        a = json.load(r)
    if str(a['id']) != '5053012748838117322' or a['status'] != 'TERMINATED':
        raise ValueError('A is not the fenced VM')
    owner, marker = PREFIX + 'owner', PREFIX + 'maintenance'
    txn = (f'version("{owner}") = "0"\nmod("{marker}") = "{generation}"\n'
           f'value("{marker}") = "{OPERATION}"\n\n'
           f'get {marker}\n\nget {marker}\n\n')
    result = subprocess.run(['docker', 'exec', '-i', 'siemcore-pod-quorum-b', 'etcdctl',
        '--endpoints=https://10.89.0.3:12379', '--cacert=/etc/siemcore-pod-quorum/client-ca.crt',
        '--cert=/etc/siemcore-pod-quorum/updater.crt', '--key=/etc/siemcore-pod-quorum/updater.key',
        '--command-timeout=5s', '--write-out=json', 'txn'], input=txn, text=True,
        capture_output=True, check=True, timeout=10)
    response = json.loads(result.stdout)
    if response.get('succeeded') is not True:
        raise ValueError('maintenance is not exclusive')
    # etcdctl 3.5.12 panics on lease comparisons. Read lease in the same
    # successful compare-and-get transaction, so it is the identical revision.
    def kvs(value):
        if isinstance(value, dict):
            for key, item in value.items():
                if key == 'kvs':
                    yield from item
                else:
                    yield from kvs(item)
        elif isinstance(value, list):
            for item in value:
                yield from kvs(item)
    records = list(kvs(response))
    if len(records) != 1 or int(records[0].get('lease', 0)) != 0 or int(records[0]['mod_revision']) != generation:
        raise ValueError('maintenance is not durable')
    for name in ['siemcore-app-b', 'siemcore-archiver-app-b', 'siemcore-lb-b', 'siemcore-redis-b']:
        running = subprocess.check_output(['docker', 'inspect', '--format', '{{.State.Running}}', name],
                                          text=True, timeout=10).strip()
        if running != 'false':
            raise ValueError('unexpected running service')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--maintenance-generation', type=int, required=True)
    p.add_argument('--approve-degraded-single-node-writes', action='store_true')
    p.add_argument('--apply', action='store_true', help='Without this flag, validate only')
    args = p.parse_args()
    if os.geteuid() != 0 or args.maintenance_generation <= 0:
        raise ValueError('root and valid maintenance generation required')
    if not args.approve_degraded_single_node_writes:
        raise ValueError('explicit degraded-write approval required')
    info = CONFIG.lstat()
    if not stat.S_ISREG(info.st_mode) or (info.st_uid, info.st_gid, stat.S_IMODE(info.st_mode)) != (999, 999, 0o600):
        raise ValueError('config ownership/type/mode changed')
    original = CONFIG.read_bytes()
    if hashlib.sha256(original).hexdigest() != EXPECTED:
        raise ValueError('config changed; re-review required')
    result = amended(original)
    guard(args.maintenance_generation)
    if args.apply:
        fd, name = tempfile.mkstemp(prefix='.recovery-', dir=CONFIG.parent)
        try:
            with os.fdopen(fd, 'wb') as f:
                f.write(result)
                os.fchown(f.fileno(), info.st_uid, info.st_gid)
                os.fchmod(f.fileno(), stat.S_IMODE(info.st_mode))
                f.flush()
                os.fsync(f.fileno())
            guard(args.maintenance_generation)
            if CONFIG.is_symlink() or CONFIG.read_bytes() != original:
                raise ValueError('config changed during preflight')
            os.replace(name, CONFIG)
            d = os.open(CONFIG.parent, os.O_RDONLY)
            try:
                os.fsync(d)
            finally:
                os.close(d)
        finally:
            if os.path.exists(name):
                os.unlink(name)
    print(json.dumps({'applied': args.apply, 'sha256': hashlib.sha256(result).hexdigest(),
                      'maintenance_retained': True, 'services_started': False}))


if __name__ == '__main__':
    main()
