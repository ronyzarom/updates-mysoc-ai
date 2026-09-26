package releases

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// parseVersion extracts up to four numeric components from a version string
// (MAJOR.MINOR.PATCH.BUILD — the platform's own convention, and SWF's).
// Handles formats like "1.0.0.1", "v1.0.0", "1.0", "1". Missing components
// compare as zero.
func parseVersion(version string) (parts [4]int, ok bool) {
	// Remove leading 'v' or 'V' if present
	version = strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V")

	re := regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:\.(\d+))?`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 2 {
		return parts, false
	}
	for i := 0; i < 4; i++ {
		if len(matches) > i+1 && matches[i+1] != "" {
			parts[i], _ = strconv.Atoi(matches[i+1])
		}
	}
	return parts, true
}

// isNewerVersion returns true if newVersion is greater than currentVersion.
// All four components participate: a rebuild (x.y.z.1 -> x.y.z.2) is an
// update, both for our own artifacts and for four-part product versions.
func isNewerVersion(currentVersion, newVersion string) bool {
	if currentVersion == "" {
		return true // No current version means any version is newer
	}

	curr, currOk := parseVersion(currentVersion)
	next, newOk := parseVersion(newVersion)

	if !currOk || !newOk {
		// If we can't parse versions, fall back to string comparison
		return newVersion > currentVersion
	}

	for i := 0; i < 4; i++ {
		if next[i] != curr[i] {
			return next[i] > curr[i]
		}
	}
	return false
}

// Service handles release business logic
type Service struct {
	repo       *Repository
	keys       *KeyRepository
	storage    storage.Storage
	signingKey ed25519.PrivateKey // nil disables signing
}

// NewService creates a new release service
func NewService(db *database.DB, store storage.Storage) *Service {
	return &Service{
		repo:    NewRepository(db),
		keys:    NewKeyRepository(db),
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

// SigningKeyID returns the key id of the server's release key, or empty.
func (s *Service) SigningKeyID() string {
	if s.signingKey == nil {
		return ""
	}
	return KeyID(s.signingKey.Public().(ed25519.PublicKey))
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
	ArtifactKind      string
	// Optional issuer seal over the canonical release message.
	IssuerSignature string
	IssuerKeyID     string
}

// CreateRelease creates a new release. An existing product+version is never
// overwritten (ErrReleaseExists). The issuer seal is checked and recorded but
// never causes a rejection.
func (s *Service) CreateRelease(ctx context.Context, req CreateReleaseRequest) (*types.Release, error) {
	if req.ArtifactKind == "update" {
		return nil, fmt.Errorf("update artifacts require artifact_metadata or paired artifact_variants")
	}
	exists, err := s.repo.existsExact(ctx, req.ProductName, req.Version, req.ArtifactKind)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing release: %w", err)
	}
	if exists {
		return nil, ErrReleaseExists
	}

	// Stage under a unique name: only the upload that wins the database insert
	// moves its bytes into place, so a losing concurrent upload cannot replace
	// the published artifact.
	hasher := sha256.New()
	staged := ".staging-" + uuid.NewString()
	if _, err := s.storage.Save(req.ProductName, req.Version, staged, io.TeeReader(req.File, hasher)); err != nil {
		return nil, fmt.Errorf("failed to save artifact: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.storage.Delete(req.ProductName, req.Version, staged)
		}
	}()

	checksum := hex.EncodeToString(hasher.Sum(nil))

	activeKeys, err := s.keys.Active(ctx)
	if err != nil {
		return nil, err
	}
	seal := EvaluateSeal(activeKeys, req.ProductName, req.Version, checksum, req.IssuerSignature, req.IssuerKeyID)
	logSeal(req.ProductName, req.Version, seal)

	// The fleet verifies release.Signature. A verified seal is that signature;
	// otherwise the server keeps signing during the transition.
	var signature string
	if seal.Status == SealSealed {
		signature = seal.Signature
	} else if s.signingKey != nil {
		signature = signing.Sign(s.signingKey, req.ProductName, req.Version, checksum)
	}

	release := &types.Release{
		ProductName:       req.ProductName,
		Version:           req.Version,
		Channel:           req.Channel,
		ArtifactPath:      s.storage.GetPath(req.ProductName, req.Version, req.Filename),
		ArtifactSize:      req.FileSize,
		Checksum:          checksum,
		Signature:         signature,
		ReleaseNotes:      req.ReleaseNotes,
		MinUpdaterVersion: req.MinUpdaterVersion,
		TargetGroups:      req.TargetGroups,
		SealStatus:        seal.Status,
		Issuer:            seal.Issuer,
		IssuerKeyID:       seal.KeyID,
		IssuerSignature:   seal.Signature,
		Manifest: types.Manifest{
			Product: req.ProductName,
			Version: req.Version,
			Channel: req.Channel,
			Artifacts: []types.Artifact{
				{
					Name:     req.Filename,
					Size:     req.FileSize,
					Checksum: checksum,
					Kind:     req.ArtifactKind,
				},
			},
		},
	}

	if err := s.repo.Create(ctx, release); err != nil {
		if errors.Is(err, ErrReleaseExists) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to create release: %w", err)
	}
	if _, err := s.storage.Rename(req.ProductName, req.Version, staged, req.Filename); err != nil {
		_ = s.repo.Delete(ctx, release.ID)
		return nil, fmt.Errorf("failed to publish artifact: %w", err)
	}
	committed = true

	return release, nil
}

func logSeal(product, version string, seal SealResult) {
	switch seal.Status {
	case SealInvalid:
		log.Printf("ALERT issuer-seal: INVALID seal on %s %s (issuer=%s key_id=%q); accepted as unsealed and server-signed", product, version, seal.Issuer, seal.KeyID)
	case SealUnknownKey:
		log.Printf("WARNING issuer-seal: unknown key on %s %s (issuer=%s key_id=%q); accepted as unsealed and server-signed", product, version, seal.Issuer, seal.KeyID)
	default:
		log.Printf("issuer-seal: %s %s %s (issuer=%s key_id=%q)", product, version, seal.Status, seal.Issuer, seal.KeyID)
	}
}

// ListTrustedKeys returns the trusted key registry.
func (s *Service) ListTrustedKeys(ctx context.Context) ([]TrustedKey, error) {
	return s.keys.List(ctx)
}

// AddTrustedKey registers a key issuers may seal with.
func (s *Service) AddTrustedKey(ctx context.Context, publicKeyHex, issuer, label, createdBy string) (*TrustedKey, error) {
	return s.keys.Add(ctx, publicKeyHex, issuer, label, createdBy)
}

// RetireTrustedKey stops accepting seals from a key.
func (s *Service) RetireTrustedKey(ctx context.Context, id string) (*TrustedKey, error) {
	return s.keys.Retire(ctx, id)
}

// EnsureSigningKeyTrusted seeds the registry with the server's release key.
func (s *Service) EnsureSigningKeyTrusted(ctx context.Context) (bool, error) {
	if s.signingKey == nil {
		return false, nil
	}
	return s.keys.EnsureKey(ctx, s.signingKey.Public().(ed25519.PublicKey), "server release key")
}

// GetRelease retrieves a release by product and version
func (s *Service) GetRelease(ctx context.Context, product, version string, kinds ...string) (*types.Release, error) {
	return s.repo.GetByProductVersion(ctx, product, version, kinds...)
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

	compatible := releases[:0]
	for _, r := range releases {
		if r.Manifest.ArtifactKind == "" {
			compatible = append(compatible, r)
		}
	}
	release := findHighestVersion(compatible)
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

	compatible := releases[:0]
	for _, r := range releases {
		if r.Manifest.ArtifactKind == "" {
			compatible = append(compatible, r)
		}
	}
	release := findHighestVersion(compatible)
	if release == nil {
		return nil, nil
	}

	// Only mark as update available if the server version is actually higher
	updateAvailable := isNewerVersion(currentVersion, release.Version)

	return releaseInfo(release, currentVersion, updateAvailable), nil
}

// HighestReleaseForGroup returns the highest-version release for a product,
// channel, and target group (nil when none) without comparing against a
// caller version. This is the DB-touching part of the update check and is
// safe to memoize per (product, channel, group) — the version comparison is
// applied separately by ReleaseInfoFor.
func (s *Service) HighestReleaseForGroup(ctx context.Context, product, channel, targetGroup string) (*types.Release, error) {
	all, err := s.repo.GetAllByProductChannelAndGroup(ctx, product, channel, targetGroup)
	if err != nil {
		return nil, err
	}
	legacy := all[:0]
	for _, r := range all {
		if r.Manifest.ArtifactKind == "" {
			legacy = append(legacy, r)
		}
	}
	return findHighestVersion(legacy), nil
}

// ReleaseInfoFor builds the check response for a (cached) release against the
// caller's current version. Pure and cheap — no I/O.
func (s *Service) ReleaseInfoFor(release *types.Release, currentVersion string) *types.ReleaseInfo {
	if release == nil {
		return nil
	}
	return releaseInfo(release, currentVersion, isNewerVersion(currentVersion, release.Version))
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
