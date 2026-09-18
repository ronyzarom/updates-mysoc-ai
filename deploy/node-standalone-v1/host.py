"""Concrete root measurements/staging; no service or database mutation here."""
import hashlib
import os
from pathlib import Path
import shutil
import ssl
import subprocess
import urllib.request
from urllib.parse import urlsplit
from adapter import atomic_json
from artifacts import ArtifactStager
from configuration_inventory import configuration_digest
from protocol import strict_json
from source_loader import SourceLoader
from transaction import admit, digest

POLICY = '/etc/siemcore-cascade-updater/node-standalone-policy.json'
INPUTS = '/etc/siemcore-cascade-updater/standalone-inputs'
ROOT = Path('/var/lib/siemcore-node-standalone')


def canonical_data_identity(value):
    if not isinstance(value,dict) or set(value)!={'postgres','redis'}:
        raise ValueError('exact_data_identity_required')
    result={}
    for role,record in value.items():
        if not isinstance(record,dict) or set(record)!={'id','image','mounts'} or not isinstance(record['mounts'],list):
            raise ValueError('exact_data_record_required')
        mounts=[];destinations=set()
        for mount in record['mounts']:
            if not isinstance(mount,dict) or set(mount)!={'Type','Source','Destination','RW'} or type(mount['RW']) is not bool or any(not isinstance(mount[k],str) for k in ('Type','Source','Destination')):
                raise ValueError('exact_data_mount_required')
            if mount['Destination'] in destinations:raise ValueError('duplicate_data_mount_destination')
            destinations.add(mount['Destination'])
            mounts.append((mount['Destination'],mount['Type'],mount['Source'],mount['RW']))
        result[role]={'id':record['id'],'image':record['image'],'mounts':sorted(mounts)}
    return result


