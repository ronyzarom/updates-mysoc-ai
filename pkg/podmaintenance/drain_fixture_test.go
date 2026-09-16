package podmaintenance

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// This tests canonical signing vectors only. There is intentionally no drain
// recovery executor or advertised runtime capability in this package yet.
func TestCanonicalDrainSigningVectors(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/pod-maintenance-drain-recovery-v1/canonical.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Key           string                `json:"observer_test_public_key"`
		Domain        string                `json:"signature_domain"`
		Now           time.Time             `json:"verification_time"`
		Authorization RecoveryAuthorization `json:"authorization"`
		Vectors       []struct {
			Name          string                `json:"name"`
			Authorization RecoveryAuthorization `json:"authorization"`
			LedgerHash    string                `json:"ledger_payload_sha256"`
			Evidence      time.Time             `json:"node_evidence_observed_at"`
		} `json:"vectors"`
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	key, e := hex.DecodeString(f.Key)
	if e != nil {
		t.Fatal(e)
	}
	if f.Domain != "mysoc-pod-drain-recovery-v1\n" {
		t.Fatal("domain drift")
	}
	verify := func(a RecoveryAuthorization) bool {
		p, e := base64.StdEncoding.DecodeString(a.PayloadBase64)
		if e != nil {
			return false
		}
		s, e := base64.StdEncoding.DecodeString(a.Signature)
		return e == nil && ed25519.Verify(key, append([]byte(f.Domain), p...), s)
	}
	if !verify(f.Authorization) {
		t.Fatal("positive signature failed")
	}
	if _, e := DecodeRecoveryAuthorization(f.Authorization, key); e == nil {
		t.Fatal("v2 accepted drain authorization")
	}
	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			if got := verify(v.Authorization); got != (v.Name != "tampered-action") {
				t.Fatal("signature expectation", got)
			}
			p, _ := base64.StdEncoding.DecodeString(v.Authorization.PayloadBase64)
			switch v.Name {
			case "expired":
				var c struct {
					ExpiresAt time.Time `json:"expires_at"`
				}
				json.Unmarshal(p, &c)
				if !c.ExpiresAt.Before(f.Now) {
					t.Fatal("expired vector is not expired")
				}
			case "same-id-changed-signed-bytes":
				h := sha256.Sum256(p)
				if hex.EncodeToString(h[:]) == v.LedgerHash {
					t.Fatal("replay vector bytes unchanged")
				}
			case "stale-node-evidence":
				if f.Now.Sub(v.Evidence) <= 5*time.Second {
					t.Fatal("evidence not stale")
				}
			}
		})
	}
}
