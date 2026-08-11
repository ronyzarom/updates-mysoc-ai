package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/auth"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
)

// newTestServer builds a Server wired with a real auth service (backed by a nil
// repository, which is never reached in these tests because authentication
// fails before any database lookup) and the given admin API key.
func newTestServer(apiKey string) *Server {
	svc := auth.NewService(auth.NewRepository(nil), "test-secret", "test-issuer")
	cfg := &config.Config{}
	cfg.Server.APIKey = apiKey
	cfg.Server.CORSOrigins = []string{"*"}
	s := &Server{config: cfg, authService: svc}
	s.setupRoutes()
	return s
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func runAdminAuth(s *Server, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	s.adminAuth(okHandler()).ServeHTTP(rr, req)
	return rr
}

// TestAdminAuthFailsClosedWithoutConfiguredKey is the core regression guard:
// when no admin API key is configured, adminAuth must NOT bypass auth. A caller
// presenting an X-API-Key with no key configured must be rejected.
func TestAdminAuthFailsClosedWithoutConfiguredKey(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", nil)
	req.Header.Set("X-API-Key", "anything")
	if rr := runAdminAuth(s, req); rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when no admin key configured, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/releases/", nil)
	if rr := runAdminAuth(s, req); rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no credentials, got %d", rr.Code)
	}
}

func TestAdminAuthAPIKey(t *testing.T) {
	s := newTestServer("secret-key")

	t.Run("correct header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", nil)
		req.Header.Set("X-API-Key", "secret-key")
		if rr := runAdminAuth(s, req); rr.Code != http.StatusOK {
			t.Fatalf("expected 200 with correct API key, got %d", rr.Code)
		}
	})

	t.Run("wrong header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", nil)
		req.Header.Set("X-API-Key", "wrong")
		if rr := runAdminAuth(s, req); rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 with wrong API key, got %d", rr.Code)
		}
	})

	// Query-string API keys must never be honored.
	t.Run("query string rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/?api_key=secret-key", nil)
		if rr := runAdminAuth(s, req); rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for api_key query string, got %d", rr.Code)
		}
	})

	t.Run("no credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", nil)
		if rr := runAdminAuth(s, req); rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 with no credentials, got %d", rr.Code)
		}
	})

	t.Run("malformed bearer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", nil)
		req.Header.Set("Authorization", "Bearer not-a-real-token")
		if rr := runAdminAuth(s, req); rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 with invalid token, got %d", rr.Code)
		}
	})
}

// TestProtectedRoutesRejectAnonymous asserts that sensitive fleet/license reads
// and rollout mutations are unreachable without credentials.
func TestProtectedRoutesRejectAnonymous(t *testing.T) {
	s := newTestServer("secret-key")
	router := s.Router()

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/instances"},
		{http.MethodGet, "/api/v1/instances/paged"},
		{http.MethodGet, "/api/v1/instances/abc"},
		{http.MethodGet, "/api/v1/admin/licenses"},
		{http.MethodGet, "/api/v1/admin/licenses/abc"},
		{http.MethodPut, "/api/v1/instances/abc/update-group"},
		{http.MethodPut, "/api/v1/instances/abc/auto-update"},
		{http.MethodPost, "/api/v1/releases/"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for anonymous %s %s, got %d", tc.method, tc.path, rr.Code)
			}
		})
	}
}

// TestPublicRoutesRemainOpen guards the updater-facing contract: enrollment,
// heartbeat, policy check, latest-release, and health must not require auth.
func TestPublicRoutesRemainOpen(t *testing.T) {
	s := newTestServer("secret-key")
	router := s.Router()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("health endpoint must not require auth, got %d", rr.Code)
	}
}
