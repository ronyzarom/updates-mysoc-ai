package signing

import (
	"strings"
	"testing"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	seed, err := GenerateSeedHex()
	if err != nil {
		t.Fatal(err)
	}
	priv, err := ParseSeedHex(seed)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicKeyHex(PublicKeyHex(priv))
	if err != nil {
		t.Fatal(err)
	}

	sig := Sign(priv, "swf", "2.2.0", "ABCDEF0123")
	if err := Verify(pub, "swf", "2.2.0", "abcdef0123", sig); err != nil {
		t.Fatalf("verify failed (checksum case must not matter): %v", err)
	}
	if err := Verify(pub, "swf", "2.2.0", "sha256:abcdef0123", sig); err != nil {
		t.Fatalf("verify failed (sha256: prefix must be stripped): %v", err)
	}

	if err := Verify(pub, "swf", "2.2.1", "abcdef0123", sig); err == nil {
		t.Fatal("tampered version must fail verification")
	}
	if err := Verify(pub, "swf", "2.2.0", "ffffff", sig); err == nil {
		t.Fatal("tampered checksum must fail verification")
	}
	if err := Verify(pub, "siemcore", "2.2.0", "abcdef0123", sig); err == nil {
		t.Fatal("tampered product must fail verification")
	}
	if err := Verify(pub, "swf", "2.2.0", "abcdef0123", ""); err != ErrMissingSignature {
		t.Fatalf("empty signature must return ErrMissingSignature, got %v", err)
	}
	if err := Verify(pub, "swf", "2.2.0", "abcdef0123", "not-base64!!"); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed base64 must fail clearly, got %v", err)
	}
}

func TestParseSeedValidation(t *testing.T) {
	if _, err := ParseSeedHex("abcd"); err == nil {
		t.Fatal("short seed must be rejected")
	}
	if _, err := ParseSeedHex("zz"); err == nil {
		t.Fatal("non-hex seed must be rejected")
	}
}
