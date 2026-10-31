package licensing

import (
	"context"
	"encoding/json"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// RecordArtifactReport preserves immediate dual-delivery status in the same
// heartbeat JSON used by rollups, scoped to the authenticated operator license.
func (r *InstanceRepository) RecordArtifactReport(ctx context.Context, instanceID, licenseID string, attempt types.UpdateAttempt) (bool, error) {
	data, err := json.Marshal(attempt)
	if err != nil {
		return false, err
	}
	result, err := r.db.Pool.Exec(ctx, `UPDATE instances SET
 last_heartbeat_data=jsonb_set(COALESCE(last_heartbeat_data,'{}'::jsonb),'{last_update_attempt}',$3::jsonb,true),
 last_update_from_version=$4,last_update_target_version=$5,last_update_success=$6,last_update_error=$7,last_update_at=$8
 WHERE instance_id=$1 AND license_id::text=$2 AND (last_update_at IS NULL OR last_update_at <= $8)`, instanceID, licenseID, data, attempt.FromVersion, attempt.TargetVersion, attempt.Success, attempt.Error, attempt.Timestamp)
	return result.RowsAffected() == 1, err
}

// CountAvailabilityViolations returns how many instances of a product tier
// reported an availability_violation (invariant 7) for a specific target
// version. The updates server uses this to refuse widening a release beyond the
// alpha ring: a version that has only reached alpha can only have been installed
// by alpha-ring nodes, so any violation for it is an alpha-ring violation.
func (r *InstanceRepository) CountAvailabilityViolations(ctx context.Context, productTier, version string) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM instances
 WHERE product_tier=$1 AND last_update_target_version=$2
 AND (last_heartbeat_data->'last_update_attempt'->>'availability_violation')='true'`,
		productTier, version).Scan(&count)
	return count, err
}
