package updatersim

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type retryExecutor struct {
	applies, rollbacks int
	fail               bool
}

func (e *retryExecutor) Apply(context.Context, Update) error {
	e.applies++
	if e.fail {
		return fmt.Errorf("partial mutation failure")
	}
	return nil
}
func (e *retryExecutor) Validate(context.Context, Update) error { return nil }
func (e *retryExecutor) Rollback(context.Context, Update) error { e.rollbacks++; return nil }

func TestDurableBackoffKeepsHeartbeatAndSelfUpdateRunning(t *testing.T) {
	payload := []byte("retry fixture")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	heartbeats, selfChecks, downloads := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/heartbeat":
			heartbeats++
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case strings.Contains(r.URL.Path, "/updates/updater-"):
			selfChecks++
			writeTestJSON(t, w, UpdateCheckResponse{})
		case strings.HasSuffix(r.URL.Path, "/check"):
			writeTestJSON(t, w, UpdateCheckResponse{UpdateAvailable: true, LatestVersion: "1.1.0", SHA256: digest, DownloadURL: "/download"})
		case r.URL.Path == "/download":
			downloads++
			w.Write(payload)
		case strings.HasSuffix(r.URL.Path, "/report"):
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	e := &retryExecutor{fail: true}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	makeSim := func() *Simulator {
		s, err := NewSimulator(cfg, e, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		s.binaryVersion = "1.16.1.16"
		s.nowFn = func() time.Time { return now }
		return s
	}
	s := makeSim()
	if err := s.RunCycle(context.Background(), ModeReal); err == nil {
		t.Fatal("expected failure")
	}
	if e.applies != 1 || e.rollbacks != 1 {
		t.Fatal("unknown failure must roll back")
	}
	stored, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	r := stored.ProductRetries["siemcore"]
	if r.Attempts != 1 || !r.NextRetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("wrong durable delay: %#v", r)
	}
	if stored.LastUpdateAttempt.NextRetryAt == nil {
		t.Fatal("heartbeat status missing retry deadline")
	}
	// A fresh process must still check policy, heartbeat and self-update without
	// downloading/applying the deferred product again.
	s = makeSim()
	now = now.Add(30 * time.Second)
	if err = s.RunCycle(context.Background(), ModeReal); err != nil {
		t.Fatal(err)
	}
	if e.applies != 1 || downloads != 1 || heartbeats != 2 || selfChecks != 2 {
		t.Fatalf("deferred cycle suppressed infrastructure or retried app: %d %d %d %d", e.applies, downloads, heartbeats, selfChecks)
	}
	now = now.Add(31 * time.Second)
	if err = s.RunCycle(context.Background(), ModeReal); err == nil {
		t.Fatal("expected second failure")
	}
	if s.state.ProductRetries["siemcore"].Attempts != 2 || !s.state.ProductRetries["siemcore"].NextRetryAt.Equal(now.Add(2*time.Minute)) {
		t.Fatal("delay did not grow")
	}
	// Changing the updater implementation is allowed to retry the same bytes.
	s.binaryVersion = "1.16.1.17"
	e.fail = false
	if err = s.RunCycle(context.Background(), ModeReal); err != nil {
		t.Fatal(err)
	}
	if e.applies != 3 || s.state.ProductRetries["siemcore"] != nil || s.state.ProductVersions["siemcore"] != "1.1.0" {
		t.Fatal("new updater did not retry and clear state")
	}
}

func TestRetryIdentityIsolationExplicitRetryAndDelayCap(t *testing.T) {
	cfg := newSimulatorTestConfig(t, "http://127.0.0.1", ModeReal)
	s, err := NewSimulator(cfg, &retryExecutor{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	s.state.ProductRetries = map[string]*ProductRetry{"siemcore": {TargetVersion: "1.1", ArtifactDigest: "abc", Attempts: 4, NextRetryAt: future}, "mysoc": {TargetVersion: "2.0", ArtifactDigest: "def", NextRetryAt: future}}
	if err = s.RetryProduct("siemcore", "1.1", "wrong"); err == nil {
		t.Fatal("identity mismatch accepted")
	}
	if err = s.RetryProduct("siemcore", "1.1", "abc"); err != nil {
		t.Fatal(err)
	}
	if !s.state.ProductRetries["siemcore"].NextRetryAt.IsZero() || s.state.ProductRetries["mysoc"].NextRetryAt != future {
		t.Fatal("retry affected another product")
	}
	for i, want := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
		if retryDelay(i+1) != want {
			t.Fatal("delay cap")
		}
	}
	if retryDelay(1000000) != 15*time.Minute {
		t.Fatal("large counter must remain bounded")
	}
}

func TestRetryReservationFailurePreventsExecution(t *testing.T) {
	cfg := newSimulatorTestConfig(t, "http://127.0.0.1", ModeReal)
	e := &retryExecutor{}
	s, err := NewSimulator(cfg, e, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	// A path under a regular file cannot be durably reserved.
	parent := filepath.Join(t.TempDir(), "file")
	os.WriteFile(parent, []byte("x"), 0600)
	s.config.Simulation.StateFile = filepath.Join(parent, "state")
	err = s.processOffer(context.Background(), ModeReal, &UpdateOffer{Product: "siemcore", LatestVersion: "1.1", Checksum: "abc"})
	if err == nil || e.applies != 0 {
		t.Fatal("execution started without durable reservation")
	}
}

func TestRetryReservationAndReportSurviveJSON(t *testing.T) {
	r := ProductRetry{TargetVersion: "1", ArtifactDigest: "abc", UpdaterVersion: "new", Attempts: 2, NextRetryAt: time.Now().UTC().Truncate(time.Second)}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got ProductRetry
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != r {
		t.Fatal("retry identity lost through restart")
	}
}

func TestChangedReleaseIdentityBypassesOldDelay(t *testing.T) {
	for _, change := range []string{"version", "digest", "updater", "product"} {
		t.Run(change, func(t *testing.T) {
			data := []byte("different verified content")
			sum := sha256.Sum256(data)
			digest := hex.EncodeToString(sum[:])
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/download" {
					w.Write(data)
				} else {
					writeTestJSON(t, w, map[string]string{"status": "ok"})
				}
			}))
			defer server.Close()
			cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
			e := &retryExecutor{}
			s, err := NewSimulator(cfg, e, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			s.binaryVersion = "new-build"
			s.state.ProductRetries = map[string]*ProductRetry{"siemcore": {TargetVersion: "1.1.0", ArtifactDigest: digest, UpdaterVersion: "new-build", Attempts: 4, NextRetryAt: time.Now().Add(time.Hour)}}
			offer := &UpdateOffer{Product: "siemcore", CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Checksum: digest, DownloadURL: "/download"}
			switch change {
			case "version":
				offer.LatestVersion = "1.2.0"
			case "digest":
				s.state.ProductRetries["siemcore"].ArtifactDigest = "old-digest"
			case "updater":
				s.state.ProductRetries["siemcore"].UpdaterVersion = "old-build"
			case "product":
				offer.Product = "mysoc"
			}
			if err = s.processOffer(context.Background(), ModeReal, offer); err != nil || e.applies != 1 {
				t.Fatalf("new identity should retry immediately: %v", err)
			}
			if change == "product" && s.state.ProductRetries["siemcore"] == nil {
				t.Fatal("other product delay lost")
			}
		})
	}
}
