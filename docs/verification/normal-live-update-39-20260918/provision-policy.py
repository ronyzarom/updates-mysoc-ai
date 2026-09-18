#!/usr/bin/env python3
"""One-time testing-only policy provisioning. No app apply or database access."""
import argparse, copy, fcntl, hashlib, importlib, json, os, pathlib, signal, socket, stat, subprocess, sys, tempfile, time
P=pathlib.Path
BASELINE='61ce8758c2d361f29aa9483c6d2defc5b3d7cb26088e3d225d5f5af6ec435167'
ARTIFACT='87880575107db69282ee727bd61ef04a33de63196fe407a8de3fb216094522af'
POLICY=P('/etc/siemcore-cascade-updater/recovery-policy.json')
RUNTIME=P('/usr/local/lib/siemcore-recovery/1.0.0.4')
MODULES={'artifact.py':'7ea03e78aa32789cb86e6b3d538315518f006bd384c3d305d1e977293e454dbc','recovery.py':'95b5c64cb8ccf32a449bca2bf7851118abb1d55742bd4513846487010d0eccea','supervise.py':'7a6355b55b4a064adcd1a5a8cac679d5db60d8c6f162b13348de631ac5d52c3d'}
def require(ok,msg):
 if not ok:raise ValueError(msg)
def sha(data):return hashlib.sha256(data).hexdigest()
def trusted(p):
 for x in (p,*p.parents):
  s=x.lstat();require(not stat.S_ISLNK(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,'untrusted path: '+str(x))
def validate_change(old, new):
 require(old['hostname']=='ip-172-26-13-54','wrong policy host')
 require('3.3.152.39' not in old['releases'],'target already present')
 entry=new['releases']['3.3.152.39']
 require(set(entry)=={'artifact','sha256','signature'},'unexpected release fields')
 require(entry['artifact']=='/var/lib/siemcore-cascade-updater/artifacts/siemcore-3.3.152.39.artifact' and entry['sha256']==ARTIFACT,'wrong release bytes/path')
 expected=copy.deepcopy(old);expected['policy_revision']=old['policy_revision']+1
 expected['allowed_transitions'].append(['3.3.152.36','3.3.152.39'])
 expected['releases']['3.3.152.39']=entry
 expected['expected_mounts_by_version']['3.3.152.39']=copy.deepcopy(old['expected_mounts_by_version']['3.3.152.36'])
 require(expected==new,'policy change exceeds approved transition')
def main():
 p=argparse.ArgumentParser();p.add_argument('candidate');p.add_argument('--sha256',required=True);p.add_argument('--install',action='store_true');args=p.parse_args()
 require(os.geteuid()==0 and socket.gethostname()=='ip-172-26-13-54','root/testing host required')
 os.environ.clear();os.environ.update(PATH='/usr/sbin:/usr/bin:/sbin:/bin',HOME='/root',LANG='C.UTF-8')
 state=subprocess.check_output(['systemctl','show','-p','ActiveState','--value','siemcore-cascade-updater.service'],text=True).strip()
 require(state=='inactive','updater must be inactive')
 for name,digest in MODULES.items():
  path=RUNTIME/name;trusted(path);require(sha(path.read_bytes())==digest,'runtime drift')
 archive=P('/opt/siemcore-app-app-a/docker-compose.archive.yml');require(not archive.exists() and not archive.is_symlink(),'unreviewed archive overlay')
 trusted(POLICY);candidate=P(args.candidate);trusted(candidate)
 new_raw=candidate.read_bytes();require(sha(new_raw)==args.sha256,'candidate checksum mismatch');new=json.loads(new_raw)
 sys.path.insert(0,str(RUNTIME));recovery=importlib.import_module('recovery');a=importlib.import_module('artifact')
 recovery.DEADLINE=time.monotonic()+780
 for signum in (signal.SIGINT,signal.SIGTERM):signal.signal(signum,recovery.interrupted)
 store=P('/var/lib/siemcore-recovery');trusted(store)
 with open(store/'lock','a') as lock:
  fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB);recovery.LOCK_FD=lock.fileno()
  old_raw=POLICY.read_bytes();require(sha(old_raw)==BASELINE,'baseline policy changed');old=json.loads(old_raw);validate_change(old,new)
  journal=store/'transaction.json';before=journal.read_bytes() if journal.exists() else None
  if before:require(json.loads(before)['stage'] in ('applied','restored'),'pending transaction')
  with tempfile.TemporaryDirectory(dir=store,prefix='policy39-preflight-') as tmp:
   tx=recovery.Lifecycle(dict(new,journal_dir=tmp)).snapshot('3.3.152.39')
   require(tx['from_version']=='3.3.152.36','wrong predecessor')
  require((journal.read_bytes() if journal.exists() else None)==before,'active journal changed')
  require(POLICY.read_bytes()==old_raw,'policy changed during preflight')
  result={'preflight':'passed','from_version':'3.3.152.36','target_version':'3.3.152.39','candidate_sha256':sha(new_raw),'active_journal_unchanged':True,'installed':False}
  if args.install:
   history=store/'policy-provisioning';history.mkdir(mode=0o700,exist_ok=True);trusted(history)
   recovery.durable(history/(BASELINE+'.json'),old_raw)
   recovery.durable(history/(sha(new_raw)+'.json'),new_raw)
   recovery.durable(POLICY,new_raw);result['installed']=True
  print(json.dumps(result))
if __name__=='__main__':main()
