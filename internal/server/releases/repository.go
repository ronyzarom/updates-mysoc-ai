package releases

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

// Repository handles release database operations
type Repository struct {
	db *database.DB
}

// NewRepository creates a new release repository
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new release
func (r *Repository) Create(ctx context.Context, release *types.Release) error {
	release.ID = uuid.New().String()
	release.CreatedAt = time.Now()
	if release.ReleasedAt.IsZero() {
		release.ReleasedAt = time.Now()
	}

	manifestJSON, err := json.Marshal(release.Manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO releases (id, product_name, version, channel, manifest, artifact_path, artifact_size, checksum, signature, release_notes, min_updater_version, released_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, release.ID, release.ProductName, release.Version, release.Channel, manifestJSON,
		release.ArtifactPath, release.ArtifactSize, release.Checksum, release.Signature,
		release.ReleaseNotes, release.MinUpdaterVersion, release.ReleasedAt, release.CreatedAt)

	return err
}

// GetByProductVersion retrieves a release by product and version
func (r *Repository) GetByProductVersion(ctx context.Context, product, version string) (*types.Release, error) {
	var release types.Release
	var manifestJSON []byte

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, product_name, version, channel, manifest, artifact_path, artifact_size, checksum, signature, release_notes, min_updater_version, released_at, created_at
		FROM releases
		WHERE product_name = $1 AND version = $2
	`, product, version).Scan(
		&release.ID, &release.ProductName, &release.Version, &release.Channel, &manifestJSON,
		&release.ArtifactPath, &release.ArtifactSize, &release.Checksum, &release.Signature,
		&release.ReleaseNotes, &release.MinUpdaterVersion, &release.ReleasedAt, &release.CreatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	if manifestJSON != nil {
		if err := json.Unmarshal(manifestJSON, &release.Manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
	}

	return &release, nil
}

// GetLatestByProduct retrieves the latest release for a product and channel
func (r *Repository) GetLatestByProduct(ctx context.Context, product, channel string) (*types.Release, error) {
	var release types.Release
	var manifestJSON []byte

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, product_name, version, channel, manifest, artifact_path, artifact_size, checksum, signature, release_notes, min_updater_version, released_at, created_at
		FROM releases
		WHERE product_name = $1 AND channel = $2
		ORDER BY released_at DESC
		LIMIT 1
	`, product, channel).Scan(
		&release.ID, &release.ProductName, &release.Version, &release.Channel, &manifestJSON,
		&release.ArtifactPath, &release.ArtifactSize, &release.Checksum, &release.Signature,
		&release.ReleaseNotes, &release.MinUpdaterVersion, &release.ReleasedAt, &release.CreatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	if manifestJSON != nil {
		if err := json.Unmarshal(manifestJSON, &release.Manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
	}

	return &release, nil
}

// List retrieves all releases
func (r *Repository) List(ctx context.Context) ([]types.Release, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, product_name, version, channel, manifest, artifact_path, artifact_size, checksum, signature, release_notes, min_updater_version, released_at, created_at
		FROM releases
		ORDER BY released_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}
	defer rows.Close()

	var releases []types.Release
	for rows.Next() {
		var release types.Release
		var manifestJSON []byte

		err := rows.Scan(
			&release.ID, &release.ProductName, &release.Version, &release.Channel, &manifestJSON,
			&release.ArtifactPath, &release.ArtifactSize, &release.Checksum, &release.Signature,
			&release.ReleaseNotes, &release.MinUpdaterVersion, &release.ReleasedAt, &release.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan release: %w", err)
		}

		if manifestJSON != nil {
			if err := json.Unmarshal(manifestJSON, &release.Manifest); err != nil {
				return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
			}
		}

		releases = append(releases, release)
	}

	return releases, nil
}

// ListByProduct retrieves releases for a product
func (r *Repository) ListByProduct(ctx context.Context, product string) ([]types.Release, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, product_name, version, channel, manifest, artifact_path, artifact_size, checksum, signature, release_notes, min_updater_version, released_at, created_at
		FROM releases
		WHERE product_name = $1
		ORDER BY released_at DESC
	`, product)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}
	defer rows.Close()

	var releases []types.Release
	for rows.Next() {
		var release types.Release
		var manifestJSON []byte

		err := rows.Scan(
			&release.ID, &release.ProductName, &release.Version, &release.Channel, &manifestJSON,
			&release.ArtifactPath, &release.ArtifactSize, &release.Checksum, &release.Signature,
			&release.ReleaseNotes, &release.MinUpdaterVersion, &release.ReleasedAt, &release.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan release: %w", err)
		}

		if manifestJSON != nil {
			if err := json.Unmarshal(manifestJSON, &release.Manifest); err != nil {
				return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
			}
		}

		releases = append(releases, release)
	}

	return releases, nil
}

// Delete deletes a release
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM releases WHERE id = $1`, id)
	return err
}

