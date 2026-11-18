package licensing

import "context"

// InstalledVersion is one version some device reports. Kind is "product"
// (Product is set), "updater" (the updater's own version), or "attempt" (a
// last update from/target version, whose product is not recorded).
type InstalledVersion struct {
	Kind    string
	Product string
	Version string
}

// InstalledVersions returns the device count and every distinct version the
// fleet reports, covering directly connected nodes and relay-reported ones.
func (r *InstanceRepository) InstalledVersions(ctx context.Context) (int, []InstalledVersion, error) {
	var devices int
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM instances WHERE deleted_at IS NULL`).Scan(&devices); err != nil {
		return 0, nil, err
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT DISTINCT 'product', p->>'name', p->>'version'
		FROM instances,
		     jsonb_array_elements(CASE WHEN jsonb_typeof(last_heartbeat_data->'products') = 'array'
		                               THEN last_heartbeat_data->'products' ELSE '[]'::jsonb END) p
		WHERE deleted_at IS NULL AND COALESCE(p->>'name', '') <> '' AND COALESCE(p->>'version', '') <> ''
		UNION
		SELECT DISTINCT 'updater', '', last_heartbeat_data->>'updater_version'
		FROM instances
		WHERE deleted_at IS NULL AND COALESCE(last_heartbeat_data->>'updater_version', '') <> ''
		UNION
		SELECT DISTINCT 'attempt', '', v
		FROM instances, LATERAL (VALUES (last_update_from_version), (last_update_target_version)) t(v)
		WHERE deleted_at IS NULL AND COALESCE(v, '') <> ''`)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	var out []InstalledVersion
	for rows.Next() {
		var v InstalledVersion
		if err := rows.Scan(&v.Kind, &v.Product, &v.Version); err != nil {
			return 0, nil, err
		}
		out = append(out, v)
	}
	return devices, out, rows.Err()
}
