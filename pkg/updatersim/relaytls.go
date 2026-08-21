package updatersim

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	relayCertFilename = "cert.pem"
	relayKeyFilename  = "key.pem"

	relayCertificateCommonName  = "mysoc-cascade-relay"
	relayCertificateLifetime    = 10 * 365 * 24 * time.Hour
	relayCertificateMinValidity = 30 * 24 * time.Hour
)

// ensureRelayTLS returns the TLS material the relay listener serves. Operator
// material (relay.tls.cert_file/key_file) is used verbatim when configured;
// otherwise a self-signed certificate is provisioned under relay.tls.dir and
// reused across restarts while it still covers every required host and has
// more than 30 days of validity left.
//
// The self-provisioned cert.pem is also the CA file operators distribute to
// child updaters (server.ca_file) so children can verify this relay.
func ensureRelayTLS(cfg *Config, logger *slog.Logger) (certFile, keyFile string, err error) {
	if cfg.Relay.TLS.CertFile != "" && cfg.Relay.TLS.KeyFile != "" {
		return cfg.Relay.TLS.CertFile, cfg.Relay.TLS.KeyFile, nil
	}

	hosts, err := relayTLSHosts(cfg.Relay.TLS.Hosts)
	if err != nil {
		return "", "", fmt.Errorf("determine relay TLS hosts: %w", err)
	}

	if err := os.MkdirAll(cfg.Relay.TLS.Dir, 0700); err != nil {
		return "", "", fmt.Errorf("create relay TLS directory: %w", err)
	}

	certFile = filepath.Join(cfg.Relay.TLS.Dir, relayCertFilename)
	keyFile = filepath.Join(cfg.Relay.TLS.Dir, relayKeyFilename)

	reusable, err := reusableRelayTLS(certFile, keyFile, hosts, time.Now())
	if err != nil {
		return "", "", fmt.Errorf("check existing relay TLS material: %w", err)
	}
	if reusable {
		logger.Info("reusing relay TLS certificate",
			"cert", certFile, "sans", hosts.all)
		return certFile, keyFile, nil
	}

	logger.Info("generating relay TLS certificate",
		"cert", certFile, "sans", hosts.all)
	if err := generateRelayTLS(certFile, keyFile, hosts, time.Now()); err != nil {
		return "", "", fmt.Errorf("generate relay TLS material: %w", err)
	}
	return certFile, keyFile, nil
}

// relayTLSHostSet holds the SANs a relay certificate must cover, split the
// way x509 wants them.
type relayTLSHostSet struct {
	dnsNames []string
	ipAddrs  []net.IP
	all      []string
}

// relayTLSHosts builds the SAN set: the OS hostname, loopback names, and
// every operator-configured host, deduplicated and classified as IP or DNS.
func relayTLSHosts(configured []string) (relayTLSHostSet, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return relayTLSHostSet{}, fmt.Errorf("read hostname: %w", err)
	}

	rawHosts := make([]string, 0, 4+len(configured))
	rawHosts = append(rawHosts, hostname, "localhost", "127.0.0.1", "::1")
	rawHosts = append(rawHosts, configured...)

	var hosts relayTLSHostSet
	seen := make(map[string]struct{}, len(rawHosts))
	for _, rawHost := range rawHosts {
		host := strings.TrimSpace(rawHost)
		if host == "" {
			return relayTLSHostSet{}, errors.New("relay.tls.hosts entries must not be empty")
		}
		if ip := net.ParseIP(host); ip != nil {
			canonical := ip.String()
			if _, ok := seen[canonical]; ok {
				continue
			}
			seen[canonical] = struct{}{}
			hosts.ipAddrs = append(hosts.ipAddrs, ip)
			hosts.all = append(hosts.all, canonical)
			continue
		}
		key := strings.ToLower(host)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		hosts.dnsNames = append(hosts.dnsNames, host)
		hosts.all = append(hosts.all, host)
	}
	return hosts, nil
}

// reusableRelayTLS reports whether existing on-disk material is a loadable
// keypair that still covers every required host with comfortable validity.
// Any defect means "regenerate", never an error: the material is ours to
// replace.
func reusableRelayTLS(
	certFile, keyFile string,
	hosts relayTLSHostSet,
	now time.Time,
) (bool, error) {
	if _, err := os.Stat(certFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat relay certificate: %w", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat relay key: %w", err)
	}

	keyPair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil || len(keyPair.Certificate) == 0 {
		return false, nil
	}
	cert, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return false, nil
	}
	if now.Before(cert.NotBefore) ||
		!cert.NotAfter.After(now.Add(relayCertificateMinValidity)) {
		return false, nil
	}
	for _, host := range hosts.all {
		if err := cert.VerifyHostname(host); err != nil {
			return false, nil
		}
	}
	return true, nil
}

// generateRelayTLS writes a fresh self-signed serving certificate. IsCA is
// set so the certificate itself works as the root children pin.
func generateRelayTLS(
	certFile, keyFile string,
	hosts relayTLSHostSet,
	now time.Time,
) error {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate relay key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate certificate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: relayCertificateCommonName},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(relayCertificateLifetime),
		DNSNames:     hosts.dnsNames,
		IPAddresses:  hosts.ipAddrs,
		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageKeyEncipherment |
			x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create relay certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("encode relay key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	if err := writeFileAtomic(keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("write relay key: %w", err)
	}
	if err := writeFileAtomic(certFile, certPEM, 0644); err != nil {
		return fmt.Errorf("write relay certificate: %w", err)
	}
	return nil
}

// writeFileAtomic writes data to path via a temp file + rename so a crash
// never leaves partial key material behind.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("set temp file mode: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
