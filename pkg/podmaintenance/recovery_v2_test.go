package podmaintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recoveryFake struct {
	calls        []string
	fail         string
	completed    bool
	wrongHealth  bool
	wrongOutcome bool
	unsupported  bool
	permission   time.Time
}

func (f *recoveryFake) CallRecovery(_ context.Context, action string, q RecoveryRequest) (RecoveryResponse, error) {
	f.calls = append(f.calls, action)
	raw, _ := base64.StdEncoding.DecodeString(q.Authorization.PayloadBase64)
	var claims RecoveryClaims
	json.Unmarshal(raw, &claims)
	r := RecoveryResponse{Protocol: RecoveryProtocol, Binding: q.Binding, Generation: q.Generation, Outcome: q.Outcome, AuthorizationID: claims.AuthorizationID, Phase: "paused", PermissionExpires: claims.ExpiresAt, Capabilities: []string{RecoveryProtocol}}
	if f.unsupported {
		r.Capabilities = nil
	}
	if f.wrongOutcome {
		r.Outcome = "wrong"
	}
	if f.completed {
		r.Phase = "completed"
	}
	if action == "complete" {
		f.completed = true
		r.Phase = "completed"
	}
	if !f.permission.IsZero() {
		r.PermissionExpires = f.permission
	}
	if action == f.fail {
		return r, errors.New("injected failure")
	}
	h := Health{Version: q.Artifact.Version, SHA256: q.Artifact.SHA256, Role: "STBY", ManagementReady: true, ProcessingDisabled: true, TrafficDisabled: true, AuthorityValid: true, ReplicationStatus: "ready"}
	if f.wrongHealth {
		h.Version = q.Binding.TargetVersion
		h.SHA256 = q.Binding.ArtifactSHA256
	}
	if action == "health" {
		r.Health = &h
	}
	if action == "acceptance" {
		r.Acceptance = &Acceptance{Health: h, ServiceHealthy: true}
		r.PermissionExpires = time.Now().Add(time.Hour)
	}
	return r, nil
}

type recoveryFixture struct {
	c        *RecoveryCoordinator
	f        *recoveryFake
	b        Binding
	a        RetainedArtifact
	auth     RecoveryAuthorization
	claims   RecoveryClaims
	observer ed25519.PrivateKey
}

