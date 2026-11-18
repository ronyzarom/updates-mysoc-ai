package releases

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPruneReleasesDatabase(t *testing.T) {
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
	schema := "retention_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, _ := pgxpool.ParseConfig(url)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
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
	if err == nil {
		_, err = pool.Exec(ctx, `CREATE TABLE instances (
		 deleted_at timestamptz,
		 id uuid PRIMARY KEY, instance_id text UNIQUE, instance_type text, hostname text, display_name text,
		 license_id text, api_key_hash text, last_heartbeat timestamptz, last_heartbeat_data jsonb,
		 status text, last_ip_address text, last_ip_seen_at timestamptz, product_tier text,
		 parent_instance_id text, customer_id text, customer_name text, reported_via text, reported_at timestamptz,
		 last_update_from_version text, last_update_target_version text, last_update_success boolean,
		 last_update_error text, last_update_at timestamptz,
		 auto_update_enabled boolean NOT NULL DEFAULT true, update_group text DEFAULT 'stable',
		 created_at timestamptz, updated_at timestamptz)`)
	}
	if err != nil {
		t.Fatal(err)
	}
	db := &database.DB{Pool: pool}
	dir := t.TempDir()
	store, err := storage.NewLocalStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, store)
	instances := licensing.NewInstanceRepository(db)
	loadInstalled := func(ctx context.Context) (*Installed, error) {
		devices, versions, err := instances.InstalledVersions(ctx)
		if err != nil {
			return nil, err
		}
		installed := NewInstalled(devices)
		for _, v := range versions {
			installed.AddReported(v.Kind, v.Product, v.Version)
		}
		return installed, nil
	}

	publish := func(version string, groups ...string) {
		t.Helper()
		_, err := svc.CreateRelease(ctx, CreateReleaseRequest{ProductName: "mysoc", Version: version, Channel: "stable",
			TargetGroups: groups, Filename: "mysoc.tar.gz", File: strings.NewReader("payload " + version)})
		if err != nil {
			t.Fatal(version, err)
		}
	}
	all := []string{"alpha", "beta", "stable", "production"}
	for i := 1; i <= 10; i++ {
		publish("1.0.0."+strconv.Itoa(i), all...)
	}
	publish("0.9.0.1", "alpha")
	publish("1.0.0.11", "alpha")
	if _, err := pool.Exec(ctx, `UPDATE releases SET created_at = NOW() - INTERVAL '30 days', released_at = NOW() - INTERVAL '30 days'`); err != nil {
		t.Fatal(err)
	}
	policy := RetentionPolicy{ProductionKeep: 5, MinAge: 7 * 24 * time.Hour, MaxPerRun: 100, QualificationPrefix: "dual-alpha-"}

	// With no devices known the pass must refuse and delete nothing.
	if _, err := svc.PruneReleases(ctx, policy, loadInstalled, time.Now()); err == nil {
		t.Fatal("prune with an empty fleet must refuse")
	}
	if n := countReleases(t, pool); n != 12 {
		t.Fatalf("empty-fleet refusal deleted releases: %d left", n)
	}

	// A direct device on 1.0.0.2, a relay-reported child on 1.0.0.3, a deleted
	// device on 1.0.0.4 (no protection), and a last update target of 1.0.0.5.
	if _, err := pool.Exec(ctx, `INSERT INTO instances (id, instance_id, status, last_heartbeat_data, last_update_target_version, created_at, updated_at)
		VALUES ($1, 'direct', 'online', '{"updater_version":"1.18.2.1","products":[{"name":"mysoc","version":"1.0.0.2"}]}', '1.0.0.5', NOW(), NOW()),
		       ($2, 'gone', 'decommissioned', '{"products":[{"name":"mysoc","version":"1.0.0.4"}]}', NULL, NOW(), NOW())`,
		uuid.NewString(), uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE instances SET deleted_at = NOW() WHERE instance_id = 'gone'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := instances.UpsertReportedChildren(ctx, "direct", "", []types.ChildReport{{
		InstanceID: "child", ProductTier: "mysoc", Status: "online", LastSeen: time.Now(),
		Products: []types.ProductStatus{{Name: "mysoc", Version: "1.0.0.3"}},
	}}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.PruneReleases(ctx, policy, loadInstalled, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var deleted []string
	for _, c := range res.Deleted {
		deleted = append(deleted, c.Release.Version+":"+c.Reason)
	}
	sort.Strings(deleted)
	want := "0.9.0.1:never-reached-production,1.0.0.1:old-production,1.0.0.4:old-production"
	if got := strings.Join(deleted, ","); got != want || res.Failed != 0 {
		t.Fatalf("deleted %s (failed %d), want %s", got, res.Failed, want)
	}
	if n := countReleases(t, pool); n != 9 {
		t.Fatalf("want 9 releases left, got %d", n)
	}
	for _, v := range []string{"0.9.0.1", "1.0.0.1", "1.0.0.4"} {
		if _, err := os.Stat(filepath.Join(dir, "mysoc", v)); !os.IsNotExist(err) {
			t.Fatalf("files of deleted %s remain: %v", v, err)
		}
	}
	for _, v := range []string{"1.0.0.2", "1.0.0.3", "1.0.0.5", "1.0.0.6", "1.0.0.10", "1.0.0.11"} {
		if _, err := os.Stat(filepath.Join(dir, "mysoc", v, "mysoc.tar.gz")); err != nil {
			t.Fatalf("kept %s lost its file: %v", v, err)
		}
	}

	// A second pass finds nothing more to delete.
	res, err = svc.PruneReleases(ctx, policy, loadInstalled, time.Now())
	if err != nil || len(res.Deleted) != 0 {
		t.Fatalf("second pass: deleted %d, err %v", len(res.Deleted), err)
	}
}

func countReleases(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM releases`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
