#!/usr/bin/env python3
"""Disposable-only signed .34 staging fixture; no application/data-service activation."""
import hashlib,json,os,shutil,socket,subprocess,time,urllib.request
from pathlib import Path
assert os.geteuid()==0 and socket.gethostname().split('.')[0]=='updater-boundary-qual-20260916'
BASE=Path(__file__).resolve().parent
RESULT=Path('/var/lib/updates-recovery-native-result.json')
if RESULT.exists():raise SystemExit('qualification already complete')
IMAGE='quay.io/coreos/etcd:v3.5.12'
IMAGE_ID='sha256:14a8055c1e3dd23fdd1f0198a9d313a9ac75775515d2331190b3213ed74fbb91'
SHA='e2174f30a6104980f69dd39a0e5a6e419cb09f7246cae78ba0d0dcbeb2cd3537'
RUNTIME='de02aa0337f3556e9e833a452fb0040a92e767d4a25ad6febed5440b3c83aa1f'
PRIVATE=Path('/root/recovery-native-fixture');PRIVATE.mkdir(mode=0o700,exist_ok=True)
def run(*args):return subprocess.check_output([str(x) for x in args],stderr=subprocess.STDOUT,text=True,timeout=180).strip()
def write(path,raw,mode=0o600):
 path=Path(path);path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(raw if isinstance(raw,bytes) else raw.encode());path.chmod(mode)
def js(path,value):write(path,json.dumps(value,sort_keys=True)+'\n')
def digest(path):return hashlib.sha256(Path(path).read_bytes()).hexdigest()
req=urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token',headers={'Metadata-Flavor':'Google'})
token=json.load(urllib.request.urlopen(req,timeout=10))['access_token']
for name,sha in [('siemcore-universal-3.3.152.34.tar.gz',SHA),('recovery-etcd-amd64.tar','ca873681de9b388775691a9fd602362532b7e6f7849c9889a4ba62bdbcdeadb0')]:
 p=PRIVATE/name
 if not p.exists():
  request=urllib.request.Request('https://storage.googleapis.com/osherad-graylog-boundary-qual-20260916/'+name,headers={'Authorization':'Bearer '+token})
  with urllib.request.urlopen(request,timeout=120) as src,p.open('wb') as dst:shutil.copyfileobj(src,dst)
  p.chmod(0o600)
 assert digest(p)==sha
run('docker','load','-i',PRIVATE/'recovery-etcd-amd64.tar')
assert run('docker','image','inspect',IMAGE,'--format','{{.Id}}')==IMAGE_ID
# Synthetic TLS credentials and one real etcd member exposed through three
# localhost endpoints; this qualifies CAS, not three-member availability.
cert=PRIVATE/'certs';cert.mkdir(exist_ok=True)
if not (cert/'ca.crt').exists():
 run('openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',cert/'ca.key','-out',cert/'ca.crt','-days','2','-subj','/CN=qualification-ca')
 for name,cn,usage in [('server','qualification-server','serverAuth'),('updater','updater-b','clientAuth')]:
  run('openssl','req','-newkey','rsa:2048','-nodes','-keyout',cert/(name+'.key'),'-out',cert/(name+'.csr'),'-subj','/CN='+cn)
  write(cert/(name+'.ext'),'subjectAltName=IP:127.0.0.1\nextendedKeyUsage='+usage+'\n')
  run('openssl','x509','-req','-in',cert/(name+'.csr'),'-CA',cert/'ca.crt','-CAkey',cert/'ca.key','-CAcreateserial','-out',cert/(name+'.crt'),'-days','2','-extfile',cert/(name+'.ext'))
for p in cert.iterdir():p.chmod(0o600)
existing=run('docker','ps','-a','--format','{{.Names}}').splitlines()
if 'recovery-qualification-etcd' not in existing:
 run('docker','run','-d','--name','recovery-qualification-etcd','--user','0','-p','127.0.0.1:23791:2379','-p','127.0.0.1:23792:2379','-p','127.0.0.1:23793:2379','-v',str(cert)+':/cert:ro',IMAGE,'/usr/local/bin/etcd','--listen-client-urls=https://0.0.0.0:2379','--advertise-client-urls=https://127.0.0.1:23791','--cert-file=/cert/server.crt','--key-file=/cert/server.key','--client-cert-auth=true','--trusted-ca-file=/cert/ca.crt')
