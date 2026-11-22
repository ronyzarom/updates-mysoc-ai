package updatersim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// TestFirstInstallNotAnAvailabilityViolation proves a fresh-host first_install
// (which legitimately runs far longer than a steady-state restart) is not
// measured as an availability window, so it never flags a violation.
func TestFirstInstallNotAnAvailabilityViolation(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(t.TempDir(), "slow-first-install.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	makeTarGz(t, artifact, map[string]string{"VERSION": "v1.0.0"})

	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:           root,
		RestartCommand:        []string{script},
		CommandTimeout:        Duration{Duration: defaultCommandTimeout},
		FirstInstallTimeout:   Duration{Duration: defaultCommandTimeout},
		MaxUnavailableSeconds: 1, // tiny budget: a measured ~1s window would breach it
	}, nil)

	// FromVersion 0.0.0 => fresh host => first_install phase.
	update := Update{Product: "siemcore", FromVersion: "0.0.0", ToVersion: "v1.0.0", ArtifactPath: artifact}
	if err := e.Apply(context.Background(), update); err != nil {
		t.Fatalf("first install apply: %v", err)
	}
	seconds, violation := e.LastUnavailable()
	if violation {
		t.Fatalf("first_install must not be an availability violation (seconds=%d)", seconds)
	}
	if seconds != 0 {
		t.Fatalf("first_install window must not be measured, got %ds", seconds)
	}
}

// TestResetUnavailableBeforeApply proves a stale measurement from a prior update
// is cleared so a failure before the restart does not report an old window.
func TestResetUnavailableBeforeApply(t *testing.T) {
	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:           t.TempDir(),
		MaxUnavailableSeconds: 1,
	}, nil)
	e.recordUnavailable(600 * time.Second) // simulate a prior big window + violation
	if s, v := e.LastUnavailable(); s == 0 || !v {
		t.Fatalf("precondition: expected a recorded violation, got %d/%v", s, v)
	}
	// Apply fails early (no artifact path) but must reset first.
	_ = e.Apply(context.Background(), Update{Product: "p", FromVersion: "1", ToVersion: "2"})
	if s, v := e.LastUnavailable(); s != 0 || v {
		t.Fatalf("stale measurement not reset at Apply start: %d/%v", s, v)
	}
}

// TestRecordAttemptCarriesAvailabilityIntoHeartbeat proves the measured window
// reaches the heartbeat's last_update_attempt JSON the promotion gate reads.
func TestRecordAttemptCarriesAvailabilityIntoHeartbeat(t *testing.T) {
	cfg := &Config{}
	cfg.Instance.ID = "node-1"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "mysoc"
	cfg.Simulation.StateFile = t.TempDir() + "/state.json"
	cfg.Simulation.ArtifactDir = t.TempDir()
	cfg.Server.URL = "http://127.0.0.1:1"

	fe := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:           t.TempDir(),
		MaxUnavailableSeconds: 10,
	}, discardLogger())
	fe.recordUnavailable(45 * time.Second) // 45s > 10s budget => violation

	sim, err := NewSimulator(cfg, fe, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	sim.recordAttempt(Update{Product: "mysoc", FromVersion: "1.0.0", ToVersion: "1.1.0"}, true, "")

	hb := sim.buildHeartbeat()
	if hb.LastUpdateAttempt == nil {
		t.Fatal("heartbeat carries no last_update_attempt")
	}
	raw, err := json.Marshal(hb.LastUpdateAttempt)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["unavailable_seconds"] == nil {
		t.Fatalf("unavailable_seconds absent from heartbeat JSON: %s", raw)
	}
	if v, _ := decoded["availability_violation"].(bool); !v {
		t.Fatalf("availability_violation not true in heartbeat JSON: %s", raw)
	}
}

// TestWideCoreTierAndAvailabilityProbeConfig verifies config validation for
// the widecore tier (child of mysoc, requires parent_id) and
// availability_probe_url.
func TestWideCoreTierAndAvailabilityProbeConfig(t *testing.T) {
	base := func() *Config {
		c := &Config{}
		c.Instance.ID = "wc-node-1"
		c.Instance.ProductTier = TierWideCore
		c.Instance.ParentID = "mysoc-parent-1"
		c.Server.URL = "http://127.0.0.1:8080"
		c.Server.Timeout = Duration{Duration: 10 * time.Second}
		c.Simulation.Executor = "filesystem"
		c.Simulation.Filesystem.InstallRoot = t.TempDir()
		c.Simulation.Filesystem.AvailabilityProbeURL = "http://127.0.0.1:8443/health/live"
		c.setDefaults()
		return c
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("valid widecore config rejected: %v", err)
	}

	noParent := base()
	noParent.Instance.ParentID = ""
	if err := noParent.Validate(); err == nil {
		t.Fatal("widecore without parent_id must be rejected")
	}

	for _, bad := range []string{
		"ftp://127.0.0.1/health/live",
		"/health/live",
		"http://user:pass@127.0.0.1:8443/health/live",
		"http:///health/live",
	} {
		c := base()
		c.Simulation.Filesystem.AvailabilityProbeURL = bad
		if err := c.Validate(); err == nil {
			t.Fatalf("expected availability_probe_url %q to be rejected", bad)
		}
	}
}

