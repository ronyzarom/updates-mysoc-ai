package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
)

func TestValidateCumulativeGroups(t *testing.T) {
	cases := []struct {
		name   string
		groups []string
		ok     bool
	}{
		{"alpha only", []string{"alpha"}, true},
		{"through beta", []string{"alpha", "beta"}, true},
		{"through stable", []string{"alpha", "beta", "stable"}, true},
		{"all four", []string{"alpha", "beta", "stable", "production"}, true},
		{"unordered all four", []string{"production", "alpha", "stable", "beta"}, true},
		{"empty", nil, false},
		{"production only", []string{"production"}, false},
		{"skips beta", []string{"alpha", "stable"}, false},
		{"stable without beta", []string{"alpha", "beta", "production"}, false},
		{"unknown ring", []string{"alpha", "nightly"}, false},
		{"duplicate", []string{"alpha", "alpha"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCumulativeGroups(tc.groups)
			if (err == nil) != tc.ok {
				t.Fatalf("validateCumulativeGroups(%v) err=%v, want ok=%v", tc.groups, err, tc.ok)
			}
		})
	}
}

// An explicit empty list on /target-groups withdraws a release (the documented
// way to pull a bad release while keeping its record); a body without
// target_groups must not, and a non-empty list must still be cumulative.
func TestTargetGroupsWithdrawHTTP(t *testing.T) {
	url := os.Getenv("UPDATES_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable UPDATES_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "withdraw_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	pcfg, _ := pgxpool.ParseConfig(url)
	pcfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := &database.DB{Pool: pool}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Server.APIKey = "admin-key"
	cfg.Server.CORSOrigins = []string{"*"}
	cfg.Auth.JWTSecret = "test-secret"
	router := NewServer(cfg, db, store).Router()

	do := func(req *http.Request) int {
		req.Header.Set("X-API-Key", "admin-key")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr.Code
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("product", "swf")
	_ = mw.WriteField("version", "2.0.0.1")
	_ = mw.WriteField("target_groups", "alpha,beta")
	fw, _ := mw.CreateFormFile("artifact", "swf.tar.gz")
	_, _ = fw.Write([]byte("artifact"))
	_ = mw.Close()
	up := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", &buf)
	up.Header.Set("Content-Type", mw.FormDataContentType())
	if code := do(up); code != http.StatusCreated {
		t.Fatalf("upload: want 201, got %d", code)
	}

	groups := func() []string {
		var g []string
		if err := pool.QueryRow(ctx, "SELECT target_groups FROM releases WHERE product_name='swf' AND version='2.0.0.1'").Scan(&g); err != nil {
			t.Fatal(err)
		}
		return g
	}
	put := func(body string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/releases/swf/2.0.0.1/target-groups", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		return do(req)
	}

	if code := put(`{}`); code != http.StatusBadRequest {
		t.Fatalf("missing target_groups: want 400, got %d", code)
	}
	if g := groups(); len(g) != 2 {
		t.Fatalf("missing target_groups must not change rings, got %v", g)
	}
	if code := put(`{"target_groups":["beta","stable"]}`); code != http.StatusBadRequest {
		t.Fatalf("skipped ring: want 400, got %d", code)
	}
	if code := put(`{"target_groups":[]}`); code != http.StatusOK {
		t.Fatalf("withdraw: want 200, got %d", code)
	}
	if g := groups(); len(g) != 0 {
		t.Fatalf("withdraw should clear every ring, got %v", g)
	}
	if code := put(`{"target_groups":["alpha"]}`); code != http.StatusOK {
		t.Fatalf("re-offer to alpha: want 200, got %d", code)
	}
	if g := groups(); len(g) != 1 || g[0] != "alpha" {
		t.Fatalf("re-offer should land on alpha, got %v", g)
	}

	// POST /api/v1/releases/ must also enforce cumulative target_groups:
	// production-only or skipped rings are 400; omitted defaults to alpha only;
	// product=widecore is accepted.
	uploadGroups := func(product, ver, tg string) int {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		_ = w.WriteField("product", product)
		_ = w.WriteField("version", ver)
		if tg != "" {
			_ = w.WriteField("target_groups", tg)
		}
		fw, _ := w.CreateFormFile("artifact", product+".tar.gz")
		_, _ = fw.Write([]byte("artifact"))
		_ = w.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", &b)
		req.Header.Set("Content-Type", w.FormDataContentType())
		return do(req)
	}
	if code := uploadGroups("widecore", "1.0.0.1", "production"); code != http.StatusBadRequest {
		t.Fatalf("POST production-only: want 400, got %d", code)
	}
	if code := uploadGroups("widecore", "1.0.0.1", "alpha,stable"); code != http.StatusBadRequest {
		t.Fatalf("POST skipped beta: want 400, got %d", code)
	}
	if code := uploadGroups("widecore", "1.0.0.1", "alpha,alpha"); code != http.StatusBadRequest {
		t.Fatalf("POST duplicate ring: want 400, got %d", code)
	}
	if code := uploadGroups("widecore", "1.0.0.1", "alpha"); code != http.StatusCreated {
		t.Fatalf("POST widecore alpha: want 201, got %d", code)
	}
}
