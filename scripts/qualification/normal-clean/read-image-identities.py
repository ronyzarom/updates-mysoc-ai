import subprocess,json,hashlib
roles={'etcd':'quay.io/coreos/etcd:v3.5.12','redis':'redis:7.2-alpine','nginx':'nginx:1.27-alpine','patroni':'siemcore/patroni-pg16:normal-prereq-20260919'}
def content(digest):
 raw=subprocess.check_output(['ctr','--namespace','moby','content','get',digest]);assert 'sha256:'+hashlib.sha256(raw).hexdigest()==digest;return json.loads(raw)
results={}
for role,reference in roles.items():
 info=json.loads(subprocess.check_output(['docker','image','inspect',reference]))[0];original=info['Descriptor']['digest'];descriptor=original;manifest=content(descriptor)
 if 'manifests' in manifest:
  choices=[x for x in manifest['manifests'] if x.get('platform',{}).get('os')=='linux' and x.get('platform',{}).get('architecture')=='amd64'];assert len(choices)==1;descriptor=choices[0]['digest'];manifest=content(descriptor)
 config_digest=manifest['config']['digest'];config=content(config_digest);assert config['os']=='linux' and config['architecture']=='amd64'
 results[role]=dict(reference=reference,runtime_image_id=info['Id'],descriptor_digest=original,platform_manifest_digest=descriptor,config_digest=config_digest,content_hashes_verified=True,architecture='linux/amd64')
print(json.dumps(results,indent=2))
