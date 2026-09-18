package licensing

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInstallationIdentityRetainedAcrossHeartbeats(t *testing.T) {
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
	schema := "installation_" + uuid.New().String()[:8]
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
 deleted_at timestamptz,
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

	for _, route := range []string{"upsert", "update", "rollup"} {
		for _, withAttempt := range []bool{false, true} {
			for _, kind := range []string{"normal", "pod", "pod-node", "observer-unlinked"} {
				name := route + "/" + kind
				if withAttempt {
					name += "/attempt"
				}
				t.Run(name, func(t *testing.T) {
					id := uuid.NewString()
					fixed := &types.InstallationIdentity{Kind: kind}
					if kind == "pod" {
						fixed.PodID = "pod"
						fixed.NodeID = "1"
					}
					if kind == "pod-node" {
						fixed.NodeID = "1"
					}
					send := func(identity *types.InstallationIdentity) error {
						hb := &types.Heartbeat{InstanceID: id, ProductTier: "siemcore", Installation: identity, UpdaterVersion: "new"}
						if withAttempt {
							hb.LastUpdateAttempt = &types.UpdateAttempt{FromVersion: "1", TargetVersion: "2", Success: true, Timestamp: time.Now()}
						}
						switch route {
						case "update":
							return repo.UpdateHeartbeat(ctx, id, hb, "127.0.0.1")
						case "rollup":
							_, _, err := repo.UpsertReportedChildren(ctx, "relay", "", []types.ChildReport{{InstanceID: id, ProductTier: "siemcore", Installation: identity, Status: "online", LastSeen: time.Now(), UpdaterVersion: "new", LastUpdateAttempt: hb.LastUpdateAttempt}})
							return err
						default:
							return repo.UpsertFromHeartbeat(ctx, id, hb, "", "127.0.0.1")
						}
					}
					if route == "update" {
						if err := repo.UpsertFromHeartbeat(ctx, id, &types.Heartbeat{InstanceID: id, Installation: fixed}, "", ""); err != nil {
							t.Fatal(err)
						}
					}
					if err := send(fixed); err != nil {
						t.Fatal(err)
					}
					changed := &types.InstallationIdentity{Kind: "pod", PodID: "other", NodeID: "2"}
					if kind == "pod" {
						changed = &types.InstallationIdentity{Kind: "normal"}
					}
					for _, next := range []*types.InstallationIdentity{nil, changed, nil} {
						if err := send(next); err != nil {
							t.Fatal(err)
						}
						var raw []byte
						if err := pool.QueryRow(ctx, "SELECT last_heartbeat_data FROM instances WHERE instance_id=$1", id).Scan(&raw); err != nil {
							t.Fatal(err)
						}
						var got types.Heartbeat
						if err := json.Unmarshal(raw, &got); err != nil {
							t.Fatal(err)
						}
						if got.Installation == nil || *got.Installation != *fixed || got.UpdaterVersion != "new" {
							t.Fatalf("immutable identity lost or heartbeat stalled: %s", raw)
						}
					}
				})
			}
		}
	}
}
