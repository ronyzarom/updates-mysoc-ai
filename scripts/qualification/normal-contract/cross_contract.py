#!/usr/bin/env python3
"""Local synthetic contract checks against immutable Updates/SiemCore commits."""
import base64
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
from unittest.mock import Mock, patch

UPDATES = 'fdb1902'
PRODUCT = 'cd5f94f72bdbcb8d1248fb19b1d4485bae46475e'


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def export(repo, commit, name, root):
    raw = subprocess.check_output(['git', 'show', commit + ':' + name], cwd=repo)
    target = root / name
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(raw)
    return target


def main():
    updates = Path(__file__).resolve().parents[3]
    product = Path(sys.argv[1]).resolve()
    checks = []
    with tempfile.TemporaryDirectory() as folder:
        root = Path(folder)
        bootstrap = load('contract_bootstrap', export(updates, UPDATES, 'kits/siemcore/greenfield-bootstrap.py', root))
        packaging = load('contract_packaging', export(updates, UPDATES, 'scripts/packaging/normal_prerequisite_capability.py', root))
        hook_path = export(product, PRODUCT, 'deploy/cascade/greenfield-hook.py', root)
        hook = load('contract_hook', hook_path)
        for name in ('greenfield.py', 'greenfield_state.py', 'normal_prerequisites.py'):
            export(product, PRODUCT, 'deploy/cluster/updater/' + name, root)
        sys.path.insert(0, str(root / 'deploy/cluster/updater'))
        import greenfield
        import normal_prerequisites
        app = dict(schema=3, topology='single', cluster_id='fixture', instance_id='fixture-app',
                   updater_instance_id='fixture-updater', database_name='siemcore', machine_id='a'*32,
                   frontend_url='https://normal.example', mysoc_url='https://mysoc.example',
                   admin_email='test@example.com', mysoc_api_key='synthetic-only',
                   normal_prerequisites=dict(schema=1, path='/etc/siemcore/provisioning/normal.json', sha256='a'*64))
        release = dict(version='3.3.152.99', channel='stable', sha256='b'*64, public_key='c'*64,
                       signature=base64.b64encode(b'x'*64).decode(), required_capabilities=['normal-prerequisites-v1'])
        original = copy.deepcopy(app)
        bootstrap.validate(dict(application=app, release=release))
        greenfield.validate(app, 'a'*32)
        assert app == original
        checks.append('both validators accept unchanged schema3 Normal without archive')
        kit = root / 'kit'; kit.mkdir()
        (kit / 'greenfield-hook.py').write_bytes(hook_path.read_bytes())
        (kit / 'PROVISIONING_COMMIT').write_text(PRODUCT + '\n')
        marker = packaging.marker_for_hook(hook_path, PRODUCT)
        (kit / 'NORMAL-PREREQUISITES.json').write_text(json.dumps(marker))
        bootstrap.require_delivery(dict(application=app), kit)
        checks.append('actual committed product hook passes exact marker/commit binding')
        bundle = root / 'bundle'; bundle.mkdir(); (bundle / 'updater').mkdir()
        manifest = bundle / 'MANIFEST.json'
        for metadata, executor, accepted in [({}, False, False),
                ({'normal_capabilities':['normal-prerequisites-v1']}, False, False),
                ({'normal_capabilities':['normal-prerequisites-v1']}, True, True)]:
            manifest.write_text(json.dumps(metadata))
            if executor:
                (bundle / 'updater/normal_prerequisites.py').write_bytes((root / 'deploy/cluster/updater/normal_prerequisites.py').read_bytes())
            try:
                hook.require_node_capability(bundle, app)
            except ValueError:
                assert not accepted
            else:
                assert accepted
        checks.append('actual outer hook rejects missing capability/executor and accepts matching product payload')
        legacy = copy.deepcopy(app); del legacy['normal_prerequisites']; legacy['schema'] = 1
        legacy_release = dict(release); del legacy_release['required_capabilities']
        bootstrap.validate(dict(application=legacy, release=legacy_release))
        greenfield.validate(legacy, 'a'*32)
        manifest.write_text('{}'); hook.require_node_capability(bundle, legacy)
        checks.append('legacy Normal bypasses optional capability gate in both implementations')
        receipt = bootstrap.execution_receipt('d'*64, app)
        canonical = json.dumps(app, sort_keys=True, separators=(',', ':'), ensure_ascii=False, allow_nan=False).encode()
        assert receipt['application_canonical_sha256'] == hashlib.sha256(canonical).hexdigest()
        assert receipt['prerequisite_manifest_sha256'] == app['normal_prerequisites']['sha256']
        checks.append('execution receipt uses agreed canonical application and raw manifest hashes')
        raw = json.dumps(dict(schema=1, profile='normal', security_qualification={'status':'blocked'})).encode()
        blocked = copy.deepcopy(app); blocked['normal_prerequisites']['sha256'] = hashlib.sha256(raw).hexdigest()
        run = Mock()
        with patch.object(normal_prerequisites, 'protected_file', return_value=raw):
            try:
                normal_prerequisites.load(blocked, run)
            except ValueError as error:
                assert 'not approved' in str(error)
            else:
                raise AssertionError('security qualification bypassed')
        run.assert_not_called()
        checks.append('product refuses unapproved prerequisite before Docker or execution')
        print(json.dumps(dict(updates_commit=subprocess.check_output(['git','rev-parse',UPDATES],cwd=updates,text=True).strip(),
                              product_commit=PRODUCT, marker=marker, checks=checks,
                              result='passed', live_install=False, signature_verification_tested=False,
                              limitation='Synthetic contract checks; not artifact signing, native installation or security qualification.'), indent=2))


if __name__ == '__main__':
    main()
