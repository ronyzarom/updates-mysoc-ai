package licensing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/catalog"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// ErrInstanceNotFound is returned when an instance doesn't exist
var ErrInstanceNotFound = errors.New("instance not found")

// ValidUpdateGroups defines the allowed update group values
var ValidUpdateGroups = map[string]bool{
	"alpha":      true,
	"beta":       true,
	"stable":     true,
	"production": true,
}

// ValidateUpdateGroup validates an update group value
func ValidateUpdateGroup(group string) error {
	if !ValidUpdateGroups[group] {
		return fmt.Errorf("invalid update group: %s (must be alpha, beta, stable, or production)", group)
	}
	return nil
}

// SQL column lists - explicitly different to prevent scan mismatches
const (
	// derivedStatusCol computes liveness at read time for directly-heartbeating
	// nodes only: a direct node (reported_via IS NULL) that has not been heard
	// from within the threshold reads as offline, regardless of the stored
	// status. Nothing writes 'offline' back; the stored value flips to
	// 'online' again on the next contact.
	//
	// Rollup-reported rows (reported_via IS NOT NULL) are exempt from the
	// time rule (Fleet Scalability 1.12): their status is authoritative from
	// the reporting relay, which under delta reporting updates reported_at
	// only on change. A leaf going offline arrives as a delta carrying
	// status='offline'; a relay going dark is itself a direct node that flips
	// offline, and the operator reads its whole subtree as stale by that
	// signal rather than by every leaf's reported_at ageing out.
	derivedStatusExpr = `CASE
		WHEN reported_via IS NULL
		     AND status IN ('online', 'degraded')
		     AND last_heartbeat < NOW() - INTERVAL '5 minutes'
		THEN 'offline'
		ELSE status
	END`

	derivedStatusCol = derivedStatusExpr + ` AS status`

	// selectInstanceFullCols includes last_heartbeat_data for detail views
	selectInstanceFullCols = `id, instance_id, instance_type, hostname, display_name, license_id, 
		api_key_hash, last_heartbeat, last_heartbeat_data, ` + derivedStatusCol + `, auto_update_enabled, 
		update_group, last_ip_address, last_ip_seen_at, last_update_from_version, 
		last_update_target_version, last_update_success, last_update_error, last_update_at,
		product_tier, parent_instance_id, customer_id, customer_name, reported_via, reported_at,
		created_at, updated_at`

	// selectInstanceListCols excludes last_heartbeat_data and update details for lighter list queries
	selectInstanceListCols = `id, instance_id, instance_type, hostname, display_name, license_id, 
		api_key_hash, last_heartbeat, ` + derivedStatusCol + `, auto_update_enabled, update_group, 
		last_ip_address, product_tier, parent_instance_id, customer_id, customer_name,
		reported_via, reported_at, created_at, updated_at`
)

// InstanceRepository handles instance database operations
type InstanceRepository struct {
	db *database.DB
}

// rowScanner is an interface that both pgx.Row and pgx.Rows implement
type rowScanner interface {
	Scan(dest ...any) error
}