def etcd(*args):return run('docker','exec','recovery-qualification-etcd','/usr/local/bin/etcdctl','--endpoints=https://127.0.0.1:2379','--cacert=/cert/ca.crt','--cert=/cert/updater.crt','--key=/cert/updater.key',*args)
end=time.monotonic()+30
while True:
 try:etcd('endpoint','health');break
 except subprocess.CalledProcessError:
  if time.monotonic()>end:raise
  time.sleep(1)
# Named, stopped synthetic containers/volumes only, never PostgreSQL/Redis data.
for name in ['siemcore-db-b-postgres','siemcore-redis-b','siemcore-app-b','siemcore-archiver-app-b','siemcore-lb-b']:
 if name not in existing:run('docker','create','--name',name,'--mount','type=volume,source=recovery-qual-'+name+',target=/synthetic-data',IMAGE,'/usr/local/bin/etcd')
mounts={}
for name in ('siemcore-db-b-postgres','siemcore-redis-b'):
 mount=json.loads(run('docker','inspect',name))[0]['Mounts'][0]['Source'];p=Path(mount)/'synthetic-marker';
 if not p.exists():write(p,'synthetic fixture, never production data\n')
 mounts[str(p)]=digest(p)
# Provision inert synthetic config; protected policy is deliberately minimal
# because signed recovery does not perform greenfield installation.
machine=Path('/etc/machine-id').read_text().strip();pod='qualification-pod'
js('/etc/siemcore/greenfield.json',{'machine_id':machine,'topology':'pod','pod_role':'b','cluster_id':pod})
for src,dst in [('ca.crt','client-ca.crt'),('updater.crt','updater.crt'),('updater.key','updater.key')]:write(Path('/opt/siemcore-pod-quorum/config')/dst,(cert/src).read_bytes())
config={'pod_id':pod,'node_id':'b','peer_id':'a','suffix':'b','initial_node':'a','public_whoami_url':'https://pod.example.invalid/__lb_whoami','quorum':{'endpoints':['https://127.0.0.1:'+str(port) for port in (23791,23792,23793)],'ttl_seconds':15,'ca':str(cert/'ca.crt'),'cert':str(cert/'updater.crt'),'key':str(cert/'updater.key')},'peer_agent':{'url':'https://192.0.2.1:18443','ca':str(cert/'ca.crt'),'cert':str(cert/'updater.crt'),'key':str(cert/'updater.key')},'peer_gcp':{'project':'qualification-invalid','zone':'qualification-invalid','name':'qualification-invalid','id':'1'}}
js('/etc/siemcore-pod-controller/controller.json',config)
for unit in ('siemcore-pod-agent','siemcore-pod-controller','siemcore-pod-observer','siemcore-pod-pair'):
 path=Path('/etc/systemd/system')/(unit+'.service')
 if not path.exists():write(path,'[Unit]\nDescription=Inert qualification prerequisite\n[Service]\nType=oneshot\nExecStart=/usr/bin/true\n',0o644)
subprocess.run(['useradd','--system','--no-create-home','siemcore-pod-agent'],capture_output=True)
run('systemctl','daemon-reload')
# Install local runner candidate/guard only on this disposable fixture.
normal=Path('/usr/local/lib/siemcore-pod-boundary/1.0.0.3');normal.mkdir(parents=True,exist_ok=True)
for name in ('reconcile.py','incident.json'):write(normal/name,(BASE/name).read_bytes(),0o644)
write(normal/'boundary.py',(BASE/'normal_boundary.py').read_bytes(),0o644)
write('/usr/local/sbin/siemcore-apply-update','#!/bin/sh\nexec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/bin/python3 /usr/local/lib/siemcore-pod-boundary/1.0.0.3/boundary.py "$@"\n',0o755)
installed=Path('/usr/local/lib/updates-pod-recovery/1.0.0.1/runner.py');write(installed,(BASE/'runner.py').read_bytes(),0o644)
normalstate=Path('/var/lib/siemcore-pod-boundary');normalstate.mkdir(mode=0o700,exist_ok=True)
for p in [normalstate/'boundary.lock',normalstate/'work.lock',Path('/var/lib/siemcore-cascade-updater/state.json.cycle-lock')]:
 if not p.exists():write(p,b'')