// TestAvailabilityProbeURLMeasuresOutage proves availability_probe_url drives
// outage polling (HTTP 200 on any version = healthy; non-200 = down) without
// invoking health_command during the restart, while Validate() still runs
// health_command as the final validation gate.
func TestAvailabilityProbeURLMeasuresOutage(t *testing.T) {
	var down atomic.Bool
	var probeHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeHits.Add(1)
		if down.Load() {
			http.Error(w, "restarting", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"any"}`))
	}))
	defer srv.Close()

	// health_command logs each call to a marker file and exits 0; it must NOT
	// be called during Apply (availability_probe_url handles polling) and MUST
	// be called during Validate.
	tmp := t.TempDir()
	healthLog := filepath.Join(tmp, "health.log")
	healthScript := filepath.Join(tmp, "health.sh")
	if err := os.WriteFile(healthScript, []byte("#!/bin/sh\necho hit >> "+healthLog+"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Restart script sleeps 1.5s while the liveness endpoint is toggled down,
	// then recovers after the restart finishes.
	restartScript := filepath.Join(tmp, "restart.sh")
	if err := os.WriteFile(restartScript, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(tmp, "widecore-v1.1.0.artifact")
	makeTarGz(t, artifact, map[string]string{"VERSION": "v1.1.0"})

	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:           filepath.Join(tmp, "install"),
		RestartCommand:        []string{restartScript},
		HealthCommand:         []string{healthScript},
		AvailabilityProbeURL:  srv.URL + "/health/live",
		CommandTimeout:        Duration{Duration: defaultCommandTimeout},
		MaxUnavailableSeconds: 10,
	}, discardLogger())

	// Toggle the liveness endpoint down right as restart runs, then back up.
	down.Store(true)
	go func() {
		time.Sleep(1100 * time.Millisecond)
		down.Store(false)
	}()

	update := Update{Product: "widecore", FromVersion: "1.0.0", ToVersion: "1.1.0", ArtifactPath: artifact}
	if err := e.Apply(context.Background(), update); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if probeHits.Load() == 0 {
		t.Fatal("expected availability_probe_url to be polled during restart")
	}
	secs, violation := e.LastUnavailable()
	if secs < 1 || violation {
		t.Fatalf("expected measured outage >= 1s without violation (budget 10s), got secs=%d violation=%v", secs, violation)
	}
	if _, err := os.Stat(healthLog); err == nil {
		t.Fatal("health_command must not be invoked for restart probing when availability_probe_url is set")
	}
	if err := e.Validate(context.Background(), update); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if raw, err := os.ReadFile(healthLog); err != nil || !strings.Contains(string(raw), "hit") {
		t.Fatalf("expected Validate() to run health_command, got err=%v content=%q", err, string(raw))
	}
}

// TestRelayPreservesChildAvailabilityTelemetry proves Relay.handleChildReport
// preserves unavailable_seconds and availability_violation so ChildReports()
// forwards them upstream even when the report arrives after the heartbeat.
func TestRelayPreservesChildAvailabilityTelemetry(t *testing.T) {
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstreamSrv.Close()

	cfg := &Config{}
	cfg.Instance.ID = "mysoc-parent"
	cfg.Instance.ProductTier = TierMySoc
	cfg.Server.LicenseKey = "LIC-TEST"
	cfg.Server.URL = upstreamSrv.URL
	cfg.Relay.Enabled = true
	cfg.Relay.Listen = ":0"
	cfg.Relay.CacheDir = t.TempDir()

	client, err := NewClient(cfg.Server)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	relay, err := NewRelay(cfg, client, discardLogger())
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}

	// Child sends heartbeat first, then posts its update report with a newer timestamp.
	hbBody, _ := json.Marshal(platformtypes.Heartbeat{
		InstanceID:       "sc-child-1",
		ProductTier:      TierSiemCore,
		ParentInstanceID: "mysoc-parent",
		Products:         []platformtypes.ProductStatus{{Name: "siemcore", Version: "1.18.2.1"}},
	})
	rec := httptest.NewRecorder()
	hbReq := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", bytes.NewReader(hbBody))
	hbReq.Header.Set("X-License-Key", "child-tok-1")
	relay.handleChildHeartbeat(rec, hbReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("relay heartbeat status=%d body=%s", rec.Code, rec.Body.String())
	}
	var hbResp struct {
		RelayToken string `json:"relay_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hbResp); err != nil || hbResp.RelayToken == "" {
		t.Fatalf("expected relay_token in heartbeat response, got %s (err=%v)", rec.Body.String(), err)
	}

	reportBody, _ := json.Marshal(UpdateReportRequest{
		InstanceID:            "sc-child-1",
		FromVersion:           "1.18.1.5",
		ToVersion:             "1.18.2.1",
		Success:               true,
		UnavailableSeconds:    27,
		AvailabilityViolation: true,
	})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/updates/siemcore/report", bytes.NewReader(reportBody))
	req.Header.Set("X-License-Key", "child-tok-1")
	req.Header.Set("X-Relay-Token", hbResp.RelayToken)
	req.SetPathValue("product", "siemcore")
	relay.handleChildReport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("relay report status=%d body=%s", rec.Code, rec.Body.String())
	}

	reports := relay.ChildrenReport()
	if len(reports) != 1 {
		t.Fatalf("expected 1 child report, got %d", len(reports))
	}
	att := reports[0].LastUpdateAttempt
	if att == nil {
		t.Fatal("expected LastUpdateAttempt on ChildReport")
	}
	if att.UnavailableSeconds != 27 || !att.AvailabilityViolation {
		t.Fatalf("relay dropped availability telemetry: got unavailable_seconds=%d violation=%v",
			att.UnavailableSeconds, att.AvailabilityViolation)
	}
}

