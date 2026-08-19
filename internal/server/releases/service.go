package releases

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// parseVersion extracts major, minor, patch from a version string
// Handles formats like "1.0.0", "v1.0.0", "1.0", "1"
func parseVersion(version string) (major, minor, patch int, ok bool) {
	// Remove leading 'v' or 'V' if present
	version = strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")

	// Extract version numbers using regex
	re := regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 2 {
		return 0, 0, 0, false
	}

	major, _ = strconv.Atoi(matches[1])
	if len(matches) > 2 && matches[2] != "" {
		minor, _ = strconv.Atoi(matches[2])
	}
	if len(matches) > 3 && matches[3] != "" {
		patch, _ = strconv.Atoi(matches[3])
	}

	return major, minor, patch, true
}

// isNewerVersion returns true if newVersion is greater than currentVersion
func isNewerVersion(currentVersion, newVersion string) bool {
	if currentVersion == "" {
		return true // No current version means any version is newer
	}

	currMajor, currMinor, currPatch, currOk := parseVersion(currentVersion)
	newMajor, newMinor, newPatch, newOk := parseVersion(newVersion)

	if !currOk || !newOk {
		// If we can't parse versions, fall back to string comparison
		return newVersion > currentVersion
	}

	if newMajor != currMajor {
		return newMajor > currMajor
	}
	if newMinor != currMinor {
		return newMinor > currMinor
	}
	return newPatch > currPatch
}

// Service handles release business logic
type Service struct {
	repo       *Repository
	storage    storage.Storage
	signingKey ed25519.PrivateKey // nil disables signing
}

// NewService creates a new release service
func NewService(db *database.DB, store storage.Storage) *Service {
	return &Service{
		repo:    NewRepository(db),
		storage: store,
	}
}

// SetSigningKey enables release signing at publish time.
func (s *Service) SetSigningKey(key ed25519.PrivateKey) {
	s.signingKey = key
}

// SigningPublicKeyHex returns the hex public key, or empty if signing is disabled.
func (s *Service) SigningPublicKeyHex() string {
	if s.signingKey == nil {
		return ""
	}
	return signing.PublicKeyHex(s.signingKey)
}

// CreateReleaseRequest is the request to create a release
type CreateReleaseRequest struct {
	ProductName       string
	Version           string
	Channel           string
	ReleaseNotes      string
	MinUpdaterVersion string
	TargetGroups      []string
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

	var signature string
	if s.signingKey != nil {
		signature = signing.Sign(s.signingKey, req.ProductName, req.Version, checksum)
	}

	// Create release record
	release := &types.Release{
		ProductName:       req.ProductName,
		Version:           req.Version,
		Channel:           req.Channel,
		ArtifactPath:      artifactPath,
		ArtifactSize:      req.FileSize,
		Checksum:          checksum,
		Signature:         signature,
		ReleaseNotes:      req.ReleaseNotes,
		MinUpdaterVersion: req.MinUpdaterVersion,
		TargetGroups:      req.TargetGroups,
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

// findHighestVersion finds the release with the highest semantic version from a list
func findHighestVersion(releases []types.Release) *types.Release {
	if len(releases) == 0 {
		return nil
	}

	highest := &releases[0]
	for i := 1; i < len(releases); i++ {
		if isNewerVersion(highest.Version, releases[i].Version) {
			highest = &releases[i]
		}
	}
	return highest
}

// GetLatestRelease retrieves the highest version release for a product
func (s *Service) GetLatestRelease(ctx context.Context, product, channel, currentVersion string) (*types.ReleaseInfo, error) {
	// Fetch all releases and find the highest version
	releases, err := s.repo.GetAllByProductAndChannel(ctx, product, channel)
	if err != nil {
		return nil, err
	}

	release := findHighestVersion(releases)
	if release == nil {
		return nil, nil
	}

	// Only mark as update available if the server version is actually higher
	updateAvailable := isNewerVersion(currentVersion, release.Version)

	return releaseInfo(release, currentVersion, updateAvailable), nil
}

// GetLatestReleaseForGroup retrieves the highest version release for a product and target group
func (s *Service) GetLatestReleaseForGroup(ctx context.Context, product, channel, currentVersion, targetGroup string) (*types.ReleaseInfo, error) {
	// Fetch all releases for the group and find the highest version
	releases, err := s.repo.GetAllByProductChannelAndGroup(ctx, product, channel, targetGroup)
	if err != nil {
		return nil, err
	}

	release := findHighestVersion(releases)
	if release == nil {
		return nil, nil
	}

	// Only mark as update available if the server version is actually higher
	updateAvailable := isNewerVersion(currentVersion, release.Version)

	return releaseInfo(release, currentVersion, updateAvailable), nil
}

func releaseInfo(release *types.Release, currentVersion string, updateAvailable bool) *types.ReleaseInfo {
	return &types.ReleaseInfo{
		Product:         release.ProductName,
		CurrentVersion:  currentVersion,
		LatestVersion:   release.Version,
		UpdateAvailable: updateAvailable,
		Channel:         release.Channel,
		DownloadURL:     fmt.Sprintf("/api/v1/releases/%s/%s/download", release.ProductName, release.Version),
		Checksum:        release.Checksum,
		Signature:       release.Signature,
		Size:            release.ArtifactSize,
		ReleaseNotes:    release.ReleaseNotes,
		ReleasedAt:      release.ReleasedAt,
	}
}

// UpdateReleaseTargetGroups updates which groups can receive a release
func (s *Service) UpdateReleaseTargetGroups(ctx context.Context, id string, groups []string) error {
	return s.repo.UpdateTargetGroups(ctx, id, groups)
}

// UpdateRelease updates release notes and/or target groups
func (s *Service) UpdateRelease(ctx context.Context, id string, notes *string, groups []string) error {
	return s.repo.UpdateRelease(ctx, id, notes, groups)
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
