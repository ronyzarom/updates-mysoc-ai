package podmaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fake struct {
	calls       []string
	fail        string
	completed   bool
	badHealth   bool
	unsupported bool
	mismatch    bool
}

func (f *fake) Call(_ context.Context, a string, q Request) (Response, error) {
	f.calls = append(f.calls, a)
	r := Response{Capabilities: []string{Protocol}, Binding: q.Binding, Generation: 7, Phase: "paused", PermissionExpires: time.Now().Add(time.Hour)}
	if f.unsupported {
		r.Capabilities = nil
	}
	if f.mismatch {
		r.Binding.NodeID = "2"
	}
	if f.completed {
		r.Phase = "completed"
	}
	if a == "complete" {
		f.completed = true
		r.Phase = "completed"
	}
	if a == f.fail {
		return r, errors.New("injected lost response")
	}
	h := Health{Version: q.Binding.TargetVersion, SHA256: q.Binding.ArtifactSHA256, Role: "STBY", ManagementReady: true, ProcessingDisabled: true, TrafficDisabled: true, AuthorityValid: true, ReplicationStatus: "degraded"}
	if f.badHealth {
		h.ProcessingDisabled = false
	}
	if a == "health" {
		r.Health = &h
	}
	if a == "acceptance" {
		r.Acceptance = &Acceptance{Health: h, ServiceHealthy: true}
	}
	return r, nil
}
func target() Binding {
	return Binding{ArtifactSignature: "test-signature", Protocol: Protocol, PodID: "pod-test", NodeID: "1", UpdaterID: "updater-1", Product: "siemcore", FromVersion: "1", TargetVersion: "2", ArtifactPath: "/private/artifact.tar.gz", ArtifactSHA256: strings.Repeat("a", 64), PreviousArtifactSHA256: strings.Repeat("b", 64)}
}
func setup(t *testing.T) (*Coordinator, *fake) {
	t.Helper()
	f := &fake{}
	d := filepath.Join(t.TempDir(), "private")
	return &Coordinator{Directory: d, Adapter: f}, f
}
func journal(t *testing.T, c *Coordinator) Journal {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(c.Directory, "operation.json"))
	if e != nil {
		t.Fatal(e)
	}
	var j Journal
	if e = json.Unmarshal(b, &j); e != nil {
		t.Fatal(e)
	}
	return j
}
func TestSuccessfulStandbyAndIdempotence(t *testing.T) {
	c, f := setup(t)
	if e := c.Run(context.Background(), target()); e != nil {
		t.Fatal(e)
	}
	if journal(t, c).Phase != "accepted" {
		t.Fatal("not accepted")
	}
	n := len(f.calls)
	if e := c.Run(context.Background(), target()); e != nil || len(f.calls) != n {
		t.Fatal("repeated completed operation", e)
	}
}
func TestInterruptedApplyRecoversSameOperation(t *testing.T) {
	c, f := setup(t)
	f.fail = "apply"
	if c.Run(context.Background(), target()) == nil {
		t.Fatal("expected failure")
	}
	before := journal(t, c)
	f.fail = ""
	f.calls = nil
	if e := c.Run(context.Background(), target()); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before.Binding, journal(t, c).Binding) {
		t.Fatal("identity changed")
	}
	if strings.Contains(strings.Join(f.calls, ","), "apply") {
		t.Fatal(f.calls)
	}
	if !strings.Contains(strings.Join(f.calls, ","), "recover") {
		t.Fatal(f.calls)
	}
}
func TestLostCompletionResponseDoesNotReapply(t *testing.T) {
	c, f := setup(t)
	f.fail = "complete"
	if c.Run(context.Background(), target()) == nil {
		t.Fatal("expected failure")
	}
	f.fail = ""
	f.calls = nil
	if e := c.Run(context.Background(), target()); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(strings.Join(f.calls, ","), "apply") {
		t.Fatal(f.calls)
	}
}
func TestAcceptanceFailureRetainsCompleted(t *testing.T) {
	c, f := setup(t)
	f.fail = "acceptance"
	if c.Run(context.Background(), target()) == nil {
		t.Fatal("expected failure")
	}
	if journal(t, c).Phase != "completed" {
		t.Fatal("must retain completed")
	}
	f.fail = ""
	f.calls = nil
	if e := c.Run(context.Background(), target()); e != nil {
		t.Fatal(e)
	}
	for _, a := range f.calls {
		if a == "apply" || a == "complete" {
			t.Fatal(f.calls)
		}
	}
}
func TestFailClosed(t *testing.T) {
	for _, kind := range []string{"unsupported", "mismatch", "health", "recover"} {
		t.Run(kind, func(t *testing.T) {
			c, f := setup(t)
			switch kind {
			case "unsupported":
				f.unsupported = true
			case "mismatch":
				f.mismatch = true
			case "health":
				f.badHealth = true
			case "recover":
				f.fail = "apply"
				_ = c.Run(context.Background(), target())
				f.fail = "recover"
			}
			if c.Run(context.Background(), target()) == nil {
				t.Fatal("expected failure")
			}
			if f.completed {
				t.Fatal("barrier cleared")
			}
		})
	}
}
func TestDifferentReleaseCannotReplacePending(t *testing.T) {
	c, f := setup(t)
	f.fail = "apply"
	_ = c.Run(context.Background(), target())
	b := target()
	b.TargetVersion = "3"
	f.fail = ""
	n := len(f.calls)
	if c.Run(context.Background(), b) == nil || len(f.calls) != n {
		t.Fatal("replaced pending operation")
	}
}
func TestMalformedJournalAndLock(t *testing.T) {
	c, _ := setup(t)
	os.MkdirAll(c.Directory, 0700)
	p := filepath.Join(c.Directory, "operation.json")
	os.WriteFile(p, []byte("{}"), 0600)
	if c.Run(context.Background(), target()) == nil {
		t.Fatal("invalid journal accepted")
	}
	os.Remove(p)
	unlock, e := lock(c.Directory)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	if c.Run(context.Background(), target()) == nil {
		t.Fatal("concurrent operation accepted")
	}
}
func TestStrictJSON(t *testing.T) {
	for _, s := range []string{`{"phase":"paused","phase":"completed"}`, `{"unknown":1}`, `{} {}`} {
		var r Response
		if decode([]byte(s), &r) == nil {
			t.Fatal(s)
		}
	}
}
func TestExpiredPendingCannotApply(t *testing.T) {
	c, f := setup(t)
	b := target()
	b.Deadline = time.Now().Add(-time.Minute)
	if c.Run(context.Background(), b) == nil {
		t.Fatal("expired operation ran")
	}
	for _, a := range f.calls {
		if a == "apply" || a == "begin-or-resume" {
			t.Fatal(f.calls)
		}
	}
}
func TestExpiredLostCompletionCanReconcile(t *testing.T) {
	c, f := setup(t)
	f.fail = "complete"
	_ = c.Run(context.Background(), target())
	c.Now = func() time.Time { return time.Now().Add(31 * time.Minute) }
	f.fail = ""
	if e := c.Run(context.Background(), target()); e != nil {
		t.Fatal(e)
	}
}
