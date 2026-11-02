package updatersim

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// TestChildContractGolden freezes the relay's child-facing (:18443) HTTP
// contract byte-for-byte. SWF 2.3.0.x and every leaf agent speak exactly this
// protocol; the hard invariant for the updater-hardening work is that the
// child-facing side never changes. The golden bodies were recorded against the
// relay as shipped in 1.18.0.2 (verified: the child-facing handlers, auth, and
// response helpers are byte-identical to that tag — the only relay changes since
// are the listener self-probe and the loopback /health guard exemption, neither
// of which touches a child response). Any release (1.19.0 simplify, 1.20.0
// hardening, ...) that changes a byte here breaks a deployed SWF fleet and must
// fail this test.
//
// Regenerate intentionally (only when the change is reviewed and expected) with:
//
//	UPDATE_GOLDEN=1 go test ./pkg/updatersim/ -run TestChildContractGolden
func TestChildContractGolden(t *testing.T) {
	const (
		childInstanceID = "swf-child-0001"
		childLicense    = "customer-key-fixture"
		childToken      = "relay-token-fixture-abc123"
		product         = "swf"
		latestVersion   = "2.3.0.20"
		currentVersion  = "2.3.0.19"
		checksum        = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
		signature       = "c2lnbmF0dXJlLWZpeHR1cmU="
	)

	// Deterministic upstream: the relay forwards child checks here with its own
	// credential and fetches release metadata here. Fixed responses keep the
	// child-facing bytes deterministic.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/check"):
			writeTestJSON(t, w, UpdateCheckResponse{
				UpdateAvailable: true,
				CurrentVersion:  currentVersion,
				LatestVersion:   latestVersion,
				DownloadURL:     "/upstream/should/be/rewritten",
				SHA256:          checksum,
				Signature:       signature,
				Channel:         "stable",
				UpdateGroup:     "stable",
				ReleaseNotes:    "frozen contract fixture",
			})
		case strings.Contains(req.URL.Path, "/releases/"):
			writeTestJSON(t, w, platformtypes.Release{
				ProductName:  product,
				Version:      latestVersion,
				Channel:      "stable",
				Checksum:     checksum,
				Signature:    signature,
				ArtifactSize: 4096,
			})
		default:
			t.Errorf("unexpected upstream path %s", req.URL.Path)
			http.NotFound(w, req)
		}
	}))
	defer upstream.Close()

	cfg := newSimulatorTestConfig(t, upstream.URL, ModeReal)
	cfg.Instance.ID = "relay-node-fixture"
	cfg.Instance.CustomerID = "cust-fixture"
	cfg.Instance.CustomerName = "Fixture Customer"
	cfg.Server.LicenseKey = "relay-upstream-credential"
	cfg.Relay.Enabled = true
	cfg.Relay.CacheDir = t.TempDir()
	cfg.Relay.MaxArtifactBytes = 1 << 20
	client, err := NewClient(cfg.Server)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := NewRelay(cfg, client, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Full child-facing stack: the same routes and guard middleware a child hits
	// on :18443 (TLS is transport only and does not shape the HTTP contract).
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", relay.handleHealth)
	mux.HandleFunc("POST /api/v1/heartbeat", relay.handleChildHeartbeat)
	mux.HandleFunc("POST /api/v1/decommission", relay.handleChildDecommission)
	mux.HandleFunc("POST /api/v1/updates/{product}/check", relay.handleChildCheck)
	mux.HandleFunc("POST /api/v1/updates/{product}/report", relay.handleChildReport)
	mux.HandleFunc("GET /api/v1/releases/{product}/{version}", relay.handleChildReleaseMeta)
	mux.HandleFunc("GET /api/v1/releases/{product}/{version}/download", relay.handleChildDownload)
	srv := httptest.NewServer(relay.guard.middleware(mux))
	defer srv.Close()

	hc := srv.Client()
	do := func(method, path string, body string, headers map[string]string) (*http.Response, []byte) {
		t.Helper()
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, rdr)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp, raw
	}

	childAuth := map[string]string{"X-License-Key": childLicense, "X-Relay-Token": childToken}

	// Ordered so guard state is deterministic: /health (loopback-exempt) first,
	// then a heartbeat that enrolls the child and learns the source IP so the
	// non-enroll endpoints are reachable, then the happy-path endpoints, then a
	// bounded set of auth-error cases (3 failures, below the 5-failure ban
	// threshold, so no ban interferes).
	type step struct {
		name       string
		method     string
		path       string
		body       string
		headers    map[string]string
		wantStatus int
	}
	hbBody := `{"instance_id":"` + childInstanceID + `","instance_type":"server","product_tier":"swf"}`
	checkBody := `{"instance_id":"` + childInstanceID + `","current_version":"` + currentVersion + `","os":"windows","arch":"amd64"}`
	reportBody := `{"instance_id":"` + childInstanceID + `","from_version":"` + currentVersion + `","to_version":"` + latestVersion + `","success":true}`
	decomBody := `{"instance_id":"` + childInstanceID + `"}`

	steps := []step{
		{"health", http.MethodGet, "/health", "", nil, http.StatusOK},
		{"heartbeat_enroll", http.MethodPost, "/api/v1/heartbeat", hbBody, childAuth, http.StatusOK},
		{"check", http.MethodPost, "/api/v1/updates/" + product + "/check", checkBody, childAuth, http.StatusOK},
		{"report", http.MethodPost, "/api/v1/updates/" + product + "/report", reportBody, childAuth, http.StatusOK},
		{"release_meta", http.MethodGet, "/api/v1/releases/" + product + "/" + latestVersion, "", childAuth, http.StatusOK},
		{"decommission", http.MethodPost, "/api/v1/decommission", decomBody, childAuth, http.StatusOK},
		// Auth-error contract (bodies + codes are part of the frozen protocol).
		{"err_check_no_license", http.MethodPost, "/api/v1/updates/" + product + "/check", checkBody, nil, http.StatusUnauthorized},
		{"err_check_token_mismatch", http.MethodPost, "/api/v1/updates/" + product + "/check", checkBody,
			map[string]string{"X-License-Key": childLicense, "X-Relay-Token": "wrong-token"}, http.StatusUnauthorized},
		{"err_heartbeat_no_license", http.MethodPost, "/api/v1/heartbeat", hbBody, nil, http.StatusUnauthorized},
	}

	goldenDir := filepath.Join("testdata", "child-contract")
	update := os.Getenv("UPDATE_GOLDEN") != ""
	if update {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			resp, body := do(s.method, s.path, s.body, s.headers)
			if resp.StatusCode != s.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, s.wantStatus, body)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			goldenPath := filepath.Join(goldenDir, s.name+".json")
			if update {
				if err := os.WriteFile(goldenPath, body, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden (regenerate with UPDATE_GOLDEN=1): %v", err)
			}
			if !bytes.Equal(body, want) {
				t.Fatalf("child contract drift for %s:\n got: %s\nwant: %s", s.name, body, want)
			}
		})
	}

	// The download endpoint's response headers are also part of the child
	// contract (the leaf verifies the artifact from X-Checksum-SHA256 /
	// X-Signature-Ed25519). Its body is raw artifact bytes (not a golden), but a
	// missing credential must 401 with the frozen message before any upstream
	// work — assert that invariant here.
	resp, body := do(http.MethodGet, "/api/v1/releases/"+product+"/"+latestVersion+"/download", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("download without license: status = %d, want 401 (body: %s)", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "X-License-Key") {
		t.Fatalf("download 401 body changed: %s", body)
	}

	// Guard against an accidental checksum-constant typo in the fixture.
	if _, err := hex.DecodeString(checksum); err != nil {
		t.Fatalf("fixture checksum not hex: %v", err)
	}
}
