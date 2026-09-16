// Package policyauth authenticates narrowly scoped recovery-policy transitions.
// It does not install a privileged consumer or apply an application release.
package policyauth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const Protocol = "mysoc-policy-authorization-v1"
const MaxBytes = 65536

type Payload struct {
	Protocol          string `json:"protocol"`
	Hostname          string `json:"hostname"`
	Product           string `json:"product"`
	OldPolicySHA256   string `json:"old_policy_sha256"`
	PolicyRevision    int64  `json:"policy_revision"`
	FromVersion       string `json:"from_version"`
	TargetVersion     string `json:"target_version"`
	ArtifactSHA256    string `json:"artifact_sha256"`
	ArtifactSignature string `json:"artifact_signature"`
	SourceCommit      string `json:"source_commit"`
	IssuedAt          int64  `json:"issued_at"`
	ExpiresAt         int64  `json:"expires_at"`
}
type Envelope struct {
	Payload   Payload `json:"payload"`
	Signature string  `json:"signature"`
}
type Expected struct {
	Hostname, FromVersion, TargetVersion, ArtifactSHA256, ArtifactSignature, SourceCommit string
	Policy                                                                                []byte
	Revision                                                                              int64
	PolicyDigest                                                                          string
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,252}$`)

// Canonical sorts ASCII keys identically to the root consumer's canonical JSON.
func Canonical(p Payload) ([]byte, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}
func newer(from, to string) bool {
	a, b := strings.Split(from, "."), strings.Split(to, ".")
	if len(a) != 4 || len(b) != 4 {
		return false
	}
	comparison := 0
	for i := range a {
		for _, v := range []string{a[i], b[i]} {
			if v == "" || strings.Trim(v, "0123456789") != "" {
				return false
			}
		}
		x, e := strconv.ParseUint(a[i], 10, 32)
		if e != nil {
			return false
		}
		y, e := strconv.ParseUint(b[i], 10, 32)
		if e != nil {
			return false
		}
		if comparison == 0 {
			if y > x {
				comparison = 1
			} else if y < x {
				comparison = -1
			}
		}
	}
	return comparison > 0
}
func Validate(p Payload, expected Expected, now time.Time) error {
	sum := sha256.Sum256(expected.Policy)
	policyDigest := expected.PolicyDigest
	if policyDigest == "" {
		policyDigest = hex.EncodeToString(sum[:])
	}
	if p.Protocol != Protocol || p.Product != "siemcore" || !hostPattern.MatchString(p.Hostname) || p.Hostname != expected.Hostname {
		return errors.New("authorization identity mismatch")
	}
	if !hex64.MatchString(policyDigest) || p.OldPolicySHA256 != policyDigest || expected.Revision < 0 || p.PolicyRevision <= 0 || p.PolicyRevision-1 != expected.Revision {
		return errors.New("authorization predecessor policy mismatch")
	}
	if p.FromVersion != expected.FromVersion || p.TargetVersion != expected.TargetVersion || !newer(p.FromVersion, p.TargetVersion) {
		return errors.New("authorization version mismatch")
	}
	if !hex64.MatchString(p.ArtifactSHA256) || !hex40.MatchString(p.SourceCommit) || p.ArtifactSHA256 != expected.ArtifactSHA256 || p.SourceCommit != expected.SourceCommit || p.ArtifactSignature != expected.ArtifactSignature {
		return errors.New("authorization artifact mismatch")
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(p.ArtifactSignature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("invalid artifact signature")
	}
	n := now.Unix()
	if p.IssuedAt < 0 || p.ExpiresAt <= p.IssuedAt || p.ExpiresAt-p.IssuedAt > 3600 || p.IssuedAt > n || p.ExpiresAt <= n {
		return errors.New("authorization expired, future or overlong")
	}
	return nil
}
func Sign(p Payload, key ed25519.PrivateKey) (Envelope, error) {
	if len(key) != ed25519.PrivateKeySize {
		return Envelope{}, errors.New("invalid signing key")
	}
	data, err := Canonical(p)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{p, base64.StdEncoding.EncodeToString(ed25519.Sign(key, append([]byte(Protocol+"\n"), data...)))}, nil
}

// Reject duplicate keys recursively before decoding typed fields.
func unique(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim == '{' {
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := k.(string)
			if !ok || seen[s] {
				return errors.New("duplicate or invalid JSON key")
			}
			seen[s] = true
			if err = unique(d); err != nil {
				return err
			}
		}
	} else if delim == '[' {
		for d.More() {
			if err = unique(d); err != nil {
				return err
			}
		}
	} else {
		return errors.New("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}
func Verify(raw []byte, key ed25519.PublicKey, expected Expected, now time.Time) (*Payload, error) {
	if len(raw) > MaxBytes || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid envelope size or key")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := unique(d); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	var e Envelope
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&e); err != nil {
		return nil, err
	}
	if err := Validate(e.Payload, expected, now); err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(e.Signature)
	if err != nil {
		return nil, err
	}
	data, err := Canonical(e.Payload)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(key, append([]byte(Protocol+"\n"), data...), sig) {
		return nil, fmt.Errorf("invalid policy authorization signature")
	}
	return &e.Payload, nil
}
