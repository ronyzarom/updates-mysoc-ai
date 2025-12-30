package licensing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// InstanceRepository handles instance database operations
type InstanceRepository struct {
	db *database.DB
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
	var instance types.Instance
	var lastHeartbeatData []byte

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, instance_id, instance_type, hostname, license_id, api_key_hash, last_heartbeat, last_heartbeat_data, status, created_at, updated_at
		FROM instances
		WHERE id = $1
	`, id).Scan(
		&instance.ID, &instance.InstanceID, &instance.InstanceType, &instance.Hostname,
		&instance.LicenseID, &instance.APIKeyHash, &instance.LastHeartbeat, &lastHeartbeatData,
		&instance.Status, &instance.CreatedAt, &instance.UpdatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	if lastHeartbeatData != nil {
		var heartbeat types.Heartbeat
		if err := json.Unmarshal(lastHeartbeatData, &heartbeat); err == nil {
			instance.LastHeartbeatData = &heartbeat
		}
	}

	return &instance, nil
}

// GetByInstanceID retrieves an instance by instance_id
func (r *InstanceRepository) GetByInstanceID(ctx context.Context, instanceID string) (*types.Instance, error) {
	var instance types.Instance
	var lastHeartbeatData []byte

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, instance_id, instance_type, hostname, license_id, api_key_hash, last_heartbeat, last_heartbeat_data, status, created_at, updated_at
		FROM instances
		WHERE instance_id = $1
	`, instanceID).Scan(
		&instance.ID, &instance.InstanceID, &instance.InstanceType, &instance.Hostname,
		&instance.LicenseID, &instance.APIKeyHash, &instance.LastHeartbeat, &lastHeartbeatData,
		&instance.Status, &instance.CreatedAt, &instance.UpdatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	if lastHeartbeatData != nil {
		var heartbeat types.Heartbeat
		if err := json.Unmarshal(lastHeartbeatData, &heartbeat); err == nil {
			instance.LastHeartbeatData = &heartbeat
		}
	}

	return &instance, nil
}

// GetByAPIKeyHash retrieves an instance by API key hash
func (r *InstanceRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*types.Instance, error) {
	var instance types.Instance
	var lastHeartbeatData []byte

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, instance_id, instance_type, hostname, license_id, api_key_hash, last_heartbeat, last_heartbeat_data, status, created_at, updated_at
		FROM instances
		WHERE api_key_hash = $1
	`, apiKeyHash).Scan(
		&instance.ID, &instance.InstanceID, &instance.InstanceType, &instance.Hostname,
		&instance.LicenseID, &instance.APIKeyHash, &instance.LastHeartbeat, &lastHeartbeatData,
		&instance.Status, &instance.CreatedAt, &instance.UpdatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	if lastHeartbeatData != nil {
		var heartbeat types.Heartbeat
		if err := json.Unmarshal(lastHeartbeatData, &heartbeat); err == nil {
			instance.LastHeartbeatData = &heartbeat
		}
	}

	return &instance, nil
}

// List retrieves all instances
func (r *InstanceRepository) List(ctx context.Context) ([]types.Instance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, instance_id, instance_type, hostname, license_id, api_key_hash, last_heartbeat, last_heartbeat_data, status, created_at, updated_at
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

		err := rows.Scan(
			&instance.ID, &instance.InstanceID, &instance.InstanceType, &instance.Hostname,
			&instance.LicenseID, &instance.APIKeyHash, &instance.LastHeartbeat, &lastHeartbeatData,
			&instance.Status, &instance.CreatedAt, &instance.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan instance: %w", err)
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

// Update updates an instance
func (r *InstanceRepository) Update(ctx context.Context, instance *types.Instance) error {
	instance.UpdatedAt = time.Now()

	_, err := r.db.Pool.Exec(ctx, `
		UPDATE instances
		SET hostname = $2, api_key_hash = $3, status = $4, updated_at = $5
		WHERE id = $1
	`, instance.ID, instance.Hostname, instance.APIKeyHash, instance.Status, instance.UpdatedAt)

	return err
}

// UpdateHeartbeat updates the last heartbeat for an instance
func (r *InstanceRepository) UpdateHeartbeat(ctx context.Context, instanceID string, heartbeat *types.Heartbeat) error {
	heartbeatData, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	now := time.Now()

	_, err = r.db.Pool.Exec(ctx, `
		UPDATE instances
		SET last_heartbeat = $2, last_heartbeat_data = $3, status = 'online', updated_at = $4
		WHERE instance_id = $1
	`, instanceID, now, heartbeatData, now)

	return err
}

// Delete deletes an instance
func (r *InstanceRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM instances WHERE id = $1`, id)
	return err
}

// UpdateOfflineInstances marks instances as offline if no heartbeat in threshold
func (r *InstanceRepository) UpdateOfflineInstances(ctx context.Context, threshold time.Duration) error {
	cutoff := time.Now().Add(-threshold)

	_, err := r.db.Pool.Exec(ctx, `
		UPDATE instances
		SET status = 'offline', updated_at = NOW()
		WHERE last_heartbeat < $1 AND status = 'online'
	`, cutoff)

	return err
}

