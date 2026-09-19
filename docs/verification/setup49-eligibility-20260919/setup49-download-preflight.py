import hashlib,json,os,pathlib,pwd,subprocess,sys,tempfile,yaml
P=pathlib.Path
root=P(sys.argv[1]);kind=sys.argv[2]
conf=P('/etc/siemcore-cascade-updater/config.yaml');original=conf.read_bytes();c=yaml.safe_load(original)
expected={'A':'siemcore-normal-db91d16e-97a5-452d-ae54-6db5c6d8f3bf','B':'siemcore-normal-b-90881d89-d9b0-4e41-8643-3cf758903cc1','testing':'siemcore-testing-01'}
assert c['instance']['id']==expected[kind]
assert c.get('self_update',{}).get('channel','stable')=='stable'
for p in c['products']:
 if p['name']=='siemcore':
  assert p['channel']==('stable' if kind=='testing' else 'normal-a-20260919');p['channel']='obs-test-20260918'
# Only a temporary config changes product eligibility, and only --download uses it.
serviceuser=subprocess.check_output(['systemctl','show','siemcore-cascade-updater','-p','User','--value'],text=True).strip();u=pwd.getpwnam(serviceuser)
fd,temp=tempfile.mkstemp(prefix='.setup49-download-',suffix='.yaml',dir=conf.parent)
try:
 with os.fdopen(fd,'wb') as f:
  f.write(yaml.safe_dump(c).encode());f.flush();os.fsync(f.fileno());os.fchown(f.fileno(),0,u.pw_gid);os.fchmod(f.fileno(),0o640)
 subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True)
 state=json.loads(P(c['simulation']['state_file']).read_text())
 assert not state.get('operation_state') and not state.get('pending_retry_products')
 assert state.get('product_versions',{}).get('siemcore')==('3.3.152.39' if kind=='testing' else '3.3.152.47')
 log=root/'download.log'
 with log.open('w') as f:
  os.chmod(log,0o600)
  result=subprocess.run(['runuser','-u',serviceuser,'--','/usr/local/bin/siemcore-cascade-updater','once','--download','--config',temp],stdout=f,stderr=subprocess.STDOUT,timeout=600)
 assert result.returncode==0,'download failed; inspect protected download.log'
 archive=P('/var/lib/siemcore-cascade-updater/artifacts/siemcore-3.3.152.49.artifact')
 assert hashlib.sha256(archive.read_bytes()).hexdigest()=='88d4bbde2b5e9ccdbb572285f03b5f1daac1283880c575186f9be321cbdf8b70','download digest mismatch'
 cmd=['python3',str(root/'setup49-native-preflight.py'),str(root/'setup49-settings.json')]
 if kind=='testing':cmd.append('--install-testing-policy')
 result=subprocess.run(cmd,text=True,capture_output=True,timeout=850)
 (root/'preflight.log').write_text(result.stdout+result.stderr);os.chmod(root/'preflight.log',0o600)
 assert result.returncode==0,'preflight failed; inspect protected preflight.log'
 print(result.stdout.strip())
finally:
 assert conf.read_bytes()==original,'canonical config changed concurrently'
 P(temp).unlink(missing_ok=True)
 subprocess.run(['systemctl','start','siemcore-cascade-updater'],check=True)