class Host:
    def __init__(self, directory, pipeline_verifier=None):
        self.loader = SourceLoader()
        self.protected = self.loader.protected
        self.directory = self.protected(Path(directory))
        self.policy = strict_json(self.loader.read(POLICY))
        required = {'protocol', 'enabled', 'binding', 'historical_inventory', 'source_artifact',
                    'target_artifact', 'configuration_metadata', 'data_identity', 'customer_url',
                    'component_manifest_sha256'}
        if set(self.policy) not in (required,required|{'previous_standalone'}) or self.policy['protocol'] != 'pod-node-standalone-v1' or self.policy['enabled'] is not True:
            raise ValueError('standalone_capability_disabled')
        if self.directory != ROOT/'operations'/self.policy['binding']['operation_id']:
            raise ValueError('fixed_operation_directory_required')
        self.loader.expected_operation=self.policy['binding']['operation_id']
        self.loader.previous_standalone=self.policy.get('previous_standalone')
        self.loader.successor_binding=self.policy['binding']
        if ('previous_operation' in self.policy['binding']) != ('previous_standalone' in self.policy):raise ValueError('successor_policy_required')
        url = urlsplit(self.policy['customer_url'])
        if url.scheme != 'https' or not url.hostname or url.username or url.password or url.port not in (None,443) or url.query or url.fragment or url.path not in ('','/'):
            raise ValueError('fixed_https_customer_origin_required')
        qualification=strict_json(self.protected(Path(__file__).with_name('QUALIFICATION.json')).read_bytes())
        required_tests=('synthetic_event_archived','archive_retrieval_verified','archive_checksum_verified',
                        'paid_ai_guard_verified','mysoc_exact_retry_verified','mysoc_conflict_refused',
                        'interrupted_recovery_verified','bootstrap_bytes_preserved','data_identity_preserved')
        if qualification.get('target_artifact_sha256')!=self.policy['binding']['target']['artifact_sha256'] or any(qualification.get(k) is not True for k in required_tests):
            raise ValueError('native_qualification_missing')
        self.pipeline_verifier = pipeline_verifier or self.runtime_pipeline

    def data_identity(self):
        result = {}
        node = self.policy['binding']['source']['node_id']
        for role in ('postgres', 'redis'):
            raw = subprocess.check_output(['/usr/bin/docker','inspect','--type','container','siemcore-unlinked-'+node+'-'+role], timeout=15)
            record = strict_json(raw)[0]
            if record['State']['Running'] is not True:
                raise ValueError('retained_data_service_unavailable')
            result[role] = {'id':record['Id'], 'image':record['Image'],
                            'mounts':[{k:m.get(k) for k in ('Type','Source','Destination','RW')} for m in record['Mounts']]}
        return result

    def admit(self, binding):
        evidence, bootstrap, release = self.loader.measure(self.policy['historical_inventory'])
        self.release = release
        if canonical_data_identity(self.data_identity()) != canonical_data_identity(self.policy['data_identity']):
            raise ValueError('retained_data_identity_changed')
        self.protected(Path(INPUTS))
        config_sha = configuration_digest(INPUTS, self.policy['configuration_metadata'])
        return admit(binding, self.policy['binding'], evidence, bootstrap, config_sha)

    def stage(self, binding):
        stager = ArtifactStager(self.directory, '/var/lib/siemcore-cascade-updater/artifacts', bytes.fromhex(self.release['public_key']), self.protected)
        source = self.policy['source_artifact']; target = self.policy['target_artifact']
        if (source['version'], source['artifact_sha256']) != (binding['source_version'], binding['source_artifact_sha256']):
            raise ValueError('source_policy_mismatch')
        if any(target.get(k) != v for k,v in binding['target'].items() if k not in ('product','architecture')):
            raise ValueError('target_policy_mismatch')
        predecessor, previous_manifest = stager.stage_one(source, 'source')
        destination, manifest = stager.stage_one(target, 'target')
        if 'pod-node-standalone-v1' not in manifest.get('pod_node_capabilities', []):
            raise ValueError('signed_standalone_capability_missing')
        previous_schema = previous_manifest.get('schema', {})
        schema = manifest.get('schema', {})
        if type(previous_schema.get('to')) is not int or schema.get('from') != previous_schema['to'] or schema.get('to') != previous_schema['to']:
            raise ValueError('first_transition_requires_same_database_schema')
        config = self.directory/'configuration'
        if not config.exists():
            staging = self.directory/'.configuration-staging'
            if staging.exists():
                self.protected(staging); shutil.rmtree(staging)
            staging.mkdir(mode=0o700)
            for name, metadata in self.policy['configuration_metadata'].items():
                path = staging/name; path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
                with open(Path(INPUTS)/name, 'rb') as src, path.open('xb') as dst:
                    shutil.copyfileobj(src, dst); dst.flush(); os.fsync(dst.fileno())
                os.chown(path, metadata['uid'], metadata['gid']); os.chmod(path, metadata['mode'])
            if configuration_digest(staging, self.policy['configuration_metadata']) != binding['configuration_sha256']:
                raise ValueError('configuration_copy_mismatch')
            os.rename(staging,config)
            fd=os.open(self.directory,os.O_RDONLY|os.O_DIRECTORY)
            try: os.fsync(fd)
            finally: os.close(fd)
        if configuration_digest(config, self.policy['configuration_metadata']) != binding['configuration_sha256']:
            raise ValueError('staged_configuration_changed')
        self.manifest = manifest
        return dict(target_bundle=str(destination), predecessor_bundle=str(predecessor))

    def _https(self, endpoint):
        hostname=urlsplit(self.policy['customer_url']).hostname
        node=self.policy['binding']['source']['node_id']
        raw=subprocess.check_output(['/usr/bin/docker','exec','siemcore-standalone-'+node+'-app',
             'curl','--fail','--silent','--show-error','--max-time','8','--resolve',hostname+':8443:127.0.0.1',
             'https://'+hostname+':8443'+endpoint],timeout=12)
        return strict_json(raw)

    def verify_accepted(self, binding):
        if canonical_data_identity(self.data_identity()) != canonical_data_identity(self.policy['data_identity']):
            raise ValueError('retained_data_identity_changed')
        live, ready = self._https('/health/live'), self._https('/health/ready')
        expected = dict(status='alive',version=binding['target']['version'],instance_id=binding['instance_id'],
                        installation_mode='independent-standalone',installation_id=binding['source']['installation_id'],
                        updater_id=binding['source']['updater_instance_id'],node_id=binding['source']['node_id'],pod_authority_enabled=False)
        if any(type(live.get(k)) is not type(v) or live[k] != v for k,v in expected.items()):
            raise ValueError('standalone_live_identity_mismatch')
        if ready.get('status') != 'healthy' or ready.get('version') != binding['target']['version'] or ready.get('instance_id') != binding['instance_id'] or ready.get('git_commit') != binding['target']['source_commit']:
            raise ValueError('customer_readiness_failed')
        if any(ready.get('checks',{}).get(k,{}).get('ok') is not True for k in ('postgres','redis')):
            raise ValueError('data_readiness_failed')
        if self.pipeline_verifier is None:
            raise ValueError('native_pipeline_verifier_not_qualified')
        pipeline = self.pipeline_verifier(binding)
        if not isinstance(pipeline,dict) or any(pipeline.get(k) is not True for k in ('archive_ready','pipeline_verified','paid_ai_disabled','customer_attribution_verified')):
            raise ValueError('native_pipeline_acceptance_failed')
        return dict(live=live,ready=ready,pipeline=pipeline)

    def persist_effective_mode(self, binding, evidence):
        atomic_json(self.directory/'accepted-mode.json',dict(protocol=binding['protocol'],operation_sha256=digest(binding),
                    installation_identity=binding['source'],effective_mode='independent-standalone',
                    version=binding['target']['version'],routine_updates_allowed=False,health=evidence))

    def runtime_pipeline(self,binding):
        node=binding['source']['node_id'];prefix='siemcore-standalone-'+node
        records=strict_json(subprocess.check_output(['/usr/bin/docker','inspect',prefix+'-app',prefix+'-archiver'],timeout=15))
        if len(records)!=2 or any(record['Image']!=self.manifest['runtime_image_id'] or record['State']['Running'] is not True or record['State'].get('Health',{}).get('Status')!='healthy' for record in records):
            raise ValueError('standalone_runtime_image_or_health_mismatch')
        binary=subprocess.check_output(['/usr/bin/docker','exec',prefix+'-app','sha256sum','/app/siemcore'],timeout=15).decode().split()[0]
        if binary!=binding['target']['binary_sha256']:raise ValueError('running_binary_mismatch')
        syslog=self._https('/health/syslog')
        archive=strict_json(subprocess.check_output(['/usr/bin/docker','exec',prefix+'-archiver','curl','--fail','--silent','--max-time','8','http://127.0.0.1:8444/health/archive'],timeout=12))
        if syslog.get('enabled') is not True or archive.get('status') not in ('healthy','ok'):
            raise ValueError('ingest_or_archive_not_ready')
        platform=strict_json(self.protected(Path(INPUTS)/'mysoc-bootstrap.json').read_bytes())
        if platform['instance_id']!=binding['instance_id']:raise ValueError('customer_configuration_identity_mismatch')
        def literal(value):return "'"+str(value).replace("'","''")+"'"
        api_digest=hashlib.sha256(platform['api_key'].encode()).hexdigest()
        sql=("SELECT json_build_object('paid_ai_disabled',(SELECT count(*)=0 FROM llm_config WHERE enabled),"
             "'customer_attribution_verified',(SELECT count(*)=1 AND count(*) FILTER (WHERE enabled AND siemcore_instance="+literal(platform['instance_id'])+
             " AND mysoc_endpoint_url="+literal(platform['endpoint_url'])+" AND heartbeat_interval_seconds="+str(int(platform['heartbeat_seconds']))+
             " AND encode(sha256(convert_to(platform_api_key,'UTF8')),'hex')="+literal(api_digest)+")=1 FROM mysoc_platform_config));")
        result=subprocess.run(['/usr/bin/docker','exec','-i','siemcore-unlinked-'+node+'-postgres','sh','-c',
             'export PGPASSWORD="$(cat /run/secrets/application)"; exec psql -X -qAt -v ON_ERROR_STOP=1 -h /var/run/postgresql -U siemcore -d "$1"',
             'standalone-readonly-health',strict_json(self.loader.read('/opt/siemcore-node-unlinked-'+node+'/settings.json'))['database']],
             input=sql.encode(),capture_output=True,timeout=20)
        if result.returncode:raise ValueError('readonly_application_policy_check_failed')
        checks=strict_json(result.stdout)
        return dict(archive_ready=True,pipeline_verified=True,paid_ai_disabled=checks.get('paid_ai_disabled'),
                    customer_attribution_verified=checks.get('customer_attribution_verified'),
                    evidence_scope='runtime-readiness; synthetic pipeline qualified separately',binary_sha256=binary)

    def verify_restored(self, binding):
        if canonical_data_identity(self.data_identity()) != canonical_data_identity(self.policy['data_identity']):
            raise ValueError('restored_data_identity_changed')
        # Restored management has its original dedicated hostname, not the new
        # customer routing address or standalone container.
        app=strict_json(self.loader.read('/etc/siemcore/greenfield.json'))
        with urllib.request.urlopen('https://'+app['management']['hostname']+'/health/live',context=ssl.create_default_context(),timeout=10) as response:
            health=strict_json(response.read(65537))
        expected = dict(version=binding['source_version'],installation_id=binding['source']['installation_id'],
                        updater_id=binding['source']['updater_instance_id'],node_id=binding['source']['node_id'],
                        installation_state='installed-unlinked',processing_enabled=False,authority_enabled=False,data_ready=True)
        if any(type(health.get(k)) is not type(v) or health[k] != v for k,v in expected.items()):
            raise ValueError('management_recovery_not_verified')
