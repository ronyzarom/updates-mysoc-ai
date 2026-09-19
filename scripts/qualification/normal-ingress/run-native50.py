from pathlib import Path
import hashlib,json,urllib.request,subprocess,tarfile,os,sys
mode=sys.argv[1];ids={'direct':'4385378830207112216','crash':'5426915765356025313'}
q=urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
assert urllib.request.urlopen(q).read().decode()==ids[mode]
assert json.loads(Path('/etc/normal-qualification/kit-result.json').read_text())['first_install_by_kit']
assert hashlib.sha256(Path('/tmp/siemcore-universal-3.3.152.50.tar.gz').read_bytes()).hexdigest()=='9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc'
p=Path('/root/port50-source');p.mkdir(mode=0o700)
with tarfile.open('/tmp/port50-source.tar.gz') as t:t.extractall(p,filter='data')
hashes={'recovery.py':'7fa7cabf7178d99a040292f8dd80abed72509335a598286dfa6c9f03588430ed','artifact.py':'7ea03e78aa32789cb86e6b3d538315518f006bd384c3d305d1e977293e454dbc','supervise.py':'7a6355b55b4a064adcd1a5a8cac679d5db60d8c6f162b13348de631ac5d52c3d','port_normalization.py':'6f8fbd809736d7ae92c6dc05775b92321aa1bb10d3b4eb7e6cbd357ba0836958'}
for n,h in hashes.items():assert hashlib.sha256((p/n).read_bytes()).hexdigest()==h
subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True)
for n in hashes:subprocess.run(['install','-o','root','-g','root','-m','0755',str(p/n),'/usr/local/lib/siemcore-cascade/recovery/'+n],check=True)
os.environ['NORMAL_CLEAN_FIXTURE']='1'
log=Path('/root/port50-'+mode+'.log')
with log.open('xb') as out:
 os.chmod(out.name,0o600)
 cmd=['python3','-u',str(p/'normal_ingress_native_drill.py')]
 if mode=='direct':cmd.append('--direct-normalization')
 cmd+=['/etc/normal-qualification/release.tar.gz','/tmp/siemcore-universal-3.3.152.50.tar.gz']
 result=subprocess.run(cmd,stdout=out,stderr=subprocess.STDOUT)
print('drill_exit',result.returncode)
print(log.read_text())
raise SystemExit(result.returncode)
