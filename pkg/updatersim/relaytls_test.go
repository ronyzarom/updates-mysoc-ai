package updatersim

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func relayTLSTestConfig(t *testing.T, hosts ...string) *Config {
	t.Helper()
	cfg := &Config{}
	cfg.Relay.Enabled = true
	cfg.Relay.TLS.Dir = t.TempDir()
	cfg.Relay.TLS.Hosts = hosts
	return cfg
}

func parseCertFile(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	pemData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatalf("cert file %s is not PEM", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert
}

func TestEnsureRelayTLSGeneratesCoveringCertificate(t *testing.T) {
	cfg := relayTLSTestConfig(t, "10.0.0.5", "relay.internal")

	certFile, keyFile, err := ensureRelayTLS(cfg, discardLogger())
	if err != nil {
		t.Fatalf("ensureRelayTLS: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("generated material does not load as a keypair: %v", err)
	}

	cert := parseCertFile(t, certFile)
	hostname, _ := os.Hostname()
	for _, host := range []string{hostname, "localhost", "127.0.0.1", "::1", "10.0.0.5", "relay.internal"} {
		if err := cert.VerifyHostname(host); err != nil {
			t.Errorf("certificate does not cover %q: %v", host, err)
		}
	}
	if !cert.IsCA {
		t.Error("certificate must be a CA so children can pin it as a root")
	}
	if cert.NotAfter.Before(time.Now().Add(9 * 365 * 24 * time.Hour)) {
		t.Errorf("certificate lifetime too short: NotAfter=%s", cert.NotAfter)
	}

	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if mode := keyInfo.Mode().Perm(); mode != 0600 {
		t.Errorf("key file mode = %o, want 0600", mode)
	}
}

func TestEnsureRelayTLSReusesAndRegenerates(t *testing.T) {
	cfg := relayTLSTestConfig(t, "10.0.0.5")

	certFile, _, err := ensureRelayTLS(cfg, discardLogger())
	if err != nil {
		t.Fatalf("first ensureRelayTLS: %v", err)
	}
	first, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read first cert: %v", err)
	}

	// Same host set: material must be reused byte-for-byte.
	if _, _, err := ensureRelayTLS(cfg, discardLogger()); err != nil {
		t.Fatalf("second ensureRelayTLS: %v", err)
	}
	second, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("read second cert: %v", err)
	}
	if string(first) != string(second) {
		t.Error("certificate was regenerated although the host set did not change")
	}

	// A new host must force regeneration with the extended SAN set.
	cfg.Relay.TLS.Hosts = append(cfg.Relay.TLS.Hosts, "new-name.internal")
	if _, _, err := ensureRelayTLS(cfg, discardLogger()); err != nil {
		t.Fatalf("third ensureRelayTLS: %v", err)
	}
	cert := parseCertFile(t, certFile)
	if err := cert.VerifyHostname("new-name.internal"); err != nil {
		t.Errorf("regenerated certificate does not cover the added host: %v", err)
	}
}

func TestEnsureRelayTLSPrefersOperatorMaterial(t *testing.T) {
	cfg := relayTLSTestConfig(t)
	cfg.Relay.TLS.CertFile = "/etc/pki/relay.crt"
	cfg.Relay.TLS.KeyFile = "/etc/pki/relay.key"

	certFile, keyFile, err := ensureRelayTLS(cfg, discardLogger())
	if err != nil {
		t.Fatalf("ensureRelayTLS: %v", err)
	}
	if certFile != cfg.Relay.TLS.CertFile || keyFile != cfg.Relay.TLS.KeyFile {
		t.Errorf("got (%s, %s), want the operator-provided paths untouched", certFile, keyFile)
	}
}

// TestClientPinsRelayCA proves the full transport contract: a client with
// server.ca_file pointed at the relay's self-provisioned cert.pem verifies
// the TLS connection, and a client without it refuses to talk.
func TestClientPinsRelayCA(t *testing.T) {
	cfg := relayTLSTestConfig(t)
	certFile, keyFile, err := ensureRelayTLS(cfg, discardLogger())
	if err != nil {
		t.Fatalf("ensureRelayTLS: %v", err)
	}
	keyPair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("load keypair: %v", err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"product_name":"swf","version":"1.0.0"}`))
		}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{keyPair}}
	server.StartTLS()
	defer server.Close()

	timeout := Duration{Duration: 5 * time.Second}

	pinned, err := NewClient(ServerConfig{
		URL:              server.URL,
		Timeout:          timeout,
		MaxResponseBytes: defaultMaxResponseBytes,
		CAFile:           certFile,
	})
	if err != nil {
		t.Fatalf("NewClient with ca_file: %v", err)
	}
	release, err := pinned.GetReleaseMeta(context.Background(), "swf", "1.0.0")
	if err != nil {
		t.Fatalf("pinned client failed to fetch over TLS: %v", err)
	}
	if release.ProductName != "swf" {
		t.Errorf("unexpected release payload: %+v", release)
	}

	unpinned, err := NewClient(ServerConfig{
		URL:              server.URL,
		Timeout:          timeout,
		MaxResponseBytes: defaultMaxResponseBytes,
	})
	if err != nil {
		t.Fatalf("NewClient without ca_file: %v", err)
	}
	if _, err := unpinned.GetReleaseMeta(context.Background(), "swf", "1.0.0"); err == nil {
		t.Fatal("client without ca_file accepted the self-signed relay certificate")
	} else if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("expected a certificate verification error, got: %v", err)
	}
}
