package releases

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

// freshMigratedDB creates a throwaway database, migrates it to head with the
// embedded migrations and drops it when the test ends.
func freshMigratedDB(t *testing.T) *database.DB {
	t.Helper()
	url := os.Getenv("UPDATES_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable UPDATES_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	name := "seal_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close()
	})
	db := &database.DB{Pool: pool}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second migrate must be a no-op: %v", err)
	}
	return db
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestIssuerSealingDatabase(t *testing.T) {
	db := freshMigratedDB(t)
	ctx := context.Background()
	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	svc := NewService(db, store)
	svc.SetSigningKey(key)

	added, err := svc.EnsureSigningKeyTrusted(ctx)
	if err != nil || !added {
		t.Fatalf("first start must register the release key: added=%v err=%v", added, err)
	}
	if added, err := svc.EnsureSigningKeyTrusted(ctx); err != nil || added {
		t.Fatalf("restart must not re-register: added=%v err=%v", added, err)
	}
	serverKeyID := KeyID(pub)

	upload := func(product, version string, body []byte, seal, keyID string) (*CreateReleaseRequest, error) {
		req := CreateReleaseRequest{
			ProductName: product, Version: version, Channel: "stable",
			Filename: product + ".tar.gz", FileSize: int64(len(body)), File: bytes.NewReader(body),
			IssuerSignature: seal, IssuerKeyID: keyID,
		}
		_, err := svc.CreateRelease(ctx, req)
		return &req, err
	}
	get := func(product, version string) *struct{ seal, issuer, keyID, issuerSig, sig, sum string } {
		r, err := svc.GetRelease(ctx, product, version)
		if err != nil || r == nil {
			t.Fatalf("get %s %s: %v", product, version, err)
		}
		return &struct{ seal, issuer, keyID, issuerSig, sig, sum string }{r.SealStatus, r.Issuer, r.IssuerKeyID, r.IssuerSignature, r.Signature, r.Checksum}
	}

	t.Run("unsealed upload is server-signed", func(t *testing.T) {
		body := []byte("siemcore 2.0.1")
		if _, err := upload("siemcore", "2.0.1", body, "", ""); err != nil {
			t.Fatal(err)
		}
		r := get("siemcore", "2.0.1")
		if r.seal != SealUnsealed || r.issuer != IssuerSiemCore || r.keyID != "" || r.issuerSig != "" {
			t.Fatalf("unexpected seal fields: %+v", r)
		}
		if signing.Verify(pub, "siemcore", "2.0.1", sha(body), r.sig) != nil {
			t.Fatal("unsealed release must still carry a valid server signature")
		}
	})

	t.Run("sealed upload keeps the issuer seal as the release signature", func(t *testing.T) {
		body := []byte("siemcore 2.0.2")
		seal := signing.Sign(key, "siemcore", "2.0.2", sha(body))
		if _, err := upload("siemcore", "2.0.2", body, seal, serverKeyID); err != nil {
			t.Fatal(err)
		}
		r := get("siemcore", "2.0.2")
		if r.seal != SealSealed || r.keyID != serverKeyID || r.issuerSig != seal || r.sig != seal {
			t.Fatalf("unexpected seal fields: %+v", r)
		}
	})

	t.Run("invalid seal is accepted, flagged and server-signed", func(t *testing.T) {
		body := []byte("swf 3.1.0")
		wrong := signing.Sign(key, "swf", "9.9.9", sha(body))
		if _, err := upload("swf", "3.1.0", body, wrong, serverKeyID); err != nil {
			t.Fatalf("invalid seal must never reject: %v", err)
		}
		r := get("swf", "3.1.0")
		if r.seal != SealInvalid || r.issuerSig != wrong || r.sig == wrong {
			t.Fatalf("unexpected seal fields: %+v", r)
		}
		if signing.Verify(pub, "swf", "3.1.0", sha(body), r.sig) != nil {
			t.Fatal("invalid-seal release must carry a valid server signature")
		}
	})

	t.Run("unknown key is accepted, flagged and server-signed", func(t *testing.T) {
		_, stranger, _ := ed25519.GenerateKey(rand.Reader)
		body := []byte("mysoc 5.0.0")
		seal := signing.Sign(stranger, "mysoc", "5.0.0", sha(body))
		if _, err := upload("mysoc", "5.0.0", body, seal, "deadbeefdeadbeef"); err != nil {
			t.Fatalf("unknown key must never reject: %v", err)
		}
		r := get("mysoc", "5.0.0")
		if r.seal != SealUnknownKey || r.keyID != "deadbeefdeadbeef" || signing.Verify(pub, "mysoc", "5.0.0", sha(body), r.sig) != nil {
			t.Fatalf("unexpected seal fields: %+v", r)
		}
	})

	t.Run("re-upload of an existing product+version is blocked", func(t *testing.T) {
		before := get("siemcore", "2.0.1")
		_, err := upload("siemcore", "2.0.1", []byte("tampered"), "", "")
		if !errors.Is(err, ErrReleaseExists) {
			t.Fatalf("want ErrReleaseExists, got %v", err)
		}
		seal := signing.Sign(key, "siemcore", "2.0.1", sha([]byte("tampered")))
		if _, err := upload("siemcore", "2.0.1", []byte("tampered"), seal, serverKeyID); !errors.Is(err, ErrReleaseExists) {
			t.Fatalf("a sealed re-upload must be blocked too, got %v", err)
		}
		after := get("siemcore", "2.0.1")
		if *before != *after {
			t.Fatal("release record changed")
		}
		assertStored(t, store, "siemcore", "2.0.1", "siemcore.tar.gz", before.sum)
	})

	t.Run("concurrent uploads of one version publish exactly one artifact", func(t *testing.T) {
		const n = 8
		var wg sync.WaitGroup
		var mu sync.Mutex
		wins, conflicts := 0, 0
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				body := bytes.Repeat([]byte{byte('a' + i)}, 1<<18)
				_, err := upload("swf", "4.0.0", body, "", "")
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					wins++
				case errors.Is(err, ErrReleaseExists):
					conflicts++
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}(i)
		}
		wg.Wait()
		if wins != 1 || conflicts != n-1 {
			t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
		}
		assertStored(t, store, "swf", "4.0.0", "swf.tar.gz", get("swf", "4.0.0").sum)
		entries, _ := os.ReadDir(strings.TrimSuffix(store.GetPath("swf", "4.0.0", "x"), "x"))
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				t.Fatalf("staging file left behind: %s", e.Name())
			}
		}
	})

	t.Run("trusted key registry: rotation, limits and retirement", func(t *testing.T) {
		nextPub, nextPriv, _ := ed25519.GenerateKey(rand.Reader)
		next, err := svc.AddTrustedKey(ctx, hex.EncodeToString(nextPub), "", "next", "admin@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if next.KeyID != KeyID(nextPub) || next.Status != "active" || next.Issuer != "" {
			t.Fatalf("unexpected key: %+v", next)
		}
		thirdPub, _, _ := ed25519.GenerateKey(rand.Reader)
		if _, err := svc.AddTrustedKey(ctx, hex.EncodeToString(thirdPub), "", "", ""); !errors.Is(err, ErrTooManyActiveKeys) {
			t.Fatalf("third active key in one scope must be refused, got %v", err)
		}
		if _, err := svc.AddTrustedKey(ctx, hex.EncodeToString(nextPub), IssuerSWF, "", ""); !errors.Is(err, ErrTrustedKeyExists) {
			t.Fatalf("duplicate key must be refused, got %v", err)
		}
		if _, err := svc.AddTrustedKey(ctx, "zz", "", "", ""); !errors.Is(err, ErrInvalidPublicKey) {
			t.Fatalf("malformed key must be refused, got %v", err)
		}
		if _, err := svc.AddTrustedKey(ctx, hex.EncodeToString(thirdPub), "bogus", "", ""); !errors.Is(err, ErrInvalidIssuer) {
			t.Fatalf("unknown issuer must be refused, got %v", err)
		}
		if _, err := svc.AddTrustedKey(ctx, hex.EncodeToString(thirdPub), IssuerSWF, "swf only", ""); err != nil {
			t.Fatalf("a separate issuer scope has its own limit: %v", err)
		}

		body := []byte("mysoc 5.1.0")
		seal := signing.Sign(nextPriv, "mysoc", "5.1.0", sha(body))
		if _, err := upload("mysoc", "5.1.0", body, seal, ""); err != nil {
			t.Fatal(err)
		}
		if r := get("mysoc", "5.1.0"); r.seal != SealSealed || r.keyID != next.KeyID || r.sig != seal {
			t.Fatalf("seal with the next key during rotation: %+v", r)
		}

		if _, err := svc.RetireTrustedKey(ctx, next.ID); err != nil {
			t.Fatal(err)
		}
		body = []byte("mysoc 5.2.0")
		seal = signing.Sign(nextPriv, "mysoc", "5.2.0", sha(body))
		if _, err := upload("mysoc", "5.2.0", body, seal, next.KeyID); err != nil {
			t.Fatal(err)
		}
		if r := get("mysoc", "5.2.0"); r.seal != SealUnknownKey || signing.Verify(pub, "mysoc", "5.2.0", sha(body), r.sig) != nil {
			t.Fatalf("seal with a retired key: %+v", r)
		}
		if _, err := svc.RetireTrustedKey(ctx, uuid.NewString()); !errors.Is(err, ErrTrustedKeyNotFound) {
			t.Fatalf("want not found, got %v", err)
		}
		if _, err := svc.RetireTrustedKey(ctx, "not-a-uuid"); !errors.Is(err, ErrTrustedKeyNotFound) {
			t.Fatalf("want not found, got %v", err)
		}

		keys, err := svc.ListTrustedKeys(ctx)
		if err != nil || len(keys) != 3 {
			t.Fatalf("list: %d keys, err=%v", len(keys), err)
		}
		for _, k := range keys {
			if k.KeyID == serverKeyID {
				if _, err := svc.RetireTrustedKey(ctx, k.ID); err != nil {
					t.Fatal(err)
				}
			}
		}
		if added, err := svc.EnsureSigningKeyTrusted(ctx); err != nil || added {
			t.Fatalf("a retired server key must stay retired on restart: added=%v err=%v", added, err)
		}
	})

	t.Run("a 1.16.1 insert still works on the 1.16.2 schema", func(t *testing.T) {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO releases (id, product_name, version, channel, manifest, artifact_path, artifact_size, checksum, signature, release_notes, min_updater_version, target_groups, released_at, created_at)
			VALUES ($1, 'swf', '0.0.1', 'stable', '{}', '', 0, '', '', '', '', '{stable}', NOW(), NOW())`, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		if r := get("swf", "0.0.1"); r.seal != SealUnsealed || r.issuer != IssuerSWF {
			t.Fatalf("legacy insert must read as unsealed with a derived issuer, got %q %q", r.seal, r.issuer)
		}
	})
}

func assertStored(t *testing.T, store storage.Storage, product, version, name, wantSum string) {
	t.Helper()
	rc, err := store.Get(product, version, name)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if got := sha(b); got != wantSum {
		t.Fatal(fmt.Sprintf("stored artifact %s does not match the release checksum", name))
	}
}
