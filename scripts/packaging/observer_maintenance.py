#!/usr/bin/env python3
"""Package a signed binary + committed Observer maintenance component; no upload."""
import argparse,hashlib,json,pathlib,shutil,subprocess,tarfile
p=argparse.ArgumentParser();p.add_argument('--binary',required=True);p.add_argument('--receipt',required=True);p.add_argument('--output',required=True);a=p.parse_args()
root=pathlib.Path(__file__).resolve().parents[2]
if subprocess.check_output(['git','status','--porcelain'],cwd=root,text=True).strip():p.error('clean committed source required')
commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip();version=(root/'VERSION').read_text().strip()
receipt=json.loads(pathlib.Path(a.receipt).read_text());binary=pathlib.Path(a.binary)
sha=lambda f:hashlib.sha256(f.read_bytes()).hexdigest()
assert receipt['version']==version and receipt['sha256']==sha(binary)
out=pathlib.Path(a.output);out.mkdir(parents=True,exist_ok=False)
name='observer-updater-maintenance-'+version+'-r1-linux-amd64';kit=out/name;kit.mkdir(mode=0o700);(kit/'component').mkdir(mode=0o700)
for f in (root/'deploy/observer-update-v1').glob('*.py'):shutil.copyfile(f,kit/'component'/f.name)
files={f.name:sha(f) for f in sorted((kit/'component').glob('*.py'))}
(kit/'component/COMPONENT.json').write_text(json.dumps(dict(protocol='observer-unlinked-update-v1',files=files),sort_keys=True)+'\n')
shutil.copyfile(root/'deploy/observer-update-maintenance/install.py',kit/'install.py');shutil.copyfile(binary,kit/'updater-linux-amd64');(kit/'updater-linux-amd64').chmod(0o755)
package=dict(updater_version=version,source_commit=commit,updater_receipt=receipt,allowed_predecessor_updater_sha256='ff17b828018c557d0772519861282809d12cb5c5ba5cdaa10fda0642e8d5b81a',allowed_bootstrap_hook_sha256='34f6cf6891f6e571ce4a4507055dd886470718852e6521b0074149402fb5fae2',files={str(f.relative_to(kit)):sha(f) for f in kit.rglob('*') if f.is_file()})
(kit/'PACKAGE.json').write_text(json.dumps(package,indent=2)+'\n')
for f in kit.rglob('*'):
 if f.is_file():f.chmod(0o755 if f.name=='updater-linux-amd64' else 0o600)
archive=out/(name+'.tar.gz')
with tarfile.open(archive,'w:gz') as tar:tar.add(kit,arcname=name)
entry=dict(product='observer-updater-maintenance',version=version,package_revision=version+'-r1',architecture='linux-amd64',source_commit=commit,filename=archive.name,sha256=sha(archive),size=archive.stat().st_size)
(out/'manifest.json').write_text(json.dumps(dict(schema=1,packages=[entry]),indent=2)+'\n')
shutil.copyfile(root/'deploy/downloads/verify-download.py',out/'verify-download.py')
print(json.dumps(entry,indent=2))
