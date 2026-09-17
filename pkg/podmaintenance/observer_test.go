package podmaintenance

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type observerFake struct {
	now   func() time.Time
	dir   string
	calls []string
	lost  string
	bad   string
	crash string
}

func (f *observerFake) CallObserver(ctx context.Context, a string, q ObserverRequest) (ObserverResponse, error) {
	f.calls = append(f.calls, a)
	now := time.Now()
	if f.now != nil {
		now = f.now()
	}
	r := ObserverResponse{Binding: q.Binding, Generation: 7, Phase: "prepared", PermissionExpires: now.Add(time.Minute), Capabilities: []string{ObserverProtocol}, LifecycleReady: true, AuthoritySnapshotSHA256: strings.Repeat("c", 64), AuthorityPreserved: true, Serialized: true, QuorumReady: true, AuthenticationReady: true, InstalledVersion: q.Binding.TargetVersion, InstalledSHA256: q.Binding.ArtifactSHA256}
	if _, e := os.Stat(filepath.Join(f.dir, "installed")); e == nil {
		r.Phase = "ready"
	}
	if _, e := os.Stat(filepath.Join(f.dir, "completed")); e == nil {
		r.Phase = "completed"
		r.Serialized = false
	}
	if a == "apply" || a == "reconcile" {
		j, e := ReadObserverJournal(f.dir)
		if e != nil || j.Phase != "applying" || j.Generation != 7 || j.AuthoritySnapshotSHA256 != r.AuthoritySnapshotSHA256 {
			return r, errors.New("restart before durable ACK")
		}
		os.WriteFile(filepath.Join(f.dir, "installed"), []byte(q.Binding.OperationID), 0600)
		r.Phase = "ready"
	}
	if a == "complete" {
		os.WriteFile(filepath.Join(f.dir, "completed"), []byte(q.Binding.OperationID), 0600)
		r.Phase = "completed"
		r.Serialized = false
	}
	switch f.bad {
	case "capability":
		r.Capabilities = nil
	case "unqualified":
		r.LifecycleReady = false
	case "authority":
		r.AuthorityPreserved = false
	case "serialization":
		r.Serialized = false
	case "quorum":
		r.QuorumReady = false
	case "artifact":
		r.InstalledSHA256 = strings.Repeat("d", 64)
	case "epoch":
		if a != "prepare" && a != "capabilities" {
			r.Generation++
		}
	case "snapshot":
		if a != "prepare" && a != "capabilities" {
			r.AuthoritySnapshotSHA256 = strings.Repeat("e", 64)
		}
	}
	if a == f.crash {
		os.Exit(77)
	}
	if a == f.lost {
		return r, errors.New("lost response")
	}
	return r, nil
}
func observerTarget() ObserverBinding {
	b := ObserverBinding(target())
	b.Protocol = ObserverProtocol
	b.NodeID = "witness"
	return b
}
func observerSetup(t *testing.T) (*ObserverCoordinator, *observerFake) {
	dir := filepath.Join(t.TempDir(), "observer")
	f := &observerFake{dir: dir}
	return &ObserverCoordinator{Directory: dir, Adapter: f}, f
}
func TestObserverLostRepliesReconcileSameOperation(t *testing.T) {
	for _, action := range []string{"prepare", "apply", "complete", "acceptance"} {
		t.Run(action, func(t *testing.T) {
			c, f := observerSetup(t)
			f.lost = action
			if c.Run(context.Background(), observerTarget()) == nil {
				t.Fatal("expected failure")
			}
			before, e := ReadObserverJournal(c.Directory)
			if e != nil {
				t.Fatal(e)
			}
			f.lost = ""
			f.calls = nil
			if e = c.Run(context.Background(), observerTarget()); e != nil {
				t.Fatal(e)
			}
			after, _ := ReadObserverJournal(c.Directory)
			if !reflect.DeepEqual(before.Binding, after.Binding) || after.Phase != "accepted" {
				t.Fatal("identity replaced")
			}
			if action != "prepare" {
				for _, a := range f.calls {
					if a == "apply" {
						t.Fatal("blind reapply")
					}
				}
			}
		})
	}
}
func TestObserverFailClosed(t *testing.T) {
	for _, bad := range []string{"capability", "unqualified", "authority", "serialization", "quorum", "artifact", "epoch", "snapshot"} {
		t.Run(bad, func(t *testing.T) {
			c, f := observerSetup(t)
			f.bad = bad
			if c.Run(context.Background(), observerTarget()) == nil {
				t.Fatal("unsafe success")
			}
			if _, e := os.Stat(filepath.Join(c.Directory, "completed")); !os.IsNotExist(e) {
				t.Fatal("completed without verified health")
			}
		})
	}
}
func TestObserverExpiryAndDifferentReleaseRetainOperation(t *testing.T) {
	c, f := observerSetup(t)
	f.lost = "apply"
	if c.Run(context.Background(), observerTarget()) == nil {
		t.Fatal("loss expected")
	}
	before, _ := ReadObserverJournal(c.Directory)
	f.lost = ""
	f.calls = nil
	c.Now = func() time.Time { return before.Binding.Deadline.Add(time.Second) }
	if c.Run(context.Background(), observerTarget()) == nil {
		t.Fatal("expired restart accepted")
	}
	for _, a := range f.calls {
		if a == "reconcile" || a == "apply" || a == "complete" {
			t.Fatal(f.calls)
		}
	}
	b := observerTarget()
	b.TargetVersion = "3"
	if c.Run(context.Background(), b) == nil {
		t.Fatal("replaced pending operation")
	}
	after, _ := ReadObserverJournal(c.Directory)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("expired journal changed")
	}
}
func TestObserverProcessHelper(t *testing.T) {
	dir := os.Getenv("OBSERVER_TEST_DIR")
	if dir == "" {
		return
	}
	f := &observerFake{dir: dir, crash: os.Getenv("OBSERVER_TEST_CRASH")}
	c := &ObserverCoordinator{Directory: dir, Adapter: f}
	c.AfterDurableCheckpoint = func(p string) {
		if os.Getenv("OBSERVER_TEST_CRASH") == "checkpoint:"+p {
			os.Exit(77)
		}
	}
	if e := c.Run(context.Background(), observerTarget()); e != nil {
		t.Fatal(e)
	}
}
func TestObserverProcessCrashRetry(t *testing.T) {
	for _, point := range []string{"prepare", "apply", "complete", "checkpoint:acknowledged", "checkpoint:applying", "checkpoint:completing"} {
		t.Run(point, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "observer")
			cmd := exec.Command(os.Args[0], "-test.run=^TestObserverProcessHelper$")
			cmd.Env = append(os.Environ(), "OBSERVER_TEST_DIR="+dir, "OBSERVER_TEST_CRASH="+point)
			err := cmd.Run()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 77 {
				t.Fatal(err)
			}
			before, err := ReadObserverJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			c := &ObserverCoordinator{Directory: dir, Adapter: &observerFake{dir: dir}}
			if err = c.Run(context.Background(), observerTarget()); err != nil {
				t.Fatal(err)
			}
			after, _ := ReadObserverJournal(dir)
			if !reflect.DeepEqual(before.Binding, after.Binding) || after.Phase != "accepted" {
				t.Fatal("restart lost original")
			}
		})
	}
}
func TestObserverLostCompleteAfterExpiry(t *testing.T) {
	c, f := observerSetup(t)
	f.lost = "complete"
	if c.Run(context.Background(), observerTarget()) == nil {
		t.Fatal("expected loss")
	}
	j, _ := ReadObserverJournal(c.Directory)
	// Expiry of original operation permits read-only completed reconciliation; fresh service permission remains required.
	c.Now = func() time.Time { return j.Binding.Deadline.Add(time.Second) }
	f.now = c.Now
	f.lost = ""
	f.calls = nil
	if err := c.Run(context.Background(), observerTarget()); err != nil {
		t.Fatal(err)
	}
	for _, a := range f.calls {
		if a == "apply" || a == "reconcile" || a == "complete" {
			t.Fatal("mutated expired completed operation")
		}
	}
}
