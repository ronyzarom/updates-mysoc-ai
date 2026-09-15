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
func TestEnrollmentDefaultsPreserveExplicitHolds(t *testing.T) {
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
	schema := "enroll_" + uuid.New().String()[:8]
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
	 id text PRIMARY KEY, instance_id text UNIQUE, instance_type text, hostname text, display_name text,
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
	for _, path := range []string{"check-child", "check-orphan", "check-root", "rollup"} {
		t.Run(path, func(t *testing.T) {
			beat := &types.Heartbeat{ProductTier: "siemcore", ParentInstanceID: "relay"}
			if path == "check-orphan" {
				beat.ParentInstanceID = ""
			}
			if path == "check-root" {
				beat.ProductTier = "mysoc"
				beat.ParentInstanceID = ""
			}
			contact := func() error {
				if path == "rollup" {
					_, _, err := repo.UpsertReportedChildren(ctx, "relay", "", []types.ChildReport{{
						InstanceID: path, ProductTier: "siemcore", Status: "online", LastSeen: time.Now(),
					}})
					return err
				}
				return repo.TouchFromCheck(ctx, path, beat, "", "127.0.0.1")
			}
			if err := contact(); err != nil {
				t.Fatal(err)
			}
			var enabled bool
			if err := pool.QueryRow(ctx, "SELECT auto_update_enabled FROM instances WHERE instance_id=$1", path).Scan(&enabled); err != nil {
				t.Fatal(err)
			}
			if !enabled {
				t.Fatal("new enrollment must default enabled")
			}
			if _, err := pool.Exec(ctx, "UPDATE instances SET auto_update_enabled=false, update_group='alpha' WHERE instance_id=$1", path); err != nil {
				t.Fatal(err)
			}
			if err := contact(); err != nil {
				t.Fatal(err)
			}
			var group string
			if err := pool.QueryRow(ctx, "SELECT auto_update_enabled,update_group FROM instances WHERE instance_id=$1", path).Scan(&enabled, &group); err != nil {
				t.Fatal(err)
			}
			if enabled || group != "alpha" {
				t.Fatal("repeat enrollment changed explicit hold or group")
			}
		})
	}
	t.Run("dual status persists into compact fleet list", func(t *testing.T) {
		attempt := &types.UpdateAttempt{FromVersion: "1.0.0", TargetVersion: "1.0.1", Success: true, Timestamp: time.Now(), SelectedArtifactKind: "update", DependencyValidation: "complete", ArtifactDigest: "fixture-digest"}
		beat := &types.Heartbeat{InstanceID: "check-child", LastUpdateAttempt: attempt}
		if err := repo.UpdateHeartbeat(ctx, "check-child", beat, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
		row := pool.QueryRow(ctx, "SELECT "+selectInstanceListCols+" FROM instances WHERE instance_id='check-child'")
		instance, err := repo.scanInstanceList(row)
		if err != nil {
			t.Fatal(err)
		}
		if instance.LastHeartbeatData != nil || instance.LastArtifactDelivery == nil || instance.LastArtifactDelivery.ArtifactDigest != "fixture-digest" || instance.LastArtifactDelivery.SelectedArtifactKind != "update" {
			t.Fatal("compact delivery status missing or full heartbeat leaked")
		}
		if _, err := pool.Exec(ctx, "UPDATE instances SET license_id='fixture-license' WHERE instance_id='check-child'"); err != nil {
			t.Fatal(err)
		}
		attempt.Timestamp = time.Now().Add(time.Second)
		if ok, err := repo.RecordArtifactReport(ctx, "check-child", "wrong-license", *attempt); err != nil || ok {
			t.Fatal("cross-license report accepted")
		}
		if ok, err := repo.RecordArtifactReport(ctx, "check-child", "fixture-license", *attempt); err != nil || !ok {
			t.Fatal("immediate report not persisted", err)
		}

	})

}
