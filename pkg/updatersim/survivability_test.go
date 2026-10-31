package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

// TestHeartbeatFailureIsNonFatal proves invariant 1/3: a failing heartbeat never
// aborts the cycle — the self-update phase and product installs still run. A
// node whose telemetry endpoint is down must keep updating itself.
func TestHeartbeatFailureIsNonFatal(t *testing.T) {
	t.Parallel()

	artifact := []byte("artifact-bytes")
	sum := sha256.Sum256(artifact)
	checksum := hex.EncodeToString(sum[:])

	var mu sync.Mutex
	var reported bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			http.Error(w, "telemetry down", http.StatusInternalServerError)
		case "/api/v1/updates/siemcore/check":
			writeTestJSON(t, w, UpdateCheckResponse{
				UpdateAvailable: true, LatestVersion: "1.1.0",
				DownloadURL: "/artifact", SHA256: checksum,
				Channel: "stable", UpdateGroup: "alpha",
			})
		case "/artifact":
			w.Header().Set("X-Checksum-SHA256", checksum)
			_, _ = w.Write(artifact)
		case "/api/v1/updates/siemcore/report":
			mu.Lock()
			reported = true
			mu.Unlock()
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	simulator, err := NewSimulator(cfg, &recordingExecutor{}, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}
	// A heartbeat 500 is recorded, not fatal: the product still applies.
	_ = simulator.RunCycle(context.Background(), ModeReal)

	mu.Lock()
	defer mu.Unlock()
	if !reported {
		t.Fatal("product update must proceed and report despite a heartbeat failure")
	}
	if cfg.Products[0].CurrentVersion != "1.1.0" {
		t.Fatalf("product not applied: version = %s", cfg.Products[0].CurrentVersion)
	}
}

// TestProductTimeoutBoundsHungHook proves invariant 3: a hung executor hook does
// not hang the cycle forever — the per-product deadline cancels it.
func TestProductTimeoutBoundsHungHook(t *testing.T) {
	t.Parallel()

	artifact := []byte("artifact-bytes")
	sum := sha256.Sum256(artifact)
	checksum := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case "/api/v1/updates/siemcore/check":
			writeTestJSON(t, w, UpdateCheckResponse{
				UpdateAvailable: true, LatestVersion: "1.1.0",
				DownloadURL: "/artifact", SHA256: checksum,
				Channel: "stable", UpdateGroup: "alpha",
			})
		case "/artifact":
			w.Header().Set("X-Checksum-SHA256", checksum)
			_, _ = w.Write(artifact)
		case "/api/v1/updates/siemcore/report":
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			// A hung Apply may be cancelled mid-cycle; tolerate any extra path.
		}
	}))
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	cfg.Simulation.ProductTimeout = Duration{Duration: 200 * time.Millisecond}
	cfg.Simulation.CycleTimeout = Duration{Duration: 2 * time.Second}
	simulator, err := NewSimulator(cfg, &blockingExecutor{}, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = simulator.RunCycle(context.Background(), ModeReal)
		close(done)
	}()
	select {
	case <-done:
		// Returned within the bound — the product timeout cancelled the hook.
	case <-time.After(5 * time.Second):
		t.Fatal("RunCycle hung: product timeout did not bound the executor hook")
	}
}

// blockingExecutor blocks in Apply until the context is cancelled, modelling a
// hung install hook.
type blockingExecutor struct{}

func (blockingExecutor) Apply(ctx context.Context, _ Update) error {
	<-ctx.Done()
	return ctx.Err()
}
func (blockingExecutor) Validate(context.Context, Update) error { return nil }
func (blockingExecutor) Rollback(context.Context, Update) error { return nil }

// TestSelfUpdatePreflightFailureBlocksSwap proves invariant 2: a candidate that
// reports its version but fails the preflight (cannot parse THIS host's config)
// is never activated — the current symlink is unchanged and the pending marker
// is cleared.
func TestSelfUpdatePreflightFailureBlocksSwap(t *testing.T) {
	dir := t.TempDir()
	simulator := newSelfUpdateTestSimulator(t, dir)
	simulator.SetBinaryVersion("1.0.0")
	// A loaded-from-file config is required to exercise the preflight branch.
	simulator.config.source = filepath.Join(dir, "config.yaml")

	layout := &SelfUpdater{Dir: simulator.config.SelfUpdate.Dir}
	current := filepath.Join(layout.releaseDir("1.0.0"), "updater")
	fakeUpdaterBinary(t, current, "1.0.0")
	if err := layout.swapCurrent(layout.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}
	previous := executablePath
	executablePath = func() (string, error) { return current, nil }
	defer func() { executablePath = previous }()

	// The staged candidate reports 2.0.0 for "version" but FAILS "preflight".
	artifact := filepath.Join(dir, "downloaded-artifact")
	script := "#!/bin/sh\ncase \"$1\" in\n" +
		"version) echo \"updater-simulator 2.0.0\" ;;\n" +
		"preflight) echo 'config rejected by new binary' >&2; exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(artifact, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	err := simulator.applySelfUpdate(context.Background(), &UpdateOffer{
		Product: SelfUpdateProduct(), CurrentVersion: "1.0.0", LatestVersion: "2.0.0",
	}, artifact)
	if err == nil || !strings.Contains(err.Error(), "preflight") {
		t.Fatalf("expected a preflight failure, got %v", err)
	}
	target, _ := os.Readlink(layout.currentLink())
	if target != layout.releaseDir("1.0.0") {
		t.Fatalf("current must stay at 1.0.0 after a preflight failure, got %s", target)
	}
	if simulator.state.PendingSelfUpdate != nil {
		t.Fatal("pending marker must be cleared after a failed preflight")
	}
}

