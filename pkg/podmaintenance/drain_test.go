package podmaintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type drainProcessConfig struct {
	Directory     string
	Key           []byte
	Authorization RecoveryAuthorization
	Discover      bool
	Fault         string
}

// The helper runs both the real coordinator and its synthetic authenticated
// adapter seam in separate OS processes. It does not qualify product lifecycle.
func TestDrainProcessHelper(t *testing.T) {
	marker := -1
	for i, a := range os.Args {
		if a == "--drain-helper" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	args := os.Args[marker+1:]
	dir := args[1]
	if args[0] == "coordinator" {
		var cfg drainProcessConfig
		raw, _ := os.ReadFile(filepath.Join(dir, "config.json"))
		if json.Unmarshal(raw, &cfg) != nil {
			os.Exit(9)
		}
		c := DrainCoordinator{Directory: cfg.Directory, ObserverKey: cfg.Key, Adapter: CommandAdapter{Command: []string{os.Args[0], "-test.run=^TestDrainProcessHelper$", "--", "--drain-helper", "adapter", dir}}}
		c.AfterDurableCheckpoint = func(point string) {
			if cfg.Fault == "checkpoint:"+point {
				if f, e := os.OpenFile(filepath.Join(dir, "injected"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600); e == nil {
					f.Close()
					self, _ := os.FindProcess(os.Getpid())
					self.Kill()
				}
			}
		}
		var e error
		if cfg.Discover {
			_, e = c.Discover(context.Background())
		} else {
			_, e = c.Run(context.Background(), cfg.Authorization)
		}
		if e != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	action := args[2]
	var q DrainRequest
	if json.NewDecoder(os.Stdin).Decode(&q) != nil {
		os.Exit(8)
	}
	f, _ := os.OpenFile(filepath.Join(dir, "calls"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	json.NewEncoder(f).Encode(struct {
		Action  string
		Request DrainRequest
	}{action, q})
	f.Close()
	var cfg drainProcessConfig
	raw, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	json.Unmarshal(raw, &cfg)
	if action != "status" && (q.Generation == 0 || q.Authorization == nil) {
		os.Exit(7)
	}
	if _, e := os.Stat(filepath.Join(dir, "revoked")); e == nil && action != "status" {
		os.Exit(3)
	}
	now := time.Now().UTC()
	r := DrainResponse{Protocol: DrainProtocol, Binding: q.Binding, Generation: 42, CapturedOwner: CapturedOwner{"1", 7, 7}, Phase: "draining", ObservedAt: now, ValidUntil: now.Add(5 * time.Second)}
	if _, e := os.Stat(filepath.Join(dir, "paused")); e == nil || action == "resume-drain" {
		r.Phase = "paused"
		r.ProcessingStopped = true
		r.IngressStopped = true
		r.WatchdogRetired = true
		r.WatchdogExited = true
		r.LeaseReleased = true
	}
	if action != "status" {
		claims, _, e := DecodeDrainAuthorization(*q.Authorization, cfg.Key)
		if e != nil {
			os.Exit(5)
		}
		r.AuthorizationID = claims.AuthorizationID
		if action == "authorize-drain-recovery" {
			v := true
			r.Authorized = &v
			r.PermissionExpires = &claims.ExpiresAt
		} else {
			r.Quiescent = true
			r.NodeEvidenceObservedAt = &now
			os.WriteFile(filepath.Join(dir, "paused"), []byte("paused"), 0600)
		}
	}
	if cfg.Fault == "reply:"+action {
		if f, e := os.OpenFile(filepath.Join(dir, "injected"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600); e == nil {
			f.Close()
			parent, _ := os.FindProcess(os.Getppid())
			parent.Kill()
			os.Exit(4)
		}
	}
	json.NewEncoder(os.Stdout).Encode(r)
	os.Exit(0)
}
func drainFixture(t *testing.T) (string, drainProcessConfig, Journal) {
	t.Helper()
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	key, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	b := Binding{Protocol: Protocol, OperationID: "original", PodID: "pod", NodeID: "1", UpdaterID: "updater", Product: "siemcore", FromVersion: "1", TargetVersion: "2", ArtifactSHA256: strings.Repeat("a", 64), PreviousArtifactSHA256: strings.Repeat("b", 64), ArtifactSignature: "retained", ArtifactPath: "/retained/artifact", Deadline: now.Add(-time.Hour)}
	j := Journal{Binding: b, Phase: "intent"}
	if e := writeJournal(filepath.Join(dir, "operation.json"), j); e != nil {
		t.Fatal(e)
	}
	claims := DrainClaims{Protocol: DrainProtocol, AuthorizationID: "grant", Binding: b, Generation: 42, CapturedOwner: CapturedOwner{"1", 7, 7}, Action: "resume-drain", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(10 * time.Minute)}
	p, _ := json.Marshal(claims)
	a := RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(p), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(DrainSignatureDomain), p...)))}
	return dir, drainProcessConfig{Directory: dir, Key: key, Authorization: a}, j
}
func runDrainProcess(t *testing.T, dir string, c drainProcessConfig) error {
	t.Helper()
	raw, _ := json.Marshal(c)
	os.WriteFile(filepath.Join(dir, "config.json"), raw, 0600)
	return exec.Command(os.Args[0], "-test.run=^TestDrainProcessHelper$", "--", "--drain-helper", "coordinator", dir).Run()
}
func TestDrainProcessCrashesAndRetries(t *testing.T) {
	for _, fault := range []string{"reply:status", "reply:authorize-drain-recovery", "reply:resume-drain", "checkpoint:discovering", "checkpoint:draining", "checkpoint:authorizing", "checkpoint:resuming", "checkpoint:paused"} {
		t.Run(fault, func(t *testing.T) {
			dir, c, original := drainFixture(t)
			c.Fault = fault
			if e := runDrainProcess(t, dir, c); e == nil {
				t.Fatal("fault did not interrupt coordinator")
			}
			if e := runDrainProcess(t, dir, c); e != nil {
				t.Fatal("retry", e)
			}
			j, e := ReadDrainJournal(dir)
			if e != nil || j.Phase != "paused" || j.Generation != 42 || j.Binding != original.Binding {
				t.Fatalf("drain journal %#v %v", j, e)
			}
			o, e := ReadJournal(dir)
			if e != nil || o.Binding != original.Binding || o.Generation != 42 || o.Phase != "intent" {
				t.Fatal("original scope changed", o, e)
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "calls"))
			if strings.Contains(string(raw), "apply") || strings.Contains(string(raw), "complete") {
				t.Fatal("drain executed application")
			}
		})
	}
}
func TestDrainDiscoveryDoesNotMutate(t *testing.T) {
	dir, c, _ := drainFixture(t)
	c.Discover = true
	if e := runDrainProcess(t, dir, c); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if strings.Count(string(raw), "\n") != 1 || !strings.Contains(string(raw), `"generation":0`) {
		t.Fatal(string(raw))
	}
	j, _ := ReadDrainJournal(dir)
	if j.Generation != 42 || j.Phase != "draining" {
		t.Fatal(j)
	}
}
func TestDrainRevocationRetainsBarrier(t *testing.T) {
	dir, c, original := drainFixture(t)
	os.WriteFile(filepath.Join(dir, "revoked"), nil, 0600)
	for i := 0; i < 2; i++ {
		if runDrainProcess(t, dir, c) == nil {
			t.Fatal("revocation accepted")
		}
	}
	j, _ := ReadDrainJournal(dir)
	if j.Phase == "paused" || j.Binding != original.Binding {
		t.Fatal(j)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if strings.Contains(string(raw), "resume-drain") {
		t.Fatal("resumed after refusal")
	}
}
func TestDrainExpiredAuthorizationDoesNotMutate(t *testing.T) {
	dir, c, _ := drainFixture(t)
	var claims DrainClaims
	p, _ := base64.StdEncoding.DecodeString(c.Authorization.PayloadBase64)
	json.Unmarshal(p, &claims)
	key, priv, _ := ed25519.GenerateKey(rand.Reader)
	c.Key = key
	claims.IssuedAt = time.Now().Add(-10 * time.Minute)
	claims.ExpiresAt = time.Now().Add(-time.Minute)
	p, _ = json.Marshal(claims)
	c.Authorization = RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(p), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(DrainSignatureDomain), p...)))}
	if runDrainProcess(t, dir, c) == nil {
		t.Fatal("expired accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if strings.Count(string(raw), "\n") != 1 {
		t.Fatal("expired mutation", string(raw))
	}
}
func TestDrainCommandRefusesZeroMutation(t *testing.T) {
	a := CommandAdapter{}
	for _, action := range []string{"authorize-drain-recovery", "resume-drain", "apply"} {
		if _, e := a.CallDrain(context.Background(), action, DrainRequest{}); e == nil {
			t.Fatal(action)
		}
	}
}

