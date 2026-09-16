#!/usr/bin/env python3
"""Synthetic protocol responder ONLY. It installs no product and proves no health."""
import base64,json,os,pathlib,sys,tempfile,time,datetime
root=pathlib.Path(sys.argv[1]);action=sys.argv[2];q=json.load(sys.stdin)
if not os.environ.get('POD_QUALIFICATION_TEST_ID'):raise SystemExit('test driver required')
root.mkdir(parents=True,exist_ok=True,mode=0o700);path=root/'reference-state.json'
s=json.loads(path.read_text()) if path.exists() else {'phase':'paused','calls':[]}
s['calls'].append(action)
b=q.get('binding');generation=q.get('generation') or 7
expiry=(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(seconds=50)).isoformat().replace('+00:00','Z')
if action=='readiness':
    r=dict(protocol='pod-maintenance-readiness-v1',pod_id=q['pod_id'],node_id=q['node_id'],updater_id=q['updater_id'],capabilities=[],ready=False,observer_verified=False,credentials_ready=False,lifecycle_ready=False,issued_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),valid_until=expiry)
elif action=='authorize-next-operation':
    c=json.loads(base64.b64decode(q['authorization']['payload_base64']));r=dict(protocol=q['protocol'],authorization_id=c['authorization_id'],previous_binding=c['previous_binding'],previous_generation=c['previous_generation'],previous_outcome=c['previous_outcome'],next_binding=c['next_binding'],allowed=True,valid_until=c['expires_at'])
else:
    if s.get('operation_id')!=b['operation_id']:s.update(operation_id=b['operation_id'],phase='paused')
    if action=='complete':s['phase']='completed'
    r=dict(binding=b,generation=generation,phase=s['phase'],permission_expires=expiry)
    artifact=q.get('artifact',{});version=artifact.get('version',b['target_version']);sha=artifact.get('sha256',b['artifact_sha256'])
    if 'protocol' in q:
        c=json.loads(base64.b64decode(q['authorization']['payload_base64']));r.update(protocol=q['protocol'],outcome=q['outcome'],authorization_id=c['authorization_id'],permission_expires=c['expires_at'] if action!='acceptance' else expiry)
    h=dict(version=version,sha256=sha,role='STBY',management_ready=True,processing_disabled=True,traffic_disabled=True,authority_valid=True,replication_status='ready')
    if action=='capabilities':r['capabilities']=[] if (root/'unsupported').exists() else ['pod-maintenance-v1','pod-maintenance-recovery-v2']
    if action=='health':r['health']=h
    if action=='acceptance':r['acceptance']=dict(health=h,active_ip_owned=False,service_healthy=True,processing_authority_valid=False)
fd,tmp=tempfile.mkstemp(dir=root);os.write(fd,json.dumps(s).encode());os.fsync(fd);os.close(fd);os.replace(tmp,path)
d=os.open(root,os.O_RDONLY);os.fsync(d);os.close(d);json.dump(r,sys.stdout)
