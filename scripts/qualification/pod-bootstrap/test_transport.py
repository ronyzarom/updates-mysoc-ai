"""Real loopback mTLS tests; no product installation or cloud access."""
from datetime import datetime, timedelta, timezone
import hashlib
import http.server
from pathlib import Path
import ssl
import tempfile
import threading
import unittest
from unittest.mock import patch
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID, ExtendedKeyUsageOID
import client as c

class TransportTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp=tempfile.TemporaryDirectory();cls.root=Path(cls.temp.name)
        def issue(name,ca=None,server=False):
            key=rsa.generate_private_key(public_exponent=65537,key_size=2048)
            subject=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,name)])
            builder=(x509.CertificateBuilder().subject_name(subject).issuer_name(ca[1].subject if ca else subject)
                .public_key(key.public_key()).serial_number(x509.random_serial_number())
                .not_valid_before(datetime.now(timezone.utc)-timedelta(minutes=1))
                .not_valid_after(datetime.now(timezone.utc)+timedelta(hours=1))
                .add_extension(x509.BasicConstraints(ca=ca is None,path_length=None),critical=True))
            builder=builder.add_extension(x509.SubjectKeyIdentifier.from_public_key(key.public_key()),critical=False)
            builder=builder.add_extension(x509.AuthorityKeyIdentifier.from_issuer_public_key((ca[0] if ca else key).public_key()),critical=False)
            builder=builder.add_extension(x509.KeyUsage(digital_signature=True,content_commitment=False,key_encipherment=False,data_encipherment=False,key_agreement=False,key_cert_sign=ca is None,crl_sign=ca is None,encipher_only=None,decipher_only=None),critical=True)
            if ca:
                builder=builder.add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH if server else ExtendedKeyUsageOID.CLIENT_AUTH]),critical=False)
            if server:builder=builder.add_extension(x509.SubjectAlternativeName([x509.DNSName('localhost')]),critical=False)
            cert=builder.sign(ca[0] if ca else key,hashes.SHA256())
            (cls.root/(name+'.pem')).write_bytes(cert.public_bytes(serialization.Encoding.PEM))
            (cls.root/(name+'.key')).write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
            return key,cert
        ca=issue('ca');server=issue('localhost',ca,True);issue('client',ca);issue('other-ca')
        cls.pin=hashlib.sha256(server[1].public_bytes(serialization.Encoding.DER)).hexdigest()
        class Handler(http.server.BaseHTTPRequestHandler):
            def handle(self):
                try:super().handle()
                except ConnectionResetError:pass  # Expected on pre-request pin rejection.
            def do_POST(self):
                self.server.seen.append(self.path)
                self.rfile.read(int(self.headers['Content-Length']))
                self.send_response(self.server.status)
                self.send_header('Content-Length',str(len(self.server.body)))
                self.send_header('Location','https://example.invalid/never-follow')
                self.end_headers();self.wfile.write(self.server.body)
            def log_message(self,*args):pass
        cls.server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler)
        context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);context.minimum_version=ssl.TLSVersion.TLSv1_3
        context.load_cert_chain(cls.root/'localhost.pem',cls.root/'localhost.key')
        context.load_verify_locations(cls.root/'ca.pem');context.verify_mode=ssl.CERT_REQUIRED
        cls.server.socket=context.wrap_socket(cls.server.socket,server_side=True)
        cls.thread=threading.Thread(target=cls.server.serve_forever,daemon=True);cls.thread.start()
    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown();cls.server.server_close();cls.thread.join();cls.temp.cleanup()
    def setUp(self):
        self.server.seen=[];self.server.status=200;self.server.body=b'{}'
        self.config=dict(endpoint='https://localhost:'+str(self.server.server_port),
            ca_file=str(self.root/'ca.pem'),client_cert_file=str(self.root/'client.pem'),
            client_key_file=str(self.root/'client.key'),certificate_sha256=self.pin)
    def transport(self):
        # Root-only path ownership is tested separately; TLS itself is never mocked.
        with patch.object(c,'protected',side_effect=lambda path,**kw:Path(path).read_bytes()):
            return c.Transport(self.config)
    def test_authenticated_pinned_tls_sends_request(self):
        self.assertEqual(self.transport().call('status',{},2),b'{}')
        self.assertEqual(self.server.seen,['/v1/bootstrap/status'])
    def test_pin_mismatch_sends_no_http(self):
        self.config['certificate_sha256']='0'*64
        with self.assertRaises(ValueError):self.transport().call('status',{},2)
        self.assertEqual(self.server.seen,[])
    def test_untrusted_ca_sends_no_http(self):
        self.config['ca_file']=str(self.root/'other-ca.pem')
        with self.assertRaises(ssl.SSLError):self.transport().call('status',{},2)
        self.assertEqual(self.server.seen,[])
    def test_hostname_mismatch_sends_no_http(self):
        self.config['endpoint']=self.config['endpoint'].replace('localhost','127.0.0.1')
        with self.assertRaises(ssl.SSLError):self.transport().call('status',{},2)
        self.assertEqual(self.server.seen,[])
    def test_missing_client_certificate_rejected_by_server(self):
        transport=self.transport()
        transport.context=ssl.create_default_context(cafile=self.config['ca_file'])
        with self.assertRaises((ssl.SSLError,OSError,c.http.client.HTTPException)):transport.call('status',{},2)
        self.assertEqual(self.server.seen,[])
    def test_redirect_rejected_without_following(self):
        self.server.status=307
        with self.assertRaises(c.Blocked):self.transport().call('status',{},2)
        self.assertEqual(len(self.server.seen),1)
    def test_oversized_response_rejected(self):
        self.server.body=b'x'*(c.LIMIT+1)
        with self.assertRaises(ValueError):self.transport().call('status',{},2)
    def test_only_exact_pending_error_is_retryable(self):
        self.server.status=409;self.server.body=c.PENDING+b'\n'
        with self.assertRaises(c.RegistrationsPending):self.transport().call('all-registered',{},2)
        with self.assertRaises(c.Blocked) as caught:self.transport().call('status',{},2)
        self.assertNotIsInstance(caught.exception,c.RegistrationsPending)
        self.server.body=b'bootstrap blocked: stale operation'
        with self.assertRaises(c.Blocked) as caught:self.transport().call('all-registered',{},2)
        self.assertNotIsInstance(caught.exception,c.RegistrationsPending)

if __name__=='__main__':unittest.main()