func makeRecovery(t *testing.T, action string) recoveryFixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	os.MkdirAll(dir, 0700)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	opub, opriv, _ := ed25519.GenerateKey(rand.Reader)
	b := target()
	b.OperationID = "original-op"
	b.Deadline = time.Now().UTC().Add(-time.Hour)
	content := []byte("retained predecessor")
	version := b.FromVersion
	if action == "resume-target" {
		content = []byte("retained target")
		version = b.TargetVersion
	}
	sum := sha256.Sum256(content)
	sha := hex.EncodeToString(sum[:])
	if action == "restore-predecessor" {
		b.PreviousArtifactSHA256 = sha
	} else {
		b.ArtifactSHA256 = sha
	}
	path := filepath.Join(dir, "retained.tar.gz")
	os.WriteFile(path, content, 0600)
	a := RetainedArtifact{Product: b.Product, Version: version, SHA256: sha, Path: path, Signature: signing.Sign(priv, b.Product, version, sha)}
	j := Journal{Binding: b, Generation: 9, Phase: "applying"}
	if e := writeJournal(filepath.Join(dir, "operation.json"), j); e != nil {
		t.Fatal(e)
	}
	claims := RecoveryClaims{Protocol: RecoveryProtocol, AuthorizationID: "auth-1", Binding: b, Generation: 9, Action: action, IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(10 * time.Minute)}
	f := &recoveryFake{}
	c := &RecoveryCoordinator{Directory: dir, Adapter: f, ObserverKey: opub, ReleaseKey: pub}
	return recoveryFixture{c: c, f: f, b: b, a: a, auth: signAuthorization(claims, opriv), claims: claims, observer: opriv}
}
func signAuthorization(c RecoveryClaims, k ed25519.PrivateKey) RecoveryAuthorization {
	raw, _ := json.Marshal(c)
	return RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(k, append([]byte(RecoveryAuthorizationDomain), raw...)))}
}
func TestRecoveryV2Outcomes(t *testing.T) {
	for _, action := range []string{"restore-predecessor", "resume-target"} {
		t.Run(action, func(t *testing.T) {
			x := makeRecovery(t, action)
			out, e := x.c.Run(context.Background(), x.auth, x.a)
			if e != nil {
				t.Fatal(e)
			}
			want := TargetInstalled
			phase := "accepted"
			if action == "restore-predecessor" {
				want = PredecessorRestored
				phase = "rolled-back"
			}
			if out != want {
				t.Fatal(out)
			}
			j, e := ReadJournal(x.c.Directory)
			if e != nil || j.Phase != phase || j.Binding != x.b || j.Generation != 9 {
				t.Fatal("binding modified", j, e)
			}
			x.f.calls = nil
			if _, e = x.c.Run(context.Background(), x.auth, x.a); e != nil {
				t.Fatal(e)
			}
			for _, v := range x.f.calls {
				if v == "recover" || v == "complete" {
					t.Fatal("replay", x.f.calls)
				}
			}
		})
	}
}
func TestRecoveryV2FailClosed(t *testing.T) {
	for _, kind := range []string{"observer-key", "release-signature", "tampered-artifact", "generation", "changed-binding", "expired", "future", "overlong", "wrong-health", "wrong-outcome", "capability", "revoked"} {
		t.Run(kind, func(t *testing.T) {
			x := makeRecovery(t, "restore-predecessor")
			switch kind {
			case "observer-key":
				x.c.ObserverKey = make([]byte, 32)
			case "release-signature":
				x.a.Signature = "bad"
			case "tampered-artifact":
				os.WriteFile(x.a.Path, []byte("tampered"), 0600)
			case "generation":
				x.claims.Generation++
			case "changed-binding":
				x.claims.Binding.TargetVersion = "other"
			case "expired":
				x.claims.IssuedAt = time.Now().Add(-10 * time.Minute)
				x.claims.ExpiresAt = time.Now().Add(-time.Minute)
			case "future":
				x.claims.IssuedAt = time.Now().Add(time.Minute)
				x.claims.ExpiresAt = time.Now().Add(2 * time.Minute)
			case "overlong":
				x.claims.ExpiresAt = x.claims.IssuedAt.Add(time.Hour)
			case "wrong-health":
				x.f.wrongHealth = true
			case "wrong-outcome":
				x.f.wrongOutcome = true
			case "capability":
				x.f.unsupported = true
			case "revoked":
				x.f.fail = "authorize-recovery"
			}
			if kind != "observer-key" && kind != "release-signature" && kind != "tampered-artifact" {
				x.auth = signAuthorization(x.claims, x.observer)
			}
			if _, e := x.c.Run(context.Background(), x.auth, x.a); e == nil {
				t.Fatal("expected refusal")
			}
			if x.f.completed {
				t.Fatal("barrier cleared")
			}
		})
	}
}
func TestRecoveryV2InterruptedAndRenewal(t *testing.T) {
	x := makeRecovery(t, "restore-predecessor")
	x.f.fail = "recover"
	if _, e := x.c.Run(context.Background(), x.auth, x.a); e == nil {
		t.Fatal("expected failure")
	}
	x.f.fail = ""
	x.claims.AuthorizationID = "auth-2"
	x.auth = signAuthorization(x.claims, x.observer)
	if _, e := x.c.Run(context.Background(), x.auth, x.a); e != nil {
		t.Fatal(e)
	}
	j, _ := ReadRecoveryJournal(x.c.Directory)
	if len(j.AuthorizationHashes) != 2 {
		t.Fatal("lost authorization history")
	}
}
func TestRecoveryV2ImmutableAuthorizationID(t *testing.T) {
	x := makeRecovery(t, "restore-predecessor")
	x.f.fail = "recover"
	_, _ = x.c.Run(context.Background(), x.auth, x.a)
	x.claims.ExpiresAt = x.claims.ExpiresAt.Add(time.Second)
	a := signAuthorization(x.claims, x.observer)
	if _, e := x.c.Run(context.Background(), a, x.a); e == nil || !strings.Contains(e.Error(), "reused") {
		t.Fatal(e)
	}
}
func TestRecoveryV2LostCompleteAfterExpiry(t *testing.T) {
	x := makeRecovery(t, "restore-predecessor")
	x.f.fail = "complete"
	_, _ = x.c.Run(context.Background(), x.auth, x.a)
	x.f.fail = ""
	x.f.calls = nil
	x.c.Now = func() time.Time { return time.Now().Add(20 * time.Minute) }
	if _, e := x.c.Run(context.Background(), x.auth, x.a); e != nil {
		t.Fatal(e)
	}
	for _, a := range x.f.calls {
		if a == "recover" || a == "complete" || a == "authorize-recovery" {
			t.Fatal(x.f.calls)
		}
	}
}
func TestRecoveryV2FailedAcceptanceAndV1Refusal(t *testing.T) {
	x := makeRecovery(t, "restore-predecessor")
	x.f.fail = "acceptance"
	_, _ = x.c.Run(context.Background(), x.auth, x.a)
	j, _ := ReadRecoveryJournal(x.c.Directory)
	if j.Phase != "completed" {
		t.Fatal(j.Phase)
	}
	v1 := Coordinator{Directory: x.c.Directory, Adapter: &fake{}}
	if e := v1.Run(context.Background(), x.b); e == nil {
		t.Fatal("v1 bypassed recovery")
	}
}

