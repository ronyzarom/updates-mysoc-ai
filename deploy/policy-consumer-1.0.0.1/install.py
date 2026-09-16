#!/usr/bin/env python3
"""One-time root provisioning. Never stops services, changes sudo, or applies policy."""
import hashlib,json,os,pathlib,socket,stat,subprocess,tempfile
P=pathlib.Path;BASE=P(__file__).resolve().parent
DEST=P('/usr/local/lib/siemcore-policy-consumer/1.0.0.1');WRAPPER=P('/usr/local/sbin/siemcore-apply-update')
POLICY=P('/etc/siemcore-cascade-updater/recovery-policy.json')
BASELINE='d1319690e289fa3a02e81357263c9900de946d6ef56d6f7fa462e5180423f709'
def require(ok,msg):
    if not ok:raise ValueError(msg)
def sha(raw):return hashlib.sha256(raw).hexdigest()
def trusted(p):
    for x in [p,*p.parents]:
        s=x.lstat();require(not stat.S_ISLNK(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,'untrusted install path')
def atomic(p,raw,mode):
    fd,tmp=tempfile.mkstemp(dir=p.parent)
    try:
        with os.fdopen(fd,'wb') as f:f.write(raw);os.fchmod(f.fileno(),mode);f.flush();os.fsync(f.fileno())
        os.replace(tmp,p)
        fd=os.open(p.parent,os.O_RDONLY);os.fsync(fd);os.close(fd)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)
def main():
    require(os.geteuid()==0,'root provisioning required');require(socket.gethostname()=='ip-172-26-13-54','testing host only')
    trusted(BASE);trusted(POLICY);trusted(WRAPPER)
    require(sha(POLICY.read_bytes())==BASELINE,'baseline policy changed; re-review required')
    state=subprocess.run(['systemctl','show','-p','ActiveState','--value','siemcore-cascade-updater.service'],capture_output=True,text=True,check=True).stdout.strip()
    require(state=='inactive','updater must already be stopped under approved hold')
    tx=P('/var/lib/siemcore-recovery/transaction.json')
    if tx.exists():require(json.loads(tx.read_text())['stage'] in ('applied','restored'),'unfinished recovery')
    manifest=json.loads((BASE/'files.json').read_text())
    for name,want in manifest.items():
        require('/' not in name and name!='files.json','invalid package manifest')
        require(sha((BASE/name).read_bytes())==want,'package checksum mismatch')
    import importlib.util
    spec=importlib.util.spec_from_file_location('consumer',BASE/'consumer.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
    for name,want in m.MODULES.items():trusted(m.RUNTIME/name);require(sha((m.RUNTIME/name).read_bytes())==want,'installed runtime drift')
    existing=WRAPPER.read_bytes();new=(BASE/'wrapper').read_bytes()
    require(existing==new or sha(existing)=='8a824d02fc805c7890030691e66f729fa979832a595a9db12830bbfd21326bfa','unexpected wrapper')
    DEST.mkdir(parents=True,mode=0o755,exist_ok=True);trusted(DEST)
    history=P('/var/lib/siemcore-recovery/policy-consumer-provisioning');history.mkdir(mode=0o700,exist_ok=True);trusted(history)
    old=history/(sha(existing)+'.wrapper')
    if not old.exists():atomic(old,existing,0o600)
    atomic(DEST/'consumer.py',(BASE/'consumer.py').read_bytes(),0o755)
    atomic(WRAPPER,new,0o755)
    print('Consumer provisioned; runtime/policy/sudo unchanged. Updater remains stopped; holds unchanged.')
if __name__=='__main__':main()