# The disposable previous qualification journal is not a live product journal.
js(normalstate/'transaction.json',{'phase':'healthy','role':'b'})
operation='qualification-recovery-34';key='/siemcore/pods/'+pod+'/maintenance'
etcd('put',key,operation);marker=json.loads(etcd('get',key,'-w','json'))['kvs'][0]
intent={'schema':1,'product':'siemcore','operation_id':operation,'pod_id':pod,'node_id':'b','machine_id':machine,'version':'3.3.152.34','artifact_sha256':SHA,'maintenance':{'operation_id':operation,'generation':marker['mod_revision']},'expected_owner':'','activation_allowed':False}
js('/etc/siemcore-pod-recovery/intent.json',intent)
receipt=json.loads((BASE/'signed-release-receipt.json').read_text());receipt['artifact']=str(PRIVATE/'siemcore-universal-3.3.152.34.tar.gz');js('/etc/siemcore-pod-recovery/release.json',receipt)
# Successful exact signed product stage and verified repeat, with actual native
# systemd, Docker inspections and TLS etcd maintenance CAS.
result=json.loads(run('/usr/bin/python3',installed,'stage'))
assert result['status']=='recovery-staged' and result['receipt']['runtime_sha256']==RUNTIME and result['application_health_verified'] is False
again=json.loads(run('/usr/bin/python3',installed,'verify-staged'));assert again==result
assert all(digest(p)==want for p,want in mounts.items())
for name in ('siemcore-pod-controller','siemcore-pod-observer','siemcore-pod-agent'):
 assert run('systemctl','show',name,'-p','ActiveState','--value')=='inactive'
for name in ('siemcore-db-b-postgres','siemcore-redis-b','siemcore-app-b','siemcore-archiver-app-b','siemcore-lb-b'):
 assert run('docker','inspect',name,'--format','{{.State.Running}}')=='false'
assert not Path('/var/lib/siemcore-pod-update/transaction.json').exists()
owner=json.loads(etcd('get','/siemcore/pods/'+pod+'/owner','-w','json'));assert not owner.get('kvs')
# Normal updater cannot apply through the guard, even without the outer lock.
import importlib.util
spec=importlib.util.spec_from_file_location('guard',normal/'boundary.py');guard=importlib.util.module_from_spec(spec);spec.loader.exec_module(guard)
for phase in ('apply','health','rollback'):
 try:guard.Boundary(None).dispatch({}, {},phase)
 except RuntimeError:pass
 else:raise AssertionError('ordinary updater bypassed recovery barrier')
# Loss of maintenance must reject verify-staged and retain the root barrier.
etcd('del',key)
failed=subprocess.run(['/usr/bin/python3',str(installed),'verify-staged'],capture_output=True,text=True)
assert failed.returncode!=0 and Path('/var/lib/updates-pod-recovery/transaction.json').exists()
assert all(digest(p)==want for p,want in mounts.items())
checks=['real-signed-34-stage-and-repeat','real-tls-etcd-exclusive-maintenance','native-systemd-supervision','docker-container-identity/state/mount-preservation','synthetic-volume-markers-unchanged','no-product-services-started','no-normal-product-transaction','no-owner-created','ordinary-phase-guard','maintenance-loss-refused-barrier-retained']
result={'status':'passed','artifact_sha256':SHA,'runtime_sha256':RUNTIME,'checks':checks,'scope':'disposable synthetic stopped containers and volumes, one etcd member/three localhost endpoints; not production data, DB health, three-member HA, pod activation, routing or application acceptance'}
js(RESULT,result)
with open('/dev/ttyS0','w') as out:out.write('UPDATES_RECOVERY_NATIVE '+json.dumps(result)+'\n')
print(json.dumps(result))
