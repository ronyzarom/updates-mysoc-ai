#!/usr/bin/env python3
"""Isolated test-only command proxy. Never put this in a live adapter path."""
import argparse, fcntl, hashlib, json, os, pathlib, signal, subprocess, sys, tempfile, time
p=argparse.ArgumentParser();p.add_argument('--config',required=True);p.add_argument('action');a=p.parse_args()
c=json.loads(pathlib.Path(a.config).read_text());root=pathlib.Path(c['evidence_directory'])
if not c.get('isolated') or not os.environ.get('POD_QUALIFICATION_TEST_ID'):
    raise SystemExit('qualification driver context required')
root.mkdir(parents=True,exist_ok=True,mode=0o700);os.umask(0o077)
def atomic(path,data):
    fd,tmp=tempfile.mkstemp(dir=root);os.write(fd,data);os.fsync(fd);os.close(fd);os.replace(tmp,path)
    d=os.open(root,os.O_RDONLY);os.fsync(d);os.close(d)
with (root/'proxy.lock').open('a+') as lock:
    fcntl.flock(lock,fcntl.LOCK_EX)
    counts=json.loads((root/'counts.json').read_text()) if (root/'counts.json').exists() else {}
    n=counts.get(a.action,0)+1;counts[a.action]=n;atomic(root/'counts.json',json.dumps(counts).encode())
raw=sys.stdin.buffer.read(65537)
if len(raw)>65536:raise SystemExit('oversized test request')
prefix=f'{a.action}-{n:03d}';atomic(root/(prefix+'-request.json'),raw)
fault=c.get('fault',{});selected=fault.get('action')==a.action and fault.get('occurrence',1)==n
# Atomic count is persisted before a fault, so restarting cannot re-inject it.
def inject(point):
    if not selected or fault.get('point')!=point:return
    atomic(root/(prefix+'-fault.json'),json.dumps(dict(point=point,effect=fault['effect'])).encode())
    if fault['effect']=='lost-response':raise SystemExit(75)
    if fault['effect']=='crash':
        parent=os.getppid()
        if str(parent)!=os.environ.get('POD_QUALIFICATION_DRIVER_PID'):raise SystemExit('parent identity mismatch')
        os.kill(parent,signal.SIGKILL);raise SystemExit(76)
    if fault['effect']=='timeout':time.sleep(65);raise SystemExit(77)
    raise SystemExit('unknown test fault')
inject('before')
result=subprocess.run(c['adapter_command']+[a.action],input=raw,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=c.get('adapter_timeout_seconds',55))
atomic(root/(prefix+'-response.json'),result.stdout);atomic(root/(prefix+'-stderr.txt'),result.stderr)
atomic(root/(prefix+'-receipt.json'),json.dumps({'returncode':result.returncode,'request_sha256':hashlib.sha256(raw).hexdigest(),'response_sha256':hashlib.sha256(result.stdout).hexdigest(),'response_bytes':len(result.stdout)}).encode())
inject('after')
sys.stdout.buffer.write(result.stdout);sys.stderr.buffer.write(result.stderr);raise SystemExit(result.returncode)
