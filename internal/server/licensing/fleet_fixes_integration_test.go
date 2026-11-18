package licensing

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupInstancesSchema spins up a private schema with a minimal instances table
// matching the columns the fleet queries read. Skips when no test database is
// configured.
func setupInstancesSchema(t *testing.T) (*InstanceRepository, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("UPDATES_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("UPDATES_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "fleet_" + uuid.New().String()[:8]
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, `CREATE TABLE instances (
		deleted_at timestamptz,
		id uuid PRIMARY KEY, instance_id text UNIQUE, instance_type text, hostname text, display_name text,
		license_id text, api_key_hash text, last_heartbeat timestamptz, last_heartbeat_data jsonb,
		status text, last_ip_address text, last_ip_seen_at timestamptz, product_tier text,
		parent_instance_id text, customer_id text, customer_name text, reported_via text, reported_at timestamptz,
		last_update_from_version text, last_update_target_version text, last_update_success boolean,
		last_update_error text, last_update_at timestamptz,
		auto_update_enabled boolean NOT NULL DEFAULT true, update_group text DEFAULT 'stable',
		created_at timestamptz DEFAULT NOW(), updated_at timestamptz DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	return NewInstanceRepository(&database.DB{Pool: pool}), pool
}

// ins inserts one online, directly-heartbeating node with recent contact.
func ins(t *testing.T, pool *pgxpool.Pool, instanceID string, cols map[string]any) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO instances (id, instance_id, instance_type, hostname, api_key_hash, status, last_heartbeat)
		VALUES ($1, $2, 'siemcore', $2, '', 'online', NOW())`, uuid.New().String(), instanceID); err != nil {
		t.Fatal(err)
	}
	for col, val := range cols {
		if _, err := pool.Exec(ctx, `UPDATE instances SET `+col+` = $1 WHERE instance_id = $2`, val, instanceID); err != nil {
			t.Fatalf("set %s: %v", col, err)
		}
	}
}

func TestGetByID_NonUUIDAndDeletedAre404(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	ins(t, pool, "node-1", nil)
	var id string
	if err := pool.QueryRow(ctx, "SELECT id FROM instances WHERE instance_id='node-1'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, id); err != nil {
		t.Fatalf("live node should load: %v", err)
	}
	if _, err := repo.GetByID(ctx, "not-a-uuid"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("non-UUID should be not found, got %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE instances SET deleted_at=NOW() WHERE instance_id='node-1'"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, id); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("deleted node should be not found, got %v", err)
	}
}

func TestCustomerDirectoryAndSecurityEmpty_ReturnSlices(t *testing.T) {
	repo, _ := setupInstancesSchema(t)
	ctx := context.Background()
	rows, total, err := repo.CustomerDirectory(ctx, "", "exceptions", 20, 0)
	if err != nil || total != 0 {
		t.Fatalf("empty directory: %v total=%d", err, total)
	}
	if rows == nil {
		t.Fatal("customer directory must serialise as [], not null")
	}
	srows, stotal, err := repo.ListSecurityPaged(ctx, 20, 0)
	if err != nil || stotal != 0 {
		t.Fatalf("empty security: %v total=%d", err, stotal)
	}
	if srows == nil {
		t.Fatal("security list must serialise as [], not null")
	}
}

func TestSecurityReportingPredicate(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	// Reporting: non-empty firewall status.
	ins(t, pool, "sec-fw", map[string]any{"last_heartbeat_data": `{"security":{"firewall_status":"active","security_score":0}}`})
	// Reporting: non-zero score.
	ins(t, pool, "sec-score", map[string]any{"last_heartbeat_data": `{"security":{"firewall_status":"","security_score":70}}`})
	// Not reporting: security key present but empty/zero.
	ins(t, pool, "sec-empty", map[string]any{"last_heartbeat_data": `{"security":{"firewall_status":"","security_score":0}}`})
	// Not reporting: no security block at all.
	ins(t, pool, "sec-none", map[string]any{"last_heartbeat_data": `{}`})

	stats, err := repo.SecurityStatsSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reporting != 2 {
		t.Fatalf("reporting should be 2, got %d", stats.Reporting)
	}
	rows, total, err := repo.ListSecurityPaged(ctx, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("security list should have 2 reporting nodes, got total=%d len=%d", total, len(rows))
	}
}

