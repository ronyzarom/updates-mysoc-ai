package updatersim

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newDecommissionTestRelay builds a relay with a stable cache dir so tests can
// exercise tombstone persistence across "restarts" (fresh NewRelay calls).
func newDecommissionTestRelay(t *testing.T, cacheDir string, withCustomer bool) *Relay {
	t.Helper()
	cfg := &Config{}
	cfg.Instance.ID = "siemcore-relay-01"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "siemcore"
	if withCustomer {
		cfg.Instance.CustomerID = "acme"
		cfg.Instance.CustomerName = "Acme Corp"
	}
	cfg.Relay.Enabled = true
	cfg.Relay.Listen = "127.0.0.1:0"
	cfg.Relay.CacheDir = cacheDir
	cfg.Server.URL = "http://127.0.0.1:1"

	client, err := NewClient(cfg.Server)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := NewRelay(cfg, client, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	return relay
}

// heartbeatChild sends one child heartbeat and returns the decoded response.
func heartbeatChild(t *testing.T, relay *Relay, instanceID, token string) map[string]interface{} {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"instance_id": instanceID})
	req := httptest.NewRequest("POST", "/api/v1/heartbeat", bytes.NewReader(body))
	req.Header.Set("X-License-Key", "device-secret")
	if token != "" {
		req.Header.Set("X-Relay-Token", token)
	}
	rec := httptest.NewRecorder()
	relay.handleChildHeartbeat(rec, req)
	if rec.Code != 200 {
		t.Fatalf("heartbeat returned %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

// heartbeatChildRaw sends one child heartbeat and returns the raw status code
// plus the decoded JSON body, so tests can assert rejections and their reason
// code (heartbeatChild fatals on non-200).
func heartbeatChildRaw(relay *Relay, instanceID, token string) (int, map[string]string) {
	body, _ := json.Marshal(map[string]string{"instance_id": instanceID})
	req := httptest.NewRequest("POST", "/api/v1/heartbeat", bytes.NewReader(body))
	req.Header.Set("X-License-Key", "device-secret")
	if token != "" {
		req.Header.Set("X-Relay-Token", token)
	}
	rec := httptest.NewRecorder()
	relay.handleChildHeartbeat(rec, req)
	var decoded map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	return rec.Code, decoded
}

// decommissionChildRaw posts a decommission and returns the status code plus the
// decoded JSON body, exercising the authorizeChild path with its reason code.
func decommissionChildRaw(relay *Relay, instanceID, license, token string) (int, map[string]string) {
	body, _ := json.Marshal(map[string]string{"instance_id": instanceID})
	req := httptest.NewRequest("POST", "/api/v1/decommission", bytes.NewReader(body))
	if license != "" {
		req.Header.Set("X-License-Key", license)
	}
	if token != "" {
		req.Header.Set("X-Relay-Token", token)
	}
	rec := httptest.NewRecorder()
	relay.handleChildDecommission(rec, req)
	var decoded map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	return rec.Code, decoded
}

// decommissionChild posts a decommission and returns the response code.
func decommissionChild(t *testing.T, relay *Relay, instanceID, license, token string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"instance_id": instanceID})
	req := httptest.NewRequest("POST", "/api/v1/decommission", bytes.NewReader(body))
	if license != "" {
		req.Header.Set("X-License-Key", license)
	}
	if token != "" {
		req.Header.Set("X-Relay-Token", token)
	}
	rec := httptest.NewRecorder()
	relay.handleChildDecommission(rec, req)
	return rec.Code
}

func TestEnrollmentResponseCarriesIdentity(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	resp := heartbeatChild(t, relay, "swf-node-1", "")

	identity, ok := resp["identity"].(map[string]interface{})
	if !ok {
		t.Fatalf("enrollment response has no identity object: %v", resp)
	}
	if identity["parent_instance_id"] != "siemcore-relay-01" {
		t.Fatalf("identity parent wrong: %v", identity)
	}
	if identity["customer_id"] != "acme" || identity["customer_name"] != "Acme Corp" {
		t.Fatalf("identity customer fields wrong: %v", identity)
	}
}

func TestIdentityOmitsUnknownFields(t *testing.T) {
	// A relay with no configured customer identity returns only what it
	// knows — fields are omitted, never sent as empty strings.
	relay := newDecommissionTestRelay(t, t.TempDir(), false)
	resp := heartbeatChild(t, relay, "swf-node-1", "")

	identity, ok := resp["identity"].(map[string]interface{})
	if !ok {
		t.Fatalf("enrollment response has no identity object: %v", resp)
	}
	if identity["parent_instance_id"] != "siemcore-relay-01" {
		t.Fatalf("identity parent wrong: %v", identity)
	}
	if _, present := identity["customer_id"]; present {
		t.Fatalf("customer_id must be omitted when unconfigured: %v", identity)
	}
	if _, present := identity["customer_name"]; present {
		t.Fatalf("customer_name must be omitted when unconfigured: %v", identity)
	}
}