// scanInstanceFull scans a row with all columns including heartbeat JSON
func (r *InstanceRepository) scanInstanceFull(row rowScanner) (*types.Instance, error) {
	var instance types.Instance
	var lastHeartbeatData []byte
	var licenseID, displayName, updateGroup *string
	var autoUpdateEnabled *bool
	var lastIPAddress, lastUpdateFromVersion, lastUpdateTargetVersion, lastUpdateError *string
	var lastUpdateSuccess *bool
	var productTier, parentInstanceID *string
	var customerID, customerName, reportedVia *string

	err := row.Scan(
		&instance.ID, &instance.InstanceID, &instance.InstanceType, &instance.Hostname, &displayName,
		&licenseID, &instance.APIKeyHash, &instance.LastHeartbeat, &lastHeartbeatData,
		&instance.Status, &autoUpdateEnabled, &updateGroup,
		&lastIPAddress, &instance.LastIPSeenAt, &lastUpdateFromVersion,
		&lastUpdateTargetVersion, &lastUpdateSuccess, &lastUpdateError, &instance.LastUpdateAt,
		&productTier, &parentInstanceID, &customerID, &customerName, &reportedVia, &instance.ReportedAt,
		&instance.CreatedAt, &instance.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if licenseID != nil {
		instance.LicenseID = *licenseID
	}
	if displayName != nil {
		instance.DisplayName = *displayName
	}
	if productTier != nil {
		instance.ProductTier = *productTier
	}
	if parentInstanceID != nil {
		instance.ParentInstanceID = *parentInstanceID
	}
	if customerID != nil {
		instance.CustomerID = *customerID
	}
	if customerName != nil {
		instance.CustomerName = *customerName
	}
	if reportedVia != nil {
		instance.ReportedVia = *reportedVia
	}
	if lastIPAddress != nil {
		instance.LastIPAddress = *lastIPAddress
	}
	if lastUpdateFromVersion != nil {
		instance.LastUpdateFromVersion = *lastUpdateFromVersion
	}
	if lastUpdateTargetVersion != nil {
		instance.LastUpdateTargetVersion = *lastUpdateTargetVersion
	}
	if lastUpdateSuccess != nil {
		instance.LastUpdateSuccess = lastUpdateSuccess
	}
	if lastUpdateError != nil {
		instance.LastUpdateError = *lastUpdateError
	}

	// Apply defaults for nullable columns
	instance.AutoUpdateEnabled = true
	if autoUpdateEnabled != nil {
		instance.AutoUpdateEnabled = *autoUpdateEnabled
	}

	instance.UpdateGroup = "stable"
	if updateGroup != nil && *updateGroup != "" {
		instance.UpdateGroup = *updateGroup
	}

	// Parse heartbeat JSON
	if len(lastHeartbeatData) > 0 {
		var heartbeat types.Heartbeat
		if err := json.Unmarshal(lastHeartbeatData, &heartbeat); err == nil {
			instance.LastHeartbeatData = &heartbeat
		}
	}

	return &instance, nil
}

// scanInstanceList scans a row without heartbeat JSON (lighter for list views)
func (r *InstanceRepository) scanInstanceList(row rowScanner) (*types.Instance, error) {
	var instance types.Instance
	var licenseID, displayName, updateGroup, lastIPAddress *string
	var autoUpdateEnabled *bool
	var productTier, parentInstanceID *string
	var customerID, customerName, reportedVia *string

	err := row.Scan(
		&instance.ID, &instance.InstanceID, &instance.InstanceType, &instance.Hostname, &displayName,
		&licenseID, &instance.APIKeyHash, &instance.LastHeartbeat,
		&instance.Status, &autoUpdateEnabled, &updateGroup,
		&lastIPAddress, &productTier, &parentInstanceID, &customerID, &customerName,
		&reportedVia, &instance.ReportedAt, &instance.CreatedAt, &instance.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if licenseID != nil {
		instance.LicenseID = *licenseID
	}
	if displayName != nil {
		instance.DisplayName = *displayName
	}
	if productTier != nil {
		instance.ProductTier = *productTier
	}
	if parentInstanceID != nil {
		instance.ParentInstanceID = *parentInstanceID
	}
	if customerID != nil {
		instance.CustomerID = *customerID
	}
	if customerName != nil {
		instance.CustomerName = *customerName
	}
	if reportedVia != nil {
		instance.ReportedVia = *reportedVia
	}
	if lastIPAddress != nil {
		instance.LastIPAddress = *lastIPAddress
	}

	// Apply defaults for nullable columns
	instance.AutoUpdateEnabled = true
	if autoUpdateEnabled != nil {
		instance.AutoUpdateEnabled = *autoUpdateEnabled
	}

	instance.UpdateGroup = "stable"
	if updateGroup != nil && *updateGroup != "" {
		instance.UpdateGroup = *updateGroup
	}

	// LastHeartbeatData is nil for list queries (not fetched)
	return &instance, nil
}

// NewInstanceRepository creates a new instance repository
func NewInstanceRepository(db *database.DB) *InstanceRepository {
	return &InstanceRepository{db: db}
}

// Create creates a new instance
func (r *InstanceRepository) Create(ctx context.Context, instance *types.Instance) error {
	instance.ID = uuid.New().String()
	instance.CreatedAt = time.Now()
	instance.UpdatedAt = time.Now()

	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO instances (id, instance_id, instance_type, hostname, license_id, api_key_hash, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, instance.ID, instance.InstanceID, instance.InstanceType, instance.Hostname,
		instance.LicenseID, instance.APIKeyHash, instance.Status, instance.CreatedAt, instance.UpdatedAt)

	return err
}

// GetByID retrieves an instance by ID
func (r *InstanceRepository) GetByID(ctx context.Context, id string) (*types.Instance, error) {
	row := r.db.Pool.QueryRow(ctx,
		`SELECT `+selectInstanceFullCols+` FROM instances WHERE id = $1`, id)

	instance, err := r.scanInstanceFull(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	return instance, nil
}

// GetByInstanceID retrieves an instance by instance_id
func (r *InstanceRepository) GetByInstanceID(ctx context.Context, instanceID string) (*types.Instance, error) {
	row := r.db.Pool.QueryRow(ctx,
		`SELECT `+selectInstanceFullCols+` FROM instances WHERE instance_id = $1`, instanceID)

	instance, err := r.scanInstanceFull(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	return instance, nil
}

// GetByAPIKeyHash retrieves an instance by API key hash
func (r *InstanceRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*types.Instance, error) {
	row := r.db.Pool.QueryRow(ctx,
		`SELECT `+selectInstanceFullCols+` FROM instances WHERE api_key_hash = $1`, apiKeyHash)

	instance, err := r.scanInstanceFull(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	return instance, nil
}

// List retrieves all instances
func (r *InstanceRepository) List(ctx context.Context) ([]types.Instance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, instance_id, instance_type, hostname, display_name, license_id, api_key_hash, last_heartbeat, last_heartbeat_data, `+derivedStatusCol+`, auto_update_enabled, update_group, product_tier, parent_instance_id, customer_id, customer_name, reported_via, reported_at, created_at, updated_at
		FROM instances
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer rows.Close()

	var instances []types.Instance
	for rows.Next() {
		var instance types.Instance
		var lastHeartbeatData []byte
		var licenseID *string
		var displayName *string
		var autoUpdateEnabled *bool
		var updateGroup *string
		var productTier, parentInstanceID *string
		var customerID, customerName, reportedVia *string

		err := rows.Scan(
			&instance.ID, &instance.InstanceID, &instance.InstanceType, &instance.Hostname, &displayName,
			&licenseID, &instance.APIKeyHash, &instance.LastHeartbeat, &lastHeartbeatData,
			&instance.Status, &autoUpdateEnabled, &updateGroup, &productTier, &parentInstanceID,
			&customerID, &customerName, &reportedVia, &instance.ReportedAt,
			&instance.CreatedAt, &instance.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan instance: %w", err)
		}

		if licenseID != nil {
			instance.LicenseID = *licenseID
		}
		if displayName != nil {
			instance.DisplayName = *displayName
		}
		if productTier != nil {
			instance.ProductTier = *productTier
		}
		if parentInstanceID != nil {
			instance.ParentInstanceID = *parentInstanceID
		}
		if customerID != nil {
			instance.CustomerID = *customerID
		}
		if customerName != nil {
			instance.CustomerName = *customerName
		}
		if reportedVia != nil {
			instance.ReportedVia = *reportedVia
		}
		if autoUpdateEnabled != nil {
			instance.AutoUpdateEnabled = *autoUpdateEnabled
		} else {
			instance.AutoUpdateEnabled = true
		}
		if updateGroup != nil {
			instance.UpdateGroup = *updateGroup
		} else {
			instance.UpdateGroup = "stable"
		}

		if lastHeartbeatData != nil {
			var heartbeat types.Heartbeat
			if err := json.Unmarshal(lastHeartbeatData, &heartbeat); err == nil {
				instance.LastHeartbeatData = &heartbeat
			}
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

// ListInstancesResult is the response for paginated list queries
type ListInstancesResult struct {
	Items  []types.Instance `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Total  int              `json:"total"`
}

// ListPaged retrieves instances with pagination (lighter query, no heartbeat JSON)
func (r *InstanceRepository) ListPaged(ctx context.Context, limit, offset int) (*ListInstancesResult, error) {
	// Get total count
	var total int
	err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM instances`).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count instances: %w", err)
	}

	// Get paginated results with lighter query (no heartbeat JSON parsing)
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+selectInstanceListCols+` FROM instances ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer rows.Close()

	var items []types.Instance
	for rows.Next() {
		instance, err := r.scanInstanceList(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan instance: %w", err)
		}
		items = append(items, *instance)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating instances: %w", err)
	}

	return &ListInstancesResult{
		Items:  items,
		Limit:  limit,
		Offset: offset,
		Total:  total,
	}, nil
}

// InstanceListFilter narrows and orders a paged instance query. Empty fields
// are ignored. Sort/SortDir are validated against allowlists (an unknown
// value falls back to the default created_at DESC) so the column can never be
// attacker-controlled SQL.
type InstanceListFilter struct {
	Status   string // derived status: online | offline | degraded | decommissioned
	Tier     string // product_tier exact match
	Customer string // customer_id exact match
	Operator string // operator id (via the instance's license)
	Parent   string // parent_instance_id exact match (direct children of a node)
	Search   string // ILIKE across instance_id / hostname / customer_name
	Sort     string // created_at | updated_at | last_heartbeat | hostname | instance_id | product_tier | status
	SortDir  string // asc | desc
}

// sortColumns maps the public sort keys to SQL-safe expressions.
var sortColumns = map[string]string{
	"created_at":     "created_at",
	"updated_at":     "updated_at",
	"last_heartbeat": "last_heartbeat",
	"hostname":       "hostname",
	"instance_id":    "instance_id",
	"product_tier":   "product_tier",
	"status":         derivedStatusExpr,
}

// buildInstanceFilter renders the WHERE clause and positional args shared by
// the count and page queries. The returned args start at $1.
func buildInstanceFilter(f InstanceListFilter) (string, []interface{}) {
	var conds []string
	var args []interface{}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if s := strings.TrimSpace(f.Tier); s != "" {
		add("product_tier = $%d", s)
	}
	if s := strings.TrimSpace(f.Customer); s != "" {
		add("customer_id = $%d", s)
	}
	if s := strings.TrimSpace(f.Parent); s != "" {
		add("parent_instance_id = $%d", s)
	}
	if s := strings.TrimSpace(f.Operator); s != "" {
		add("EXISTS (SELECT 1 FROM licenses l WHERE l.id = instances.license_id AND l.operator_ref = $%d)", s)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		args = append(args, "%"+s+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			"(instance_id ILIKE $%d OR hostname ILIKE $%d OR customer_name ILIKE $%d)", n, n, n))
	}
	if s := strings.TrimSpace(f.Status); s != "" {
		// Filter on the derived (read-time) status, matching what the UI shows.
		args = append(args, s)
		conds = append(conds, fmt.Sprintf("(%s) = $%d", derivedStatusExpr, len(args)))
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// orderClause returns a validated ORDER BY built from the filter's sort keys.
func orderClause(f InstanceListFilter) string {
	col, ok := sortColumns[strings.ToLower(strings.TrimSpace(f.Sort))]
	if !ok {
		col = "created_at"
	}
	dir := "DESC"
	if strings.EqualFold(strings.TrimSpace(f.SortDir), "asc") {
		dir = "ASC"
	}
	return " ORDER BY " + col + " " + dir
}

// ListPagedFiltered is ListPaged with server-side filtering, text search, and
// sorting — the query behind the paged dashboard views at fleet scale. Only
// one page of rows and one count leave the database, regardless of fleet size.
func (r *InstanceRepository) ListPagedFiltered(ctx context.Context, f InstanceListFilter, limit, offset int) (*ListInstancesResult, error) {
	where, args := buildInstanceFilter(f)

	var total int
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM instances`+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count instances: %w", err)
	}

	pageArgs := append(append([]interface{}{}, args...), limit, offset)
	query := `SELECT ` + selectInstanceListCols + ` FROM instances` + where +
		orderClause(f) +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

	rows, err := r.db.Pool.Query(ctx, query, pageArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer rows.Close()

	var items []types.Instance
	for rows.Next() {
		instance, err := r.scanInstanceList(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan instance: %w", err)
		}
		items = append(items, *instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating instances: %w", err)
	}

	return &ListInstancesResult{Items: items, Limit: limit, Offset: offset, Total: total}, nil
}

// FleetStats is the SQL-aggregated fleet summary powering the dashboard's
// headline cards without shipping any per-node rows to the client.
type FleetStats struct {
	Total          int            `json:"total"`
	Online         int            `json:"online"`
	Offline        int            `json:"offline"`
	Degraded       int            `json:"degraded"`
	Decommissioned int            `json:"decommissioned"`
	FailedUpdates  int            `json:"failed_updates"`
	ByTier         map[string]int `json:"by_tier"`
}

// FleetStatsSummary computes the fleet-wide counts in a single grouped scan,
// respecting the same filters as the paged list.
func (r *InstanceRepository) FleetStatsSummary(ctx context.Context, f InstanceListFilter) (*FleetStats, error) {
	where, args := buildInstanceFilter(f)

	stats := &FleetStats{ByTier: map[string]int{}}
	// One pass: derived-status histogram + failed-update count.
	statusQuery := `SELECT ` + derivedStatusExpr + ` AS s,
		COUNT(*) AS c,
		COUNT(*) FILTER (WHERE last_update_success IS FALSE) AS failed
		FROM instances` + where + ` GROUP BY s`
	rows, err := r.db.Pool.Query(ctx, statusQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("fleet stats status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var c, failed int
		if err := rows.Scan(&s, &c, &failed); err != nil {
			return nil, fmt.Errorf("fleet stats scan: %w", err)
		}
		stats.Total += c
		stats.FailedUpdates += failed
		switch s {
		case "online":
			stats.Online += c
		case "offline":
			stats.Offline += c
		case "degraded":
			stats.Degraded += c
		case "decommissioned":
			stats.Decommissioned += c
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fleet stats rows: %w", err)
	}

	// Per-tier histogram (separate grouping; cheap, indexed).
	tierRows, err := r.db.Pool.Query(ctx,
		`SELECT COALESCE(NULLIF(product_tier, ''), 'unknown') AS t, COUNT(*) FROM instances`+where+` GROUP BY t`, args...)
	if err != nil {
		return nil, fmt.Errorf("fleet stats tier: %w", err)
	}
	defer tierRows.Close()
	for tierRows.Next() {
		var t string
		var c int
		if err := tierRows.Scan(&t, &c); err != nil {
			return nil, fmt.Errorf("fleet stats tier scan: %w", err)
		}
		stats.ByTier[t] = c
	}
	if err := tierRows.Err(); err != nil {
		return nil, fmt.Errorf("fleet stats tier rows: %w", err)
	}
	return stats, nil
}

// CustomerSummaryRow is one customer's aggregated fleet health for the
// exceptions-first customer directory.
type CustomerSummaryRow struct {
	CustomerID   string `json:"customer_id"`
	CustomerName string `json:"customer_name"`
	Total        int    `json:"total"`
	Online       int    `json:"online"`
	Offline      int    `json:"offline"`
	Failed       int    `json:"failed"`
}

// customerSortClauses maps the directory sort keys to SQL-safe ORDER BY.
var customerSortClauses = map[string]string{
	// Problem customers first: most failed updates, then most offline, then
	// biggest — the operator sees what needs attention without scrolling.
	"exceptions": "failed DESC, offline DESC, total DESC, cname ASC",
	"name":       "cname ASC",
	"nodes":      "total DESC, cname ASC",
}

// CustomerDirectory returns per-customer health aggregates, searchable and
// paged, computed entirely in SQL. It never materializes the fleet — the
// dashboard shows 20k customers as a paged, exceptions-first directory rather
// than a 20k-box tree. An empty customer_id (operator platform nodes and
// unlicensed rows) collapses into a single "" bucket the UI labels.
func (r *InstanceRepository) CustomerDirectory(ctx context.Context, search, sort string, limit, offset int) ([]CustomerSummaryRow, int, error) {
	order, ok := customerSortClauses[strings.ToLower(strings.TrimSpace(sort))]
	if !ok {
		order = customerSortClauses["exceptions"]
	}

	var searchCond string
	var args []interface{}
	if s := strings.TrimSpace(search); s != "" {
		args = append(args, "%"+s+"%")
		searchCond = " WHERE cid ILIKE $1 OR cname ILIKE $1"
	}

	base := `
		WITH agg AS (
			SELECT COALESCE(customer_id, '') AS cid,
			       COALESCE(MAX(NULLIF(customer_name, '')), '') AS cname,
			       COUNT(*) AS total,
			       COUNT(*) FILTER (WHERE derived = 'online') AS online,
			       COUNT(*) FILTER (WHERE derived = 'offline') AS offline,
			       COUNT(*) FILTER (WHERE last_update_success IS FALSE) AS failed
			FROM (
				SELECT customer_id, customer_name, last_update_success,
				       ` + derivedStatusExpr + ` AS derived
				FROM instances
			) t
			GROUP BY cid
		)`

	var total int
	if err := r.db.Pool.QueryRow(ctx, base+` SELECT COUNT(*) FROM agg`+searchCond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("customer directory count: %w", err)
	}

	pageArgs := append(append([]interface{}{}, args...), limit, offset)
	query := base + ` SELECT cid, cname, total, online, offline, failed FROM agg` + searchCond +
		` ORDER BY ` + order +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)

	rows, err := r.db.Pool.Query(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("customer directory: %w", err)
	}
	defer rows.Close()

	var out []CustomerSummaryRow
	for rows.Next() {
		var c CustomerSummaryRow
		if err := rows.Scan(&c.CustomerID, &c.CustomerName, &c.Total, &c.Online, &c.Offline, &c.Failed); err != nil {
			return nil, 0, fmt.Errorf("customer directory scan: %w", err)
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// SubtreeCounts rolls up a node's whole descendant cascade (the node itself
// included) so a collapsed relay can show its coverage without the client
// loading a single descendant row.
type SubtreeCounts struct {
	Total          int `json:"total"`
	Online         int `json:"online"`
	Offline        int `json:"offline"`
	Degraded       int `json:"degraded"`
	Decommissioned int `json:"decommissioned"`
	Failed         int `json:"failed"`
}

// TreeChildRow is one node in the lazy fleet tree: its list-level fields plus a
// SQL-computed rollup of everything beneath it. The client renders the cascade
// by expanding one node at a time, so each request stays O(page) regardless of
// fleet size.
type TreeChildRow struct {
	types.Instance
	HasChildren bool          `json:"has_children"`
	Subtree     SubtreeCounts `json:"subtree"`
}

// treeChildOrder surfaces exceptions first (offline, then failed updates), then
// relays above leaves (mysoc > siemcore > swf), then by id for stability.
var treeChildOrder = ` ORDER BY (` + derivedStatusExpr + `) = 'offline' DESC,
	last_update_success IS FALSE DESC,
	CASE product_tier WHEN 'mysoc' THEN 0 WHEN 'siemcore' THEN 1 WHEN 'swf' THEN 2 ELSE 3 END ASC,
	instance_id ASC`

// TreeChildren returns one level of the fleet cascade: the direct children of
// f.Parent, or the cascade roots (nodes with no resolvable parent link) when
// Parent is empty. Each returned node carries a rollup of its entire subtree so
// the tree can be aggregate-by-default. The level query returns one page of
// rows; the rollup is a single bounded recursive pass keyed to the returned
// nodes. Nothing materializes the whole fleet on the client.
func (r *InstanceRepository) TreeChildren(ctx context.Context, f InstanceListFilter, limit, offset int) ([]TreeChildRow, int, error) {
	var conds []string
	var args []interface{}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if p := strings.TrimSpace(f.Parent); p != "" {
		add("parent_instance_id = $%d", p)
	} else {
		// Cascade roots: no parent set at all.
		conds = append(conds, "(parent_instance_id IS NULL OR parent_instance_id = '')")
	}
	if s := strings.TrimSpace(f.Tier); s != "" {
		add("product_tier = $%d", s)
	}
	if s := strings.TrimSpace(f.Operator); s != "" {
		add("EXISTS (SELECT 1 FROM licenses l WHERE l.id = instances.license_id AND l.operator_ref = $%d)", s)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		args = append(args, "%"+s+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			"(instance_id ILIKE $%d OR hostname ILIKE $%d OR customer_name ILIKE $%d)", n, n, n))
	}
	if s := strings.TrimSpace(f.Status); s != "" {
		args = append(args, s)
		conds = append(conds, fmt.Sprintf("(%s) = $%d", derivedStatusExpr, len(args)))
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM instances`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count tree children: %w", err)
	}

	pageArgs := append(append([]interface{}{}, args...), limit, offset)
	query := `SELECT ` + selectInstanceListCols + ` FROM instances` + where + treeChildOrder +
		fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	rows, err := r.db.Pool.Query(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list tree children: %w", err)
	}
	defer rows.Close()

	var items []TreeChildRow
	var ids []string
	for rows.Next() {
		inst, err := r.scanInstanceList(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan tree child: %w", err)
		}
		items = append(items, TreeChildRow{Instance: *inst})
		ids = append(ids, inst.InstanceID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating tree children: %w", err)
	}
	if len(items) == 0 {
		return []TreeChildRow{}, total, nil
	}

	// One recursive pass rolls each descendant up to the page node it sits
	// under. The seed carries the columns derivedStatusExpr needs so the outer
	// aggregate computes read-time status without re-reading instances. Depth is
	// capped so malformed parent links can never loop forever.
	rollup := `
WITH RECURSIVE seed(root, iid, depth, reported_via, status, last_heartbeat, last_update_success) AS (
    SELECT instance_id, instance_id, 0, reported_via, status, last_heartbeat, last_update_success
    FROM instances WHERE instance_id = ANY($1)
    UNION ALL
    SELECT s.root, i.instance_id, s.depth + 1, i.reported_via, i.status, i.last_heartbeat, i.last_update_success
    FROM instances i JOIN seed s ON i.parent_instance_id = s.iid
    WHERE s.depth < 16
)
SELECT root,
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE (` + derivedStatusExpr + `) = 'online') AS online,
    COUNT(*) FILTER (WHERE (` + derivedStatusExpr + `) = 'offline') AS offline,
    COUNT(*) FILTER (WHERE (` + derivedStatusExpr + `) = 'degraded') AS degraded,
    COUNT(*) FILTER (WHERE (` + derivedStatusExpr + `) = 'decommissioned') AS decommissioned,
    COUNT(*) FILTER (WHERE last_update_success IS FALSE) AS failed
FROM seed
GROUP BY root`

	srows, err := r.db.Pool.Query(ctx, rollup, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to roll up subtree: %w", err)
	}
	defer srows.Close()

	counts := make(map[string]SubtreeCounts, len(ids))
	for srows.Next() {
		var root string
		var c SubtreeCounts
		if err := srows.Scan(&root, &c.Total, &c.Online, &c.Offline, &c.Degraded, &c.Decommissioned, &c.Failed); err != nil {
			return nil, 0, fmt.Errorf("failed to scan subtree rollup: %w", err)
		}
		counts[root] = c
	}
	if err := srows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error rolling up subtree: %w", err)
	}

	for i := range items {
		c := counts[items[i].InstanceID]
		items[i].Subtree = c
		// total counts the node itself; anything beyond one means descendants.
		items[i].HasChildren = c.Total > 1
	}
	return items, total, nil
}

// SecurityStats is the SQL-aggregated fleet security posture, computed over
// the security block inside each node's last heartbeat without shipping any
// per-node rows.
type SecurityStats struct {
	Reporting       int `json:"reporting"`
	AvgScore        int `json:"avg_score"`
	FirewallEnabled int `json:"firewall_enabled"`
	SSHHardened     int `json:"ssh_hardened"`
	PendingUpdates  int `json:"pending_updates"`
	SecurityUpdates int `json:"security_updates"`
	RebootRequired  int `json:"reboot_required"`
}

// SecurityStatsSummary aggregates the fleet's security posture in one grouped
// scan over the last_heartbeat_data JSONB.
func (r *InstanceRepository) SecurityStatsSummary(ctx context.Context) (*SecurityStats, error) {
	var s SecurityStats
	err := r.db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE sec IS NOT NULL) AS reporting,
			COALESCE(ROUND(AVG((sec->>'security_score')::numeric) FILTER (WHERE sec IS NOT NULL)), 0)::int AS avg_score,
			COUNT(*) FILTER (WHERE (sec->>'firewall_enabled')::boolean) AS firewall,
			COUNT(*) FILTER (WHERE (sec->>'ssh_hardened')::boolean) AS ssh,
			COALESCE(SUM((sec->>'pending_updates')::int), 0) AS pending,
			COALESCE(SUM((sec->>'security_updates')::int), 0) AS secupd,
			COUNT(*) FILTER (WHERE (sec->>'reboot_required')::boolean) AS reboot
		FROM (SELECT last_heartbeat_data->'security' AS sec FROM instances) t
	`).Scan(&s.Reporting, &s.AvgScore, &s.FirewallEnabled, &s.SSHHardened,
		&s.PendingUpdates, &s.SecurityUpdates, &s.RebootRequired)
	if err != nil {
		return nil, fmt.Errorf("security stats: %w", err)
	}
	return &s, nil
}

// SecurityRow is one node's security posture for the paged security view.
type SecurityRow struct {
	ID         string                `json:"id"`
	InstanceID string                `json:"instance_id"`
	Hostname   string                `json:"hostname"`
	Tier       string                `json:"product_tier"`
	Status     string                `json:"status"`
	Security   *types.SecurityStatus `json:"security"`
}

// ListSecurityPaged returns reporting nodes worst-score-first (exceptions
// first), paged — the security table never loads the whole fleet.
func (r *InstanceRepository) ListSecurityPaged(ctx context.Context, limit, offset int) ([]SecurityRow, int, error) {
	var total int
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM instances WHERE last_heartbeat_data ? 'security'`,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("security count: %w", err)
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, instance_id, COALESCE(hostname, ''), COALESCE(product_tier, ''),
		       `+derivedStatusExpr+` AS status,
		       last_heartbeat_data->'security' AS security
		FROM instances
		WHERE last_heartbeat_data ? 'security'
		ORDER BY (last_heartbeat_data->'security'->>'security_score')::int ASC NULLS LAST
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("security list: %w", err)
	}
	defer rows.Close()

	var out []SecurityRow
	for rows.Next() {
		var row SecurityRow
		var secJSON []byte
		if err := rows.Scan(&row.ID, &row.InstanceID, &row.Hostname, &row.Tier, &row.Status, &secJSON); err != nil {
			return nil, 0, fmt.Errorf("security scan: %w", err)
		}
		if len(secJSON) > 0 {
			var sec types.SecurityStatus
			if err := json.Unmarshal(secJSON, &sec); err == nil {
				row.Security = &sec
			}
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

// ParentChain resolves an instance's ancestor chain (parent, grandparent, …)
// server-side by walking parent_instance_id, so the detail view never fetches
// the whole fleet to find one node's lineage. Ordered nearest-parent first.
// The cascade is three tiers, so the walk is bounded to a handful of hops and
// a visited set guards against a malformed cycle.
func (r *InstanceRepository) ParentChain(ctx context.Context, instanceID string) ([]types.Instance, error) {
	node, err := r.GetByInstanceID(ctx, instanceID)
	if errors.Is(err, ErrInstanceNotFound) {
		return nil, ErrInstanceNotFound
	}
	if err != nil {
		return nil, err
	}

	const maxDepth = 16
	seen := map[string]bool{instanceID: true}
	var chain []types.Instance
	parentID := node.ParentInstanceID
	for i := 0; i < maxDepth; i++ {
		parentID = strings.TrimSpace(parentID)
		if parentID == "" || seen[parentID] {
			break
		}
		seen[parentID] = true
		parent, err := r.GetByInstanceID(ctx, parentID)
		if errors.Is(err, ErrInstanceNotFound) {
			break
		}
		if err != nil {
			return nil, err
		}
		chain = append(chain, *parent)
		parentID = parent.ParentInstanceID
	}
	return chain, nil
}

// Update updates an instance
func (r *InstanceRepository) Update(ctx context.Context, instance *types.Instance) error {
	instance.UpdatedAt = time.Now()

	_, err := r.db.Pool.Exec(ctx, `
		UPDATE instances
		SET hostname = $2, api_key_hash = $3, status = $4, auto_update_enabled = $5, updated_at = $6
		WHERE id = $1
	`, instance.ID, instance.Hostname, instance.APIKeyHash, instance.Status, instance.AutoUpdateEnabled, instance.UpdatedAt)

	return err
}

// SetAutoUpdate enables or disables auto-update for an instance
// This is a thin wrapper around UpdateInstance for backward compatibility
func (r *InstanceRepository) SetAutoUpdate(ctx context.Context, id string, enabled bool) error {
	_, err := r.UpdateInstance(ctx, id, nil, &enabled, nil)
	return err
}

// SetUpdateGroup sets the update group for an instance
// This is a thin wrapper around UpdateInstance for backward compatibility
func (r *InstanceRepository) SetUpdateGroup(ctx context.Context, id string, group string) error {
	_, err := r.UpdateInstance(ctx, id, nil, nil, &group)
	return err
}

// UpdateHeartbeat updates the last heartbeat for an instance
// clientIP is the IP address of the connecting client (can be empty)
func (r *InstanceRepository) UpdateHeartbeat(ctx context.Context, instanceID string, heartbeat *types.Heartbeat, clientIP string) error {
	heartbeatData, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	now := time.Now()

	// Build base update
	query := `
		UPDATE instances
		SET last_heartbeat = $2, last_heartbeat_data = $3, status = 'online', 
		    last_ip_address = $4, last_ip_seen_at = $5, updated_at = $5
		WHERE instance_id = $1
	`
	args := []interface{}{instanceID, now, heartbeatData, clientIP, now}

	// If heartbeat contains update attempt, persist it
	if heartbeat.LastUpdateAttempt != nil {
		query = `
			UPDATE instances
			SET last_heartbeat = $2, last_heartbeat_data = $3, status = 'online',
			    last_ip_address = $4, last_ip_seen_at = $5,
			    last_update_from_version = $6, last_update_target_version = $7,
			    last_update_success = $8, last_update_error = $9, last_update_at = $10,
			    updated_at = $5
			WHERE instance_id = $1
		`
		attempt := heartbeat.LastUpdateAttempt
		args = []interface{}{
			instanceID, now, heartbeatData, clientIP, now,
			attempt.FromVersion, attempt.TargetVersion, attempt.Success, attempt.Error, attempt.Timestamp,
		}
	}

	_, err = r.db.Pool.Exec(ctx, query, args...)
	return err
}

// UpsertFromHeartbeat creates or updates an instance from a heartbeat
// This is used when an instance sends its first heartbeat
// clientIP is the IP address of the connecting client (can be empty)
func (r *InstanceRepository) UpsertFromHeartbeat(ctx context.Context, instanceID string, heartbeat *types.Heartbeat, licenseID, clientIP string) error {
	heartbeatData, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	now := time.Now()
	id := uuid.New().String()

	// Determine instance type from heartbeat or default to siemcore
	instanceType := heartbeat.InstanceType
	if instanceType == "" {
		instanceType = "siemcore"
	}

	// Convert empty licenseID to nil for proper NULL handling in SQL
	var licenseIDPtr *string
	if licenseID != "" {
		licenseIDPtr = &licenseID
	}

	// Self-reported hierarchy: store NULL when the agent omits a value so an
	// existing tier/parent is preserved across heartbeats (COALESCE below).
	var productTierPtr, parentInstanceIDPtr, customerIDPtr, customerNamePtr *string
	if tier := heartbeat.ProductTier; tier != "" {
		productTierPtr = &tier
	}
	if parent := heartbeat.ParentInstanceID; parent != "" {
		parentInstanceIDPtr = &parent
	}
	if customer := heartbeat.CustomerID; customer != "" {
		customerIDPtr = &customer
	}
	if customerName := heartbeat.CustomerName; customerName != "" {
		customerNamePtr = &customerName
	}

	// Base UPSERT query with IP tracking
	query := `
		INSERT INTO instances (id, instance_id, instance_type, hostname, license_id, api_key_hash, 
		                       last_heartbeat, last_heartbeat_data, status, last_ip_address, last_ip_seen_at, 
		                       product_tier, parent_instance_id, customer_id, customer_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '', $6, $7, 'online', $8, $9, $10, $11, $12, $13, $9, $9)
		ON CONFLICT (instance_id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			license_id = COALESCE(EXCLUDED.license_id, instances.license_id),
			last_heartbeat = EXCLUDED.last_heartbeat,
			last_heartbeat_data = EXCLUDED.last_heartbeat_data,
			status = 'online',
			last_ip_address = EXCLUDED.last_ip_address,
			last_ip_seen_at = EXCLUDED.last_ip_seen_at,
			product_tier = COALESCE(EXCLUDED.product_tier, instances.product_tier),
			parent_instance_id = COALESCE(EXCLUDED.parent_instance_id, instances.parent_instance_id),
			customer_id = COALESCE(EXCLUDED.customer_id, instances.customer_id),
			customer_name = COALESCE(EXCLUDED.customer_name, instances.customer_name),
			reported_via = NULL,
			reported_at = NULL,
			updated_at = EXCLUDED.updated_at
	`
	args := []interface{}{id, instanceID, instanceType, heartbeat.Hostname, licenseIDPtr, now, heartbeatData, clientIP, now, productTierPtr, parentInstanceIDPtr, customerIDPtr, customerNamePtr}

	// If heartbeat contains update attempt, include those columns
	if heartbeat.LastUpdateAttempt != nil {
		attempt := heartbeat.LastUpdateAttempt
		query = `
			INSERT INTO instances (id, instance_id, instance_type, hostname, license_id, api_key_hash, 
			                       last_heartbeat, last_heartbeat_data, status, last_ip_address, last_ip_seen_at,
			                       last_update_from_version, last_update_target_version, last_update_success,
			                       last_update_error, last_update_at, product_tier, parent_instance_id,
			                       customer_id, customer_name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, '', $6, $7, 'online', $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $9, $9)
			ON CONFLICT (instance_id) DO UPDATE SET
				hostname = EXCLUDED.hostname,
				license_id = COALESCE(EXCLUDED.license_id, instances.license_id),
				last_heartbeat = EXCLUDED.last_heartbeat,
				last_heartbeat_data = EXCLUDED.last_heartbeat_data,
				status = 'online',
				last_ip_address = EXCLUDED.last_ip_address,
				last_ip_seen_at = EXCLUDED.last_ip_seen_at,
				last_update_from_version = EXCLUDED.last_update_from_version,
				last_update_target_version = EXCLUDED.last_update_target_version,
				last_update_success = EXCLUDED.last_update_success,
				last_update_error = EXCLUDED.last_update_error,
				last_update_at = EXCLUDED.last_update_at,
				product_tier = COALESCE(EXCLUDED.product_tier, instances.product_tier),
				parent_instance_id = COALESCE(EXCLUDED.parent_instance_id, instances.parent_instance_id),
				customer_id = COALESCE(EXCLUDED.customer_id, instances.customer_id),
				customer_name = COALESCE(EXCLUDED.customer_name, instances.customer_name),
				reported_via = NULL,
				reported_at = NULL,
				updated_at = EXCLUDED.updated_at
		`
		args = []interface{}{
			id, instanceID, instanceType, heartbeat.Hostname, licenseIDPtr, now, heartbeatData, clientIP, now,
			attempt.FromVersion, attempt.TargetVersion, attempt.Success, attempt.Error, attempt.Timestamp,
			productTierPtr, parentInstanceIDPtr, customerIDPtr, customerNamePtr,
		}
	}

	_, err = r.db.Pool.Exec(ctx, query, args...)
	return err
}

// Delete deletes an instance
func (r *InstanceRepository) Delete(ctx context.Context, id string) error {
	cmdTag, err := r.db.Pool.Exec(ctx, `DELETE FROM instances WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

// UpdateDisplayName updates the display name for an instance
func (r *InstanceRepository) UpdateDisplayName(ctx context.Context, instanceID string, displayName string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE instances
		SET display_name = $2, updated_at = NOW()
		WHERE instance_id = $1
	`, instanceID, displayName)

	return err
}

// ErrNoFieldsToUpdate is returned when UpdateInstance is called with no fields to update
var ErrNoFieldsToUpdate = errors.New("no fields to update")

// UpdateInstance updates instance settings (display_name, auto_update, update_group)
// Uses pk `id` for dashboard consistency. Returns the updated instance.
func (r *InstanceRepository) UpdateInstance(ctx context.Context, id string, displayName *string, autoUpdate *bool, updateGroup *string) (*types.Instance, error) {
	// Validate update_group if provided (repo-level validation)
	if updateGroup != nil {
		if err := ValidateUpdateGroup(*updateGroup); err != nil {
			return nil, err
		}
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{id}
	argNum := 2

	if displayName != nil {
		updates = append(updates, fmt.Sprintf("display_name = $%d", argNum))
		args = append(args, *displayName)
		argNum++
	}
	if autoUpdate != nil {
		updates = append(updates, fmt.Sprintf("auto_update_enabled = $%d", argNum))
		args = append(args, *autoUpdate)
		argNum++
	}
	if updateGroup != nil {
		updates = append(updates, fmt.Sprintf("update_group = $%d", argNum))
		args = append(args, *updateGroup)
		argNum++
	}

	if len(updates) == 0 {
		return nil, ErrNoFieldsToUpdate
	}

	// Use RETURNING to get updated instance without extra query
	query := fmt.Sprintf(`
		UPDATE instances
		SET %s, updated_at = NOW()
		WHERE id = $1
		RETURNING `+selectInstanceFullCols, strings.Join(updates, ", "))

	row := r.db.Pool.QueryRow(ctx, query, args...)
	instance, err := r.scanInstanceFull(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update instance: %w", err)
	}

	return instance, nil
}

// maxRollupNodes caps how many nodes one heartbeat rollup may carry.
const maxRollupNodes = 10000

// TouchFromCheck records an update-check sighting of an instance. Unlike
// UpsertFromHeartbeat it must NOT masquerade as a direct heartbeat: checks are
// routinely forwarded upstream by cascade relays on behalf of their children,
// so bumping last_heartbeat here would permanently starve the rollup
// freshness guard, and clearing reported_via would erase cascade provenance
// every cycle. A check therefore only creates a skeleton row on first sight
// and refreshes the IP sighting plus identity COALESCEs; liveness comes
// exclusively from heartbeats (direct or rolled up through a relay).
func (r *InstanceRepository) TouchFromCheck(ctx context.Context, instanceID string, heartbeat *types.Heartbeat, licenseID, clientIP string) error {
	heartbeatData, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	now := time.Now()
	instanceType := heartbeat.InstanceType
	if instanceType == "" {
		instanceType = "siemcore"
	}
	var licenseIDPtr *string
	if licenseID != "" {
		licenseIDPtr = &licenseID
	}
	var productTierPtr, parentInstanceIDPtr *string
	if tier := heartbeat.ProductTier; tier != "" {
		productTierPtr = &tier
	}
	if parent := heartbeat.ParentInstanceID; parent != "" {
		parentInstanceIDPtr = &parent
	}

	// Cascade children default to auto-update OFF: real product installs on a
	// freshly enrolled child must be an explicit operator decision, not a side
	// effect of enrollment. (Updater self-update is exempt from the toggle and
	// stays on.) A child is anything that declares a parent — or whose tier
	// structurally requires one (siemcore, swf), because a first-contact check
	// may legitimately omit parent_instance_id (orphan enrollment racing ahead
	// of the relay rollup) and must not get wider authorization for it.
	autoUpdate := parentInstanceIDPtr == nil && !catalog.RequiresParent(heartbeat.ProductTier)

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO instances (id, instance_id, instance_type, hostname, license_id, api_key_hash,
		                       last_heartbeat, last_heartbeat_data, status, last_ip_address, last_ip_seen_at,
		                       product_tier, parent_instance_id, auto_update_enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '', $6, $7, 'online', $8, $9, $10, $11, $12, $9, $9)
		ON CONFLICT (instance_id) DO UPDATE SET
			license_id = COALESCE(instances.license_id, EXCLUDED.license_id),
			-- For relay-reported (cascade) nodes the check arrives from the
			-- relay's egress address, not the child's: keep the address the
			-- relay observed and rolled up instead of clobbering it.
			last_ip_address = CASE WHEN instances.reported_via IS NULL
				THEN EXCLUDED.last_ip_address
				ELSE COALESCE(instances.last_ip_address, EXCLUDED.last_ip_address) END,
			last_ip_seen_at = CASE WHEN instances.reported_via IS NULL
				THEN EXCLUDED.last_ip_seen_at
				ELSE COALESCE(instances.last_ip_seen_at, EXCLUDED.last_ip_seen_at) END,
			product_tier = COALESCE(instances.product_tier, EXCLUDED.product_tier),
			parent_instance_id = COALESCE(instances.parent_instance_id, EXCLUDED.parent_instance_id),
			hostname = COALESCE(NULLIF(instances.hostname, ''), EXCLUDED.hostname),
			updated_at = EXCLUDED.updated_at
	`, uuid.New().String(), instanceID, instanceType, heartbeat.Hostname, licenseIDPtr,
		now, heartbeatData, clientIP, now, productTierPtr, parentInstanceIDPtr, autoUpdate)
	return err
}

// MarkDecommissioned records a node's own announcement of clean removal
// (contract 1.11.0 Item B). It is a state change, never a deletion: the row
// and its history stay for audit, and any subsequent genuine heartbeat or
// check flips the status back to online (visible revival). Idempotent — a
// missing row is not an error, because a goodbye must never fail loudly.
func (r *InstanceRepository) MarkDecommissioned(ctx context.Context, instanceID string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE instances SET status = 'decommissioned', updated_at = NOW()
		WHERE instance_id = $1
	`, instanceID)
	return err
}

// reportedNode is one flattened rollup entry paired with the parent it was
// reported under, so the recursive tree can be upserted as a flat batch.
type reportedNode struct {
	parentID string
	child    *types.ChildReport
}

// upsertReportedNodeSQL is the single-row rollup upsert, shared by the batched
// ingest path. The freshness guard (WHERE) keeps a direct heartbeat or a
// fresher rollup from being clobbered by a staler report. last_heartbeat_data
// is only rewritten when it actually changed — an unchanged rollup reuses the
// stored TOAST value instead of re-toasting identical JSON every cycle, which
// is the dominant write cost at fleet scale. A decommission tombstone (relay
// restarted, no retained heartbeat) likewise keeps the last real telemetry.
const upsertReportedNodeSQL = `
	INSERT INTO instances (id, instance_id, instance_type, hostname, license_id, api_key_hash,
	                       last_heartbeat, last_heartbeat_data, status, product_tier, parent_instance_id,
	                       customer_id, customer_name, reported_via, reported_at,
	                       last_update_from_version, last_update_target_version, last_update_success,
	                       last_update_error, last_update_at, last_ip_address, last_ip_seen_at,
	                       auto_update_enabled, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, '', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
	        CASE WHEN $20::text IS NULL THEN NULL ELSE $14::timestamptz END, FALSE, $14, $14)
	ON CONFLICT (instance_id) DO UPDATE SET
		hostname = COALESCE(NULLIF(EXCLUDED.hostname, ''), instances.hostname),
		license_id = COALESCE(instances.license_id, EXCLUDED.license_id),
		last_heartbeat = EXCLUDED.last_heartbeat,
		last_heartbeat_data = CASE
			WHEN EXCLUDED.status = 'decommissioned'
				THEN COALESCE(instances.last_heartbeat_data, EXCLUDED.last_heartbeat_data)
			WHEN instances.last_heartbeat_data IS DISTINCT FROM EXCLUDED.last_heartbeat_data
				THEN EXCLUDED.last_heartbeat_data
			ELSE instances.last_heartbeat_data END,
		status = EXCLUDED.status,
		product_tier = COALESCE(NULLIF(EXCLUDED.product_tier, ''), instances.product_tier),
		parent_instance_id = COALESCE(NULLIF(EXCLUDED.parent_instance_id, ''), instances.parent_instance_id),
		customer_id = COALESCE(NULLIF(EXCLUDED.customer_id, ''), instances.customer_id),
		customer_name = COALESCE(NULLIF(EXCLUDED.customer_name, ''), instances.customer_name),
		reported_via = EXCLUDED.reported_via,
		reported_at = EXCLUDED.reported_at,
		last_update_from_version = COALESCE(EXCLUDED.last_update_from_version, instances.last_update_from_version),
		last_update_target_version = COALESCE(EXCLUDED.last_update_target_version, instances.last_update_target_version),
		last_update_success = COALESCE(EXCLUDED.last_update_success, instances.last_update_success),
		last_update_error = COALESCE(EXCLUDED.last_update_error, instances.last_update_error),
		last_update_at = COALESCE(EXCLUDED.last_update_at, instances.last_update_at),
		last_ip_address = COALESCE(EXCLUDED.last_ip_address, instances.last_ip_address),
		last_ip_seen_at = COALESCE(EXCLUDED.last_ip_seen_at, instances.last_ip_seen_at),
		updated_at = EXCLUDED.updated_at
	WHERE instances.last_heartbeat IS NULL OR instances.last_heartbeat <= EXCLUDED.last_heartbeat`

// flattenReportedChildren walks the rollup tree depth-first into a flat slice,
// skipping the reporter itself and empty ids. When the node budget is reached
// the walk stops and reports truncated=true rather than failing the whole
// heartbeat: a partial fleet view beats a rejected rollup that drops the
// entire subtree (the caller logs the truncation).
func flattenReportedChildren(reporterInstanceID string, children []types.ChildReport, budget int) (nodes []reportedNode, truncated bool) {
	var walk func(parentID string, list []types.ChildReport)
	walk = func(parentID string, list []types.ChildReport) {
		for i := range list {
			child := &list[i]
			childID := strings.TrimSpace(child.InstanceID)
			if childID == "" || childID == reporterInstanceID {
				continue
			}
			if len(nodes) >= budget {
				truncated = true
				return
			}
			nodes = append(nodes, reportedNode{parentID: parentID, child: child})
			walk(childID, child.Children)
			if truncated {
				return
			}
		}
	}
	walk(reporterInstanceID, children)
	return nodes, truncated
}

// UpsertReportedChildren flattens a relay's rollup tree and upserts every node
// in a single pipelined batch (one network round trip instead of one per
// node). reporterInstanceID is the relay that sent the covering heartbeat;
// licenseID is that relay's license (inherited by all reported nodes). Nodes
// that also heartbeat directly are not overwritten with staler rollup data.
// Returns the number of nodes ingested and whether the rollup was truncated
// at maxRollupNodes.
func (r *InstanceRepository) UpsertReportedChildren(ctx context.Context, reporterInstanceID, licenseID string, children []types.ChildReport) (int, bool, error) {
	nodes, truncated := flattenReportedChildren(reporterInstanceID, children, maxRollupNodes)
	if len(nodes) == 0 {
		return 0, truncated, nil
	}

	now := time.Now()
	batch := &pgx.Batch{}
	for _, n := range nodes {
		args, err := reportedNodeUpsertArgs(reporterInstanceID, n.parentID, licenseID, n.child, now)
		if err != nil {
			return 0, truncated, err
		}
		batch.Queue(upsertReportedNodeSQL, args...)
	}

	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(nodes); i++ {
		if _, err := br.Exec(); err != nil {
			return i, truncated, fmt.Errorf("upsert reported node %s: %w", nodes[i].child.InstanceID, err)
		}
	}
	return len(nodes), truncated, nil
}

// upsertCustomerSummarySQL upserts one per-customer aggregate, keyed by the
// customer and the relay that reported it (Fleet Scalability 1.12).
const upsertCustomerSummarySQL = `
	INSERT INTO customer_summaries (
		customer_id, reporter_id, customer_name,
		total, online, offline, degraded, decommissioned, failed_updates,
		versions, status_reported_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	ON CONFLICT (customer_id, reporter_id) DO UPDATE SET
		customer_name = EXCLUDED.customer_name,
		total = EXCLUDED.total,
		online = EXCLUDED.online,
		offline = EXCLUDED.offline,
		degraded = EXCLUDED.degraded,
		decommissioned = EXCLUDED.decommissioned,
		failed_updates = EXCLUDED.failed_updates,
		versions = EXCLUDED.versions,
		status_reported_at = EXCLUDED.status_reported_at,
		updated_at = EXCLUDED.updated_at`

// IngestDelta applies a change-only delta envelope (Fleet Scalability 1.12):
// it batch-upserts the changed leaf rows exactly like the full rollup path
// (one row per node, updated on change) and upserts each customer summary into
// customer_summaries. reporterID is the relay that delivered the envelope to
// this server; forwarded summaries keep their own ReporterID (the relay deep
// in the cascade that owns them). It returns the number of inventory rows
// written. A relay sends deltas OR a full rollup, never both, so this never
// double-counts.
func (r *InstanceRepository) IngestDelta(ctx context.Context, reporterID, licenseID string, env *types.DeltaEnvelope) (int, error) {
	if env == nil {
		return 0, nil
	}

	now := time.Now()
	if len(env.Inventory) > 0 {
		batch := &pgx.Batch{}
		for i := range env.Inventory {
			node := env.Inventory[i].Node
			args, err := reportedNodeUpsertArgs(reporterID, node.ParentInstanceID, licenseID, &node, now)
			if err != nil {
				return 0, err
			}
			batch.Queue(upsertReportedNodeSQL, args...)
		}
		br := r.db.Pool.SendBatch(ctx, batch)
		for i := 0; i < len(env.Inventory); i++ {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return i, fmt.Errorf("delta inventory upsert %s: %w", env.Inventory[i].Node.InstanceID, err)
			}
		}
		br.Close()
	}

	if len(env.Summaries) > 0 {
		batch := &pgx.Batch{}
		for i := range env.Summaries {
			s := env.Summaries[i]
			versions, err := json.Marshal(s.Versions)
			if err != nil {
				return len(env.Inventory), fmt.Errorf("marshal summary versions: %w", err)
			}
			var reportedAt *time.Time
			if !s.StatusReportedAt.IsZero() {
				reportedAt = &s.StatusReportedAt
			}
			owner := strings.TrimSpace(s.ReporterID)
			if owner == "" {
				owner = reporterID
			}
			batch.Queue(upsertCustomerSummarySQL,
				s.CustomerID, owner, s.CustomerName,
				s.Total, s.Online, s.Offline, s.Degraded, s.Decommissioned, s.FailedUpdates,
				versions, reportedAt, now)
		}
		br := r.db.Pool.SendBatch(ctx, batch)
		for i := 0; i < len(env.Summaries); i++ {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return len(env.Inventory), fmt.Errorf("delta summary upsert %s: %w", env.Summaries[i].CustomerID, err)
			}
		}
		br.Close()
	}
	return len(env.Inventory), nil
}

// reportedNodeUpsertArgs builds the positional arguments for
// upsertReportedNodeSQL from one rollup node.
func reportedNodeUpsertArgs(reporterID, parentID, licenseID string, child *types.ChildReport, now time.Time) ([]interface{}, error) {
	instanceType := child.InstanceType
	if instanceType == "" {
		instanceType = child.ProductTier
	}
	status := child.Status
	if status != "online" && status != "offline" && status != "degraded" && status != "decommissioned" {
		status = "online"
	}
	lastSeen := child.LastSeen
	if lastSeen.IsZero() {
		lastSeen = now
	}

	// Synthesize a heartbeat snapshot so existing dashboard views (versions,
	// products) render reported nodes exactly like direct ones.
	snapshot := types.Heartbeat{
		InstanceID:        child.InstanceID,
		InstanceType:      instanceType,
		ProductTier:       child.ProductTier,
		ParentInstanceID:  parentID,
		CustomerID:        child.CustomerID,
		CustomerName:      child.CustomerName,
		Hostname:          child.Hostname,
		UpdaterVersion:    child.UpdaterVersion,
		Products:          child.Products,
		Timestamp:         lastSeen,
		LastUpdateAttempt: child.LastUpdateAttempt,
	}
	if child.System != nil {
		snapshot.System = *child.System
	}
	heartbeatData, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal reported heartbeat: %w", err)
	}

	var licenseIDPtr *string
	if licenseID != "" {
		licenseIDPtr = &licenseID
	}
	var attemptFrom, attemptTarget, attemptError *string
	var attemptSuccess *bool
	var attemptAt *time.Time
	if a := child.LastUpdateAttempt; a != nil {
		attemptFrom, attemptTarget, attemptError = &a.FromVersion, &a.TargetVersion, &a.Error
		attemptSuccess = &a.Success
		attemptAt = &a.Timestamp
	}

	// SourceIP is the address the relay observed the child connecting from;
	// it gives cascaded nodes an address on the dashboard (they never reach
	// this server directly).
	var sourceIP *string
	if child.SourceIP != "" {
		sourceIP = &child.SourceIP
	}

	return []interface{}{
		uuid.New().String(), child.InstanceID, instanceType, child.Hostname, licenseIDPtr,
		lastSeen, heartbeatData, status, child.ProductTier, parentID,
		child.CustomerID, child.CustomerName, reporterID, now,
		attemptFrom, attemptTarget, attemptSuccess, attemptError, attemptAt, sourceIP,
	}, nil
}
