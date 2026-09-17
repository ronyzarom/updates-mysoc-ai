package podmaintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ackDrainFixture(t *testing.T) (string, drainProcessConfig, Journal) {
	dir, c, j := drainFixture(t)
	j.Binding.Protocol = AckProtocol
	j.Generation = 42
	j.Phase = "barrier-acknowledged"
	if err := writeJournal(filepath.Join(dir, "operation.json"), j); err != nil {
		t.Fatal(err)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	c.Key = pub
	c.Protocol = AckDrainProtocol
	now := time.Now().UTC()
	claims := DrainClaims{Protocol: AckDrainProtocol, AuthorizationID: "ack-grant", Binding: j.Binding, Generation: 42, CapturedOwner: CapturedOwner{"1", 7, 7}, AckProof: &AckBarrierProof{42, 6}, Action: "resume-drain", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(10 * time.Minute)}
	raw, _ := json.Marshal(claims)
	c.Authorization = RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(AckDrainSignatureDomain), raw...)))}
	return dir, c, j
}
func TestAckDrainProcessCrashesAndRetries(t *testing.T) {
	for _, fault := range []string{"reply:status", "reply:authorize-drain-recovery", "reply:resume-drain", "checkpoint:discovering", "checkpoint:prepared", "checkpoint:authorizing", "checkpoint:resuming", "checkpoint:paused"} {
		t.Run(fault, func(t *testing.T) {
			dir, c, original := ackDrainFixture(t)
			c.Fault = fault
			if runDrainProcess(t, dir, c) == nil {
				t.Fatal("expected interruption")
			}
			if err := runDrainProcess(t, dir, c); err != nil {
				t.Fatal("retry", err)
			}
			j, err := ReadDrainJournal(dir)
			if err != nil || !ackDrainHandoff(j, original) {
				t.Fatal(j, err)
			}
			o, err := ReadJournal(dir)
			if err != nil || !reflect.DeepEqual(o, original) {
				t.Fatal("original changed", o, err)
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "calls"))
			if strings.Contains(string(raw), "apply") || strings.Contains(string(raw), "complete") {
				t.Fatal("drain exceeded scope")
			}
		})
	}
}
func TestAckDrainRefusesIncompleteAndChangedProof(t *testing.T) {
	for _, fault := range []string{"missing-proof", "epoch-change", "incomplete-paused", "stale-node"} {
		t.Run(fault, func(t *testing.T) {
			dir, c, original := ackDrainFixture(t)
			c.Fault = fault
			if runDrainProcess(t, dir, c) == nil {
				t.Fatal("accepted invalid proof")
			}
			o, _ := ReadJournal(dir)
			if !reflect.DeepEqual(o, original) {
				t.Fatal("original changed")
			}
		})
	}
}
func TestAckDrainRequiresDurableAckAndExplicitProtocol(t *testing.T) {
	for _, mode := range []string{"intent", "zero-generation", "legacy-protocol"} {
		t.Run(mode, func(t *testing.T) {
			dir, c, j := ackDrainFixture(t)
			switch mode {
			case "intent":
				j.Phase = "intent"
			case "zero-generation":
				j.Generation = 0
			case "legacy-protocol":
				c.Protocol = DrainProtocol
			}
			writeJournal(filepath.Join(dir, "operation.json"), j)
			if runDrainProcess(t, dir, c) == nil {
				t.Fatal("unsafe entry accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, "calls")); !os.IsNotExist(err) {
				t.Fatal("adapter called before local ACK validation")
			}
		})
	}
}
func TestAckDrainRecoveryRequiresSeparateGrantAndFreshProof(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "stale", "incomplete", "wrong-epoch", "drain-grant"} {
		t.Run(mode, func(t *testing.T) {
			x := makeRecovery(t, "resume-target")
			x.b.Protocol = AckProtocol
			x.claims.Binding = x.b
			x.auth = signAuthorization(x.claims, x.observer)
			original := Journal{Binding: x.b, Generation: 9, Phase: "draining"}
			writeJournal(filepath.Join(x.c.Directory, "operation.json"), original)
			now := time.Now().UTC()
			observed := now
			if mode == "stale" {
				observed = now.Add(-time.Minute)
			}
			proof := DrainResponse{Protocol: AckDrainProtocol, Binding: x.b, Generation: 9, AckProof: &AckBarrierProof{9, 6}, CapturedOwner: CapturedOwner{"1", 7, 7}, Phase: "paused", ProcessingStopped: true, IngressStopped: true, Quiescent: true, WatchdogRetired: true, WatchdogExited: true, LeaseReleased: true, ObservedAt: observed, ValidUntil: observed.Add(5 * time.Second), NodeEvidenceObservedAt: &observed}
			if mode == "incomplete" {
				proof.Quiescent = false
			}
			if mode == "wrong-epoch" {
				proof.AckProof.PreparedRevision++
			}
			if mode != "missing" {
				writeDurableJSON(filepath.Join(x.c.Directory, "drain-v1.json"), DrainJournal{Protocol: AckDrainProtocol, Binding: x.b, Generation: 9, AckProof: proof.AckProof, CapturedOwner: proof.CapturedOwner, Phase: "paused", Evidence: &proof})
			}
			if mode == "drain-grant" {
				_, cfg, _ := ackDrainFixture(t)
				x.auth = cfg.Authorization
			}
			out, err := x.c.Run(context.Background(), x.auth, x.a)
			if mode == "valid" {
				if err != nil || out != TargetInstalled {
					t.Fatal(out, err)
				}
			} else if err == nil || len(x.f.calls) > 0 {
				t.Fatal("unsafe recovery entry", out, err, x.f.calls)
			}
		})
	}
}

