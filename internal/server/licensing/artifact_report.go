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
