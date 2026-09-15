package licensing

import (
	"context"
	"time"
)

// Only offline leaf records with no activity for an hour are candidates.
// Explicit update holds remain protected. The same predicate is rechecked at removal.
const inactiveChildPredicate = `parent_instance_id = $1 AND status = 'offline'
 AND auto_update_enabled = TRUE
 AND COALESCE(last_heartbeat, created_at) < NOW() - INTERVAL '1 hour'
 AND NOT EXISTS (SELECT 1 FROM instances child WHERE child.parent_instance_id = instances.instance_id AND child.status <> 'decommissioned')`

type InactiveChild struct {
	ID            string     `json:"id"`
	InstanceID    string     `json:"instance_id"`
	LastHeartbeat *time.Time `json:"last_heartbeat"`
}

func (r *InstanceRepository) InactiveChildren(ctx context.Context, parent string) ([]InactiveChild, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT id,instance_id,last_heartbeat FROM instances WHERE `+inactiveChildPredicate+` ORDER BY instance_id LIMIT 1000`, parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InactiveChild{}
	for rows.Next() {
		var item InactiveChild
		if err := rows.Scan(&item.ID, &item.InstanceID, &item.LastHeartbeat); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Retain a tombstone so an old cached relay report cannot recreate the entry.
// A genuine later heartbeat may re-enroll it, as for normal decommissioning.
func (r *InstanceRepository) CleanupChildren(ctx context.Context, parent string, ids []string) (int64, error) {
	result, err := r.db.Pool.Exec(ctx, `UPDATE instances SET status='decommissioned',updated_at=NOW() WHERE `+inactiveChildPredicate+` AND id = ANY($2::uuid[])`, parent, ids)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
