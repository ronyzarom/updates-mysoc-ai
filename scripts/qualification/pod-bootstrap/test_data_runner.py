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
        with patch.object(r,'mount_path',side_effect=lambda p:p),patch.object(r,'docker_json',side_effect=self.inspect):
            name,argv=r.prepare('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,
                                lambda image:checks.append(image),lambda:checks.append('runtime'))
        self.assertEqual(checks,[IMAGE,'runtime'])
        self.assertIn('--pull=never',argv);self.assertIn('--read-only',argv)
        self.assertNotIn('--privileged',argv);self.assertNotIn('/var/run/docker.sock',str(argv))
        self.assertEqual(argv[-4:],[IMAGE,'pod-bootstrap-data','--config','/run/bootstrap/data.json'])
        self.assertEqual(argv[argv.index('--entrypoint')+1],'/app/siemcore')
        self.assertEqual(sum(a.endswith(',readonly') for a in argv),3)
    def test_verification_failure_prevents_container_execution(self):
        calls=[]
        def reject(image):raise ValueError('unverified')
        runner=r.Runner('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,reject,lambda:None)
        with patch.object(r,'mount_path',side_effect=lambda p:p),patch.object(r,'bounded',side_effect=lambda *args:calls.append(args)),self.assertRaises(ValueError):
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
            with patch.object(r,'mount_path',side_effect=lambda p:p),patch.object(r,'docker_json',side_effect=inspect),self.assertRaises(ValueError):
                r.prepare('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,lambda image:None,lambda:None)
    def test_timeout_stops_daemon_process_and_retains_container(self):
        commands=[];states=iter([[dict(State=dict(Running=True))],[dict(State=dict(Running=False))]])
        def bounded(argv,*args):
            commands.append(argv)
            if argv==['docker','run']:raise TimeoutError()
            return 0,b''
        runner=r.Runner('/usr/bin/docker',IMAGE,NETWORK,MOUNTS,lambda image:None,lambda:None)
        with patch.object(r,'prepare',return_value=('fixture',['docker','run'])),patch.object(r,'bounded',side_effect=bounded),patch.object(r,'docker_json',side_effect=lambda *args:next(states)),self.assertRaises(TimeoutError):
            runner(r.COMMAND)
        self.assertEqual(commands,[['docker','run'],['/usr/bin/docker','kill','fixture']])
    def test_real_process_bounds_output_and_wall_time(self):
        with self.assertRaises(ValueError):r.bounded([sys.executable,'-c','print("x"*9000)'],2,8192)
        with self.assertRaises(TimeoutError):r.bounded([sys.executable,'-c','import time; time.sleep(10)'],0.05)
    def test_real_process_returns_only_stdout_and_exit_status(self):
        code,out=r.bounded([sys.executable,'-c','import sys; print("receipt"); print("private",file=sys.stderr)'],2)
        self.assertEqual(code,0);self.assertEqual(out,b'receipt\n')
