"""Exact setup50 native preflight; never invokes product apply or reads DB data."""
import argparse,copy,fcntl,hashlib,importlib.util,json,os,pathlib,socket,stat,subprocess,sys,tempfile,time
P=pathlib.Path
SHA='9167378baaf99483a0a72b4f896c9d969e70bfab978052f5f6b482f6b01203bc'
BASELINE='9cd577e406de989e0f35a7483fde902f0ed4f225f17cf2c22d0416a0b6fda6f1'
POLICY=P('/etc/siemcore-cascade-updater/recovery-policy.json')
def require(x,msg):
 if not x:raise ValueError(msg)
def trusted(p):
 for x in (p,*p.parents):
  s=x.lstat();require(not stat.S_ISLNK(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,'unsafe root path')
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def load(name,path):
 spec=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
args=argparse.ArgumentParser();args.add_argument('receipt');args.add_argument('--install-testing-policy',action='store_true');opts=args.parse_args()
require(os.geteuid()==0,'root required')
require(subprocess.run(['systemctl','is-active','siemcore-cascade-updater'],capture_output=True,text=True).stdout.strip()=='inactive','updater must be inactive')
rp=P(opts.receipt);trusted(rp);r=json.loads(rp.read_text())
require(r['version']=='3.3.152.50' and r['sha256']==SHA,'wrong candidate')
entry=dict(artifact='/var/lib/siemcore-cascade-updater/artifacts/siemcore-3.3.152.50.artifact',sha256=SHA,signature=r['signature'])
legacy=False
require(socket.gethostname()!='ip-172-26-13-54','A/B preflight only')
oldraw=POLICY.read_bytes() if POLICY.exists() else None
if legacy:
 trusted(POLICY);require(digest(POLICY)==BASELINE,'baseline policy drift')
 old=json.loads(oldraw);p=copy.deepcopy(old)
 require(p['policy_revision']==8 and '3.3.152.50' not in p['releases'],'unexpected policy')
 p['policy_revision']=9;p['allowed_transitions'].append(['3.3.152.39','3.3.152.50']);p['releases']['3.3.152.50']=entry
 p['expected_mounts_by_version']['3.3.152.50']=copy.deepcopy(old['expected_mounts_by_version']['3.3.152.39'])
 before='3.3.152.39';runtime=P('/usr/local/lib/siemcore-recovery/1.0.0.4')
else:
 require(not opts.install_testing_policy,'cannot install explicit policy on fresh Normal')
 require(socket.gethostname() in ('bezeq-pod-test-a.me-west1-a.c.osherad-graylog.internal','bezeq-pod-test-b.me-west1-b.c.osherad-graylog.internal'),'wrong host')
 runtime=P('/usr/local/lib/siemcore-cascade/recovery')
 hookpath=P('/usr/local/lib/siemcore-cascade/greenfield-hook.py');trusted(hookpath);hook=load('preflight_greenfield',hookpath);require(digest(hookpath)=='329e855866399e59e62219fcbd7d8b59087b472f60253c6ae95fe43e8ccedea8','root prerequisite missing')
 bootstrap=hook.protected(hook.POLICY);app=hook.protected(P('/etc/siemcore/greenfield.json'))
 require(not hook.pod_installation(app) and hook.completed_greenfield(),'not completed Normal')
 before,previous=hook.receipt(hook.CURRENT,bootstrap);require(before=='3.3.152.47','wrong predecessor')
 marker=hook.protected(hook.APP_DIR/'.node.json');require(marker['role']=='app' and marker['node_id']=='app-a','wrong app identity')
 p=dict(schema=1,hostname=socket.gethostname(),install_dir=str(hook.APP_DIR),cascade_root='/opt/siemcore-cascade',journal_dir='/var/lib/siemcore-recovery',cluster_id=marker['cluster_id'],node_id='app-a',project_name='siemcore-app-app-a',containers={'siemcore':'siemcore-app-a','siemcore-archiver':'siemcore-archiver-app-a'},allow_dynamic_archiver_debug_port=True,health_url='http://127.0.0.1:8443/health',dashboard_dir=None,public_key=bootstrap['public_key'],allowed_transitions=[[before,'3.3.152.50']],apply_timeout=480,health_timeout=120,releases={before:previous,'3.3.152.50':entry})
require(p['public_key']==r['public_key']=='1f1aa11a80d6ac549a26bb25daac4798c42dd469138680e68f89832bd32e7f57','trust mismatch')
for name in ('recovery.py','artifact.py','supervise.py'):trusted(runtime/name)
sys.path.insert(0,str(runtime));import recovery
recovery.DEADLINE=time.monotonic()+780
store=P('/var/lib/siemcore-recovery');trusted(store)
with open(store/'lock','a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB);recovery.LOCK_FD=lock.fileno()
 journal=store/'transaction.json';beforejournal=journal.read_bytes() if journal.exists() else None
 if beforejournal:require(json.loads(beforejournal)['stage'] in ('applied','restored'),'unfinished operation')
 with tempfile.TemporaryDirectory(dir=store,prefix='setup50-preflight-') as temp:
  tx=recovery.Lifecycle(dict(p,journal_dir=temp)).snapshot('3.3.152.50')
  require(tx['from_version']==before,'predecessor changed')
 require((journal.read_bytes() if journal.exists() else None)==beforejournal,'active journal changed')
 require((POLICY.read_bytes() if POLICY.exists() else None)==oldraw,'policy changed')
 candidate=json.dumps(p,sort_keys=True,indent=2).encode()+b'\n'
 if opts.install_testing_policy:
  history=store/'policy-provisioning';history.mkdir(mode=0o700,exist_ok=True);trusted(history)
  recovery.durable(history/(BASELINE+'.json'),oldraw)
  recovery.durable(history/(hashlib.sha256(candidate).hexdigest()+'.json'),candidate)
  recovery.durable(POLICY,candidate)
 print(json.dumps(dict(host=socket.gethostname(),from_version=before,target_version='3.3.152.50',preflight='passed',active_journal_unchanged=True,policy_installed=opts.install_testing_policy,candidate_sha256=hashlib.sha256(candidate).hexdigest(),runtime_sha256={n:digest(runtime/n) for n in ('recovery.py','artifact.py','supervise.py')})))
