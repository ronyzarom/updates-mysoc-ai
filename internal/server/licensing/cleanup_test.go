package licensing

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run against a disposable local PostgreSQL database. A private schema keeps
// the test independent of any existing instances or migrations.
func TestCleanupInactiveChildren(t *testing.T) {
	url := os.Getenv("UPDATES_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("UPDATES_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "cleanup_" + uuid.New().String()[:8]
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE instances (
	 id uuid PRIMARY KEY, instance_id text UNIQUE, instance_type text, hostname text, display_name text,
	 license_id text, api_key_hash text, last_heartbeat timestamptz, last_heartbeat_data jsonb,
	 status text, last_ip_address text, last_ip_seen_at timestamptz, product_tier text,
	 parent_instance_id text, customer_id text, customer_name text, reported_via text, reported_at timestamptz,
	 last_update_from_version text, last_update_target_version text, last_update_success boolean,
	 last_update_error text, last_update_at timestamptz,
	 auto_update_enabled boolean NOT NULL DEFAULT true, update_group text DEFAULT 'stable',
	 created_at timestamptz, updated_at timestamptz)`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewInstanceRepository(&database.DB{Pool: pool})
	old := time.Now().Add(-2 * time.Hour)
	for _, name := range []string{"old", "active", "held", "parent", "other", "raced"} {
		parent := "relay"
		if name == "other" {
			parent = "another-relay"
		}
		_, _, err := repo.UpsertReportedChildren(ctx, parent, "", []types.ChildReport{{InstanceID: name, ProductTier: "siemcore", Status: "offline", LastSeen: old}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE instances SET status='online',last_heartbeat=NOW() WHERE instance_id='active'; UPDATE instances SET auto_update_enabled=false WHERE instance_id='held'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.UpsertReportedChildren(ctx, "parent", "", []types.ChildReport{{InstanceID: "descendant", Status: "online", LastSeen: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	items, err := repo.InactiveChildren(ctx, "relay")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("wanted old and raced, got %+v", items)
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	// A fresh contact between preview and confirmation must prevent removal.
	if _, err := pool.Exec(ctx, `UPDATE instances SET status='online',last_heartbeat=NOW() WHERE instance_id='raced'`); err != nil {
		t.Fatal(err)
	}
	// Even a submitted ID from another parent must be excluded.
	var otherID string
	if err := pool.QueryRow(ctx, "SELECT id FROM instances WHERE instance_id='other'").Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	ids = append(ids, otherID)
	n, err := repo.CleanupChildren(ctx, "relay", ids)
	if err != nil || n != 1 {
		t.Fatal("unsafe cleanup count", n, err)
	}
	report := func(seen time.Time) {
		_, _, err := repo.UpsertReportedChildren(ctx, "relay", "", []types.ChildReport{{InstanceID: "old", Status: "online", LastSeen: seen}})
		if err != nil {
			t.Fatal(err)
		}
	}
	status := func() string {
		var value string
		if err := pool.QueryRow(ctx, "SELECT status FROM instances WHERE instance_id='old'").Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	// The individual Delete action must preserve the same retirement marker.
	var retiredID string
	var retiredAt time.Time
	if err := pool.QueryRow(ctx, "SELECT id, updated_at FROM instances WHERE instance_id='old'").Scan(&retiredID, &retiredAt); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.Delete(ctx, retiredID); err != nil {
			t.Fatal(err)
		}
	}
	var retainedID string
	var retainedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT id, updated_at FROM instances WHERE instance_id='old'").Scan(&retainedID, &retainedAt); err != nil {
		t.Fatal("Delete removed retirement marker", err)
	}
	if retainedID != retiredID || !retainedAt.Equal(retiredAt) {
		t.Fatal("repeated Delete changed retirement identity or timestamp")
	}
	report(old)
	if status() != "decommissioned" {
		t.Fatal("stale relay report resurrected deleted entry")
	}
	report(time.Time{})
	if status() != "decommissioned" {
		t.Fatal("unstamped relay report resurrected deleted entry")
	}
	var removedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT updated_at FROM instances WHERE instance_id='old'").Scan(&removedAt); err != nil {
		t.Fatal(err)
	}
	// Individual deletion of an active record must retire it too.
	if err := repo.Delete(ctx, otherID); err != nil {
		t.Fatal(err)
	}
	var otherStatus string
	if err := pool.QueryRow(ctx, "SELECT status FROM instances WHERE id=$1", otherID).Scan(&otherStatus); err != nil || otherStatus != "decommissioned" {
		t.Fatal("individual retirement failed", otherStatus, err)
	}
	report(removedAt.Add(time.Second))
	if status() != "online" {
		t.Fatal("genuine fresh heartbeat did not revive host")
	}
}
