"""Additional tests inside the synthetic standalone fixture only."""
import gzip
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import uuid


def command(args, **kwargs):
    return subprocess.run(args, capture_output=True, timeout=90, **kwargs)


def sql(statement):
    result=command(['docker','exec','-i','siemcore-unlinked-1-postgres','sh','-c',
         'export PGPASSWORD="$(cat /run/secrets/application)"; exec psql -X -qAt -v ON_ERROR_STOP=1 -h /var/run/postgresql -U siemcore -d siemcore'], input=statement.encode())
    if result.returncode:raise RuntimeError('fixture SQL failed: '+result.stderr.decode()[-1500:])
    return result.stdout.decode().strip()


def main():
    if os.environ.get('INDEPENDENT_NODE_FIXTURE')!='1':raise SystemExit('disposable fixture only')
    q=json.loads(Path('/root/standalone-request.json').read_text())
    if q['binding']['source']['installation_id']!='fixture-node-1':raise SystemExit('synthetic identity required')
    sys.path.insert(0,q['target_bundle']+'/updater')
    import pod_node_standalone as product
    worker=product.Worker(q);worker.invariants();worker.configuration()
    before=Path('/var/lib/siemcore-greenfield/journal.json').read_bytes()
    data_ids=sql("SELECT count(*) FROM mysoc_platform_config;")
    assert data_ids=='1'
    worker.database_preflight()  # exact existing configuration retry
    assert sql("SELECT count(*) FROM mysoc_platform_config;")=='1'
    config=json.loads(worker.material['mysoc-bootstrap.json'])
    changed=dict(config,endpoint_url='https://conflict.fixture')
    admin=['docker','exec','-i','siemcore-standalone-1-app','/app/siemcore-admin','standalone-config']
    assert command(admin,input=json.dumps(changed).encode()).returncode!=0
    # Unknown synthetic provider cannot perform a paid call; fixture has no
    # external network. Verify guard, then remove it before restarting processing.
    worker.compose('stop','--timeout','60','application')
    sql("INSERT INTO llm_config(provider,enabled,api_key) VALUES('fixture-never-call',true,NULL);")
    try:
        try:worker.database_preflight()
        except subprocess.CalledProcessError:pass
        else:raise AssertionError('enabled AI guard missing')
    finally:sql("DELETE FROM llm_config WHERE provider='fixture-never-call';")
    assert sql('SELECT count(*) FROM llm_config WHERE enabled;')=='0'
    tenant='10000000-0000-0000-0000-000000000001'
    sql("INSERT INTO tenants(tenant_id,name) VALUES('"+tenant+"','standalone synthetic') ON CONFLICT DO NOTHING; INSERT INTO tenant_ip_mappings(tenant_id,cidr,label,enabled) VALUES('"+tenant+"','127.0.0.1/32','standalone fixture',true);")
    worker.compose('start','application')
    for attempt in range(40):
        try:worker.health();break
        except Exception:
            if attempt==39:raise
            time.sleep(2)
    marker='standalone-native-'+uuid.uuid4().hex
    from datetime import datetime,timezone
    event=('<134>1 '+datetime.now(timezone.utc).isoformat()+' fixture-host standalone - - - '+marker+'\n').encode()
    sent=command(['docker','exec','-i','siemcore-standalone-1-app','curl','--silent','--max-time','2','--upload-file','-','telnet://127.0.0.1:1514'],input=event)
    if sent.returncode not in (0,28):raise RuntimeError('synthetic event send failed')
    time.sleep(5)
    raw_count=sql("SELECT count(*) FROM raw_logs WHERE tenant_id='"+tenant+"' AND raw_message LIKE '%"+marker+"%';")
    worker.compose('stop','--timeout','60','application')
    for attempt in range(60):
        manifest=sql("SELECT coalesce(json_agg(t)::text,'[]') FROM (SELECT checksum,row_count,uploaded_at,s3_key,local_path,cloud_bucket FROM archive_manifest WHERE tenant_id='"+tenant+"') t;")
        rows=json.loads(manifest)
        if any(row.get('uploaded_at') for row in rows):break
        time.sleep(2)
    else:raise RuntimeError('synthetic archive manifest not uploaded')
    matched=[]
    for path in Path('/opt/siemcore-node-standalone-1/state/archives').rglob('*'):
        if not path.is_file():continue
        raw=path.read_bytes()
        try:decoded=gzip.decompress(raw)
        except (OSError,EOFError):decoded=raw
        if marker.encode() in decoded:
            checksum=hashlib.sha256(raw).hexdigest()
            if any(row.get('checksum')==checksum for row in rows):matched.append(str(path))
    if not matched:raise RuntimeError('archive bytes/checksum/retrieval not verified')
    worker.compose('start','application')
    for attempt in range(40):
        try:health=worker.health();break
        except Exception:
            if attempt==39:raise
            time.sleep(2)
    assert Path('/var/lib/siemcore-greenfield/journal.json').read_bytes()==before
    report=dict(status='pipeline-and-guards-passed',raw_log_rows=int(raw_count),synthetic_event_archived=True,
                archive_checksum_verified=True,archive_retrieval_verified=True,paid_ai_guard_verified=True,
                mysoc_exact_retry_verified=True,mysoc_conflict_refused=True,bootstrap_bytes_preserved=True,health=health)
    Path('/root/standalone-pipeline-result.json').write_text(json.dumps(report,indent=2))
    print('PASS synthetic event archive/retrieval, configuration guards, retained bootstrap and restarted application')


if __name__=='__main__':main()
