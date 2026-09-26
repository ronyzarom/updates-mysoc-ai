package releases

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

func newTestKey(t *testing.T, issuer, status string) (TrustedKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return TrustedKey{KeyID: KeyID(pub), PublicKey: hex.EncodeToString(pub), Issuer: issuer, Status: status}, priv
}

func TestEvaluateSeal(t *testing.T) {
	const sum = "abcdef0123456789"
	current, currentPriv := newTestKey(t, "", "active")
	next, nextPriv := newTestKey(t, "", "active")
	retired, retiredPriv := newTestKey(t, "", "retired")
	swfOnly, swfPriv := newTestKey(t, IssuerSWF, "active")
	_, strangerPriv := newTestKey(t, "", "active")
	keys := []TrustedKey{current, next, retired, swfOnly}

	sign := func(priv ed25519.PrivateKey, product string) string {
		return signing.Sign(priv, product, "1.2.3", sum)
	}

	cases := []struct {
		name, product, seal, keyID string
		want, wantKey              string
	}{
		{"no seal is unsealed", "siemcore", "", "", SealUnsealed, ""},
		{"current key with key id", "siemcore", sign(currentPriv, "siemcore"), current.KeyID, SealSealed, current.KeyID},
		{"current key without key id", "siemcore", sign(currentPriv, "siemcore"), "", SealSealed, current.KeyID},
		{"rotation: next key active alongside current", "mysoc", sign(nextPriv, "mysoc"), next.KeyID, SealSealed, next.KeyID},
		{"rotation: next key found without key id", "mysoc", sign(nextPriv, "mysoc"), "", SealSealed, next.KeyID},
		{"key id is case-insensitive", "mysoc", sign(currentPriv, "mysoc"), "  " + upper(current.KeyID), SealSealed, current.KeyID},
		{"seal over another product is invalid", "siemcore", sign(currentPriv, "swf"), current.KeyID, SealInvalid, current.KeyID},
		{"seal by a key other than the named one is invalid", "siemcore", sign(nextPriv, "siemcore"), current.KeyID, SealInvalid, current.KeyID},
		{"garbage seal is invalid", "siemcore", "not-base64!!", "", SealInvalid, ""},
		{"untrusted key without key id is invalid", "siemcore", sign(strangerPriv, "siemcore"), "", SealInvalid, ""},
		{"unregistered key id is unknown", "siemcore", sign(strangerPriv, "siemcore"), "0011223344556677", SealUnknownKey, "0011223344556677"},
		{"retired key is unknown", "siemcore", sign(retiredPriv, "siemcore"), retired.KeyID, SealUnknownKey, retired.KeyID},
		{"issuer-scoped key seals its issuer", "swf", sign(swfPriv, "swf"), swfOnly.KeyID, SealSealed, swfOnly.KeyID},
		{"issuer-scoped key is unknown for another issuer", "siemcore", sign(swfPriv, "siemcore"), swfOnly.KeyID, SealUnknownKey, swfOnly.KeyID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateSeal(keys, tc.product, "1.2.3", sum, tc.seal, tc.keyID)
			if got.Status != tc.want || got.KeyID != tc.wantKey {
				t.Fatalf("got status=%s key=%q, want status=%s key=%q", got.Status, got.KeyID, tc.want, tc.wantKey)
			}
			if got.Issuer != IssuerFor(tc.product) {
				t.Fatalf("issuer = %q", got.Issuer)
			}
			if tc.want == SealSealed && got.Signature != tc.seal {
				t.Fatal("a verified seal must be kept verbatim")
			}
		})
	}

	if got := EvaluateSeal(nil, "swf", "1.2.3", sum, sign(currentPriv, "swf"), ""); got.Status != SealUnknownKey {
		t.Fatalf("no registered keys: got %s, want unknown_key", got.Status)
	}
	long := make([]byte, 4096)
	for i := range long {
		long[i] = 'A'
	}
	if got := EvaluateSeal(keys, "swf", "1.2.3", sum, string(long), ""); len(got.Signature) != maxStoredSealLen || got.Status != SealInvalid {
		t.Fatalf("oversized seal must be truncated and invalid, got len=%d status=%s", len(got.Signature), got.Status)
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

func TestIssuerFor(t *testing.T) {
	for product, want := range map[string]string{
		"mysoc":                    IssuerMySoc,
		"mysoc-platform":           IssuerMySoc,
		"siemcore":                 IssuerSiemCore,
		"SiemCore-collector":       IssuerSiemCore,
		"swf":                      IssuerSWF,
		"swf-windows":              IssuerSWF,
		"updater-linux-amd64":      IssuerUpdates,
		"mysoc-updater":            IssuerUpdates,
		"siemcore-cascade-updater": IssuerUpdates,
		"relay-kit":                IssuerUpdates,
		"something-else":           IssuerOther,
	} {
		if got := IssuerFor(product); got != want {
			t.Errorf("IssuerFor(%q) = %q, want %q", product, got, want)
		}
	}
}

func TestKeyIDStable(t *testing.T) {
	pub, err := signing.ParsePublicKeyHex("d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
	if err != nil {
		t.Fatal(err)
	}
	id := KeyID(pub)
	if len(id) != 16 || id != KeyID(pub) {
		t.Fatalf("key id must be 16 stable hex chars, got %q", id)
	}
}
