package podmaintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type ackFixture struct {
	fake
	dir      string
	lost     string
	crash    string
	bad      string
	draining bool
}

func (f *ackFixture) Call(ctx context.Context, action string, q Request) (Response, error) {
	if action == "begin-or-resume" {
		return Response{}, errors.New("v1 fallback forbidden")
	}
	r, err := f.fake.Call(ctx, action, q)
	r.Capabilities = []string{AckProtocol}
	if f.unsupported {
		r.Capabilities = nil
	}
	if action == "prepare" || action == "start-drain" {
		path := filepath.Join(f.dir, "observer.json")
		var original Request
		raw, e := os.ReadFile(path)
		if e == nil {
			if json.Unmarshal(raw, &original) != nil || !reflect.DeepEqual(original.Binding, q.Binding) {
				return r, errors.New("operation changed")
			}
		} else if !os.IsNotExist(e) {
			return r, e
		} else {
			if action != "prepare" {
				return r, errors.New("drain before prepare")
			}
			if e = writeDurableJSON(path, q); e != nil {
				return r, e
			}
		}
		if action == "prepare" {
			r.Phase = "prepared"
		} else {
			j, e := ReadJournal(f.dir)
			if e != nil || j.Phase != "barrier-acknowledged" || j.Generation != 7 || !reflect.DeepEqual(j.Binding, q.Binding) {
				return r, errors.New("drain before durable acknowledgment")
			}
			r.Phase = "paused"
			if f.draining {
				r.Phase = "draining"
			}
		}
		if f.crash == action {
			os.Exit(77)
		}
		if f.lost == action {
			return r, errors.New("lost response")
		}
	}
	if action == "status" && f.draining {
		r.Phase = "draining"
	}
	if action == "prepare" {
		switch f.bad {
		case "identity":
			r.Binding.PodID = "wrong"
		case "generation":
			r.Generation = 0
		case "expired":
			r.PermissionExpires = time.Now().Add(-time.Second)
		case "phase":
			r.Phase = "paused"
		case "persistence":
			if e := os.Remove(filepath.Join(f.dir, "operation.json")); e != nil {
				return r, e
			}
			if e := os.Mkdir(filepath.Join(f.dir, "operation.json"), 0700); e != nil {
				return r, e
			}
		}
	}
	return r, err
}
func ackTarget() Binding { b := target(); b.Protocol = AckProtocol; return b }
func TestAckV2LostRepliesPreserveOperation(t *testing.T) {
	for _, action := range []string{"prepare", "start-drain"} {
		t.Run(action, func(t *testing.T) {
			c, _ := setup(t)
			f := &ackFixture{dir: c.Directory, lost: action}
			c.Adapter = f
			if c.Run(context.Background(), ackTarget()) == nil {
				t.Fatal("expected loss")
			}
			before := journal(t, c)
			for _, a := range f.calls {
				if a == "apply" {
					t.Fatal("applied after lost response")
				}
			}
			f.lost = ""
			if err := c.Run(context.Background(), ackTarget()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Binding, journal(t, c).Binding) {
				t.Fatal("binding changed")
			}
		})
	}
}
func TestAckV2InvalidAcknowledgmentCannotDrain(t *testing.T) {
	for _, bad := range []string{"identity", "generation", "expired", "phase", "persistence", "unsupported"} {
		t.Run(bad, func(t *testing.T) {
			c, _ := setup(t)
			f := &ackFixture{dir: c.Directory, bad: bad}
			f.unsupported = bad == "unsupported"
			c.Adapter = f
			if c.Run(context.Background(), ackTarget()) == nil {
				t.Fatal("expected refusal")
			}
			for _, a := range f.calls {
				if a == "start-drain" || a == "apply" {
					t.Fatal("mutation without acknowledgment", f.calls)
				}
			}
		})
	}
}
func TestAckV2DrainingAndExpiry(t *testing.T) {
	c, _ := setup(t)
	f := &ackFixture{dir: c.Directory, draining: true}
	c.Adapter = f
	if c.Run(context.Background(), ackTarget()) == nil || journal(t, c).Phase != "draining" {
		t.Fatal("must wait for paused")
	}
	before := journal(t, c)
	f.calls = nil
	c.Now = func() time.Time { return before.Binding.Deadline.Add(time.Second) }
	if c.Run(context.Background(), ackTarget()) == nil {
		t.Fatal("expired mutation")
	}
	for _, a := range f.calls {
		if a == "apply" || a == "start-drain" || a == "complete" {
			t.Fatal(f.calls)
		}
	}
	if !reflect.DeepEqual(before, journal(t, c)) {
		t.Fatal("expired journal changed")
	}
	c.Now = nil
	f.draining = false
	if err := c.Run(context.Background(), ackTarget()); err != nil {
		t.Fatal(err)
	}
}
func TestAckV2ProcessHelper(t *testing.T) {
	dir := os.Getenv("ACK_V2_TEST_DIR")
	if dir == "" {
		return
	}
	f := &ackFixture{dir: dir, crash: os.Getenv("ACK_V2_TEST_CRASH")}
	c := &Coordinator{Directory: dir, Adapter: f}
	if err := c.Run(context.Background(), ackTarget()); err != nil {
		t.Fatal(err)
	}
}
func TestAckV2ProcessCrashRetry(t *testing.T) {
	for _, action := range []string{"prepare", "start-drain"} {
		t.Run(action, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "journal")
			cmd := exec.Command(os.Args[0], "-test.run=^TestAckV2ProcessHelper$")
			cmd.Env = append(os.Environ(), "ACK_V2_TEST_DIR="+dir, "ACK_V2_TEST_CRASH="+action)
			err := cmd.Run()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 77 {
				t.Fatal("expected process exit", err)
			}
			before, err := ReadJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := "intent"
			if action == "start-drain" {
				want = "barrier-acknowledged"
			}
			if before.Phase != want {
				t.Fatal(before.Phase)
			}
			f := &ackFixture{dir: dir}
			c := &Coordinator{Directory: dir, Adapter: f}
			if err = c.Run(context.Background(), ackTarget()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Binding, journal(t, c).Binding) {
				t.Fatal("restart replaced operation")
			}
		})
	}
}

func TestAckV2RetainedProtocolCannotDowngrade(t *testing.T) {
	c, _ := setup(t)
	f := &ackFixture{dir: c.Directory, lost: "prepare"}
	c.Adapter = f
	if c.Run(context.Background(), ackTarget()) == nil {
		t.Fatal("expected lost reply")
	}
	before := journal(t, c)
	f.calls = nil
	if c.Run(context.Background(), target()) == nil {
		t.Fatal("downgrade accepted")
	}
	if len(f.calls) != 0 || !reflect.DeepEqual(before, journal(t, c)) {
		t.Fatal("downgrade changed state")
	}
}
