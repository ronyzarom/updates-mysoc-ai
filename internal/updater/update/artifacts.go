package update

import (
	"fmt"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// SelectArtifact applies the dual-artifact policy. It is pure so server and
// updater tests can exercise the same truth table without touching a host.
func SelectArtifact(info *types.ReleaseInfo, lifecycle string, cached []types.Dependency) (*types.Artifact, string, error) {
	if info == nil || len(info.Artifacts) == 0 {
		return nil, "legacy", nil
	}
	var boot, upd *types.Artifact
	for i := range info.Artifacts {
		a := &info.Artifacts[i]
		switch a.Kind {
		case "bootstrap":
			if boot != nil {
				return nil, "", fmt.Errorf("ambiguous bootstrap artifacts")
			}
			boot = a
		case "update":
			if upd != nil {
				return nil, "", fmt.Errorf("ambiguous update artifacts")
			}
			upd = a
		default:
			return nil, "", fmt.Errorf("unknown artifact kind %q", a.Kind)
		}
	}
	if boot == nil {
		return nil, "", fmt.Errorf("bootstrap artifact is required")
	}
	if lifecycle != "installed" {
		return boot, "missing", nil
	}
	if upd == nil {
		return boot, "missing", nil
	}
	for _, need := range upd.RequiredDependencies {
		found := false
		for _, have := range cached {
			if have.Reference == need.Reference && have.Digest == need.Digest {
				found = true
				break
			}
		}
		if !found {
			return boot, "missing", nil
		}
	}
	return upd, "complete", nil
}

func artifactURL(info *types.ReleaseInfo, artifact *types.Artifact) string {
	if artifact == nil || artifact.URL == "" {
		return info.DownloadURL
	}
	return artifact.URL
}
