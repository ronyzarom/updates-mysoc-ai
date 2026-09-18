"""Disposable nested-Docker product qualification, never a live host installer.

Requires INDEPENDENT_NODE_FIXTURE=1, no existing product, synthetic dependencies.
Fixture CA trust is added only inside this disposable host and its containers.
No host socket, live data, cloud credentials or production TLS keys are mounted.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import uuid


def run(args, **kwargs):
    result = subprocess.run(args, capture_output=True, timeout=800, **kwargs)
    if result.returncode:
        print(result.stderr.decode(errors='replace')[-6000:], file=sys.stderr)
        raise RuntimeError('fixture command failed')
    return result.stdout


def private(path, raw):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    path.write_bytes(raw); path.chmod(0o600)


def main():
    parser=argparse.ArgumentParser()
    for name in ('source','predecessor','target'):
        parser.add_argument('--'+name,type=Path,required=True)
    parser.add_argument('--postgres',required=True);parser.add_argument('--redis',required=True)
    parser.add_argument('--gcs-credentials',type=Path)
    parser.add_argument('--gcs-bucket')
    parser.add_argument('--gcs-project')
    parser.add_argument('--resume-fixture-bootstrap',action='store_true')
    parser.add_argument('--root-admission-fixture',action='store_true')
    args=parser.parse_args()
    if os.geteuid()!=0 or os.environ.get('INDEPENDENT_NODE_FIXTURE')!='1':
        raise SystemExit('fresh disposable nested host required')
    existing=Path('/var/lib/siemcore-greenfield/journal.json').exists()
    if existing:
        if not args.resume_fixture_bootstrap or json.loads(Path('/etc/siemcore/greenfield.json').read_text()).get('installation_id')!='fixture-node-1' or product_operation_exists():
            raise SystemExit('only original synthetic bootstrap may resume')
    else: Path('/etc/machine-id').write_text(uuid.uuid4().hex+'\n')
    versions={role:json.loads(getattr(args,role).joinpath('MANIFEST.json').read_text())['version'] for role in ('predecessor','target')}
    fixture_source=Path('/root/node-source')
    if not existing:
        fixture_source.mkdir()
        shutil.copytree(args.predecessor/'updater',fixture_source/'updater')
        shutil.copyfile(args.predecessor/'pod/bin/siemcore',fixture_source/'siemcore')
        shutil.copyfile(args.source/'deploy/cascade/greenfield-hook.py',fixture_source/'greenfield-hook.py')
    run(['docker','load','-i',str(args.predecessor/'images'/('siemcore-'+versions['predecessor']+'.tar'))])
    bootstrap_command=['python3',str(args.source/'deploy/cluster/updater/tests/node_signed_root_fixture.py'),'--node','1','--version',versions['predecessor'],'--postgres',args.postgres,'--redis',args.redis,'--product','siemcore:'+versions['predecessor'],'--source',str(fixture_source)]
    if args.root_admission_fixture and not existing:
        run(bootstrap_command+['--prepare-only'])
        # Sign the exact retained source archive with a disposable fixture key
        # BEFORE first bootstrap. No post-bootstrap trust/receipt rewriting.
        import base64
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        from cryptography.hazmat.primitives import serialization
        key=Ed25519PrivateKey.generate()
        private(Path('/root/fixture-signing-key.bin'),key.private_bytes(serialization.Encoding.Raw,serialization.PrivateFormat.Raw,serialization.NoEncryption()))
        archive=Path('/root/previous.tar.gz');checksum=hashlib.sha256(archive.read_bytes()).hexdigest()
        release=dict(version=versions['predecessor'],channel='stable',sha256=checksum,
                     public_key=key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex(),
                     signature=base64.b64encode(key.sign(('mysoc-release-v1\nsiemcore\n'+versions['predecessor']+'\n'+checksum).encode())).decode())
        private(Path('/etc/siemcore/greenfield-release.json'),json.dumps(release).encode())
        shutil.copyfile(archive,Path('/var/lib/siemcore-cascade-updater/artifacts')/('siemcore-'+versions['predecessor']+'.artifact'))
        run(['python3',str(fixture_source/'greenfield-hook.py'),'apply'])
        run(['python3',str(fixture_source/'greenfield-hook.py'),'health'])
        print('PREPARED exact-source signed root fixture; product transition delegated to root adapter')
        return
    run(bootstrap_command)
    sys.path.insert(0,str(args.target/'updater'))
    import pod_node_standalone as product
    app=json.loads(Path('/etc/siemcore/greenfield.json').read_text())
    bootstrap=Path('/var/lib/siemcore-greenfield/journal.json').read_bytes()
    data_names=['siemcore-unlinked-1-'+role for role in ('postgres','redis')]
    before=[record['Id'] for record in json.loads(run(['docker','inspect',*data_names]))]
    opid=str(uuid.uuid4());operation=product.ROOT/opid;operation.mkdir(parents=True,mode=0o700)
    config=operation/'configuration';config.mkdir(mode=0o700)
    certroot=Path('/root/standalone-fixture-ca');certroot.mkdir(mode=0o700)
    run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(certroot/'ca.key'),'-out',str(certroot/'ca.crt'),'-days','1','-subj','/CN=SSL.com Disposable Qualification CA'])
    run(['openssl','req','-newkey','rsa:2048','-nodes','-keyout',str(certroot/'key'),'-out',str(certroot/'csr'),'-subj','/CN=node1.fixture'])
    private(certroot/'ext',b'subjectAltName=DNS:node1.fixture\nextendedKeyUsage=serverAuth\n')
    run(['openssl','x509','-req','-in',str(certroot/'csr'),'-CA',str(certroot/'ca.crt'),'-CAkey',str(certroot/'ca.key'),'-CAcreateserial','-out',str(certroot/'leaf'),'-days','1','-extfile',str(certroot/'ext')])
    shutil.copyfile(certroot/'ca.crt','/usr/local/share/ca-certificates/standalone-fixture.crt')
    run(['update-ca-certificates'])
    node=Path('/opt/siemcore-node-unlinked-1')
    settings=json.loads((node/'settings.json').read_text())
    env={'DB_NAME':settings['database'],'DB_PASSWORD':(node/'credentials/application').read_text().strip(),
         'REDIS_URL':'redis://:'+(node/'credentials/redis').read_text().strip()+'@siemcore-unlinked-1-redis:6379/0',
         'JWT_SECRET':secrets.token_hex(32),'SIEMCORE_FRONTEND_URL':'https://node1.fixture',
         'TIERED_INGEST_ENABLED':'true','TIERED_LOCAL_DIR':'/var/lib/siemcore/archives/objects','ARCHIVE_BACKEND':'local'}
    if any((args.gcs_credentials,args.gcs_bucket,args.gcs_project)):
        if not all((args.gcs_credentials,args.gcs_bucket,args.gcs_project)):raise ValueError('complete_fixture_gcs_configuration_required')
        env.pop('TIERED_LOCAL_DIR',None)
        env.update(ARCHIVE_BACKEND='gcs',TIERED_GCS_BUCKET=args.gcs_bucket,GCP_PROJECT_ID=args.gcs_project,TIERED_STORAGE_CLASS='STANDARD',TIERED_AUTO_PROVISION_BUCKET='false',GOOGLE_APPLICATION_CREDENTIALS='/run/siemcore-archive/gcp-archiver.json')
        private(config/'archive-credentials/gcp-archiver.json',args.gcs_credentials.read_bytes())
    private(config/'application.env',''.join(key+'='+value+'\n' for key,value in env.items()).encode())
    private(config/'installation/tls.crt',(certroot/'leaf').read_bytes()+(certroot/'ca.crt').read_bytes())
    private(config/'installation/tls.key',(certroot/'key').read_bytes())
    private(config/'mysoc-bootstrap.json',json.dumps(dict(instance_id='fixture-customer-1',endpoint_url='https://mysoc.fixture',api_key=secrets.token_hex(32),heartbeat_seconds=60)).encode())
    inventory={str(path.relative_to(config)):hashlib.sha256(path.read_bytes()).hexdigest() for path in config.rglob('*') if path.is_file()}
    target_manifest=json.loads((args.target/'MANIFEST.json').read_text())
    identity={key:app[key] for key in ('machine_id','installation_id','updater_instance_id','node_id')}
    binding=dict(protocol=product.PROTOCOL,operation_id=opid,source=identity,source_version=versions['predecessor'],source_artifact_sha256='a'*64,
         bootstrap_receipt_sha256=hashlib.sha256(bootstrap).hexdigest(),source_evidence_sha256='b'*64,
         target=dict(product='siemcore',version=versions['target'],architecture='linux/amd64',source_commit=target_manifest['build']['git_commit'],artifact_sha256='c'*64,binary_sha256=hashlib.sha256((args.target/'pod/bin/siemcore').read_bytes()).hexdigest()),
         configuration_sha256=hashlib.sha256(product.canonical(inventory)).hexdigest(),instance_id='fixture-customer-1',source_mode='independent-management',target_mode='independent-standalone')
    request=dict(protocol=product.PROTOCOL,action='apply',binding=binding,operation_sha256=hashlib.sha256(product.canonical(binding)).hexdigest(),operation_directory=str(operation),target_bundle=str(args.target),predecessor_bundle=str(args.predecessor),application_policy='/etc/siemcore/greenfield.json',configuration_directory=str(config))
    # This harness exercises product execution, not root signature admission;
    # artifact hashes above are synthetic. Actual binary/image remains unchanged.
    private(Path('/root/standalone-request.json'),json.dumps(request).encode())
    execute_worker(product,request,before,bootstrap)


def execute_worker(product,request,before,bootstrap):
    data_names=['siemcore-unlinked-1-'+role for role in ('postgres','redis')]
    worker=product.Worker(product.parse_request(json.dumps(request)))
    compose=worker.compose
    def fixture_compose(*argv):
        result=compose(*argv)
        if argv and argv[0]=='up':
            # Dedicated CA trust only, inside disposable containers. Do not
            # weaken product TLS validation or alter the image's application.
            for role in ('app','archiver'):
                container='siemcore-standalone-1-'+role
                run(['docker','cp','/etc/ssl/certs/ca-certificates.crt',container+':/etc/ssl/certs/ca-certificates.crt'])
                run(['docker','restart',container])
        return result
    worker.compose=fixture_compose
    result=worker.execute()
    if result['phase']!='accepted':raise RuntimeError('standalone not accepted')
    assert [record['Id'] for record in json.loads(run(['docker','inspect',*data_names]))]==before
    assert Path('/var/lib/siemcore-greenfield/journal.json').read_bytes()==bootstrap
    request['action']='status';assert product.Worker(request).execute()['phase']=='accepted'
    private(Path('/root/standalone-fixture-result.json'),json.dumps(dict(status='functional-health-passed',health=result['health'],bootstrap_bytes_preserved=True,data_ids_preserved=True,root_signature_admission_tested=False,full_pipeline_probe_pending=True),indent=2).encode())
    print('PASS product standalone functional health and retained-data identity; full pipeline qualification still required')


def product_operation_exists():
    return Path('/var/lib/siemcore-node-standalone/operations').exists()


if __name__=='__main__':main()
