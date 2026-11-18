package releases

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupReleaseSchema builds a private schema with the releases table at head.
func setupReleaseSchema(t *testing.T) (*Service, *pgxpool.Pool) {
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
	t.Cleanup(admin.Close)
	schema := "safety_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	cfg, _ := pgxpool.ParseConfig(url)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `CREATE TABLE releases(id uuid PRIMARY KEY, product_name text, version text, channel text, manifest jsonb, artifact_path text, artifact_size bigint, checksum text, signature text, release_notes text, min_updater_version text, target_groups text[], released_at timestamptz, created_at timestamptz, UNIQUE(product_name,version))`)
	for _, file := range []string{"016_independent_artifacts.up.sql", "018_release_channel_length.up.sql", "019_issuer_sealing.up.sql"} {
		if err != nil {
			break
		}
		migration, readErr := os.ReadFile("../../../migrations/" + file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		_, err = pool.Exec(ctx, string(migration))
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(&database.DB{Pool: pool}, store)
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	svc.SetSigningKey(key)
	return svc, pool
}

// A release uploaded with no target groups starts on alpha only, never on
// production by omission.
func TestCreateReleaseDefaultsToAlphaOnly(t *testing.T) {
	svc, _ := setupReleaseSchema(t)
	ctx := context.Background()
	rel, err := svc.CreateRelease(ctx, CreateReleaseRequest{ProductName: "mysoc", Version: "1.0.0.1", Channel: "stable",
		Filename: "mysoc.tar.gz", File: strings.NewReader("payload")})
	if err != nil {
		t.Fatal(err)
	}
	if len(rel.TargetGroups) != 1 || rel.TargetGroups[0] != "alpha" {
		t.Fatalf("default target groups = %v, want [alpha]", rel.TargetGroups)
	}
}

// A bootstrap/update qualification pair may coexist with a normal release of
// the same version; existsExact must not reject it against the legacy row.
func TestDualPairCoexistsWithLegacyVersion(t *testing.T) {
	svc, _ := setupReleaseSchema(t)
	ctx := context.Background()
	if _, err := svc.CreateRelease(ctx, CreateReleaseRequest{ProductName: "mysoc", Version: "2.0.0.1", Channel: "stable",
		TargetGroups: []string{"alpha"}, Filename: "mysoc.tar.gz", File: strings.NewReader("legacy")}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"bootstrap", "update"} {
		payload := "mysoc-" + kind
		a := types.Artifact{Product: "mysoc", Version: "2.0.0.1", Kind: kind, Name: kind + ".bin", Arch: "linux/amd64",
			SourceCommit: strings.Repeat("a", 40), Size: int64(len(payload)), Checksum: sha256Hex(payload)}
		if _, err := svc.CreateDualRelease(ctx, CreateReleaseRequest{ProductName: "mysoc", Version: "2.0.0.1",
			Channel: "dual-alpha-mysoc", TargetGroups: []string{"alpha"}, ArtifactKind: kind},
			[]VariantUpload{{Artifact: a, File: strings.NewReader(payload)}}); err != nil {
			t.Fatalf("%s pair rejected against legacy version: %v", kind, err)
		}
	}
	// The normal offer path still returns the legacy release, not a qualification row.
	legacy, err := svc.GetRelease(ctx, "mysoc", "2.0.0.1")
	if err != nil || legacy == nil || legacy.Manifest.ArtifactKind != "" {
		t.Fatalf("legacy release lost: %+v (err %v)", legacy, err)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
