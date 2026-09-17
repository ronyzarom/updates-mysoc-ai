#!/usr/bin/env python3
"""Package an already-published amd64 updater; never rebuild or enroll hosts."""
import argparse, hashlib, json, pathlib, shutil, subprocess, tarfile, html, re
p=argparse.ArgumentParser()
p.add_argument('--binary',required=True);p.add_argument('--receipt',required=True)
p.add_argument('--provisioning-source',required=True,help='Committed clean SiemCore source with the matching bootstrap hook')
p.add_argument('--package-revision',required=True,help='New package revision, for example r2; never overwrite published archives')
p.add_argument('--output',required=True)
a=p.parse_args();root=pathlib.Path(__file__).resolve().parents[2]
if not re.fullmatch(r'r[1-9][0-9]*',a.package_revision):p.error('package revision must be r followed by a positive integer')
provisioning=pathlib.Path(a.provisioning_source).resolve()
if subprocess.check_output(['git','status','--porcelain'],cwd=provisioning,text=True).strip():
 p.error('provisioning source must be committed and clean')
provisioning_commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=provisioning,text=True).strip()
modules=['greenfield-hook.py','recovery/artifact.py','recovery/recovery.py','recovery/supervise.py']
for item in modules:
 src=provisioning/'deploy/cascade'/item
 if src.is_symlink() or not src.is_file():p.error('missing regular provisioning module: '+item)
out=pathlib.Path(a.output);out.mkdir(parents=True,exist_ok=False)
r=json.loads(pathlib.Path(a.receipt).read_text());version=r['version'];revision=version+'-'+a.package_revision
binary=pathlib.Path(a.binary).read_bytes();assert hashlib.sha256(binary).hexdigest()==r['sha256']
source=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()
entries=[]
for tier,name in [('mysoc','mysoc-updater'),('siemcore','siemcore-cascade-updater')]:
 kitname=f'{tier}-updater-kit-{revision}-linux-amd64';kit=out/kitname;kit.mkdir()
 for f in (root/'kits'/tier).iterdir():
  assert f.is_file() and not f.is_symlink()
  (kit/f.name).write_bytes(f.read_bytes().replace(b'@VERSION@',version.encode()))
 (kit/'bin').mkdir();b=kit/'bin'/f'{name}-linux-amd64';b.write_bytes(binary);b.chmod(0o755)
 (kit/'install.sh').chmod(0o755)
 if tier=='siemcore':
  for item in modules:
   dst=kit/item;dst.parent.mkdir(exist_ok=True);dst.write_bytes((provisioning/'deploy/cascade'/item).read_bytes())
  (kit/'PROVISIONING_COMMIT').write_text(provisioning_commit+'\n')
 (kit/'docs').mkdir()
 for doc in ['SIEMCORE-INSTALLATION-ROLES.md','SIEMCORE-CLEAN-INSTALL-PARAMETERS.md','UPDATE-ENTRYPOINT-CONTRACT.md','RELAY-DEPLOYMENT.md','UPDATER-GUIDELINES.md']:
  shutil.copyfile(root/'docs'/doc,kit/'docs'/doc)
 (kit/'UPDATER-SIGNATURE.json').write_text(json.dumps({'product':'updater-linux-amd64',**r},indent=2)+'\n')
 (kit/'PACKAGE.json').write_text(json.dumps({'package_revision':revision,'architecture':'linux-amd64','template_commit':source,'updater_version':version,'updater_sha256':r['sha256'],'roles':['standalone','pod-a','pod-b','witness'] if tier=='siemcore' else ['mysoc-relay']},indent=2)+'\n')
 (kit/'START-HERE.txt').write_text('Installation package '+revision+' (Linux amd64 only).\nVerify the archive SHA256 and repository signature before extraction.\nInside the extracted directory run: sha256sum -c SHA256SUMS\nRead README.md and docs/SIEMCORE-INSTALLATION-ROLES.md.\nMySoc requires product provisioning and an apply hook before starting its updater.\nSiemCore clean provisioning requires a complete root-owned input JSON and signed product release receipt.\nNo credentials or customer-specific inputs are included.\nTesting updater channel: stable; fleet group: alpha. Preserve explicit user holds.\nThis package does not promote any product or fleet release.\n')
 files=sorted(f for f in kit.rglob('*') if f.is_file())
 (kit/'SHA256SUMS').write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+str(f.relative_to(kit))+'\n' for f in files))
 archive=out/(kitname+'.tar.gz')
 with tarfile.open(archive,'w:gz') as tf:tf.add(kit,arcname=kitname)
 digest=hashlib.sha256(archive.read_bytes()).hexdigest()
 entries.append({'product':tier,'version':version,'package_revision':revision,'architecture':'linux-amd64','filename':archive.name,'sha256':digest,'size':archive.stat().st_size})
 shutil.rmtree(kit)