func TestRecoveryV2RenewUncertainPausedCompletion(t *testing.T) {
	x := makeRecovery(t, "restore-predecessor")
	x.f.fail = "complete"
	_, _ = x.c.Run(context.Background(), x.auth, x.a)
	x.f.completed = false
	x.f.fail = ""
	x.f.calls = nil
	x.claims.AuthorizationID = "replacement"
	x.auth = signAuthorization(x.claims, x.observer)
	if _, e := x.c.Run(context.Background(), x.auth, x.a); e != nil {
		t.Fatal(e)
	}
	for _, a := range x.f.calls {
		if a == "recover" {
			t.Fatal("replayed recovered application")
		}
	}
}
func TestRecoveryV2RejectsSupersededAuthorization(t *testing.T) {
	x := makeRecovery(t, "restore-predecessor")
	first := x.auth
	x.f.fail = "recover"
	_, _ = x.c.Run(context.Background(), first, x.a)
	x.claims.AuthorizationID = "replacement"
	second := signAuthorization(x.claims, x.observer)
	_, _ = x.c.Run(context.Background(), second, x.a)
	if _, e := x.c.Run(context.Background(), first, x.a); e == nil || !strings.Contains(e.Error(), "superseded") {
		t.Fatal(e)
	}
}
func TestRecoveryV2FixtureSignatures(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/pod-maintenance-recovery-v2/predecessor-restored.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		TestKeys      map[string]string     `json:"test_keys"`
		Authorization RecoveryAuthorization `json:"authorization"`
		Artifact      RetainedArtifact      `json:"artifact"`
		Content       string                `json:"artifact_content_base64"`
		Actions       map[string]struct {
			Request  json.RawMessage `json:"request"`
			Response json.RawMessage `json:"response"`
		} `json:"actions"`
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	key, _ := hex.DecodeString(f.TestKeys["observer_public_key"])
	claims, e := DecodeRecoveryAuthorization(f.Authorization, key)
	if e != nil {
		t.Fatal(e)
	}
	if claims.Action != "restore-predecessor" {
		t.Fatal(claims.Action)
	}
	releaseKey, _ := hex.DecodeString(f.TestKeys["release_public_key"])
	if e = signing.Verify(releaseKey, f.Artifact.Product, f.Artifact.Version, f.Artifact.SHA256, f.Artifact.Signature); e != nil {
		t.Fatal(e)
	}
	content, _ := base64.StdEncoding.DecodeString(f.Content)
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != f.Artifact.SHA256 {
		t.Fatal("fixture digest")
	}
	for action, v := range f.Actions {
		var q RecoveryRequest
		var r RecoveryResponse
		if e = decode(v.Request, &q); e != nil {
			t.Fatal(action, e)
		}
		if e = decode(v.Response, &r); e != nil {
			t.Fatal(action, e)
		}
		var legacy Response
		if decode(v.Response, &legacy) == nil {
			t.Fatal("v1 accepted v2 reply")
		}
	}
}
func TestRecoveryV2SignedAmbiguousJSONRejected(t *testing.T) {
	x := makeRecovery(t, "resume-target")
	raw, _ := base64.StdEncoding.DecodeString(x.auth.PayloadBase64)
	raw = append([]byte(`{"action":"restore-predecessor",`), raw[1:]...)
	a := RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(x.observer, append([]byte(RecoveryAuthorizationDomain), raw...)))}
	if _, e := DecodeRecoveryAuthorization(a, x.c.ObserverKey); e == nil {
		t.Fatal("signed duplicate accepted")
	}
}

func TestRecoveryV2TargetFixture(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/pod-maintenance-recovery-v2/target-installed.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		TestKeys      map[string]string     `json:"test_keys"`
		Authorization RecoveryAuthorization `json:"authorization"`
		Artifact      RetainedArtifact      `json:"artifact"`
		Actions       map[string]struct {
			Request  json.RawMessage `json:"request"`
			Response json.RawMessage `json:"response"`
		} `json:"actions"`
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	key, _ := hex.DecodeString(f.TestKeys["observer_public_key"])
	c, e := DecodeRecoveryAuthorization(f.Authorization, key)
	if e != nil || c.Action != "resume-target" {
		t.Fatal(c, e)
	}
	releaseKey, _ := hex.DecodeString(f.TestKeys["release_public_key"])
	if e = signing.Verify(releaseKey, f.Artifact.Product, f.Artifact.Version, f.Artifact.SHA256, f.Artifact.Signature); e != nil {
		t.Fatal(e)
	}
	if f.Artifact.Version != c.Binding.TargetVersion || f.Artifact.SHA256 != c.Binding.ArtifactSHA256 {
		t.Fatal("target fixture identity")
	}
	for action, v := range f.Actions {
		var q RecoveryRequest
		var r RecoveryResponse
		if e = decode(v.Request, &q); e != nil {
			t.Fatal(action, e)
		}
		if e = decode(v.Response, &r); e != nil {
			t.Fatal(action, e)
		}
		if q.Outcome != TargetInstalled || r.Outcome != TargetInstalled {
			t.Fatal(action, "outcome")
		}
	}
}
