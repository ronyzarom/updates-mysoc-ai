#!/usr/bin/env python3
"""Root standalone entry point. No kit enables this component yet."""
import fcntl
import hashlib
import os
from pathlib import Path
import sys
sys.path.insert(0,str(Path(__file__).resolve().parent))
from adapter import Adapter
from host import Host, POLICY, ROOT
from protocol import strict_json
from source_loader import SourceLoader
from worker import invoke


def main():
    if os.geteuid()!=0 or sys.platform!='linux' or len(sys.argv)!=2 or sys.argv[1] not in ('readiness','apply','status','recover'):
        raise ValueError('exact_root_action_required')
    loader=SourceLoader()
    policy=strict_json(loader.read(POLICY))
    if policy.get('enabled') is not True:
        raise ValueError('standalone_capability_disabled')
    base=Path(__file__).resolve().parent
    manifest_raw=loader.protected(base/'COMPONENT.json').read_bytes()
    if hashlib.sha256(manifest_raw).hexdigest()!=policy['component_manifest_sha256']:
        raise ValueError('component_manifest_mismatch')
    manifest=strict_json(manifest_raw)
    expected={'cli.py','host.py','source_loader.py','artifacts.py','adapter.py','protocol.py','transaction.py','configuration_inventory.py','worker.py','supervisor.py','capsule.py','QUALIFICATION.json'}
    if manifest.get('protocol')!='pod-node-standalone-v1' or set(manifest.get('files',{}))!=expected:
        raise ValueError('exact_standalone_component_required')
    for name,checksum in manifest['files'].items():
        if hashlib.sha256(loader.protected(base/name).read_bytes()).hexdigest()!=checksum:
            raise ValueError('component_integrity_mismatch')
    request=strict_json(sys.stdin.buffer.read(65537))
    binding=policy['binding']
    expected={'protocol':'pod-node-standalone-v1'}
    if sys.argv[1]!='readiness':
        expected.update(operation_id=binding['operation_id'],target={'version':binding['target']['version'],
                        'sha256':binding['target']['artifact_sha256'],'signature':policy['target_artifact']['artifact_signature']})
    if request!=expected:
        raise ValueError('protected_operation_request_required')
    lock_path=Path('/var/lib/siemcore-greenfield/hook.lock')
    loader.protected(lock_path.parent)
    if lock_path.exists():loader.protected(lock_path)
    fd=os.open(lock_path,os.O_WRONLY|os.O_CREAT|os.O_NOFOLLOW|os.O_NONBLOCK,0o600)
    try:
        fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
        directory=ROOT/'operations'/binding['operation_id']
        if sys.argv[1] in ('status','recover') and not directory.exists():
            raise ValueError('unknown_operation')
        for path in (ROOT,ROOT/'operations',directory):
            path.mkdir(mode=0o700,exist_ok=True);loader.protected(path)
        # Pipeline verification intentionally remains unavailable until native
        # acceptance interface is reviewed. No weak readiness-only fallback.
        host=Host(directory)
        if sys.argv[1]=='readiness':
            # Updater asks before downloading the offered target. Component and
            # source admission must not depend on that cache entry already existing.
            host.admit(binding)
            from datetime import datetime,timezone
            from transaction import digest
            import json
            response=dict(protocol=binding['protocol'],operation_id=binding['operation_id'],operation_sha256=digest(binding),
                 observed_at=datetime.now(timezone.utc).isoformat(),server_type='pod-node',node_id=binding['source']['node_id'],
                 adapter_manifest_sha256=policy['component_manifest_sha256'],capabilities=['pod-node-standalone-v1'],
                 target_version=binding['target']['version'],artifact_sha256=binding['target']['artifact_sha256'])
            if 'previous_operation' in binding:
                response.update(previous_operation_id=binding['previous_operation']['operation_id'],previous_operation_sha256=binding['previous_operation']['operation_sha256'])
            print(json.dumps(response,sort_keys=True))
            return
        adapter=Adapter(directory,host,lambda action,b,bundles:invoke(action,b,bundles,directory,fd,900 if action!='status' else 30))
        import json
        result=adapter.run(sys.argv[1],binding)
        result.update(target_version=binding['target']['version'],artifact_sha256=binding['target']['artifact_sha256'])
        if sys.argv[1]=='readiness':
            result.update(server_type='pod-node',node_id=binding['source']['node_id'],
                          adapter_manifest_sha256=policy['component_manifest_sha256'],
                          capabilities=['pod-node-standalone-v1'] if result['phase']=='prepared' else [])
        print(json.dumps(result,sort_keys=True))
    finally:os.close(fd)


if __name__=='__main__':
    try:main()
    except Exception:
        print('Standalone operation unavailable or outcome unverified; inspect protected journal',file=sys.stderr)
        sys.exit(1)