func TestTreeRoots_IncludeOrphans(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	// A genuine root (no parent).
	ins(t, pool, "root-1", nil)
	// An orphan: parent id points at a row that does not exist.
	ins(t, pool, "orphan-1", map[string]any{"parent_instance_id": "ghost"})
	// A non-root: parent exists and is live.
	ins(t, pool, "child-1", map[string]any{"parent_instance_id": "root-1"})

	roots, _, err := repo.TreeChildren(ctx, InstanceListFilter{}, true, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range roots {
		got[r.InstanceID] = true
	}
	if !got["root-1"] || !got["orphan-1"] {
		t.Fatalf("roots must include genuine root and orphan, got %v", got)
	}
	if got["child-1"] {
		t.Fatalf("a node with a live parent must not be a root")
	}
}

func TestSubtreeRollup_ExcludesSelfAndSupersededFailures(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	ins(t, pool, "relay", nil)
	// Healthy child.
	ins(t, pool, "leaf-ok", map[string]any{"parent_instance_id": "relay"})
	// Failed but superseded: target reached by a reported product version.
	ins(t, pool, "leaf-superseded", map[string]any{
		"parent_instance_id":         "relay",
		"last_update_success":        false,
		"last_update_target_version": "1.2.0.1",
		"last_heartbeat_data":        `{"products":[{"name":"swf","version":"1.2.0.1"}]}`,
	})
	// Genuinely failed: still behind the target.
	ins(t, pool, "leaf-failed", map[string]any{
		"parent_instance_id":         "relay",
		"last_update_success":        false,
		"last_update_target_version": "2.0.0.1",
		"last_heartbeat_data":        `{"products":[{"name":"swf","version":"1.0.0.0"}]}`,
	})

	roots, _, err := repo.TreeChildren(ctx, InstanceListFilter{Parent: ""}, true, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	var relay *TreeChildRow
	for i := range roots {
		if roots[i].InstanceID == "relay" {
			relay = &roots[i]
		}
	}
	if relay == nil {
		t.Fatal("relay not found among roots")
	}
	// Subtree is descendants only: 3 children, not 4 (self excluded).
	if relay.Subtree.Total != 3 {
		t.Fatalf("subtree total should exclude self (want 3), got %d", relay.Subtree.Total)
	}
	// Only the un-superseded failure counts.
	if relay.Subtree.Failed != 1 {
		t.Fatalf("subtree failed should skip superseded (want 1), got %d", relay.Subtree.Failed)
	}
	if !relay.HasChildren {
		t.Fatal("relay should report has_children")
	}
}

func TestFleetStatsAndFilter_FailedSkipsSuperseded(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	ins(t, pool, "f-superseded", map[string]any{
		"last_update_success":        false,
		"last_update_target_version": "1.2.0.1",
		"last_heartbeat_data":        `{"updater_version":"1.3.0.1"}`,
	})
	ins(t, pool, "f-real", map[string]any{
		"last_update_success":        false,
		"last_update_target_version": "2.0.0.1",
		"last_heartbeat_data":        `{"updater_version":"1.0.0.0"}`,
	})
	stats, err := repo.FleetStatsSummary(ctx, InstanceListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.FailedUpdates != 1 {
		t.Fatalf("failed count should skip superseded (want 1), got %d", stats.FailedUpdates)
	}
	page, err := repo.ListPagedFiltered(ctx, InstanceListFilter{Failed: true}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].InstanceID != "f-real" {
		t.Fatalf("failed filter should return only f-real, got %+v", page)
	}
}

// Malformed heartbeat values must never fail a fleet-wide query: one node with
// a non-numeric version or an odd JSON shape would otherwise blank every view.
func TestFleetQueries_TolerateMalformedHeartbeats(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	ins(t, pool, "bad-versions", map[string]any{
		"last_update_success":        false,
		"last_update_target_version": "1.2.0.1",
		"last_heartbeat_data":        `{"updater_version":"dev","products":null,"self_update":{"capable":"yes"},"relay_listener_ok":"maybe","security":{"firewall_status":"","security_score":"high"}}`,
	})
	ins(t, pool, "bad-target", map[string]any{
		"last_update_success":        false,
		"last_update_target_version": "2.0.0-rc1",
		"last_heartbeat_data":        `{"updater_version":"99999999999.0.0.1","products":{"siemcore":"1.0.0"},"security":{"security_score":82.5}}`,
	})
	ins(t, pool, "scalar-products", map[string]any{
		"last_update_success":        false,
		"last_update_target_version": "1.0.0.1",
		"last_heartbeat_data":        `{"products":["siemcore",{"version":null},{"version":"99999999999.0.0.0"}]}`,
	})

	stats, err := repo.FleetStatsSummary(ctx, InstanceListFilter{})
	if err != nil {
		t.Fatalf("fleet stats must tolerate malformed heartbeats: %v", err)
	}
	// bad-versions and bad-target cannot be compared, so they stay failed;
	// scalar-products is superseded by its long numeric product version.
	if stats.FailedUpdates != 2 {
		t.Fatalf("want 2 failed updates, got %d", stats.FailedUpdates)
	}
	page, err := repo.ListPagedFiltered(ctx, InstanceListFilter{}, 20, 0)
	if err != nil {
		t.Fatalf("list must tolerate malformed heartbeats: %v", err)
	}
	for _, it := range page.Items {
		if it.SelfUpdateCapable != nil || it.RelayListenerOK != nil {
			t.Fatalf("%s: non-boolean flags must read as unreported, got %v/%v", it.InstanceID, it.SelfUpdateCapable, it.RelayListenerOK)
		}
	}
	if _, _, err := repo.ListSecurityPaged(ctx, 20, 0); err != nil {
		t.Fatalf("security list must tolerate malformed scores: %v", err)
	}
	if _, err := repo.SecurityStatsSummary(ctx); err != nil {
		t.Fatalf("security stats must tolerate malformed scores: %v", err)
	}
}

func TestUnassignedCustomerFilter(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	ins(t, pool, "assigned", map[string]any{"customer_id": "cust-1"})
	ins(t, pool, "unassigned-null", nil)
	ins(t, pool, "unassigned-empty", map[string]any{"customer_id": ""})

	page, err := repo.ListPagedFiltered(ctx, InstanceListFilter{Customer: unassignedCustomer}, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("unassigned filter should match 2 nodes, got %d", page.Total)
	}
	for _, it := range page.Items {
		if it.CustomerID != "" {
			t.Fatalf("unassigned filter leaked assigned node %q", it.InstanceID)
		}
	}
}

func TestOperatorFleetSummary(t *testing.T) {
	repo, pool := setupInstancesSchema(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE licenses (id text PRIMARY KEY, operator_ref text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO licenses VALUES ('lic-a', 'op-a'), ('lic-b', 'op-b'), ('lic-legacy', NULL)`); err != nil {
		t.Fatal(err)
	}
	ins(t, pool, "a-mysoc", map[string]any{"license_id": "lic-a", "product_tier": "mysoc"})
	ins(t, pool, "a-siem-1", map[string]any{"license_id": "lic-a", "product_tier": "siemcore"})
	ins(t, pool, "a-siem-2", map[string]any{"license_id": "lic-a", "product_tier": "siemcore"})
	ins(t, pool, "a-notier", map[string]any{"license_id": "lic-a"})
	ins(t, pool, "a-deleted", map[string]any{"license_id": "lic-a", "product_tier": "swf", "deleted_at": "2026-01-01T00:00:00Z"})
	ins(t, pool, "b-swf", map[string]any{"license_id": "lic-b", "product_tier": "swf"})
	ins(t, pool, "legacy", map[string]any{"license_id": "lic-legacy", "product_tier": "mysoc"})
	ins(t, pool, "unlicensed", nil)

	got, err := repo.OperatorFleetSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 operators (legacy and unlicensed excluded), got %d: %+v", len(got), got)
	}
	a := got["op-a"]
	if a == nil || a.Total != 4 || a.ByTier["mysoc"] != 1 || a.ByTier["siemcore"] != 2 || a.ByTier["unknown"] != 1 || a.ByTier["swf"] != 0 {
		t.Fatalf("op-a rollup wrong (deleted must be excluded, untiered = unknown): %+v", a)
	}
	if a.LastHeartbeat == nil {
		t.Fatal("op-a should carry the latest heartbeat")
	}
	if b := got["op-b"]; b == nil || b.Total != 1 || b.ByTier["swf"] != 1 {
		t.Fatalf("op-b rollup wrong: %+v", b)
	}
}
