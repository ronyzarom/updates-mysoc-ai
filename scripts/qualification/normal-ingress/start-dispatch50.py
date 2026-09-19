from pathlib import Path
import hashlib,json,urllib.request,subprocess,tarfile,os
q=urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
assert urllib.request.urlopen(q).read().decode()=='8169611730880778800'
root=Path('/etc/normal-qualification')
assert json.loads((root/'kit-result.json').read_text())['first_install_by_kit']
assert json.loads(Path('/etc/siemcore/greenfield.json').read_text())['cluster_id']=='normal-fixture'
p=Path('/root/port50-source');p.mkdir(mode=0o700)
with tarfile.open('/tmp/port50-source.tar.gz') as t:t.extractall(p,filter='data')
hashes={'recovery.py':'7fa7cabf7178d99a040292f8dd80abed72509335a598286dfa6c9f03588430ed','artifact.py':'7ea03e78aa32789cb86e6b3d538315518f006bd384c3d305d1e977293e454dbc','supervise.py':'7a6355b55b4a064adcd1a5a8cac679d5db60d8c6f162b13348de631ac5d52c3d','port_normalization.py':'6f8fbd809736d7ae92c6dc05775b92321aa1bb10d3b4eb7e6cbd357ba0836958'}
for n,h in hashes.items():assert hashlib.sha256((p/n).read_bytes()).hexdigest()==h
hook=Path('/tmp/reviewed-ingress50/ingress50-greenfield-hook.py')
assert hashlib.sha256(hook.read_bytes()).hexdigest()=='329e855866399e59e62219fcbd7d8b59087b472f60253c6ae95fe43e8ccedea8'
subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True)
for n in hashes:subprocess.run(['install','-o','root','-g','root','-m','0755',str(p/n),'/usr/local/lib/siemcore-cascade/recovery/'+n],check=True)
subprocess.run(['install','-o','root','-g','root','-m','0755',str(hook),'/usr/local/lib/siemcore-cascade/greenfield-hook.py'],check=True)
os.environ['NORMAL_CLEAN_FIXTURE']='1'
with open('/root/cascade50-dispatch.log','xb') as out:
 os.chmod(out.name,0o600)
 result=subprocess.run(['python3','-u','/tmp/reviewed-ingress50/normal-cascade50-fixture.py'],stdout=out,stderr=subprocess.STDOUT)
print('dispatch_exit',result.returncode)
print(Path('/root/cascade50-dispatch.log').read_text())
raise SystemExit(result.returncode)
