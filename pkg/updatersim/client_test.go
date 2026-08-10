package updatersim

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func TestClientSendHeartbeatParsesUpdates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/heartbeat" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-License-Key"); got != "test-license" {
			t.Fatalf("unexpected license header: %q", got)
		}
		if got := r.Header.Get("X-API-Key"); got != "test-api-key" {
			t.Fatalf("unexpected API key header: %q", got)
		}

		var heartbeat platformtypes.Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
			t.Fatalf("decode heartbeat: %v", err)
		}
		if heartbeat.InstanceID != "sim-test" {
			t.Fatalf("unexpected instance id: %q", heartbeat.InstanceID)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(HeartbeatResponse{
			Status: "ok",
			Updates: []platformtypes.ReleaseInfo{{
				Product:         "siemcore",
				CurrentVersion:  "1.0.0",
				LatestVersion:   "1.1.0",
				UpdateAvailable: true,
				DownloadURL:     "/artifact",
				Checksum:        "abc",
			}},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.SendHeartbeat(context.Background(), platformtypes.Heartbeat{
		InstanceID: "sim-test",
	})
	if err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}
	if len(response.Updates) != 1 || response.Updates[0].LatestVersion != "1.1.0" {
		t.Fatalf("unexpected heartbeat response: %#v", response)
	}
}

func TestClientCheckUpdateUsesPolicyContract(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/updates/swf/check" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var request UpdateCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode update check: %v", err)
		}
		if request.InstanceID != "sim-swf" || request.CurrentVersion != "2.0.0" {
			t.Fatalf("unexpected update check: %#v", request)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UpdateCheckResponse{
			UpdateAvailable: true,
			LatestVersion:   "2.1.0",
			DownloadURL:     serverURL(r) + "/artifact",
			SHA256:          "sha256:ABCDEF",
			Channel:         "stable",
			UpdateGroup:     "alpha",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	offer, err := client.CheckUpdate(context.Background(), "swf", UpdateCheckRequest{
		InstanceID:     "sim-swf",
		CurrentVersion: "2.0.0",
	})
	if err != nil {
		t.Fatalf("check update: %v", err)
	}
	if !offer.UpdateAvailable ||
		offer.LatestVersion != "2.1.0" ||
		offer.Checksum != "abcdef" ||
		offer.Source != "policy" {
		t.Fatalf("unexpected offer: %#v", offer)
	}
}

func TestClientDownloadArtifactResolvesRelativeURLAndVerifiesSHA256(t *testing.T) {
	t.Parallel()

	content := []byte("verified simulator artifact")
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifacts/test" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("X-Checksum-SHA256", checksum)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.DownloadArtifact(
		context.Background(),
		"/artifacts/test",
		"sha256:"+checksum,
		t.TempDir(),
		"../siemcore 1.2.3.bin",
		1024,
	)
	if err != nil {
		t.Fatalf("download artifact: %v", err)
	}
	if filepath.Base(result.Path) != "siemcore_1.2.3.bin" {
		t.Fatalf("artifact name was not sanitized: %s", result.Path)
	}
	if result.Checksum != checksum || result.Size != int64(len(content)) {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("unexpected artifact contents: %q", data)
	}
}

func TestClientReturnsTypedAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"test credential rejected"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.SendHeartbeat(context.Background(), platformtypes.Heartbeat{
		InstanceID: "sim-test",
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "test credential rejected" {
		t.Fatalf("unexpected APIError: %#v", apiErr)
	}
}

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(ServerConfig{
		URL:              serverURL,
		LicenseKey:       "test-license",
		APIKey:           "test-api-key",
		Timeout:          Duration{Duration: time.Second},
		MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