func TestAckDrainGrantRefusals(t *testing.T) {
	for _, mode := range []string{"expired", "future", "wrong-domain", "wrong-epoch", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			dir, c, original := ackDrainFixture(t)
			raw, _ := base64.StdEncoding.DecodeString(c.Authorization.PayloadBase64)
			var claims DrainClaims
			json.Unmarshal(raw, &claims)
			pub, priv, _ := ed25519.GenerateKey(rand.Reader)
			c.Key = pub
			domain := AckDrainSignatureDomain
			switch mode {
			case "expired":
				claims.IssuedAt = time.Now().Add(-2 * time.Minute)
				claims.ExpiresAt = time.Now().Add(-time.Minute)
			case "future":
				claims.IssuedAt = time.Now().Add(time.Minute)
				claims.ExpiresAt = time.Now().Add(2 * time.Minute)
			case "wrong-domain":
				domain = DrainSignatureDomain
			case "wrong-epoch":
				claims.AckProof.AssignmentRevision++
			case "revoked":
				os.WriteFile(filepath.Join(dir, "revoked"), []byte("revoked"), 0600)
			}
			raw, _ = json.Marshal(claims)
			c.Authorization = RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(domain), raw...)))}
			if runDrainProcess(t, dir, c) == nil {
				t.Fatal("invalid grant accepted")
			}
			calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
			if strings.Contains(string(calls), `"Action":"resume-drain"`) {
				t.Fatal("resumed without grant")
			}
			got, _ := ReadJournal(dir)
			if !reflect.DeepEqual(got, original) {
				t.Fatal("changed original")
			}
		})
	}
}

func TestAckDrainCanonicalFixture(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/qualification/pod-maintenance/ack-drain-recovery-v1.fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Key      string                `json:"observer_public_key"`
		Auth     RecoveryAuthorization `json:"authorization"`
		Claims   DrainClaims           `json:"claims"`
		Original Journal               `json:"initial_operation"`
		Paused   DrainResponse         `json:"paused_status"`
	}
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	key, err := hex.DecodeString(v.Key)
	if err != nil {
		t.Fatal(err)
	}
	claims, _, err := DecodeDrainAuthorization(v.Auth, key)
	if err != nil || !reflect.DeepEqual(claims, v.Claims) {
		t.Fatal(claims, err)
	}
	d := DrainJournal{Protocol: AckDrainProtocol, Binding: v.Original.Binding, Generation: v.Original.Generation, AckProof: v.Paused.AckProof, CapturedOwner: v.Paused.CapturedOwner, Phase: "paused", Evidence: &v.Paused}
	if !ackDrainHandoff(d, v.Original) {
		t.Fatal("invalid fixture handoff")
	}
}

func TestAckDrainPinsDiscoveryEpochAcrossRestart(t *testing.T) {
	dir, c, original := ackDrainFixture(t)
	c.Discover = true
	if err := runDrainProcess(t, dir, c); err != nil {
		t.Fatal(err)
	}
	d, err := ReadDrainJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.AckProof.AssignmentRevision++
	if err = writeDurableJSON(filepath.Join(dir, "drain-v1.json"), d); err != nil {
		t.Fatal(err)
	}
	if runDrainProcess(t, dir, c) == nil {
		t.Fatal("changed retained epoch was replaced")
	}
	got, _ := ReadJournal(dir)
	if !reflect.DeepEqual(got, original) {
		t.Fatal("original changed")
	}
}
