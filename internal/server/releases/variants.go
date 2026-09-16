package releases

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"github.com/google/uuid"
)

var ErrPrerequisites = errors.New("prerequisite validation failed: no compatible bootstrap is published")

type VariantUpload struct {
	Artifact types.Artifact
	File     io.Reader
}

// CreateDualRelease publishes a signed pair or one independent artifact.
// Files become visible only after all bytes have been validated and signed.
// An independent artifact owns its release identity and target groups.
func (s *Service) CreateDualRelease(ctx context.Context, req CreateReleaseRequest, uploads []VariantUpload) (*types.Release, error) {
	if s.signingKey == nil {
		return nil, fmt.Errorf("dual artifacts require release signing")
	}
	if len(req.TargetGroups) == 0 {
		return nil, fmt.Errorf("explicit target groups required")
	}
	if err := req.UpdaterRequirements.Validate(); err != nil {
		return nil, err
	}
	variants := make([]types.Artifact, len(uploads))
	for i, u := range uploads {
		variants[i] = u.Artifact
		if u.File == nil {
			return nil, fmt.Errorf("variant file required")
		}
	}
	validate := artifactprotocol.ValidatePair
	if req.ArtifactKind != "" {
		validate = artifactprotocol.ValidateArtifacts
		if len(variants) != 1 || variants[0].Kind != req.ArtifactKind {
			return nil, fmt.Errorf("one matching independent artifact required")
		}
	}
	if err := validate(req.ProductName, req.Version, variants); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetByProductVersion(ctx, req.ProductName, req.Version, req.ArtifactKind)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("release already exists; existing releases are immutable")
	}
	saved := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, name := range saved {
				_ = s.storage.Delete(req.ProductName, req.Version, name)
			}
		}
	}()
	var primaryPath string
	for i, u := range uploads {
		// Unique storage names prevent a concurrent publication from overwriting bytes.
		variants[i].Name = uuid.NewString() + "-" + u.Artifact.Name
		h := sha256.New()
		counter := &countReader{Reader: io.TeeReader(u.File, h)}
		path, err := s.storage.Save(req.ProductName, req.Version, variants[i].Name, counter)
		if err != nil {
			return nil, err
		}
		saved = append(saved, variants[i].Name)
		if counter.n != u.Artifact.Size || hex.EncodeToString(h.Sum(nil)) != u.Artifact.Checksum {
			return nil, fmt.Errorf("%s artifact size/checksum mismatch", u.Artifact.Kind)
		}
		variants[i].URL = fmt.Sprintf("/api/v1/releases/%s/%s/download?artifact_kind=%s", req.ProductName, req.Version, u.Artifact.Kind)
		artifactprotocol.Sign(s.signingKey, &variants[i])
		if u.Artifact.Kind == "bootstrap" || req.ArtifactKind != "" {
			primaryPath = path
		}
	}
	var bootstrap types.Artifact
	for _, a := range variants {
		if a.Kind == "bootstrap" || req.ArtifactKind != "" {
			bootstrap = a
		}
	}
	release := &types.Release{ProductName: req.ProductName, Version: req.Version, Channel: req.Channel, ReleaseNotes: req.ReleaseNotes, TargetGroups: req.TargetGroups, ArtifactPath: primaryPath, ArtifactSize: bootstrap.Size, Checksum: bootstrap.Checksum, Signature: bootstrap.Signature, Manifest: types.Manifest{UpdaterRequirements: req.UpdaterRequirements, ArtifactKind: req.ArtifactKind, Product: req.ProductName, Version: req.Version, Channel: req.Channel, ArtifactVariants: variants, Artifacts: []types.Artifact{{Name: bootstrap.Name, Arch: bootstrap.Arch, Size: bootstrap.Size, Checksum: bootstrap.Checksum}}}}
	if err := s.repo.Create(ctx, release); err != nil {
		return nil, err
	}
	committed = true
	return release, nil
}

type countReader struct {
	io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.n += int64(n)
	return n, err
}

// DualOffer leaves the legacy offer untouched if metadata is absent, invalid,
// or for another architecture. Clients never infer dual support from filenames.
func (s *Service) DualOffer(release *types.Release, info *types.ReleaseInfo, arch string, evidence artifactprotocol.Evidence) bool {
	if release == nil || info == nil || s.signingKey == nil {
		return false
	}
	variants := release.Manifest.ArtifactVariants
	if artifactprotocol.ValidatePair(release.ProductName, release.Version, variants) != nil || variants[0].Arch != arch {
		return false
	}
	for _, a := range variants {
		if artifactprotocol.Verify(s.signingKey.Public().(ed25519.PublicKey), a) != nil {
			return false
		}
	}
	if evidence.InstalledVersion != info.CurrentVersion {
		evidence.Lifecycle = "unknown"
	}
	selected, status := artifactprotocol.Select(variants, evidence)
	info.Artifacts = variants
	info.SelectedArtifactKind = selected.Kind
	info.DependencyValidation = status
	info.RequiredDependencies = selected.RequiredDependencies
	info.DownloadURL = selected.URL
	info.Checksum = selected.Checksum
	info.Signature = selected.Signature
	info.Size = selected.Size
	return true
}

// IndependentOffer selects only from releases matching the caller's product/channel/ring.
// Independent artifacts are invisible to clients without the explicit capability.
func (s *Service) IndependentOffer(ctx context.Context, product, channel, group, current, arch string, evidence artifactprotocol.Evidence) (*types.Release, *types.ReleaseInfo, error) {
	all, err := s.repo.GetAllByProductChannelAndGroup(ctx, product, channel, group)
	if err != nil {
		return nil, nil, err
	}
	var updates, bootstraps []types.Release
	blocked := false
	for _, r := range all {
		if r.Manifest.ArtifactKind == "" || !isNewerVersion(current, r.Version) {
			continue
		}
		aa := r.Manifest.ArtifactVariants
		if len(aa) != 1 || artifactprotocol.ValidateArtifacts(product, r.Version, aa) != nil || aa[0].Kind != r.Manifest.ArtifactKind || aa[0].Arch != arch || s.signingKey == nil {
			continue
		}
		a := aa[0]
		if artifactprotocol.Verify(s.signingKey.Public().(ed25519.PublicKey), a) != nil {
			continue
		}
		if a.Kind == "bootstrap" {
			bootstraps = append(bootstraps, r)
		} else if evidence.Lifecycle == "installed" && evidence.InstalledVersion == current && current != "" && evidence.Dependencies != nil && artifactprotocol.DependencyStatus(a.RequiredDependencies, evidence.Dependencies) == "complete" {
			updates = append(updates, r)
		} else {
			blocked = true
		}
	}
	selected := findHighestVersion(updates)
	if selected == nil {
		selected = findHighestVersion(bootstraps)
	}
	if selected == nil {
		if blocked {
			return nil, nil, ErrPrerequisites
		}
		return nil, nil, nil
	}
	a := selected.Manifest.ArtifactVariants[0]
	info := s.ReleaseInfoFor(selected, current)
	info.Artifacts = selected.Manifest.ArtifactVariants
	info.SelectedArtifactKind = a.Kind
	info.DependencyValidation = "missing"
	if evidence.Lifecycle == "installed" && evidence.InstalledVersion == current && evidence.Dependencies != nil {
		info.DependencyValidation = artifactprotocol.DependencyStatus(a.RequiredDependencies, evidence.Dependencies)
	}
	info.RequiredDependencies = a.RequiredDependencies
	info.DownloadURL = a.URL
	info.Checksum = a.Checksum
	info.Signature = a.Signature
	info.Size = a.Size
	return selected, info, nil
}
