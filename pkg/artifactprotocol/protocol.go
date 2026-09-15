// Package artifactprotocol defines the additive, signed dual-artifact contract.
// It never resolves, downloads, or installs dependencies.
package artifactprotocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

const Version = "dual-artifact-v1"
const IndependentCapability = "independent-artifacts-v1"

const Capability = "verified-dependencies-v1"

var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Evidence comes from a fresh local product-owned verifier, never directory existence.
type Evidence struct {
	Lifecycle        string             `json:"lifecycle"`
	InstalledVersion string             `json:"installed_version"`
	Dependencies     []types.Dependency `json:"cached_dependencies"`
}

func Advertised(version string, capabilities []string) bool {
	return version == Version && slices.Contains(capabilities, Capability)
}

func validateDependencies(deps []types.Dependency) error {
	seen := map[string]bool{}
	for _, d := range deps {
		if d.Reference == "" || seen[d.Reference] || !strings.HasPrefix(d.Digest, "sha256:") || !digest.MatchString(strings.TrimPrefix(d.Digest, "sha256:")) {
			return fmt.Errorf("invalid or duplicate dependency %q", d.Reference)
		}
		seen[d.Reference] = true
		caps := map[string]bool{}
		for _, c := range d.Capabilities {
			if strings.TrimSpace(c) == "" || caps[c] {
				return fmt.Errorf("invalid dependency capability")
			}
			caps[c] = true
		}
	}
	return nil
}

func ValidatePair(product, version string, variants []types.Artifact) error {
	if len(variants) != 2 {
		return fmt.Errorf("exactly one bootstrap and one update artifact required")
	}
	return ValidateArtifacts(product, version, variants)
}

// ValidateArtifacts accepts one independently published artifact or an existing pair.
func ValidateArtifacts(product, version string, variants []types.Artifact) error {
	if product != "mysoc" && product != "siemcore" && product != "swf" {
		return fmt.Errorf("unsupported dual-artifact product %q", product)
	}
	if len(variants) < 1 || len(variants) > 2 {
		return fmt.Errorf("one independent artifact or one bootstrap/update pair required")
	}
	kinds := map[string]bool{}
	for _, a := range variants {
		if len(a.CriticalFields) > 0 {
			return fmt.Errorf("unsupported critical artifact fields")
		}
		if (a.Kind != "bootstrap" && a.Kind != "update") || kinds[a.Kind] {
			return fmt.Errorf("invalid or duplicate artifact kind")
		}
		kinds[a.Kind] = true
		if a.Product != product || a.Version != version || a.Arch == "" || a.Arch != variants[0].Arch || a.SourceCommit != variants[0].SourceCommit || !commit.MatchString(a.SourceCommit) {
			return fmt.Errorf("variant identity mismatch or missing identity")
		}
		if a.Size <= 0 || !digest.MatchString(a.Checksum) || a.Name == "" || strings.ContainsAny(a.Name, "/\\") || a.Name == "." || a.Name == ".." {
			return fmt.Errorf("invalid variant file metadata")
		}
		if err := validateDependencies(a.RequiredDependencies); err != nil {
			return err
		}
	}
	if len(variants) == 2 && variants[0].Name == variants[1].Name {
		return fmt.Errorf("variant filenames must differ")
	}
	return nil
}

// MetadataDigest binds all execution-relevant fields. URL and signatures are
// transport fields and are excluded so relays can rewrite origin-relative URLs.
func CanonicalMetadata(a types.Artifact) []byte {
	a.URL = ""
	a.Signature = ""
	a.MetadataSignature = ""
	a.RequiredDependencies = append([]types.Dependency(nil), a.RequiredDependencies...)
	for i := range a.RequiredDependencies {
		a.RequiredDependencies[i].Capabilities = append([]string(nil), a.RequiredDependencies[i].Capabilities...)
		slices.Sort(a.RequiredDependencies[i].Capabilities)
	}
	slices.SortFunc(a.RequiredDependencies, func(a, b types.Dependency) int { return strings.Compare(a.Reference, b.Reference) })
	data, _ := json.Marshal(struct {
		Protocol string         `json:"protocol"`
		Artifact types.Artifact `json:"artifact"`
	}{Version, a})
	return data
}
func MetadataDigest(a types.Artifact) string {
	sum := sha256.Sum256(CanonicalMetadata(a))
	return hex.EncodeToString(sum[:])
}
func Sign(key ed25519.PrivateKey, a *types.Artifact) {
	a.Signature = signing.Sign(key, a.Product, a.Version, a.Checksum)
	a.MetadataSignature = signing.Sign(key, a.Product, a.Version, MetadataDigest(*a))
}
func Verify(key ed25519.PublicKey, a types.Artifact) error {
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("dual artifacts require a pinned signing key")
	}
	if err := signing.Verify(key, a.Product, a.Version, a.Checksum, a.Signature); err != nil {
		return err
	}
	return signing.Verify(key, a.Product, a.Version, MetadataDigest(a), a.MetadataSignature)
}

func DependencyStatus(required, cached []types.Dependency) string {
	if err := validateDependencies(cached); err != nil {
		return "mismatch"
	}
	for _, need := range required {
		found := false
		for _, have := range cached {
			if have.Reference != need.Reference {
				continue
			}
			found = true
			if have.Digest != need.Digest {
				return "mismatch"
			}
			for _, cap := range need.Capabilities {
				if !slices.Contains(have.Capabilities, cap) {
					return "mismatch"
				}
			}
		}
		if !found {
			return "missing"
		}
	}
	return "complete"
}

// Select falls back to bootstrap for any incomplete installation or evidence.
// Callers validate the signed pair before invoking selection.
func Select(variants []types.Artifact, evidence Evidence) (types.Artifact, string) {
	var bootstrap, update types.Artifact
	for _, a := range variants {
		if a.Kind == "bootstrap" {
			bootstrap = a
		}
		if a.Kind == "update" {
			update = a
		}
	}
	status := DependencyStatus(update.RequiredDependencies, evidence.Dependencies)
	if evidence.Lifecycle != "installed" || evidence.InstalledVersion == "" || evidence.Dependencies == nil {
		return bootstrap, "missing"
	}
	if status != "complete" {
		return bootstrap, status
	}
	return update, "complete"
}
