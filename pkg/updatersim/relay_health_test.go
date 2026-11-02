package updatersim

import (
	"context"
	"net"
	"testing"
	"time"
)

// freeLoopbackAddr reserves and releases an ephemeral loopback port, returning
// the address so a relay can bind it and a self-probe can reach it.
func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// TestListenerHealthyPassesGuard is the regression test for the production bug
// where the relay's own loopback GET /health was denied by the relay guard
// (unlearned source), so ListenerHealthy() was always false and the watchdog
// never fed. With the loopback /health guard exemption the self-probe must
// succeed against a live listener.
func TestListenerHealthyPassesGuard(t *testing.T) {
	addr := freeLoopbackAddr(t)

	cfg := &Config{}
	cfg.Instance.ID = "mysoc-op1"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "mysoc"
	cfg.Relay.Enabled = true
	cfg.Relay.Listen = addr
	cfg.Relay.CacheDir = t.TempDir()
	cfg.Relay.TLS.Dir = t.TempDir()
	cfg.Simulation.StateFile = t.TempDir() + "/state.json"
	cfg.Simulation.ArtifactDir = t.TempDir()
	cfg.Server.URL = "http://127.0.0.1:1"

	client, err := NewClient(cfg.Server)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := NewRelay(cfg, client, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = relay.Serve(ctx) }()

	// Poll until the listener answers its own health probe (or time out).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if relay.ListenerHealthy() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ListenerHealthy never returned true: loopback /health self-probe blocked")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestIsLoopbackRemote(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:5000": true,
		"[::1]:5000":     true,
		"10.0.0.5:5000":  false,
		"8.8.8.8:443":    false,
		"127.0.0.1":      true, // no port
		"":               false,
	}
	for in, want := range cases {
		if got := isLoopbackRemote(in); got != want {
			t.Errorf("isLoopbackRemote(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSelfProbeHost(t *testing.T) {
	cases := map[string]string{
		"":          "127.0.0.1",
		"0.0.0.0":   "127.0.0.1",
		"::":        "::1",
		"127.0.0.1": "127.0.0.1",
		"10.0.0.5":  "10.0.0.5",
	}
	for in, want := range cases {
		if got := selfProbeHost(in); got != want {
			t.Errorf("selfProbeHost(%q) = %q, want %q", in, got, want)
		}
	}
}
