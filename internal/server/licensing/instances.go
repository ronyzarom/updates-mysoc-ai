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
	// derivedStatusCol computes liveness at read time: a node that has not
	// been heard from (directly via last_heartbeat, or through rollup via
	// reported_at — GREATEST ignores NULLs) within the threshold reads as
	// offline, regardless of the stored status. Nothing writes 'offline'
	// back; the stored value flips to 'online' again on the next contact.
	derivedStatusCol = `CASE
		WHEN status IN ('online', 'degraded')
		     AND GREATEST(last_heartbeat, reported_at) < NOW() - INTERVAL '5 minutes'
		THEN 'offline'
		ELSE status
	END AS status`

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

// UpsertReportedChildren flattens a relay's rollup tree and upserts every node.
// reporterInstanceID is the relay that sent the covering heartbeat; licenseID
// is that relay's license (inherited by all reported nodes). Nodes that also
// heartbeat directly are not overwritten with staler rollup data.
func (r *InstanceRepository) UpsertReportedChildren(ctx context.Context, reporterInstanceID, licenseID string, children []types.ChildReport) (int, error) {
	now := time.Now()
	count := 0

	var walk func(parentID string, nodes []types.ChildReport) error
	walk = func(parentID string, nodes []types.ChildReport) error {
		for i := range nodes {
			child := &nodes[i]
			childID := strings.TrimSpace(child.InstanceID)
			if childID == "" || childID == reporterInstanceID {
				continue
			}
			if count >= maxRollupNodes {
				return fmt.Errorf("rollup exceeds %d nodes", maxRollupNodes)
			}
			count++

			if err := r.upsertReportedNode(ctx, reporterInstanceID, parentID, licenseID, child, now); err != nil {
				return fmt.Errorf("upsert reported node %s: %w", childID, err)
			}
			if err := walk(childID, child.Children); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(reporterInstanceID, children); err != nil {
		return count, err
	}
	return count, nil
}

func (r *InstanceRepository) upsertReportedNode(ctx context.Context, reporterID, parentID, licenseID string, child *types.ChildReport, now time.Time) error {
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
		return fmt.Errorf("marshal reported heartbeat: %w", err)
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

	// The freshness guard keeps a direct heartbeat (or a fresher rollup) from
	// being clobbered by an older report.
	// Every rollup-reported node is a cascade child: default auto-update OFF
	// on first sight (see TouchFromCheck). Existing rows keep their setting.
	// SourceIP is the address the relay observed the child connecting from;
	// it gives cascaded nodes an address on the dashboard (they never reach
	// this server directly).
	var sourceIP *string
	if child.SourceIP != "" {
		sourceIP = &child.SourceIP
	}

	_, err = r.db.Pool.Exec(ctx, `
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
			-- A decommission mark can arrive as a skeletal tombstone (relay
			-- restarted, no retained heartbeat): keep the last real telemetry
			-- instead of blanking the row's history.
			last_heartbeat_data = CASE WHEN EXCLUDED.status = 'decommissioned'
				THEN COALESCE(instances.last_heartbeat_data, EXCLUDED.last_heartbeat_data)
				ELSE EXCLUDED.last_heartbeat_data END,
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
		WHERE instances.last_heartbeat IS NULL OR instances.last_heartbeat <= EXCLUDED.last_heartbeat
	`, uuid.New().String(), child.InstanceID, instanceType, child.Hostname, licenseIDPtr,
		lastSeen, heartbeatData, status, child.ProductTier, parentID,
		child.CustomerID, child.CustomerName, reporterID, now,
		attemptFrom, attemptTarget, attemptSuccess, attemptError, attemptAt, sourceIP)
	return err
}