func TestDecommissionMarksChildAndRollsUp(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	resp := heartbeatChild(t, relay, "swf-node-1", "")
	token, _ := resp["relay_token"].(string)

	if code := decommissionChild(t, relay, "swf-node-1", "device-secret", token); code != 200 {
		t.Fatalf("decommission returned %d", code)
	}
	// Idempotent: the second goodbye acks too.
	if code := decommissionChild(t, relay, "swf-node-1", "device-secret", token); code != 200 {
		t.Fatalf("repeat decommission returned %d", code)
	}

	reports := relay.ChildrenReport()
	if len(reports) != 1 {
		t.Fatalf("expected 1 child report, got %d", len(reports))
	}
	if reports[0].Status != "decommissioned" {
		t.Fatalf("expected decommissioned status, got %q", reports[0].Status)
	}
	// The mark's own timestamp is the reported last-seen moment, so the
	// server's rollup freshness guard cannot discard the status.
	relay.mu.Lock()
	markAt := relay.children["swf-node-1"].DecommissionedAt
	relay.mu.Unlock()
	if !reports[0].LastSeen.Equal(markAt) {
		t.Fatalf("report LastSeen %v != mark time %v", reports[0].LastSeen, markAt)
	}
}

func TestDecommissionUnknownChildCreatesTombstone(t *testing.T) {
	// A relay restart may have forgotten the child; the goodbye must still
	// mark and roll up.
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	if code := decommissionChild(t, relay, "swf-forgotten", "device-secret", ""); code != 200 {
		t.Fatalf("decommission returned %d", code)
	}
	reports := relay.ChildrenReport()
	if len(reports) != 1 || reports[0].Status != "decommissioned" {
		t.Fatalf("tombstone did not roll up: %+v", reports)
	}
}

func TestHeartbeatRevivesDecommissionedChild(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	resp := heartbeatChild(t, relay, "swf-node-1", "")
	token, _ := resp["relay_token"].(string)
	if code := decommissionChild(t, relay, "swf-node-1", "device-secret", token); code != 200 {
		t.Fatalf("decommission returned %d", code)
	}

	// A genuine heartbeat contradicts the mark: honest revival.
	heartbeatChild(t, relay, "swf-node-1", token)
	reports := relay.ChildrenReport()
	if len(reports) != 1 || reports[0].Status != "online" {
		t.Fatalf("expected revival to online, got %+v", reports)
	}
}

func TestReenrollAfterDecommissionPurge(t *testing.T) {
	// A decommission followed by a client-state purge wipes the child's saved
	// relay token. The reinstalled node must be able to reclaim its instance_id
	// by presenting no token — the relay re-binds a fresh one and revives it,
	// rather than locking it out with a token mismatch.
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	resp := heartbeatChild(t, relay, "swf-node-1", "")
	token, _ := resp["relay_token"].(string)
	if token == "" {
		t.Fatal("enrollment did not issue a relay token")
	}
	if code := decommissionChild(t, relay, "swf-node-1", "device-secret", token); code != 200 {
		t.Fatalf("decommission returned %d", code)
	}

	// Purge: the client lost its token and re-enrolls with none.
	revived := heartbeatChild(t, relay, "swf-node-1", "")
	newToken, _ := revived["relay_token"].(string)
	if newToken == "" {
		t.Fatal("re-enrollment after purge did not issue a fresh relay token")
	}
	reports := relay.ChildrenReport()
	if len(reports) != 1 || reports[0].Status != "online" {
		t.Fatalf("expected re-enrollment to revive to online, got %+v", reports)
	}
}

func TestLiveTokenMismatchStillRejected(t *testing.T) {
	// A node that loses its token WITHOUT decommissioning still faces a live
	// binding: presenting a wrong or empty token must be rejected (anti-hijack).
	// The reject carries a distinguishable code so a client can tell the
	// retryable first-contact race (absent token) from a genuine mismatch.
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	heartbeatChild(t, relay, "swf-node-1", "") // enroll: establishes a live token binding

	code, body := heartbeatChildRaw(relay, "swf-node-1", "wrong-token")
	if code != 401 || body["code"] != "relay_token_mismatch" {
		t.Fatalf("wrong token: got %d code=%q, want 401 relay_token_mismatch", code, body["code"])
	}
	code, body = heartbeatChildRaw(relay, "swf-node-1", "")
	if code != 401 || body["code"] != "relay_token_absent" {
		t.Fatalf("empty token: got %d code=%q, want 401 relay_token_absent", code, body["code"])
	}
}

