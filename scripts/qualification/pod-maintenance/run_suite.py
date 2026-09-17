#!/usr/bin/env python3
"""Run an explicitly configured isolated product-adapter suite; never deploy."""
import argparse,hashlib,json,os,pathlib,signal,subprocess,time
from product_fixture import validate_plan, validate_case
p=argparse.ArgumentParser();p.add_argument('--plan',required=True);p.add_argument('--driver',required=True);args=p.parse_args()
plan=json.loads(pathlib.Path(args.plan).read_text());tier=plan.get('evidence_tier')
if not plan.get('isolated') or tier not in ('component','native-with-synthetic-host','executable-real-host'):
    raise SystemExit('explicit isolated plan/evidence tier required')
if plan.get('adapter_kind')=='reference' and tier!='component':raise SystemExit('reference adapter cannot qualify native/product evidence')
validate_plan(plan)
root=pathlib.Path(plan['evidence_directory']);root.mkdir(parents=True,exist_ok=False,mode=0o700);os.umask(0o077)
def run(argv,path,timeout=60):
    with path.open('wb') as log:return subprocess.run(argv,stdout=log,stderr=subprocess.STDOUT,timeout=timeout).returncode
def digest(path):return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()
summary={'evidence_tier':tier,'adapter_kind':plan.get('adapter_kind'),'driver_sha256':digest(args.driver),'plan_sha256':digest(args.plan),'deployment_qualified':False,'product_binaries':plan.get('product_binaries',{}),'scenarios':[]}
try:
 for index,scenario in enumerate(plan['scenarios']):
    case=root/f'{index:03d}';case.mkdir(mode=0o700);receipt={'name':scenario['name'],'runs':[]};process=None;launch_log=None
    try:
      if scenario.get('reset_command') and run(scenario['reset_command'],case/'reset.log')!=0:raise RuntimeError('reset failed')
      if scenario.get('launch_command'):
        launch_log=(case/'launch.log').open('wb');process=subprocess.Popen(scenario['launch_command'],stdout=launch_log,stderr=subprocess.STDOUT,start_new_session=True)
      if scenario.get('ready_command'):
        until=time.monotonic()+30
        while run(scenario['ready_command'],case/'ready.log',10)!=0:
          if time.monotonic()>until:raise RuntimeError('isolated service did not become ready')
          time.sleep(.2)
      for number,step in enumerate(scenario['runs']):
        config=pathlib.Path(step['config']);c=json.loads(config.read_text())
        if c.get('evidence_tier')!=tier or not c.get('isolated'):raise RuntimeError('driver tier/context mismatch')
        proxy_data=None
        if step.get('proxy_config'):
          proxy_data=json.loads(pathlib.Path(step['proxy_config']).read_text())
          if str(pathlib.Path(step['proxy_config'])) not in c.get('adapter_command',[]):raise RuntimeError('proxy config is not invoked by driver')
        product_pins=validate_case(plan,c,proxy_data)
        if 'fault' in step:
          proxy=pathlib.Path(step['proxy_config']);pc=json.loads(proxy.read_text());pc['fault']=step['fault'];proxy.write_text(json.dumps(pc))
        result=run([args.driver,'--config',str(config)],case/f'run-{number:03d}.log',step.get('timeout_seconds',60))
        passed=(result==0)==(step['expected']=='success');entry={'returncode':result,'expected':step['expected'],'matched':passed,'config_sha256':digest(config),'product_binaries':product_pins,'journals':{}}
        for name in ('operation.json','drain-v1.json','recovery-v2.json','next-operation.json'):
          path=pathlib.Path(c['directory'])/name
          if path.exists():
            raw=path.read_bytes();(case/f'run-{number:03d}-{name}').write_bytes(raw);entry['journals'][name]=hashlib.sha256(raw).hexdigest()
        entry['archives']={}
        for archived in (pathlib.Path(c['directory'])/'archive').glob('*/*.json'):
          if archived.is_symlink() or archived.name not in ('operation.json','drain-v1.json','recovery-v2.json','transition.json'):continue
          rel=str(archived.relative_to(pathlib.Path(c['directory'])));entry['archives'][rel]=digest(archived)
        receipt['runs'].append(entry)
        if not passed:raise RuntimeError('coordinator result did not match scenario')
      if scenario.get('verify_command') and run(scenario['verify_command'],case/'product-verification.log')!=0:raise RuntimeError('product verification failed')
      receipt['passed']=True
    except Exception as e:receipt.update(passed=False,error=str(e))
    finally:
      if scenario.get('stop_command'):
        try:
          receipt['stop_returncode']=run(scenario['stop_command'],case/'stop.log')
          if receipt['stop_returncode']!=0:receipt['passed']=False
        except Exception as e:receipt['stop_error']=str(e);receipt['passed']=False
      if process:
        try:os.killpg(process.pid,signal.SIGTERM);process.wait(timeout=5)
        except ProcessLookupError:pass
        except subprocess.TimeoutExpired:os.killpg(process.pid,signal.SIGKILL);process.wait()
      if launch_log:launch_log.close()
      summary['scenarios'].append(receipt);(root/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
finally:
 summary['suite_passed']=all(x.get('passed') for x in summary['scenarios']) and len(summary['scenarios'])==len(plan['scenarios'])
 (root/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps({'summary':str(root/'summary.json'),'suite_passed':summary['suite_passed'],'deployment_qualified':False}));raise SystemExit(0 if summary['suite_passed'] else 1)
