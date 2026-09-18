#!/usr/bin/env python3
"""Compare Updates and SiemCore node admission; no install, network, or writes."""
import argparse
import base64
import copy
import importlib.util
import json
from pathlib import Path
from unittest.mock import patch


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--siemcore-source', required=True, type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[3]
    updates = load('updates_contract', root / 'kits/siemcore/greenfield-bootstrap.py')
    product = load('siemcore_contract', args.siemcore_source / 'deploy/cluster/updater/pod_node_unlinked.py')
    release = dict(channel='alpha-test', version='3.3.152.40', sha256='a'*64,
                   public_key='b'*64, signature=base64.b64encode(b'x'*64).decode())
    base = dict(schema=5, topology='node-unlinked', machine_id='a'*32,
                installation_id='install-1', updater_instance_id='updater-1', node_id='1',
                management=dict(listen='0.0.0.0:443', hostname='node.example.org',
                                certificate='/etc/siemcore/cert.pem', key='/etc/siemcore/key.pem'),
                settings_file='/etc/siemcore/settings.json', settings_sha256='c'*64)
    cases = [('node1', base, True), ('node2', dict(base, node_id='2'), True)]
    for key in base:
        app = copy.deepcopy(base); del app[key]
        cases.append(('missing-'+key, app, False))
    for key, value in [('schema',True),('schema',4),('node_id',1),('node_id','witness'),
                       ('node_id',''),('pod_id','fake'),('peer','host'),('observer','host'),
                       ('authority_enabled',True),('topology','pod'),('settings_file','../settings'),
                       ('settings_file','/etc/../settings'),('settings_sha256','bad'),
                       ('installation_id','../install'),('updater_instance_id',''),
                       ('machine_id','b'*32)]:
        app=copy.deepcopy(base); app[key]=value
        cases.append(('invalid-'+key+'-'+str(value),app,False))
    for key,value in [('listen',':80'),('hostname','host/path'),('key','../key'),('certificate','relative')]:
        app=copy.deepcopy(base);app['management'][key]=value
        cases.append(('invalid-management-'+key,app,False))
    results=[]
    for name,app,expected in cases:
        decisions=[]
        for check in (lambda: updates.validate(dict(application=app,release=release)),
                      lambda: product.validate(app, 'a'*32)):
            try:
                check(); accepted=True
            except (ValueError,TypeError,KeyError):
                accepted=False
            decisions.append(accepted)
        # Host binding is a local check in Updates, part of product validation.
        if name == 'invalid-machine_id-'+'b'*32:
            with patch.object(updates.Path, 'read_text', return_value='a'*32):
                try:
                    updates.validate_local_observer(app)
                except ValueError:
                    decisions[0] = False
        results.append(dict(case=name,updates=decisions[0],siemcore=decisions[1],expected=expected))
    failures=[row for row in results if row['updates'] != row['expected'] or row['siemcore'] != row['expected']]
    print(json.dumps(dict(scope='admission-only; no runtime qualification',cases=len(results),failures=failures),indent=2))
    return bool(failures)


if __name__ == '__main__':
    raise SystemExit(main())