func TestAuthorizeChildRejectCodes(t *testing.T) {
	// The other child endpoints (via authorizeChild) carry the same reason
	// codes: an absent token against a live binding is the retryable race,
	// a wrong token is a genuine mismatch.
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	heartbeatChild(t, relay, "swf-node-1", "") // enroll

	code, body := decommissionChildRaw(relay, "swf-node-1", "device-secret", "")
	if code != 401 || body["code"] != "relay_token_absent" {
		t.Fatalf("absent token: got %d code=%q, want 401 relay_token_absent", code, body["code"])
	}
	code, body = decommissionChildRaw(relay, "swf-node-1", "device-secret", "wrong-token")
	if code != 401 || body["code"] != "relay_token_mismatch" {
		t.Fatalf("wrong token: got %d code=%q, want 401 relay_token_mismatch", code, body["code"])
	}
	// The rejected calls must not have applied a decommission mark.
	for _, rep := range relay.ChildrenReport() {
		if rep.Status == "decommissioned" {
			t.Fatalf("rejected call applied a mark: %+v", rep)
		}
	}
}

func TestTombstonesSurviveRelayRestart(t *testing.T) {
	cacheDir := t.TempDir()
	relay := newDecommissionTestRelay(t, cacheDir, true)
	resp := heartbeatChild(t, relay, "swf-node-1", "")
	token, _ := resp["relay_token"].(string)
	if code := decommissionChild(t, relay, "swf-node-1", "device-secret", token); code != 200 {
		t.Fatalf("decommission returned %d", code)
	}

	// "Restart": a fresh relay over the same cache dir must still deliver
	// the mark in its rollup.
	restarted := newDecommissionTestRelay(t, cacheDir, true)
	reports := restarted.ChildrenReport()
	if len(reports) != 1 || reports[0].Status != "decommissioned" {
		t.Fatalf("mark lost across restart: %+v", reports)
	}

	// Revival must also clear the persisted tombstone, not only memory.
	heartbeatChild(t, restarted, "swf-node-1", token)
	secondRestart := newDecommissionTestRelay(t, cacheDir, true)
	for _, rep := range secondRestart.ChildrenReport() {
		if rep.Status == "decommissioned" {
			t.Fatalf("tombstone not cleared after revival: %+v", rep)
		}
	}
}

func TestDecommissionExpiryPrunesRelayState(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	if code := decommissionChild(t, relay, "swf-old", "device-secret", ""); code != 200 {
		t.Fatalf("decommission returned %d", code)
	}
	relay.mu.Lock()
	relay.children["swf-old"].DecommissionedAt = time.Now().Add(-decommissionRetention - time.Hour)
	relay.mu.Unlock()

	if reports := relay.ChildrenReport(); len(reports) != 0 {
		t.Fatalf("expired decommissioned child not pruned: %+v", reports)
	}
	relay.mu.Lock()
	_, still := relay.children["swf-old"]
	relay.mu.Unlock()
	if still {
		t.Fatal("expired child still occupies relay state")
	}
}

func TestGuardPermitsDecommissionFromUnknownSource(t *testing.T) {
	// Guard learning is in-memory: a goodbye arriving right after a relay
	// restart comes from an IP the guard has never seen and must still pass
	// path policy (authentication then happens in the handler as usual),
	// while other routes stay learned-only.
	g := newRelayGuard(0)
	reached := ""
	mw := g.middleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reached = req.URL.Path
	}))

	req := httptest.NewRequest("POST", "/api/v1/decommission", nil)
	req.RemoteAddr = "203.0.113.9:4444"
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if reached != "/api/v1/decommission" {
		t.Fatalf("decommission blocked by guard for unknown source: %d %s", rec.Code, rec.Body.String())
	}

	reached = ""
	req = httptest.NewRequest("GET", "/api/v1/releases/swf/1.0.0.0/download", nil)
	req.RemoteAddr = "203.0.113.9:4444"
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if reached != "" || rec.Code != 403 {
		t.Fatalf("download must stay learned-only for unknown sources, got %d (reached %q)", rec.Code, reached)
	}
}

func TestDecommissionRequiresCredentials(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	resp := heartbeatChild(t, relay, "swf-node-1", "")
	token, _ := resp["relay_token"].(string)

	if code := decommissionChild(t, relay, "swf-node-1", "", token); code != 401 {
		t.Fatalf("missing credential must 401, got %d", code)
	}
	if code := decommissionChild(t, relay, "swf-node-1", "device-secret", "wrong-token"); code != 401 {
		t.Fatalf("token mismatch must 401, got %d", code)
	}
	// The mark must not have been applied by the rejected calls.
	for _, rep := range relay.ChildrenReport() {
		if rep.Status == "decommissioned" {
			t.Fatalf("unauthorized call applied a mark: %+v", rep)
		}
	}
}