// TestFirstCycleHeartbeatsBeforeSelfUpdate proves the updater sends a
// non-fatal heartbeat before the self-update check on its first cycle after
// boot (eliminating post-self-update 403s on relays) and still runs
// selfUpdatePhase even if that heartbeat fails.
func TestFirstCycleHeartbeatsBeforeSelfUpdate(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var failHeartbeat atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/heartbeat":
			if failHeartbeat.Load() {
				http.Error(w, "upstream down", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/updates/") && strings.HasSuffix(r.URL.Path, "/check"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"update_available":false}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	makeSim := func() *Simulator {
		cfg := &Config{}
		cfg.Instance.ID = "node-boot-1"
		cfg.Instance.Type = "server"
		cfg.Instance.ProductTier = TierMySoc
		cfg.Server.LicenseKey = "LIC-TEST"
		cfg.Server.URL = srv.URL
		cfg.Simulation.StateFile = filepath.Join(t.TempDir(), "state.json")
		cfg.Simulation.ArtifactDir = t.TempDir()
		cfg.setDefaults()
		sim, err := NewSimulator(cfg, NewFilesystemExecutor(FilesystemConfig{InstallRoot: t.TempDir()}, discardLogger()), discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		sim.binaryVersion = "1.19.0.8"
		return sim
	}

	selfCheckSuffix := "/" + SelfUpdateProduct() + "/check"

	// Cycle 1: heartbeat must precede self-update check.
	sim := makeSim()
	if err := sim.RunCycle(context.Background(), ModeReal); err != nil {
		t.Fatalf("cycle 1 error: %v", err)
	}
	mu.Lock()
	c1 := append([]string(nil), paths...)
	paths = nil
	mu.Unlock()
	if len(c1) < 2 || c1[0] != "/api/v1/heartbeat" || !strings.Contains(c1[1], selfCheckSuffix) {
		t.Fatalf("first cycle must heartbeat before self-update check, got order: %v", c1)
	}

	// Cycle 2 (steady state): self-update check runs before heartbeat.
	if err := sim.RunCycle(context.Background(), ModeReal); err != nil {
		t.Fatalf("cycle 2 error: %v", err)
	}
	mu.Lock()
	c2 := append([]string(nil), paths...)
	paths = nil
	mu.Unlock()
	if len(c2) < 2 || !strings.Contains(c2[0], selfCheckSuffix) || c2[1] != "/api/v1/heartbeat" {
		t.Fatalf("steady-state cycle must check self-update before heartbeat, got order: %v", c2)
	}

	// Invariant 1: a failing boot heartbeat must NOT block self-update check.
	failHeartbeat.Store(true)
	sim2 := makeSim()
	sim2.RunCycle(context.Background(), ModeReal)
	mu.Lock()
	c3 := append([]string(nil), paths...)
	mu.Unlock()
	sawSelfCheck := false
	for _, p := range c3 {
		if strings.Contains(p, selfCheckSuffix) {
			sawSelfCheck = true
		}
	}
	if !sawSelfCheck {
		t.Fatalf("self-update check must still run when boot heartbeat fails; got paths: %v", c3)
	}
}
