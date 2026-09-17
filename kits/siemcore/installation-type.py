#!/usr/bin/env python3
"""Render explicit installation identity; never assign live pod authority."""
import argparse
import json
import os
from pathlib import Path
import re
import stat
import subprocess

KINDS = ('normal', 'pod-active', 'pod-stby', 'pod-observer')
FIELDS = ('server_type', 'pod_id', 'node_id')


def validate_identity(kind, pod_id='', node_id=''):
    if kind not in KINDS:
        raise ValueError('unknown SiemCore server type')
    if kind == 'normal':
        if pod_id or node_id:
            raise ValueError('normal installation cannot have pod identity')
    else:
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,100}', pod_id):
            raise ValueError('pod ID required')
        expected = ('witness',) if kind == 'pod-observer' else ('1', '2')
        if node_id not in expected:
            raise ValueError('node identity conflicts with server type')
    return dict(zip(FIELDS, (kind, pod_id, node_id)))


def from_application(app, requested=''):
    shape = (app.get('schema'), app.get('topology'))
    if type(app.get('schema')) is not int:
        raise ValueError('invalid bootstrap schema')
    if shape in ((1, 'single'), (3, 'single')):
        if requested and requested != 'normal':
            raise ValueError('standalone bootstrap conflicts with server type')
        return validate_identity('normal')
    if shape not in ((2, 'pod'), (3, 'pod')):
        raise ValueError('unsupported bootstrap topology')
    role = app.get('pod_role')
    if role not in ('a', 'b', 'witness'):
        raise ValueError('unsupported bootstrap node role')
    # A/B are node IDs, never proof of primary ownership. Without an explicit
    # label both data-node installations start with standby intent.
    kind = requested or app.get('server_type') or ('pod-observer' if role == 'witness' else 'pod-stby')
    if (role == 'witness') != (kind == 'pod-observer') or kind == 'normal':
        raise ValueError('bootstrap node role conflicts with server type')
    return validate_identity(kind, app.get('cluster_id', ''), {'a': '1', 'b': '2', 'witness': 'witness'}[role])


def product_span(text):
    matches = list(re.finditer(r'^  - name: siemcore\s*$', text, re.M))
    if len(matches) != 1:
        raise ValueError('expected one SiemCore product')
    start = matches[0].end()
    end = re.search(r'^(?:\S|  - name:)', text[start:], re.M)
    return start, start + end.start() if end else len(text)


def existing_identity(text):
    start, end = product_span(text)
    block = text[start:end]
    values = {}
    for field in FIELDS:
        matches = re.findall(r'^    ' + field + r':\s*([^\n]+)$', block, re.M)
        if len(matches) > 1:
            raise ValueError('duplicate installation identity')
        if matches:
            value = matches[0].strip()
            if value.startswith('"'):
                value = json.loads(value)
            values[field] = value
    if not values:
        return None
    return validate_identity(values.get('server_type', ''), values.get('pod_id', ''), values.get('node_id', ''))


def render(text, identity):
    old = existing_identity(text)
    if old and old != identity:
        raise ValueError('existing installation identity differs')
    start, end = product_span(text)
    block = text[start:end]
    for field in FIELDS:
        block = re.sub(r'^    ' + field + r':[^\n]*\n?', '', block, flags=re.M)
    rows = ''.join('    ' + field + ': ' + json.dumps(identity[field]) + '\n' for field in FIELDS if identity[field])
    return text[:start] + '\n' + rows + block.lstrip('\n') + text[end:]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--config', required=True)
    parser.add_argument('--binary')
    parser.add_argument('--existing-config')
    parser.add_argument('--server-type', default='')
    parser.add_argument('--pod-id', default='')
    parser.add_argument('--node-id', default='')
    parser.add_argument('--greenfield-input')
    args = parser.parse_args()
    identity = None
    if args.greenfield_input:
        path = Path(args.greenfield_input)
        info = path.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
            raise ValueError('bootstrap input must be root-owned and private')
        identity = from_application(json.loads(path.read_text())['application'], args.server_type)
        if args.pod_id and args.pod_id != identity['pod_id'] or args.node_id and args.node_id != identity['node_id']:
            raise ValueError('explicit identity conflicts with bootstrap input')
    elif args.server_type:
        identity = validate_identity(args.server_type, args.pod_id, args.node_id)
    elif args.pod_id or args.node_id:
        raise ValueError('pod identity requires server type')
    if args.existing_config and Path(args.existing_config).exists():
        old = existing_identity(Path(args.existing_config).read_text())
        if old and identity and old != identity:
            raise ValueError('installer refuses to reclassify existing host')
        identity = identity or old
    if identity:
        if args.binary and identity['server_type'] != 'normal':
            result = subprocess.run([args.binary, 'installation-types'], capture_output=True,
                                    text=True, timeout=10, check=True)
            capabilities = json.loads(result.stdout)
            if capabilities.get('schema') != 1 or identity['server_type'] not in capabilities.get('server_types', []):
                raise ValueError('updater binary does not support explicit pod installation identity')
        path = Path(args.config)
        path.write_text(render(path.read_text(), identity))


if __name__ == '__main__':
    main()
