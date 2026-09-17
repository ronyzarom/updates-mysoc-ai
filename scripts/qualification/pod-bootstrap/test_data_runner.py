import sys
import unittest
from unittest.mock import patch
import data_runner as r

IMAGE='sha256:'+'a'*64
NETWORK='b'*64
MOUNTS={p:'/protected'+p for p in r.MOUNTS}

class RunnerTests(unittest.TestCase):
    def inspect(self,docker,args):
        if args[0]=='image':return [dict(Id=IMAGE,Os='linux',Config={})]
        if args[0]=='network':return [dict(Id=NETWORK,Internal=True,Driver='bridge')]
        return [dict(State=dict(Running=False))]
    def test_fixed_command_no_pull_or_host_socket_and_readonly_mounts(self):
        checks=[]
        with patch.object(r,'mount_path',side_effect=lambda p,**kw:p),patch.object(r,'docker_json',side_effect=self.inspect):
            name,argv=r.prepare('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,
                                lambda image:checks.append(image),lambda:checks.append('runtime'),lambda:999)
        self.assertEqual(checks,[IMAGE,'runtime'])
        self.assertIn('--pull=never',argv);self.assertIn('--read-only',argv)
        self.assertNotIn('--privileged',argv);self.assertNotIn('/var/run/docker.sock',str(argv))
        self.assertEqual(argv[-4:],[IMAGE,'pod-bootstrap-data','--config','/run/bootstrap/data.json'])
        self.assertEqual(argv[argv.index('--entrypoint')+1],'/app/siemcore')
        self.assertEqual(sum(a.endswith(',readonly') for a in argv),3)
    def test_verification_failure_prevents_container_execution(self):
        calls=[]
        def reject(image):raise ValueError('unverified')
        runner=r.Runner('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,reject,lambda:None,lambda:999)
        with patch.object(r,'mount_path',side_effect=lambda p,**kw:p),patch.object(r,'bounded',side_effect=lambda *args:calls.append(args)),self.assertRaises(ValueError):
            runner(r.COMMAND)
        self.assertEqual(calls,[])
    def test_noninternal_network_and_implicit_volumes_rejected(self):
        for kind in ('network','image'):
            def inspect(docker,args):
                result=self.inspect(docker,args)
                if args[0]==kind:
                    if kind=='network':result[0]['Internal']=False
                    else:result[0]['Config']['Volumes']={'/data':{}}
                return result
            with patch.object(r,'mount_path',side_effect=lambda p,**kw:p),patch.object(r,'docker_json',side_effect=inspect),self.assertRaises(ValueError):
                r.prepare('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,lambda image:None,lambda:None,lambda:999)
    def test_timeout_stops_daemon_process_and_retains_container(self):
        commands=[];states=iter([[dict(State=dict(Running=True))],[dict(State=dict(Running=False))]])
        def bounded(argv,*args):
            commands.append(argv)
            if argv==['docker','run']:raise TimeoutError()
            return 0,b''
        runner=r.Runner('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,lambda image:None,lambda:None,lambda:999)
        with patch.object(r,'prepare',return_value=('fixture',['docker','run'])),patch.object(r,'bounded',side_effect=bounded),patch.object(r,'docker_json',side_effect=lambda *args:next(states)),self.assertRaises(TimeoutError):
            runner(r.COMMAND)
        self.assertEqual(commands,[['docker','run'],['/usr/bin/docker','kill','fixture']])
    def test_real_process_bounds_output_and_wall_time(self):
        with self.assertRaises(ValueError):r.bounded([sys.executable,'-c','print("x"*9000)'],2,8192)
        with self.assertRaises(TimeoutError):r.bounded([sys.executable,'-c','import time; time.sleep(10)'],0.05)
    def test_real_process_returns_only_stdout_and_exit_status(self):
        code,out=r.bounded([sys.executable,'-c','import sys; print("receipt"); print("private",file=sys.stderr)'],2)
        self.assertEqual(code,0);self.assertEqual(out,b'receipt\n')

    def socket_fixture(self,mode=0o1775,uid=999,socket_mode=None,extra=False,parent_writable=False):
        import stat
        from pathlib import Path
        from types import SimpleNamespace
        leaf=Path('/qualification/socket')
        socket=leaf/'.s.PGSQL.5432'
        def info(path):
            if path==leaf:return SimpleNamespace(st_mode=stat.S_IFDIR|mode,st_uid=uid,st_nlink=2)
            if path==socket:return SimpleNamespace(st_mode=socket_mode or (stat.S_IFSOCK|0o777),st_uid=999,st_nlink=1)
            return SimpleNamespace(st_mode=stat.S_IFDIR|(0o777 if parent_writable else 0o755),st_uid=0,st_nlink=2)
        return leaf,info,[socket,leaf/'unrelated'] if extra else [socket]
    def test_verified_socket_leaf_modes_are_narrowly_accepted(self):
        from pathlib import Path
        for mode in (0o1775,0o3775):
            leaf,info,children=self.socket_fixture(mode=mode)
            with patch.object(Path,'lstat',info),patch.object(Path,'iterdir',return_value=iter(children)):
                self.assertEqual(r.mount_path(str(leaf),socket_uid=999),str(leaf))
    def test_socket_exception_rejects_wrong_owner_mode_symlink_peer_and_parent(self):
        import stat
        from pathlib import Path
        for settings in ({'uid':1000},{'mode':0o777},{'socket_mode':stat.S_IFLNK|0o777},
                         {'extra':True},{'parent_writable':True}):
            leaf,info,children=self.socket_fixture(**settings)
            with self.subTest(settings=settings),patch.object(Path,'lstat',info),patch.object(Path,'iterdir',return_value=iter(children)),self.assertRaises(ValueError):
                r.mount_path(str(leaf),socket_uid=999)
    def test_socket_exception_does_not_relax_other_mounts(self):
        from pathlib import Path
        leaf,info,children=self.socket_fixture()
        with patch.object(Path,'lstat',info),self.assertRaises(ValueError):r.mount_path(str(leaf))

    def test_qualified_peer_namespace_requires_exact_image_network_and_ip(self):
        target='c'*64;pgimage='sha256:'+'d'*64
        namespace=dict(container_id=target,image_id=pgimage,network_id=NETWORK,ip_address='172.30.97.12')
        def inspect(docker,args):
            if args[0]=='container':return [dict(Id=target,Image=pgimage,State=dict(Running=True),
                NetworkSettings=dict(Networks={'peer':dict(NetworkID=NETWORK,IPAddress='172.30.97.12')}))]
            return self.inspect(docker,args)
        with patch.object(r,'mount_path',side_effect=lambda p,**kw:p),patch.object(r,'docker_json',side_effect=inspect):
            _,argv=r.prepare('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,lambda image:None,lambda:None,lambda:999,namespace)
            self.assertEqual(argv[argv.index('--network')+1],'container:'+target)
            for field,value in [('container_id','e'*64),('image_id','sha256:'+'f'*64),('network_id','f'*64),('ip_address','172.30.97.3')]:
                changed=dict(namespace);changed[field]=value
                with self.subTest(field=field),self.assertRaises(ValueError):
                    r.prepare('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,lambda image:None,lambda:None,lambda:999,changed)
