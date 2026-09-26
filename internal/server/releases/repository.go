package releases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// ErrReleaseExists is returned when a product+version (per artifact kind) is
// already published. Published releases are immutable.
var ErrReleaseExists = errors.New("release already exists; existing releases are immutable")

const releaseColumns = `id, product_name, version, channel, manifest, artifact_path, artifact_size, checksum, signature, release_notes, min_updater_version, target_groups, released_at, created_at, seal_status, issuer, issuer_key_id, issuer_signature`

// Repository handles release database operations
type Repository struct {
	db *database.DB
}

// NewRepository creates a new release repository
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

func scanRelease(row pgx.Row) (*types.Release, error) {
	var release types.Release
	var manifestJSON []byte
	if err := row.Scan(
		&release.ID, &release.ProductName, &release.Version, &release.Channel, &manifestJSON,
		&release.ArtifactPath, &release.ArtifactSize, &release.Checksum, &release.Signature,
		&release.ReleaseNotes, &release.MinUpdaterVersion, &release.TargetGroups, &release.ReleasedAt, &release.CreatedAt,
		&release.SealStatus, &release.Issuer, &release.IssuerKeyID, &release.IssuerSignature); err != nil {
		return nil, err
	}
	if manifestJSON != nil {
		if err := json.Unmarshal(manifestJSON, &release.Manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
	}
	// Rows written before 1.16.2, or by a rolled-back 1.16.1 server, carry no
	// issuer.
	if release.Issuer == "" {
		release.Issuer = IssuerFor(release.ProductName)
	}
	return &release, nil
}

func (r *Repository) queryReleases(ctx context.Context, query string, args ...any) ([]types.Release, error) {
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query releases: %w", err)
	}
	defer rows.Close()

	var releases []types.Release
	for rows.Next() {
		release, err := scanRelease(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan release: %w", err)
		}
		releases = append(releases, *release)
	}
	return releases, rows.Err()
}

func (r *Repository) queryRelease(ctx context.Context, query string, args ...any) (*types.Release, error) {
	release, err := scanRelease(r.db.Pool.QueryRow(ctx, query, args...))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get release: %w", err)
	}
	return release, nil
}

