package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/releases"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

func TestHealthReportsVersionAndCommit(t *testing.T) {
	oldV, oldC := Version, GitCommit
	Version, GitCommit = "1.16.2.1", "0123456789ab"
	defer func() { Version, GitCommit = oldV, oldC }()

	rr := httptest.NewRecorder()
	newTestServer("k").Router().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "1.16.2.1" || body["commit"] != "0123456789ab" {
		t.Fatalf("unexpected health body: %v", body)
	}
}

func TestIssuerSealingHTTP(t *testing.T) {
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
	name := "sealapi_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	seed, _ := signing.GenerateSeedHex()
	key, _ := signing.ParseSeedHex(seed)
	cfg := &config.Config{}
	cfg.Server.APIKey = "admin-key"
	cfg.Server.CORSOrigins = []string{"*"}
	cfg.Server.SigningKeySeed = seed
	cfg.Auth.JWTSecret = "test-secret"
	router := NewServer(cfg, db, store).Router()

	do := func(req *http.Request) (int, map[string]any) {
		req.Header.Set("X-API-Key", "admin-key")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		var body map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		return rr.Code, body
	}
	upload := func(product, version string, payload []byte, fields map[string]string) (int, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("product", product)
		_ = mw.WriteField("version", version)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		fw, _ := mw.CreateFormFile("artifact", product+".tar.gz")
		_, _ = fw.Write(payload)
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/releases/", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return do(req)
	}

	_, sk := do(httptest.NewRequest(http.MethodGet, "/api/v1/signing-key", nil))
	keyID, _ := sk["key_id"].(string)
	if keyID != releases.KeyID(key.Public().(ed25519.PublicKey)) {
		t.Fatalf("signing-key must publish the key id, got %v", sk)
	}

	code, list := do(httptest.NewRequest(http.MethodGet, "/api/v1/admin/trusted-keys", nil))
	if keys, _ := list["keys"].([]any); code != http.StatusOK || len(keys) != 1 {
		t.Fatalf("registry must start with the server key: %d %v", code, list)
	}

	code, rel := upload("siemcore", "2.0.1", []byte("unsealed"), nil)
	if code != http.StatusCreated || rel["seal_status"] != "unsealed" || rel["issuer"] != "siemcore" || rel["signature"] == "" {
		t.Fatalf("unsealed upload: %d %v", code, rel)
	}

	payload := []byte("sealed")
	sum := sha256.Sum256(payload)
	seal := signing.Sign(key, "swf", "3.0.0", hex.EncodeToString(sum[:]))
	code, rel = upload("swf", "3.0.0", payload, map[string]string{"issuer_signature": seal, "issuer_key_id": keyID})
	if code != http.StatusCreated || rel["seal_status"] != "sealed" || rel["issuer_key_id"] != keyID || rel["signature"] != seal {
		t.Fatalf("sealed upload: %d %v", code, rel)
	}

	code, rel = upload("swf", "3.0.1", []byte("x"), map[string]string{"issuer_signature": "bogus"})
	if code != http.StatusCreated || rel["seal_status"] != "invalid" {
		t.Fatalf("invalid seal must be accepted and flagged: %d %v", code, rel)
	}

	if code, _ := upload("swf", "3.0.0", []byte("overwrite"), nil); code != http.StatusConflict {
		t.Fatalf("re-upload must be 409, got %d", code)
	}

	put := func() int {
		code, _ := do(httptest.NewRequest(http.MethodPut, "/api/v1/releases/siemcore/2.0.1/siemcore-linux-arm64", strings.NewReader("bin")))
		return code
	}
	if c := put(); c != http.StatusOK {
		t.Fatalf("first binary upload: %d", c)
	}
	if c := put(); c != http.StatusConflict {
		t.Fatalf("binary overwrite must be 409, got %d", c)
	}
	if code, _ := do(httptest.NewRequest(http.MethodPut, "/api/v1/releases/siemcore/2.0.1/siemcore.tar.gz", strings.NewReader("swap"))); code != http.StatusConflict {
		t.Fatalf("overwriting a release artifact through the binary route must be 409, got %d", code)
	}

	addKey := func() (int, map[string]any) {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		body, _ := json.Marshal(map[string]string{"public_key": hex.EncodeToString(pub), "label": "next"})
		return do(httptest.NewRequest(http.MethodPost, "/api/v1/admin/trusted-keys", bytes.NewReader(body)))
	}
	code, next := addKey()
	if code != http.StatusCreated {
		t.Fatalf("add next key: %d %v", code, next)
	}
	if code, _ := addKey(); code != http.StatusConflict {
		t.Fatalf("third active key must be 409, got %d", code)
	}
	if code, _ := do(httptest.NewRequest(http.MethodPost, "/api/v1/admin/trusted-keys", strings.NewReader(`{"public_key":"nothex"}`))); code != http.StatusBadRequest {
		t.Fatalf("malformed key must be 400, got %d", code)
	}
	code, retired := do(httptest.NewRequest(http.MethodPost, "/api/v1/admin/trusted-keys/"+next["id"].(string)+"/retire", nil))
	if code != http.StatusOK || retired["status"] != "retired" {
		t.Fatalf("retire: %d %v", code, retired)
	}
	if code, _ := do(httptest.NewRequest(http.MethodPost, "/api/v1/admin/trusted-keys/"+uuid.NewString()+"/retire", nil)); code != http.StatusNotFound {
		t.Fatalf("retire unknown must be 404, got %d", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/trusted-keys", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("trusted keys must require admin auth, got %d", rr.Code)
	}
}