(out/'manifest.json').write_text(json.dumps({'schema':1,'packages':entries},indent=2)+'\n')
(out/'SHA256SUMS').write_text(''.join(e['sha256']+'  '+e['filename']+'\n' for e in entries))
shutil.copyfile(root/'docs/SIEMCORE-INSTALLATION-ROLES.md',out/'SIEMCORE-INSTALLATION-ROLES.txt')
shutil.copyfile(root/'docs/SIEMCORE-CLEAN-INSTALL-PARAMETERS.md',out/'SIEMCORE-CLEAN-INSTALL-PARAMETERS.txt')
cards=''.join(f'<article><h2>{"MySoc" if e["product"]=="mysoc" else "SiemCore · standalone & pod"}</h2><p>{"Platform relay updater. MySoc application provisioning is required before starting." if e["product"]=="mysoc" else "One kit for standalone, pod A, pod B and witness. Supply the corresponding provisioning JSON."}</p><p>Updater {version} · package {a.package_revision} · Linux x86-64 · {e["size"]/1048576:.1f} MB</p><a class="button" href="{e["filename"]}">Download {e["product"]} kit</a><details><summary>SHA-256</summary><code>{e["sha256"]}</code></details></article>' for e in entries)
(out/'index.html').write_text('''<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Installation packages · MySoc Updates</title><style>body{font:16px system-ui;background:#0b1220;color:#e5edf7;max-width:1000px;margin:48px auto;padding:0 24px;line-height:1.6}a{color:#50d7ee}h1{font-size:34px}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:24px}article{background:#1d293b;padding:24px;border:1px solid #37445b;border-radius:12px}.button{display:inline-block;background:#087f99;color:white;padding:10px 18px;border-radius:6px;text-decoration:none}code{overflow-wrap:anywhere}details{margin-top:20px}small{color:#b3c0d2}</style><a href="/">← Updates dashboard</a><h1>Installation packages</h1><p>Download the updater installer for a new server. Existing enrolled servers receive signed updates automatically.</p><div class="grid">'''+cards+'''</div><h2>Before installation</h2><p>These packages contain installers, the updater binary and documentation. They contain no customer credentials or application database. They are not complete unattended product installations. Use an approved Linux amd64 host and product-provided provisioning inputs.</p><ol><li>Download the package and <a href="SHA256SUMS">SHA256SUMS</a>.</li><li>Verify the package checksum and <a href="manifest.sig">repository signature</a> using the <a href="verify-download.py">verification helper</a> and your pinned signing public key.</li><li>Extract the archive; read START-HERE.txt and README.md.</li><li>Run the installer with your provisioning values. For testing, use updater channel stable and assign alpha in Fleet.</li></ol><p><a href="SIEMCORE-INSTALLATION-ROLES.txt">SiemCore standalone and pod installation guide</a> · <a href="SIEMCORE-CLEAN-INSTALL-PARAMETERS.txt">Clean-install parameters</a> · <a href="manifest.json">Package manifest</a> · <a href="VERIFY.txt">Verification instructions</a></p><p><small>MySoc application provisioning remains a separate product step. Windows/SWF and Linux ARM64 packages are not included in this repository release.</small></p></html>''')
for name in ['verify-download.py','VERIFY.txt']:
 shutil.copyfile(root/'deploy/downloads'/name,out/name)
print(json.dumps(entries,indent=2))
