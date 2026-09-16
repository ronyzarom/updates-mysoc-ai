package policyauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (Payload, Expected, ed25519.PublicKey, ed25519.PrivateKey, time.Time) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	policy := []byte(`{"policy_revision":3}`)
	sum := sha256.Sum256(policy)
	p := Payload{Protocol: Protocol, Hostname: "testing-fixture", Product: "siemcore", OldPolicySHA256: hex.EncodeToString(sum[:]), PolicyRevision: 4, FromVersion: "3.3.152.26", TargetVersion: "3.3.152.32", ArtifactSHA256: strings.Repeat("a", 64), ArtifactSignature: base64.StdEncoding.EncodeToString(make([]byte, 64)), SourceCommit: strings.Repeat("b", 40), IssuedAt: now.Unix() - 1, ExpiresAt: now.Unix() + 300}
	e := Expected{p.Hostname, p.FromVersion, p.TargetVersion, p.ArtifactSHA256, p.ArtifactSignature, p.SourceCommit, policy, 3}
	return p, e, pub, priv, now
}
func TestVerifyAndPythonConsumerCompatibility(t *testing.T) {
	p, w, pub, priv, now := fixture(t)
	e, err := Sign(p, priv)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	if _, err = Verify(raw, pub, w, now); err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available for consumer compatibility")
	}
	path, err := filepath.Abs("../../deploy/policy-consumer-1.0.0.1/consumer.py")
	if err != nil {
		t.Fatal(err)
	}
	script := "import importlib.util,json,sys\ns=importlib.util.spec_from_file_location('consumer',sys.argv[1]);m=importlib.util.module_from_spec(s);s.loader.exec_module(m)\ne=json.load(sys.stdin);m.verify(sys.argv[2],e['payload'],e['signature'])\n"
	cmd := exec.Command(python, "-c", script, path, hex.EncodeToString(pub))
	cmd.Stdin = strings.NewReader(string(raw))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("root consumer rejected Go signature: %v %s", err, out)
	}
}
func TestRejectsTamperReplayAndWrongScope(t *testing.T) {
	p, w, pub, priv, now := fixture(t)
	for name, change := range map[string]func(*Payload){"host": func(p *Payload) { p.Hostname = "other" }, "product": func(p *Payload) { p.Product = "mysoc" }, "policy": func(p *Payload) { p.OldPolicySHA256 = strings.Repeat("c", 64) }, "revision": func(p *Payload) { p.PolicyRevision++ }, "target": func(p *Payload) { p.TargetVersion = "3.3.152.25" }, "commit": func(p *Payload) { p.SourceCommit = strings.Repeat("c", 40) }, "artifact": func(p *Payload) { p.ArtifactSHA256 = strings.Repeat("c", 64) }, "expired": func(p *Payload) { p.ExpiresAt = now.Unix() }, "future": func(p *Payload) { p.IssuedAt = now.Unix() + 1 }, "overlong": func(p *Payload) { p.ExpiresAt = p.IssuedAt + 3601 }} {
		t.Run(name, func(t *testing.T) {
			bad := p
			change(&bad)
			e, _ := Sign(bad, priv)
			raw, _ := json.Marshal(e)
			if _, err := Verify(raw, pub, w, now); err == nil {
				t.Fatal("accepted wrong scope")
			}
		})
	}
	e, _ := Sign(p, priv)
	e.Payload.ExpiresAt--
	raw, _ := json.Marshal(e)
	if _, err := Verify(raw, pub, w, now); err == nil {
		t.Fatal("accepted tampered signature")
	}
	e, _ = Sign(p, priv)
	raw, _ = json.Marshal(e)
	for _, bad := range []string{strings.Replace(string(raw), `"hostname":`, `"hostname":"duplicate","hostname":`, 1), strings.Replace(string(raw), `"payload":{`, `"payload":{"command":"sh",`, 1), string(raw) + `{}`, strings.Repeat(" ", MaxBytes+1)} {
		if _, err := Verify([]byte(bad), pub, w, now); err == nil {
			t.Fatal("accepted invalid envelope")
		}
	}
}
