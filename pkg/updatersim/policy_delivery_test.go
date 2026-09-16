package updatersim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPolicyAuthorizationSurvivesTwoRelays(t *testing.T) {
	raw := json.RawMessage(`{"payload":{"protocol":"mysoc-policy-authorization-v1"},"signature":"fixture"}`)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var check UpdateCheckRequest
		if json.NewDecoder(r.Body).Decode(&check) != nil {
			t.Error("bad check")
		}
		if check.PolicyAuthorizationVersion != "mysoc-policy-authorization-v1" {
			t.Error("capability lost")
		}
		writeTestJSON(t, w, UpdateCheckResponse{UpdateAvailable: true, LatestVersion: "3.3.152.32", DownloadURL: "/artifact", UpdateGroup: "alpha", PolicyAuthorization: raw})
	}))
	defer origin.Close()
	hop := func(parent string) *httptest.Server {
		cfg := newSimulatorTestConfig(t, parent, ModeReal)
		cfg.Server.LicenseKey = "fixture"
		cfg.Relay.Enabled = true
		cfg.Relay.CacheDir = t.TempDir()
		client, err := NewClient(cfg.Server)
		if err != nil {
			t.Fatal(err)
		}
		relay, err := NewRelay(cfg, client, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		mux := http.NewServeMux()
		mux.HandleFunc("POST /api/v1/updates/{product}/check", relay.handleChildCheck)
		return httptest.NewServer(mux)
	}
	first := hop(origin.URL)
	defer first.Close()
	second := hop(first.URL)
	defer second.Close()
	cfg := newSimulatorTestConfig(t, second.URL, ModeReal)
	cfg.Server.LicenseKey = "fixture"
	client, err := NewClient(cfg.Server)
	if err != nil {
		t.Fatal(err)
	}
	offer, err := client.CheckUpdate(context.Background(), "siemcore", UpdateCheckRequest{InstanceID: "testing", CurrentVersion: "3.3.152.26", PolicyAuthorizationVersion: "mysoc-policy-authorization-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(offer.PolicyAuthorization) != string(raw) {
		t.Fatalf("grant changed: %s", offer.PolicyAuthorization)
	}
	if !strings.HasPrefix(offer.DownloadURL, "/api/v1/releases/siemcore/") {
		t.Fatal("artifact bypassed relay")
	}
}
