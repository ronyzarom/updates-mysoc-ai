import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

COMPONENT=Path(__file__).resolve().parents[2]/'deploy/node-standalone-v1'
sys.path.insert(0,str(COMPONENT))
spec=importlib.util.spec_from_file_location('standalone_prerequisite',COMPONENT/'prerequisite_install.py')
installer=importlib.util.module_from_spec(spec);spec.loader.exec_module(installer)
from source_loader import SourceLoader
from configuration_inventory import configuration_digest
from scripts.tests import test_standalone_transaction as fixtures


class PrerequisiteTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name);self.loader=SourceLoader(self.root,os.geteuid())
        self.identity=dict(machine_id='a'*32,installation_id='node-a',updater_instance_id='updater-a',node_id='1',vm_id='1')
        self.app=dict({k:v for k,v in self.identity.items() if k!='vm_id'},schema=5,topology='node-unlinked')
        for name in ('ROOT','INPUTS','CONFIG','POLICY','WRAPPER','SUDO','KIT','COMPONENT_ROOT'):
            relative={'ROOT':'var/lib/siemcore-node-standalone','INPUTS':'etc/siemcore-cascade-updater/standalone-inputs','CONFIG':'etc/siemcore-cascade-updater/config.yaml','POLICY':'etc/siemcore-cascade-updater/node-standalone-policy.json','WRAPPER':'usr/local/sbin/siemcore-node-standalone','SUDO':'etc/sudoers.d/siemcore-node-standalone','KIT':'kit','COMPONENT_ROOT':'usr/local/libexec/siemcore-node-standalone'}[name]
            value=self.root/relative;value.parent.mkdir(parents=True,exist_ok=True)
            self.addCleanup(patch.stopall);patch.object(installer,name,value).start()
        put=installer.put
        patch.object(installer,'put',lambda path,raw,mode=0o600,uid=0,gid=0:put(path,raw,mode,os.geteuid(),os.getegid())).start()
        self.write('/etc/machine-id','a'*32)
        self.write('/etc/siemcore/greenfield.json',json.dumps(self.app))
        self.write('/etc/siemcore/greenfield-release.json',json.dumps(dict(version='3.3.152.41',sha256='b'*64,signature='fixture',public_key='a'*64)))
        self.write('/var/lib/siemcore-greenfield/journal.json',json.dumps(dict(status='complete',installation_state='installed-unlinked',policy_sha256=hashlib.sha256(json.dumps(self.app,sort_keys=True).encode()).hexdigest())))
        self.write('/etc/siemcore-cascade-updater/config.yaml','self_update:\n  channel: stable\nproducts:\n  - name: siemcore\n    server_type: pod-node\n    channel: fixture\nsimulation:\n  filesystem:\n    independent_node_update: true\n')
        self.write('/opt/siemcore-node-unlinked-1/settings.json',json.dumps(dict(database='siemcore',postgres_uid=os.geteuid())))
        for name,value in [('credentials/application','fixture-password'),('credentials/redis','fixture-redis'),('management.crt','fixture-certificate'),('management.key','fixture-tls-key')]:self.write('/opt/siemcore-node-unlinked-1/'+name,value)
        self.package=dict(source_version='3.3.152.41',target_version='3.3.152.42',kit_version='fixture-r1',minimum_updater_version='1.16.1.31',product_channel='fixture',standalone_product_channel='fixture-standalone',public_environment={'SIEMCORE_FRONTEND_URL':'https://fixture.invalid'})
        original_check_output=installer.subprocess.check_output
        def observed(args,**kwargs):
            if args[:2]==['docker','inspect']:
                return json.dumps([dict(Id='fixture-'+args[-1],Image='sha256:'+'a'*64,State={'Running':True},Mounts=[])]).encode()
            return original_check_output(args,**kwargs)
        patch.object(installer.subprocess,'check_output',side_effect=observed).start()

    def write(self,name,raw):
        path=self.loader.path(name);path.parent.mkdir(parents=True,exist_ok=True)
        path.write_text(raw);path.chmod(0o600)

    def test_enrollment_retry_retains_key_and_never_outputs_secrets(self):
        first=installer.enroll(self.loader,self.package,self.app,self.identity)
        key=(installer.ROOT/'recipient/key.pem').read_bytes()
        jwt=(installer.ROOT/'recipient/jwt.secret').read_bytes()
        second=installer.enroll(self.loader,self.package,self.app,self.identity)
        self.assertEqual(first,second)
        self.assertEqual((installer.ROOT/'recipient/key.pem').read_bytes(),key)
        self.assertEqual((installer.ROOT/'recipient/jwt.secret').read_bytes(),jwt)
        output=json.dumps(first)
        for secret in ('fixture-password','fixture-redis','fixture-tls-key',jwt.decode(),'PRIVATE KEY'):self.assertNotIn(secret,output)

    def test_interrupted_install_reconciles_exact_files_and_successful_replay(self):
        installer.enroll(self.loader,self.package,self.app,self.identity)
        component=installer.KIT/'component';component.mkdir(parents=True)
        (component/'cli.py').write_text('# fixture')
        (component/'COMPONENT.json').write_text('{"fixture":true}')
        (installer.KIT/'PACKAGE.json').write_text(json.dumps(self.package))
        capsule=installer.KIT/'capsule';capsule.mkdir()
        for name,value in [('manifest.json','{}'),('inputs.cms','cipherfixture'),('manifest.sig','signaturefixture')]: (capsule/name).write_text(value)
        fixture=fixtures.StandaloneAdmissionTests();fixture.setUp()
        binding=fixture.binding
        files={'mysoc-bootstrap.json':b'{"fixture":true}','archive-credentials/gcp-archiver.json':b'{"fixture":true}'}
        hashes={str(p.relative_to(installer.INPUTS)):hashlib.sha256(p.read_bytes()).hexdigest() for p in installer.INPUTS.rglob('*') if p.is_file()}
        hashes.update({k:hashlib.sha256(v).hexdigest() for k,v in files.items()})
        binding['configuration_sha256']=hashlib.sha256(json.dumps(hashes,sort_keys=True,separators=(',',':')).encode()).hexdigest()
        policy={'binding':binding,'configuration_metadata':{k:dict(uid=os.geteuid(),gid=os.getegid(),mode=0o600) for k in hashes},'component_manifest_sha256':hashlib.sha256((component/'COMPONENT.json').read_bytes()).hexdigest()}
        with patch.object(installer,'decrypt_capsule',return_value=(files,policy)),patch.object(installer,'run',return_value='updater-simulator 1.16.1.31'):
            realput=installer.put
            def interrupted(path,*args,**kwargs):
                if path==installer.SUDO:raise OSError('fixture power loss')
                return realput(path,*args,**kwargs)
            with patch.object(installer,'put',side_effect=interrupted):
                with self.assertRaises(OSError):installer.install(self.loader,self.package,self.identity)
            self.assertIn('independent_node_update: true',installer.CONFIG.read_text())
            first=installer.install(self.loader,self.package,self.identity)
            second=installer.install(self.loader,self.package,self.identity)
            self.assertEqual(first,second)
            self.assertFalse(first['product_execution'])
            self.assertIn('independent_node_standalone: true',installer.CONFIG.read_text())
            self.assertIn('readiness',installer.SUDO.read_text())
            self.assertNotIn('ALL=(root) NOPASSWD: ALL',installer.SUDO.read_text())


if __name__=='__main__':unittest.main()