type drainAdapterFunc func(context.Context, string, DrainRequest) (DrainResponse, error)

func (f drainAdapterFunc) CallDrain(c context.Context, a string, q DrainRequest) (DrainResponse, error) {
	return f(c, a, q)
}
func TestDrainRejectsUntrustedOrIncompleteDiscovery(t *testing.T) {
	for _, bad := range []string{"binding", "generation", "capture", "stale", "future", "permission", "completed", "paused-incomplete"} {
		t.Run(bad, func(t *testing.T) {
			dir, c, original := drainFixture(t)
			now := time.Now().UTC()
			calls := 0
			adapter := drainAdapterFunc(func(_ context.Context, action string, q DrainRequest) (DrainResponse, error) {
				calls++
				if action != "status" {
					t.Fatal("mutation")
				}
				r := DrainResponse{Protocol: DrainProtocol, Binding: original.Binding, Generation: 42, CapturedOwner: CapturedOwner{"1", 7, 7}, Phase: "draining", ObservedAt: now.Add(-time.Second), ValidUntil: now.Add(time.Second)}
				switch bad {
				case "binding":
					r.Binding.OperationID = "foreign"
				case "generation":
					r.Generation = 0
				case "capture":
					r.CapturedOwner.OwnerGeneration = 0
				case "stale":
					r.ValidUntil = now.Add(-time.Second)
				case "future":
					r.ObservedAt = now.Add(time.Second)
				case "permission":
					v := true
					r.Authorized = &v
				case "completed":
					r.Phase = "completed"
				case "paused-incomplete":
					r.Phase = "paused"
				}
				return r, nil
			})
			coordinator := DrainCoordinator{Directory: dir, Adapter: adapter, ObserverKey: c.Key, Now: func() time.Time { return now }}
			if _, e := coordinator.Discover(context.Background()); e == nil {
				t.Fatal("accepted invalid discovery")
			}
			j, _ := ReadJournal(dir)
			if j.Generation != 0 || j.Binding != original.Binding || calls != 1 {
				t.Fatal("changed original", j)
			}
		})
	}
}
func TestDrainRetainedGenerationCannotChange(t *testing.T) {
	dir, c, _ := drainFixture(t)
	c.Discover = true
	if e := runDrainProcess(t, dir, c); e != nil {
		t.Fatal(e)
	}
	j, _ := ReadDrainJournal(dir)
	j.Generation = 43
	if e := writeDurableJSON(filepath.Join(dir, "drain-v1.json"), j); e != nil {
		t.Fatal(e)
	}
	if runDrainProcess(t, dir, c) == nil {
		t.Fatal("changed generation accepted")
	}
}
func TestDrainDomainAndScope(t *testing.T) {
	dir, c, original := drainFixture(t)
	claims, _, e := DecodeDrainAuthorization(c.Authorization, c.Key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := DecodeRecoveryAuthorization(c.Authorization, c.Key); e == nil {
		t.Fatal("drain grant accepted as v2")
	}
	key, priv, _ := ed25519.GenerateKey(rand.Reader)
	c.Key = key
	claims.Binding.Deadline = original.Binding.Deadline.Add(time.Hour)
	raw, _ := json.Marshal(claims)
	c.Authorization = RecoveryAuthorization{PayloadBase64: base64.StdEncoding.EncodeToString(raw), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(DrainSignatureDomain), raw...)))}
	if runDrainProcess(t, dir, c) == nil {
		t.Fatal("deadline rewrite accepted")
	}
}
