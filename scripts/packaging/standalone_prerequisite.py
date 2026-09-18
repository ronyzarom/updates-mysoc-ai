#!/usr/bin/env python3
"""Build a credential-free, host-bound root prerequisite kit; no publication."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tarfile

def main():
    parser=argparse.ArgumentParser()
    for name in ('identity','public-environment','output','source-version','target-version','original-product-channel','product-channel'):
        parser.add_argument('--'+name,required=True)
    args=parser.parse_args();repo=Path(__file__).resolve().parents[2]
    if subprocess.check_output(['git','status','--porcelain'],cwd=repo,text=True).strip():raise SystemExit('clean committed source required')
    version=(repo/'VERSION').read_text().strip();revision=version+'-r1'
    commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()
    output=Path(args.output);output.mkdir(mode=0o700,parents=True,exist_ok=False)
    name='node-a-standalone-prerequisite-'+revision;kit=output/name;component=kit/'component';component.mkdir(mode=0o700,parents=True)
    names=('cli.py','host.py','source_loader.py','artifacts.py','adapter.py','protocol.py','transaction.py','configuration_inventory.py','worker.py','supervisor.py','capsule.py','QUALIFICATION.json')
    source=repo/'deploy/node-standalone-v1'
    for filename in names:shutil.copyfile(source/filename,component/filename)
    sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
    (component/'COMPONENT.json').write_text(json.dumps(dict(protocol='pod-node-standalone-v1',files={filename:sha(component/filename) for filename in names}),sort_keys=True,indent=2)+'\n')
    shutil.copyfile(source/'prerequisite_install.py',kit/'install.py')
    package=dict(protocol='pod-node-standalone-prerequisite-v1',kit_version=revision,source_commit=commit,
                 identity=json.loads(Path(args.identity).read_text()),public_environment=json.loads(Path(args.public_environment).read_text()),
                 minimum_updater_version=version,source_version=args.source_version,target_version=args.target_version,
                 product_channel=args.original_product_channel,standalone_product_channel=args.product_channel,
                 files={str(p.relative_to(kit)):sha(p) for p in kit.rglob('*') if p.is_file()})
    (kit/'PACKAGE.json').write_text(json.dumps(package,sort_keys=True,indent=2)+'\n')
    for path in kit.rglob('*'):
        path.chmod(0o700 if path.is_dir() else 0o600)
    archive=output/(name+'.tar.gz')
    with tarfile.open(archive,'w:gz') as stream:stream.add(kit,arcname=name)
    manifest=dict(scope='node-a-standalone-prerequisite',version=revision,source_commit=commit,archive=archive.name,
                  sha256=sha(archive),size=archive.stat().st_size,files={str(p.relative_to(kit)):sha(p) for p in kit.rglob('*') if p.is_file()})
    (output/'manifest.json').write_text(json.dumps(manifest,sort_keys=True,indent=2)+'\n')
    print(json.dumps(dict(archive=str(archive),sha256=manifest['sha256'],component_manifest_sha256=sha(component/'COMPONENT.json'))))

if __name__=='__main__':main()
