package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// TestOperatorPlatformKeyAuthorizesWideCoreAndCheckHonorsRequestedRing proves
// two enrolment invariants end-to-end against Postgres:
//  1. CreateOperator mints a platform key whose Products include "widecore",
//     so a WideCore leaf behind that operator's mysoc relay is authorized
//     without manual license edits.
//  2. When a brand-new relayed leaf's first POST /api/v1/updates/widecore/check
//     arrives upstream before the relay's first rollup heartbeat, the server
//     honors requested_update_group on INSERT (offering alpha immediately) and
//     never lets a subsequent check self-promote the ring on ON CONFLICT.
func TestOperatorPlatformKeyAuthorizesWideCoreAndCheckHonorsRequestedRing(t *testing.T) {
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
	name := "reqgroup_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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

	// 1. Create an operator and confirm its platform license covers widecore.
	opSvc := licensing.NewService(db)
	op, err := opSvc.CreateOperator(ctx, "op-widecore", "WideCore Operator", time.Now().AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("CreateOperator: %v", err)
	}
	if op.License == nil {
		t.Fatal("CreateOperator returned nil license")
	}
	if !slices.Contains(op.License.Products, "widecore") {
		t.Fatalf("operator platform license products = %v, want widecore included", op.License.Products)
	}
	licKey := op.License.LicenseKey

	// 2. Upload a widecore 1.1.0.2 release targeted only at alpha.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("product", "widecore")
	_ = mw.WriteField("version", "1.1.0.2")
	_ = mw.WriteField("target_groups", "alpha")
	fw, _ := mw.CreateFormFile("artifact", "widecore-1.1.0.2.tar.gz")
	_, _ = fw.Write([]byte("fake-widecore-tarball"))
	_ = mw.Close()
	upReq := httptest.NewRequest(http.MethodPost, "/api/v1/releases", &body)
	upReq.Header.Set("Content-Type", mw.FormDataContentType())
	upReq.Header.Set("X-API-Key", "admin-key")
	upRec := httptest.NewRecorder()
	router.ServeHTTP(upRec, upReq)
	if upRec.Code != http.StatusCreated {
		t.Fatalf("upload widecore alpha: status=%d body=%s", upRec.Code, upRec.Body.String())
	}

	// 3. Register the parent mysoc relay via heartbeat.
	hbPayload, _ := json.Marshal(types.Heartbeat{
		InstanceID:  "mysoc-op-relay",
		ProductTier: "mysoc",
		Hostname:    "mysoc-op-relay",
	})
	hbReq := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", bytes.NewReader(hbPayload))
	hbReq.Header.Set("Content-Type", "application/json")
	hbReq.Header.Set("X-License-Key", licKey)
	hbRec := httptest.NewRecorder()
	router.ServeHTTP(hbRec, hbReq)
	if hbRec.Code != http.StatusOK {
		t.Fatalf("parent relay heartbeat: status=%d body=%s", hbRec.Code, hbRec.Body.String())
	}

	// 4. First contact from a brand-new widecore leaf arrives as an update check
	//    requesting alpha (before the relay's first rollup heartbeat).
	checkPayload, _ := json.Marshal(UpdateCheckRequest{
		InstanceID:           "widecore-leaf-1",
		CurrentVersion:       "0.0.0",
		UpdaterVersion:       "1.21.1.1",
		OS:                   "linux",
		Arch:                 "amd64",
		Hostname:             "widecore-leaf-1",
		Channel:              "stable",
		ProductTier:          "widecore",
		ParentInstanceID:     "mysoc-op-relay",
		RequestedUpdateGroup: "alpha",
	})
	chkReq := httptest.NewRequest(http.MethodPost, "/api/v1/updates/widecore/check", bytes.NewReader(checkPayload))
	chkReq.Header.Set("Content-Type", "application/json")
	chkReq.Header.Set("X-License-Key", licKey)
	chkRec := httptest.NewRecorder()
	router.ServeHTTP(chkRec, chkReq)
	if chkRec.Code != http.StatusOK {
		t.Fatalf("widecore first check: status=%d body=%s", chkRec.Code, chkRec.Body.String())
	}
	var chkResp struct {
		UpdateAvailable bool   `json:"update_available"`
		LatestVersion   string `json:"latest_version"`
		UpdateGroup     string `json:"update_group"`
	}
	if err := json.Unmarshal(chkRec.Body.Bytes(), &chkResp); err != nil {
		t.Fatalf("decode check response: %v", err)
	}
	if !chkResp.UpdateAvailable || chkResp.LatestVersion != "1.1.0.2" || chkResp.UpdateGroup != "alpha" {
		t.Fatalf("first check response = %+v, want update_available=true latest_version=1.1.0.2 update_group=alpha", chkResp)
	}

	// 5. Subsequent check requesting "production" must NOT mutate the existing row's ring.
	promotePayload, _ := json.Marshal(UpdateCheckRequest{
		InstanceID:           "widecore-leaf-1",
		CurrentVersion:       "1.1.0.2",
		UpdaterVersion:       "1.21.1.1",
		OS:                   "linux",
		Arch:                 "amd64",
		Hostname:             "widecore-leaf-1",
		Channel:              "stable",
		ProductTier:          "widecore",
		ParentInstanceID:     "mysoc-op-relay",
		RequestedUpdateGroup: "production",
	})
	promoReq := httptest.NewRequest(http.MethodPost, "/api/v1/updates/widecore/check", bytes.NewReader(promotePayload))
	promoReq.Header.Set("Content-Type", "application/json")
	promoReq.Header.Set("X-License-Key", licKey)
	promoRec := httptest.NewRecorder()
	router.ServeHTTP(promoRec, promoReq)
	if promoRec.Code != http.StatusOK {
		t.Fatalf("second check: status=%d body=%s", promoRec.Code, promoRec.Body.String())
	}
	inst, err := licensing.NewInstanceRepository(db).GetByInstanceID(ctx, "widecore-leaf-1")
	if err != nil || inst == nil {
		t.Fatalf("GetByInstanceID: inst=%v err=%v", inst, err)
	}
	if inst.UpdateGroup != "alpha" {
		t.Fatalf("instance update_group mutated to %q on subsequent check, want alpha", inst.UpdateGroup)
	}
}