// TestDownloadStallGuardAborts proves invariant 1/3: a download that makes no
// progress is aborted by the stall guard rather than hanging the cycle.
func TestDownloadStallGuardAborts(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Checksum-SHA256", strings.Repeat("a", 64))
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("partial"))
			f.Flush()
		}
		<-release // stall: never send the rest
	}))
	defer server.Close()
	defer close(release)

	client, err := NewClient(ServerConfig{
		URL:                  server.URL,
		Timeout:              Duration{Duration: time.Second},
		MaxResponseBytes:     1 << 20,
		DownloadStallTimeout: Duration{Duration: 200 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, e := client.DownloadArtifact(context.Background(), "/artifact",
			"sha256:"+strings.Repeat("a", 64), t.TempDir(), "artifact.bin", 1<<20)
		done <- e
	}()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("stalled download should have failed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stall guard did not abort a stalled download")
	}
}

// TestSelfUpdateFallsBackToOrigin proves invariant 1: when the parent fails the
// self-update check, a tier-1 node retries the check+download via the configured
// origin fallback and still self-updates.
func TestSelfUpdateFallsBackToOrigin(t *testing.T) {
	dir := t.TempDir()

	// Primary (parent) fails the self-update check outright.
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "parent unavailable", http.StatusInternalServerError)
	}))
	defer primary.Close()

	// The staged artifact the fallback serves reports the new version.
	artifactPath := filepath.Join(dir, "updater-2.0.0")
	fakeUpdaterBinary(t, artifactPath, "2.0.0")
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(artifact)
	checksum := hex.EncodeToString(sum[:])

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/check"):
			writeTestJSON(t, w, UpdateCheckResponse{
				UpdateAvailable: true, LatestVersion: "2.0.0",
				DownloadURL: "/artifact", SHA256: checksum,
			})
		case r.URL.Path == "/artifact":
			w.Header().Set("X-Checksum-SHA256", checksum)
			_, _ = w.Write(artifact)
		default:
			t.Fatalf("unexpected fallback request: %s", r.URL.Path)
		}
	}))
	defer fallback.Close()

	cfg := newSimulatorTestConfig(t, primary.URL, ModeReal)
	cfg.Server.FallbackURL = fallback.URL
	cfg.SelfUpdate.Dir = filepath.Join(dir, "self-update")
	simulator, err := NewSimulator(cfg, &recordingExecutor{}, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}
	simulator.SetBinaryVersion("1.0.0")

	layout := &SelfUpdater{Dir: cfg.SelfUpdate.Dir}
	current := filepath.Join(layout.releaseDir("1.0.0"), filepath.Base("updater"))
	fakeUpdaterBinary(t, current, "1.0.0")
	if err := layout.swapCurrent(layout.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}
	previous := executablePath
	executablePath = func() (string, error) { return current, nil }
	defer func() { executablePath = previous }()

	err = simulator.selfUpdatePhase(context.Background(), ModeReal)
	if err == nil || err != ErrRestartPending {
		t.Fatalf("expected ErrRestartPending via fallback, got %v", err)
	}
	if target, _ := os.Readlink(layout.currentLink()); target != layout.releaseDir("2.0.0") {
		t.Fatalf("current should point at 2.0.0 after fallback self-update, got %s", target)
	}
}

// TestVerifyAnyKeyAcceptsSecondTrustedKey proves invariant 5: a release signed
// by the rotated-in key still verifies while both keys are trusted, so a key
// rollover never strands a node.
func TestVerifyAnyKeyAcceptsSecondTrustedKey(t *testing.T) {
	pubA, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const product, version, checksum = "updater-linux-amd64", "1.18.0.1", "deadbeef"
	sig := signing.Sign(privB, product, version, checksum)

	// Signed by B, but A is listed first: verify-any must still accept.
	if err := verifyAnyKey([]ed25519.PublicKey{pubA, pubB}, product, version, checksum, sig); err != nil {
		t.Fatalf("a signature from the second trusted key must verify: %v", err)
	}
	// Only A trusted: the B signature must be rejected.
	if err := verifyAnyKey([]ed25519.PublicKey{pubA}, product, version, checksum, sig); err == nil {
		t.Fatal("a signature from an untrusted key must be rejected")
	}
}