// Create creates a new release. A duplicate product+version+kind returns
// ErrReleaseExists.
func (r *Repository) Create(ctx context.Context, release *types.Release) error {
	release.ID = uuid.New().String()
	release.CreatedAt = time.Now()
	if release.ReleasedAt.IsZero() {
		release.ReleasedAt = time.Now()
	}
	if release.SealStatus == "" {
		release.SealStatus = SealUnsealed
	}

	// Default target groups to all if not specified
	if len(release.TargetGroups) == 0 {
		release.TargetGroups = []string{"alpha", "beta", "stable", "production"}
	}

	manifestJSON, err := json.Marshal(release.Manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO releases (`+releaseColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, release.ID, release.ProductName, release.Version, release.Channel, manifestJSON,
		release.ArtifactPath, release.ArtifactSize, release.Checksum, release.Signature,
		release.ReleaseNotes, release.MinUpdaterVersion, release.TargetGroups, release.ReleasedAt, release.CreatedAt,
		release.SealStatus, release.Issuer, release.IssuerKeyID, release.IssuerSignature)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrReleaseExists
	}
	return err
}

// GetByProductVersion retrieves a release by product and version
func (r *Repository) GetByProductVersion(ctx context.Context, product, version string, kinds ...string) (*types.Release, error) {
	kind := ""
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	return r.queryRelease(ctx, `
		SELECT `+releaseColumns+`
		FROM releases
		WHERE product_name = $1 AND version = $2
          AND (COALESCE(manifest->>'artifact_kind','') = $3 OR COALESCE(manifest->>'artifact_kind','') = '')
        ORDER BY (COALESCE(manifest->>'artifact_kind','') = $3) DESC
        LIMIT 1
	`, product, version, kind)
}

// existsExact reports whether product+version is published with exactly this
// artifact kind ("" for legacy single-artifact releases).
func (r *Repository) existsExact(ctx context.Context, product, version, kind string) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM releases
		WHERE product_name = $1 AND version = $2 AND COALESCE(manifest->>'artifact_kind','') = $3)
	`, product, version, kind).Scan(&exists)
	return exists, err
}

// GetAllByProductAndChannel retrieves all releases for a product and channel
func (r *Repository) GetAllByProductAndChannel(ctx context.Context, product, channel string) ([]types.Release, error) {
	return r.queryReleases(ctx, `
		SELECT `+releaseColumns+`
		FROM releases
		WHERE product_name = $1 AND channel = $2
	`, product, channel)
}

// GetAllByProductChannelAndGroup retrieves all releases for a product, channel, and target group
func (r *Repository) GetAllByProductChannelAndGroup(ctx context.Context, product, channel, targetGroup string) ([]types.Release, error) {
	return r.queryReleases(ctx, `
		SELECT `+releaseColumns+`
		FROM releases
		WHERE product_name = $1 AND channel = $2 AND $3 = ANY(target_groups)
	`, product, channel, targetGroup)
}

// GetLatestByProduct retrieves the latest release for a product and channel (by released_at)
// Note: For proper semantic version comparison, use GetAllByProductAndChannel and compare in service layer
func (r *Repository) GetLatestByProduct(ctx context.Context, product, channel string) (*types.Release, error) {
	return r.queryRelease(ctx, `
		SELECT `+releaseColumns+`
		FROM releases
		WHERE product_name = $1 AND channel = $2
		ORDER BY released_at DESC
		LIMIT 1
	`, product, channel)
}

// GetLatestByProductForGroup retrieves the latest release for a product, channel, and target group
// Note: For proper semantic version comparison, use GetAllByProductChannelAndGroup and compare in service layer
func (r *Repository) GetLatestByProductForGroup(ctx context.Context, product, channel, targetGroup string) (*types.Release, error) {
	return r.queryRelease(ctx, `
		SELECT `+releaseColumns+`
		FROM releases
		WHERE product_name = $1 AND channel = $2 AND $3 = ANY(target_groups)
		ORDER BY released_at DESC
		LIMIT 1
	`, product, channel, targetGroup)
}

// List retrieves all releases
func (r *Repository) List(ctx context.Context) ([]types.Release, error) {
	return r.queryReleases(ctx, `
		SELECT `+releaseColumns+`
		FROM releases
		ORDER BY released_at DESC
	`)
}

// ListByProduct retrieves releases for a product
func (r *Repository) ListByProduct(ctx context.Context, product string) ([]types.Release, error) {
	return r.queryReleases(ctx, `
		SELECT `+releaseColumns+`
		FROM releases
		WHERE product_name = $1
		ORDER BY released_at DESC
	`, product)
}

// UpdateTargetGroups updates the target groups for a release
func (r *Repository) UpdateTargetGroups(ctx context.Context, id string, groups []string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE releases
		SET target_groups = $2
		WHERE id = $1
	`, id, groups)

	return err
}

// UpdateReleaseNotes updates the release notes for a release
func (r *Repository) UpdateReleaseNotes(ctx context.Context, id string, notes string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE releases
		SET release_notes = $2
		WHERE id = $1
	`, id, notes)

	return err
}

// UpdateRelease updates release notes and target groups
func (r *Repository) UpdateRelease(ctx context.Context, id string, notes *string, groups []string) error {
	if notes != nil && len(groups) > 0 {
		_, err := r.db.Pool.Exec(ctx, `
			UPDATE releases
			SET release_notes = $2, target_groups = $3
			WHERE id = $1
		`, id, *notes, groups)
		return err
	} else if notes != nil {
		return r.UpdateReleaseNotes(ctx, id, *notes)
	} else if len(groups) > 0 {
		return r.UpdateTargetGroups(ctx, id, groups)
	}
	return nil
}

// Delete deletes a release
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM releases WHERE id = $1`, id)
	return err
}
