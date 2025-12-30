package releases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Service handles release business logic
type Service struct {
	repo    *Repository
	storage storage.Storage
}

// NewService creates a new release service
func NewService(db *database.DB, store storage.Storage) *Service {
	return &Service{
		repo:    NewRepository(db),
		storage: store,
	}
}

// CreateReleaseRequest is the request to create a release
type CreateReleaseRequest struct {
	ProductName       string
	Version           string
	Channel           string
	ReleaseNotes      string
	MinUpdaterVersion string
	Filename          string
	FileSize          int64
	File              io.Reader
}

// CreateRelease creates a new release
func (s *Service) CreateRelease(ctx context.Context, req CreateReleaseRequest) (*types.Release, error) {
	// Calculate checksum while saving
	hasher := sha256.New()
	teeReader := io.TeeReader(req.File, hasher)

	// Save artifact to storage
	artifactPath, err := s.storage.Save(req.ProductName, req.Version, req.Filename, teeReader)
	if err != nil {
		return nil, fmt.Errorf("failed to save artifact: %w", err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Create release record
	release := &types.Release{
		ProductName:       req.ProductName,
		Version:           req.Version,
		Channel:           req.Channel,
		ArtifactPath:      artifactPath,
		ArtifactSize:      req.FileSize,
		Checksum:          checksum,
		ReleaseNotes:      req.ReleaseNotes,
		MinUpdaterVersion: req.MinUpdaterVersion,
		Manifest: types.Manifest{
			Product: req.ProductName,
			Version: req.Version,
			Channel: req.Channel,
			Artifacts: []types.Artifact{
				{
					Name:     req.Filename,
					Size:     req.FileSize,
					Checksum: checksum,
				},
			},
		},
	}

	if err := s.repo.Create(ctx, release); err != nil {
		// Try to clean up the artifact
		s.storage.Delete(req.ProductName, req.Version, req.Filename)
		return nil, fmt.Errorf("failed to create release: %w", err)
	}

	return release, nil
}

// GetRelease retrieves a release by product and version
func (s *Service) GetRelease(ctx context.Context, product, version string) (*types.Release, error) {
	return s.repo.GetByProductVersion(ctx, product, version)
}

// GetLatestRelease retrieves the latest release for a product
func (s *Service) GetLatestRelease(ctx context.Context, product, channel, currentVersion string) (*types.ReleaseInfo, error) {
	release, err := s.repo.GetLatestByProduct(ctx, product, channel)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, nil
	}

	updateAvailable := currentVersion == "" || currentVersion != release.Version

	return &types.ReleaseInfo{
		Product:         release.ProductName,
		CurrentVersion:  currentVersion,
		LatestVersion:   release.Version,
		UpdateAvailable: updateAvailable,
		Channel:         release.Channel,
		DownloadURL:     fmt.Sprintf("/api/v1/releases/%s/%s/download", release.ProductName, release.Version),
		Checksum:        release.Checksum,
		Size:            release.ArtifactSize,
		ReleaseNotes:    release.ReleaseNotes,
		ReleasedAt:      release.ReleasedAt,
	}, nil
}

// ListReleases retrieves all releases
func (s *Service) ListReleases(ctx context.Context) ([]types.Release, error) {
	return s.repo.List(ctx)
}

// ListProductReleases retrieves releases for a product
func (s *Service) ListProductReleases(ctx context.Context, product string) ([]types.Release, error) {
	return s.repo.ListByProduct(ctx, product)
}

// DeleteRelease deletes a release
func (s *Service) DeleteRelease(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

