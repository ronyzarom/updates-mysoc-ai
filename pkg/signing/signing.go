// Package signing implements release manifest signing for the update cascade.
//
// The updates server signs every release at publish time with an ed25519 key.
// Every updater in the cascade — relay or leaf — verifies the signature before
// caching or installing, so intermediate hops cannot substitute artifacts.
package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// domain separates release signatures from any future signed payloads.
const domain = "mysoc-release-v1"

var (
	// ErrInvalidSignature indicates the signature does not match the release.
	ErrInvalidSignature = errors.New("release signature verification failed")
	// ErrMissingSignature indicates a release without a signature where one is required.
	ErrMissingSignature = errors.New("release signature is required")
)

// Message builds the canonical byte string that is signed for a release.
// checksum is the lowercase hex SHA-256 of the artifact.
func Message(product, version, checksum string) []byte {
	checksum = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(checksum, "sha256:")))
	return []byte(domain + "\n" + strings.TrimSpace(product) + "\n" + strings.TrimSpace(version) + "\n" + checksum)
}

// Sign returns the base64 ed25519 signature for a release.
func Sign(priv ed25519.PrivateKey, product, version, checksum string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, Message(product, version, checksum)))
}

// Verify checks a base64 release signature against the public key.
func Verify(pub ed25519.PublicKey, product, version, checksum, signatureB64 string) error {
	signatureB64 = strings.TrimSpace(signatureB64)
	if signatureB64 == "" {
		return ErrMissingSignature
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("%w: malformed base64", ErrInvalidSignature)
	}
	if !ed25519.Verify(pub, Message(product, version, checksum), sig) {
		return ErrInvalidSignature
	}
	return nil
}

// ParseSeedHex derives a private key from a 32-byte hex-encoded seed.
func ParseSeedHex(seedHex string) (ed25519.PrivateKey, error) {
	seed, err := hex.DecodeString(strings.TrimSpace(seedHex))
	if err != nil {
		return nil, fmt.Errorf("decode signing seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// ParsePublicKeyHex parses a hex-encoded ed25519 public key.
func ParsePublicKeyHex(pubHex string) (ed25519.PublicKey, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(pubHex))
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be %d bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// PublicKeyHex returns the hex encoding of the key's public half.
func PublicKeyHex(priv ed25519.PrivateKey) string {
	return hex.EncodeToString(priv.Public().(ed25519.PublicKey))
}

// GenerateSeedHex creates a fresh signing seed (for key provisioning tooling).
func GenerateSeedHex() (string, error) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return "", fmt.Errorf("generate signing seed: %w", err)
	}
	return hex.EncodeToString(seed), nil
}
