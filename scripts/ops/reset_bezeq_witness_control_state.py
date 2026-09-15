#!/usr/bin/env python3
"""Local coordinator for approved disposable-pod rebuild prerequisites.

Does not install/apply a product, start the updater, replace a VM, or copy data.
Default is read-only. Requires old A/B fenced and all three delivery flags held.
"""
import argparse
import json
from pathlib import Path
import subprocess

PROJECT = 'osherad-graylog'
NODES = {'a': ('me-west1-a', '5053012748838117322'),
         'b': ('me-west1-b', '6272051641424551474'),
         'witness': ('me-west1-c', '8916397017694216169')}
IDS = ['3d69bb59-c3d0-4ae3-a48e-d18d03b8c1c1',
       '4b9e76b1-8f77-491f-89c9-e1887f567b11',
       '54c75109-183e-4ef6-ae31-ab62ec862fdc']
ROOT = Path('/Users/ronyzaromil/Documents/code/updates-mysoc-ai')

def reset_updater_state(data):
    import copy
    result = copy.deepcopy(data)
    versions = result.setdefault('product_versions', {})
    sole_product = set(versions) == {'siemcore'}
    versions['siemcore'] = '0.0.0'
    retries = result.get('product_retries')
    if isinstance(retries, dict):
        retries.pop('siemcore', None)
    attempt = result.get('last_update_attempt')
    if isinstance(attempt, dict) and (attempt.get('product') == 'siemcore' or
                                    ('product' not in attempt and sole_product)):
        result['last_update_attempt'] = None
    return result


def validate_reset_receipt(receipt, expected):
    if receipt.get('authorization') != expected or receipt.get('stage') not in ('authorized', 'complete'):
        raise ValueError('reset receipt identity mismatch')


def remove_remaining(names, exists, remove):
    for name in names:
        if exists(name):
            remove(name)


