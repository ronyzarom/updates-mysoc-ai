from pathlib import Path
import json,secrets,subprocess
root=Path('/etc/normal-qualification');root.mkdir(mode=0o700);c=root/'credentials';c.mkdir(mode=0o700)
def run(a):subprocess.run(a,check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(c/'server.key'),'-out',str(c/'server.crt'),'-days','2','-subj','/CN=normal.fixture','-addext','subjectAltName=DNS:normal.fixture,DNS:mysoc.fixture'])
(c/'ca.crt').write_bytes((c/'server.crt').read_bytes());Path('/etc/hosts').open('a').write('\n127.0.0.1 normal.fixture mysoc.fixture\n')
a=dict(schema=1,topology='single',cluster_id='normal-fixture',instance_id='fixture-customer',updater_instance_id='fixture-normal-updater',database_name='siemcore',frontend_url='https://normal.fixture',mysoc_url='https://mysoc.fixture',admin_email='fixture@example.invalid',mysoc_api_key=secrets.token_hex(32))
r=json.loads(Path('/root/product-receipt.json').read_text());r={k:r[k] for k in ('version','sha256','public_key','signature')};r['channel']='normal-a-20260919'
(root/'envelope.json').write_text(json.dumps(dict(application=a,release=r)));(root/'envelope.json').chmod(0o600)
print('Prepared synthetic Normal schema1 baseline; GCS schema3 integration remains separate')
