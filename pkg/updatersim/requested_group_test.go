package updatersim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// A relay must forward a child's requested enrolment ring up the cascade so the
// server can honor it when it first creates the child's row. Pure; no network.
func TestChildReportForwardsRequestedUpdateGroup(t *testing.T) {
	hb := platformtypes.Heartbeat{
		InstanceID:           "pod-a",
		ProductTier:          "siemcore",
		RequestedUpdateGroup: "alpha",
	}
	got := childReport(hb, "online", time.Now(), "203.0.113.7")
	if got.RequestedUpdateGroup != "alpha" {
		t.Fatalf("childReport did not forward requested ring: got %q, want alpha", got.RequestedUpdateGroup)
	}
	// Absent request forwards empty (server then defaults to stable).
	if r := childReport(platformtypes.Heartbeat{InstanceID: "x"}, "online", time.Now(), ""); r.RequestedUpdateGroup != "" {
		t.Fatalf("expected empty requested ring, got %q", r.RequestedUpdateGroup)
	}
}

// Simulator.Check must include requested_update_group in UpdateCheckRequest so
// a brand-new relayed child's first check (which arrives upstream before the
// relay's next rollup heartbeat) honors its configured enrolment ring.
func TestSimulatorCheckSendsRequestedUpdateGroup(t *testing.T) {
	var gotReq UpdateCheckRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/updates/widecore/check" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode check req: %v", err)
		}
		writeTestJSON(t, w, UpdateCheckResponse{
			UpdateAvailable: false,
			CurrentVersion:  "0.0.0",
			UpdateGroup:     "alpha",
		})
	}))
	defer srv.Close()

	cfg := &Config{
		Server: ServerConfig{
			URL:              srv.URL,
			LicenseKey:       "test-key",
			Timeout:          Duration{5 * time.Second},
			MaxResponseBytes: defaultMaxResponseBytes,
		},
		Instance: InstanceConfig{
			ID:             "widecore-test-1",
			Type:           "widecore",
			ProductTier:    TierWideCore,
			ParentID:       "mysoc-parent-1",
			Hostname:       "widecore-test-1",
			UpdaterVersion: "1.21.1.1",
			UpdateGroup:    "alpha",
		},
		Products: []ProductConfig{
			{Name: "widecore", CurrentVersion: "0.0.0", Channel: "stable"},
		},
	}
	sim, err := NewSimulator(cfg, nil, discardLogger())
	if err != nil {
		t.Fatalf("NewSimulator: %v", err)
	}
	if _, err := sim.Check(context.Background(), "widecore"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if gotReq.RequestedUpdateGroup != "alpha" {
		t.Fatalf("UpdateCheckRequest.RequestedUpdateGroup = %q, want alpha", gotReq.RequestedUpdateGroup)
	}
}

// Relay.handleChildCheck must both forward requested_update_group when sent by
// a new child and backfill it from the child's cached heartbeat when an older
// child omits it from the check payload.
func TestRelayCheckForwardsAndBackfillsRequestedUpdateGroup(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req UpdateCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		seen[req.InstanceID] = req.RequestedUpdateGroup
		mu.Unlock()
		writeTestJSON(t, w, UpdateCheckResponse{
			UpdateAvailable: false,
			CurrentVersion:  req.CurrentVersion,
			UpdateGroup:     req.RequestedUpdateGroup,
		})
	}))
	defer upstream.Close()

	cfg := &Config{
		Server: ServerConfig{URL: upstream.URL, LicenseKey: "relay-lic", Timeout: Duration{5 * time.Second}},
		Instance: InstanceConfig{
			ID:             "mysoc-parent",
			Type:           "mysoc",
			ProductTier:    TierMySoc,
			UpdaterVersion: "1.21.1.1",
		},
		Relay: RelayConfig{
			Enabled:  true,
			CacheDir: t.TempDir(),
		},
	}
	client := newTestClient(t, upstream.URL)
	relay, err := NewRelay(cfg, client, discardLogger())
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/heartbeat", relay.handleChildHeartbeat)
	mux.HandleFunc("POST /api/v1/updates/{product}/check", relay.handleChildCheck)

	enroll := func(id, reqGroup string) string {
		t.Helper()
		body, _ := json.Marshal(platformtypes.Heartbeat{
			InstanceID:           id,
			ProductTier:          TierWideCore,
			ParentInstanceID:     "mysoc-parent",
			RequestedUpdateGroup: reqGroup,
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", bytes.NewReader(body))
		req.Header.Set("X-License-Key", "child-lic-"+id)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("enroll %s: status=%d body=%s", id, rec.Code, rec.Body.String())
		}
		var resp HeartbeatResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode hb resp: %v", err)
		}
		return resp.RelayToken
	}

	// Case 1: newer child includes requested_update_group directly in check.
	tokNew := enroll("wc-new", "beta")
	bodyNew, _ := json.Marshal(UpdateCheckRequest{
		InstanceID:           "wc-new",
		CurrentVersion:       "0.0.0",
		ProductTier:          TierWideCore,
		ParentInstanceID:     "mysoc-parent",
		RequestedUpdateGroup: "beta",
	})
	recNew := httptest.NewRecorder()
	reqNew := httptest.NewRequest(http.MethodPost, "/api/v1/updates/widecore/check", bytes.NewReader(bodyNew))
	reqNew.Header.Set("X-License-Key", "child-lic-wc-new")
	reqNew.Header.Set("X-Relay-Token", tokNew)
	mux.ServeHTTP(recNew, reqNew)
	if recNew.Code != http.StatusOK {
		t.Fatalf("check wc-new: status=%d body=%s", recNew.Code, recNew.Body.String())
	}

	// Case 2: older child only sent requested_update_group in heartbeat, not check.
	tokOld := enroll("wc-old", "alpha")
	bodyOld, _ := json.Marshal(UpdateCheckRequest{
		InstanceID:       "wc-old",
		CurrentVersion:   "0.0.0",
		ProductTier:      TierWideCore,
		ParentInstanceID: "mysoc-parent",
	})
	recOld := httptest.NewRecorder()
	reqOld := httptest.NewRequest(http.MethodPost, "/api/v1/updates/widecore/check", bytes.NewReader(bodyOld))
	reqOld.Header.Set("X-License-Key", "child-lic-wc-old")
	reqOld.Header.Set("X-Relay-Token", tokOld)
	mux.ServeHTTP(recOld, reqOld)
	if recOld.Code != http.StatusOK {
		t.Fatalf("check wc-old: status=%d body=%s", recOld.Code, recOld.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if seen["wc-new"] != "beta" {
		t.Fatalf("upstream saw wc-new requested_update_group=%q, want beta", seen["wc-new"])
	}
	if seen["wc-old"] != "alpha" {
		t.Fatalf("upstream saw wc-old requested_update_group=%q, want alpha (backfilled from cached heartbeat)", seen["wc-old"])
	}
}
