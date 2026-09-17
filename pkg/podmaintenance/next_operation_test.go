package podmaintenance

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type nextFake struct {
	calls int
	deny  bool
}

func (f *nextFake) AuthorizeNext(_ context.Context, q NextOperationRequest) (NextOperationResponse, error) {
	f.calls++
	raw, _ := base64.StdEncoding.DecodeString(q.Authorization.PayloadBase64)
	var c NextOperationClaims
	json.Unmarshal(raw, &c)
	return NextOperationResponse{Protocol: NextOperationProtocol, AuthorizationID: c.AuthorizationID, PreviousBinding: c.PreviousBinding, PreviousGeneration: c.PreviousGeneration, PreviousOutcome: c.PreviousOutcome, NextBinding: c.NextBinding, Allowed: !f.deny, ValidUntil: c.ExpiresAt}, nil
}
func nextFixture(t *testing.T) (*TransitionCoordinator, RecoveryAuthorization, NextOperationClaims, *nextFake) {
	t.Helper()
	x := makeRecovery(t, "restore-predecessor")
	if _, e := x.c.Run(context.Background(), x.auth, x.a); e != nil {
		t.Fatal(e)
	}
	next := x.b
	next.OperationID = "next-operation"
	next.TargetVersion = "3"
	next.FromVersion = x.b.FromVersion
	next.PreviousArtifactSHA256 = x.b.PreviousArtifactSHA256
	next.ArtifactPath = filepath.Join(x.c.Directory, "next-artifact")
	data := []byte("next release")
	os.WriteFile(next.ArtifactPath, data, 0600)
	next.ArtifactSHA256 = hashBytes(data)
	next.ArtifactSignature = signing.Sign(x.release, next.Product, next.TargetVersion, next.ArtifactSHA256)
	next.Deadline = time.Now().UTC().Add(30 * time.Minute)
	c := NextOperationClaims{Protocol: NextOperationProtocol, AuthorizationID: "next-auth", PreviousBinding: x.b, PreviousGeneration: 9, PreviousOutcome: PredecessorRestored, NextBinding: next, IssuedAt: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	raw, _ := json.Marshal(c)
	auth := RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(x.observer, append([]byte(NextOperationDomain), raw...)))}
	f := &nextFake{}
	return &TransitionCoordinator{Directory: x.c.Directory, Adapter: f, ObserverKey: x.c.ObserverKey, ReleaseKey: x.c.ReleaseKey}, auth, c, f
}
func TestNextOperationArchivesExactEvidenceAndStartsOneIntent(t *testing.T) {
	c, a, claims, f := nextFixture(t)
	old, _ := os.ReadFile(filepath.Join(c.Directory, "operation.json"))
	recovery, _ := os.ReadFile(filepath.Join(c.Directory, "recovery-v2.json"))
	if e := c.Advance(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	archive := filepath.Join(c.Directory, "archive", hashBytes([]byte(claims.PreviousBinding.OperationID)))
	got, _ := os.ReadFile(filepath.Join(archive, "operation.json"))
	if string(got) != string(old) {
		t.Fatal("old journal bytes changed")
	}
	got, _ = os.ReadFile(filepath.Join(archive, "recovery-v2.json"))
	if string(got) != string(recovery) {
		t.Fatal("recovery bytes changed")
	}
	j, e := ReadJournal(c.Directory)
	if e != nil || j.Phase != "intent" || j.Generation != 0 || j.Binding.OperationID != claims.NextBinding.OperationID {
		t.Fatal(j, e)
	}
	if _, e = os.Stat(filepath.Join(c.Directory, "recovery-v2.json")); !os.IsNotExist(e) {
		t.Fatal("old recovery left live")
	}
	if e = c.Advance(context.Background(), a); e != nil || f.calls != 1 {
		t.Fatal("repeat transition", e, f.calls)
	}
}
func TestNextOperationCrashResumesWithoutEvidenceLoss(t *testing.T) {
	c, a, _, f := nextFixture(t)
	c.afterPrepare = func() error { return errors.New("crash") }
	if c.Advance(context.Background(), a) == nil {
		t.Fatal("expected interruption")
	}
	j, _ := ReadJournal(c.Directory)
	if j.Phase != "rolled-back" {
		t.Fatal("premature replace")
	}
	c.afterPrepare = nil
	c.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if e := c.Advance(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	if f.calls != 1 {
		t.Fatal("repeated external authorization")
	}
}
func TestNextOperationRefusals(t *testing.T) {
	for _, kind := range []string{"expired", "denied", "artifact", "nonterminal", "archive-tamper"} {
		t.Run(kind, func(t *testing.T) {
			c, a, claims, f := nextFixture(t)
			switch kind {
			case "expired":
				c.Now = func() time.Time { return time.Now().Add(time.Hour) }
			case "denied":
				f.deny = true
			case "artifact":
				os.WriteFile(claims.NextBinding.ArtifactPath, []byte("wrong"), 0600)
			case "nonterminal":
				j, _ := ReadJournal(c.Directory)
				j.Phase = "applying"
				writeJournal(filepath.Join(c.Directory, "operation.json"), j)
			case "archive-tamper":
				c.afterPrepare = func() error { return errors.New("stop") }
				_ = c.Advance(context.Background(), a)
				c.afterPrepare = nil
				os.WriteFile(filepath.Join(c.Directory, "archive", hashBytes([]byte(claims.PreviousBinding.OperationID)), "operation.json"), []byte("{}"), 0600)
			}
			if c.Advance(context.Background(), a) == nil {
				t.Fatal("unsafe transition accepted")
			}
		})
	}
}
func TestNextOperationCanonicalFixture(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/pod-maintenance-next-operation-v1/transition.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Key      string                `json:"observer_public_key"`
		Request  NextOperationRequest  `json:"request"`
		Response NextOperationResponse `json:"response"`
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	key, e := hex.DecodeString(f.Key)
	if e != nil {
		t.Fatal(e)
	}
	claims, e := parseNext(f.Request.Authorization, key)
	if e != nil {
		t.Fatal(e)
	}
	if claims.NextBinding.OperationID != f.Response.NextBinding.OperationID || claims.PreviousGeneration != f.Response.PreviousGeneration {
		t.Fatal("fixture drift")
	}
}

func TestNextOperationPreservesDrainLedgerAcrossCrash(t *testing.T) {
	c, a, claims, _ := nextFixture(t)
	proof := DrainResponse{Protocol: DrainProtocol, Binding: claims.PreviousBinding, Generation: 9, Phase: "paused", ProcessingStopped: true, IngressStopped: true, WatchdogRetired: true, WatchdogExited: true, LeaseReleased: true}
	j := DrainJournal{Protocol: DrainProtocol, Binding: claims.PreviousBinding, Generation: 9, Phase: "paused", Evidence: &proof, Grants: map[string]DrainGrantRecord{"old": {PayloadSHA256: "retained", Signature: "signed", Superseded: true}}}
	path := filepath.Join(c.Directory, "drain-v1.json")
	if e := writeDurableJSON(path, j); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	c.afterPrepare = func() error { return errors.New("interruption") }
	if c.Advance(context.Background(), a) == nil {
		t.Fatal("not interrupted")
	}
	c.afterPrepare = nil
	if e := c.Advance(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	archived, _ := os.ReadFile(filepath.Join(c.Directory, "archive", hashBytes([]byte(claims.PreviousBinding.OperationID)), "drain-v1.json"))
	if string(before) != string(archived) {
		t.Fatal("drain ledger changed")
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("old drain blocks next operation")
	}
	if e := c.Advance(context.Background(), a); e != nil {
		t.Fatal("retry", e)
	}
}