REMOTE = r"""
import base64,fcntl,json,os,pathlib,shutil,stat,subprocess,tempfile,urllib.request
P=pathlib.Path
assert os.geteuid()==0
req=urllib.request.Request('http://metadata.google.internal/computeMetadata/v1/instance/id',headers={'Metadata-Flavor':'Google'})
assert urllib.request.urlopen(req,timeout=5).read().decode()=='8916397017694216169'
containers={'siemcore-pod-quorum-witness':('siemcore-pod-quorum-witness','siemcore-pod-quorum-witness_pod-quorum-data'),
            'siemcore-witness-etcd':('siemcore-witness','siemcore-witness_witness-etcd-data'),
            'siemcore-witness-sentinel':('siemcore-witness','siemcore-witness_witness-sentinel-data')}
dirs=[P('/opt/siemcore-pod-quorum'),P('/opt/siemcore-witness-witness')]
files=[P('/etc/siemcore')/f for f in ['greenfield.json','greenfield-release.json','updater-bootstrap.json']]
state=P('/var/lib/siemcore-cascade-updater/state.json')
product=P('/opt/siemcore-cascade/siemcore')
current=product/'current';previous=product/'.previous'
audit=P('/var/lib/siemcore-reset-15230');receipt_path=audit/'receipt.json'
expected={'schema':1,'pod_id':'bezeq-pod-test','operation':operation,'generation':generation,
          'vm_ids':{'a':'5053012748838117322','b':'6272051641424551474','witness':'8916397017694216169'},
          'containers':list(containers),'volumes':[v[1] for v in containers.values()],
          'directories':list(map(str,dirs)),'files':list(map(str,files))}
def safe(path,private=False):
 for item in [path,*path.parents]:
  if item.exists() or item.is_symlink():
   st=item.lstat();assert not stat.S_ISLNK(st.st_mode),str(item)
   if item!=path:assert st.st_uid in ((0,999) if item==state.parent else (0,)) and not st.st_mode&0o022,str(item)
 if private and path.exists():
  st=path.stat();assert st.st_uid==0 and not st.st_mode&0o077

def atomic(path,data,uid=0,gid=0,mode=0o600):
 fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.reset-')
 try:
  with os.fdopen(fd,'w') as f:
   json.dump(data,f);f.flush();os.fchown(f.fileno(),uid,gid);os.fchmod(f.fileno(),mode);os.fsync(f.fileno())
  os.replace(tmp,path)
  directory=os.open(path.parent,os.O_RDONLY)
  try:os.fsync(directory)
  finally:os.close(directory)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)

def inspect(kind,name):
 p=subprocess.run(['docker',kind,'inspect',name],capture_output=True,text=True,timeout=10)
 if p.returncode:
  if 'no such' in p.stderr.lower():return None
  raise RuntimeError('Docker inspection failed: '+name)
 return json.loads(p.stdout)[0]

safe(receipt_path,True)
receipt=json.loads(receipt_path.read_text()) if receipt_path.exists() else None
if receipt:validate_reset_receipt(receipt,expected)
for path in [*dirs,*files,state,P('/var/lib/siemcore-greenfield')]:safe(path)
assert state.is_file()
if not receipt:
 assert current.is_symlink() and current.resolve().parent==(product/'releases').resolve()
 assert not previous.is_symlink()
 assert json.loads((dirs[0]/'.node.json').read_text())['pod_id']=='bezeq-pod-test'
 assert json.loads((dirs[1]/'.node.json').read_text())['cluster_id']=='bezeq-pod-test'
 j=json.loads(P('/var/lib/siemcore-greenfield/journal.json').read_text())
 assert j['pod_role']=='witness' and set(j['owned_paths'])==set(map(str,dirs))
 app=json.loads(files[0].read_text());assert app['pod_role']=='witness' and app['cluster_id']=='bezeq-pod-test'
 # Quorum was deliberately fenced. Require the previously replicated durable
 # marker locally before issuing the reset receipt, never force a new cluster.
 c=['docker','exec','siemcore-pod-quorum-witness','etcdctl','--endpoints=https://10.89.0.4:12379',
    '--cacert=/etc/siemcore-pod-quorum/client-ca.crt','--cert=/etc/siemcore-pod-quorum/updater.crt',
    '--key=/etc/siemcore-pod-quorum/updater.key','--command-timeout=5s','--write-out=json','get','--consistency=s']
 prefix='/siemcore/pods/bezeq-pod-test/'
 owner=json.loads(subprocess.check_output(c+[prefix+'owner'],timeout=10))
 marker=json.loads(subprocess.check_output(c+[prefix+'maintenance'],timeout=10))
 assert not owner.get('kvs')
 kv=marker['kvs'];assert len(kv)==1
 assert base64.b64decode(kv[0]['value']).decode()==operation
 assert int(kv[0]['mod_revision'])==generation and int(kv[0].get('lease',0))==0
for name,(project,volume) in containers.items():
 d=inspect('container',name)
 if d:
  assert d['Config']['Labels']['com.docker.compose.project']==project
  assert [m['Name'] for m in d['Mounts'] if m['Type']=='volume']==[volume]
 elif not receipt:raise ValueError('initial container missing')
 v=inspect('volume',volume)
 if v:assert v['Labels']['com.docker.compose.project']==project
 elif not receipt:raise ValueError('initial volume missing')
for p in P('/opt').glob('siemcore-*'):
 assert p.name=='siemcore-cascade' or p in dirs,'unexpected product directory'
print(json.dumps({'preflight':'passed','resume':bool(receipt),'authorization':expected,'execute':execute}),flush=True)
if execute:
 audit.mkdir(mode=0o700,exist_ok=True);safe(audit,True)
 external_lock=open(audit/'lock','a');fcntl.flock(external_lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 subprocess.run(['systemctl','stop','siemcore-cascade-updater'],check=True,timeout=45)
 active=subprocess.run(['systemctl','is-active','--quiet','siemcore-cascade-updater']).returncode
 assert active!=0
 processes=subprocess.check_output(['ps','-eo','args='],text=True)
 assert not any('greenfield-bootstrap.py' in line for line in processes.splitlines()),'installer concurrent'
 locks=[]
 for filename in ['hook.lock','lock']:
  p=P('/var/lib/siemcore-greenfield')/filename
  if p.exists():
   safe(p);lock=open(p,'a');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB);locks.append(lock)
 if not receipt:
  receipt={'authorization':expected,'stage':'authorized'};atomic(receipt_path,receipt)
 # Receipt is durable before the first removal. Local coordinator rechecks
 # exact old VM IDs, fencing, and delivery holds on EVERY resumed invocation.
 remove_remaining(list(containers),lambda name:inspect('container',name),
                  lambda name:subprocess.run(['docker','rm','-f',name],check=True,timeout=30,capture_output=True))
 remove_remaining([v[1] for v in containers.values()],lambda name:inspect('volume',name),
                  lambda name:subprocess.run(['docker','volume','rm',name],check=True,timeout=20,capture_output=True))
 for p in dirs:
  if p.exists():shutil.rmtree(p)
 for p in files:
  if p.exists():p.unlink()
 # Retain lock inodes and previous install log; never unlink a held lock.
 for name in ['journal.json','pod-runtime.json','quorum-input']:
  p=P('/var/lib/siemcore-greenfield')/name;safe(p)
  if p.is_dir():shutil.rmtree(p)
  elif p.exists():p.unlink()
 if current.exists() or current.is_symlink():
  assert current.is_symlink() and current.resolve().parent==(product/'releases').resolve();current.unlink()
 if previous.exists():
  assert not previous.is_symlink();previous.unlink()
 d=reset_updater_state(json.loads(state.read_text()));st=state.stat()
 atomic(state,d,st.st_uid,st.st_gid,stat.S_IMODE(st.st_mode))
 receipt['stage']='complete';atomic(receipt_path,receipt)
 print(json.dumps({'reset_complete':True,'updater_started':False,'application_applied':False}),flush=True)
"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--maintenance-generation', type=int, required=True)
    parser.add_argument('--operation', default='clean-rebuild-15230')
    parser.add_argument('--execute', action='store_true')
    args = parser.parse_args()
    assert args.maintenance_generation > 0 and args.operation == 'clean-rebuild-15230'
    # Must run before VM replacement: names alone are not accepted as fencing.
    for role, (zone, identity) in NODES.items():
        d = json.loads(subprocess.check_output(['gcloud','compute','instances','describe',
            'bezeq-pod-test-'+role,'--zone='+zone,'--project='+PROJECT,'--format=json']))
        assert d['id'] == identity
        assert d['status'] == ('RUNNING' if role == 'witness' else 'TERMINATED')
    origin = ['ssh','-o','BatchMode=yes','-i',str(ROOT/'LightsailDefaultKey-eu-west-1-updates-mysoc-ai.pem'),
              'bitnami@updates.mysoc.ai']
    sql = "SELECT json_agg(t) FROM (SELECT id,auto_update_enabled FROM instances WHERE id IN (" + ','.join("'"+i+"'" for i in IDS) + '))t;'
    rows = json.loads(subprocess.check_output(origin+['sudo -n -u postgres psql -d mysoc_updates -At'],input=sql.encode()))
    assert len(rows)==3 and all(r['auto_update_enabled'] is False for r in rows)
    witness = ['ssh','-o','BatchMode=yes','-o','ConnectTimeout=20','-i',str(Path.home()/'.ssh/google_compute_engine'),
        '-o','IdentitiesOnly=yes','-o','HostKeyAlias=compute.8916397017694216169','-o','StrictHostKeyChecking=yes',
        '-o','UserKnownHostsFile='+str(Path.home()/'.ssh/google_compute_known_hosts'),'-o',
        'ProxyCommand=gcloud compute start-iap-tunnel bezeq-pod-test-witness 22 --listen-on-stdin --project=osherad-graylog --zone=me-west1-c --verbosity=error',
        'rony_cyfox_com@compute.8916397017694216169']
    import inspect
    helpers = '\n'.join(inspect.getsource(f) for f in (reset_updater_state, validate_reset_receipt, remove_remaining)) + '\n'
    script = helpers + 'generation='+str(args.maintenance_generation)+'\noperation='+repr(args.operation)+'\nexecute='+repr(args.execute)+'\n'+REMOTE
    subprocess.run(witness+['sudo -n python3 -'],input=script,text=True,check=True,timeout=240)


if __name__ == '__main__':
    main()
