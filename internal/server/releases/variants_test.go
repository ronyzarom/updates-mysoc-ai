package releases

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func TestDualOfferAndLegacyFallback(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	svc := &Service{signingKey: key}
	for _, product := range []string{"mysoc", "siemcore", "swf"} {
		a := types.Artifact{Product: product, Version: "1.0.1", Kind: "bootstrap", Name: "boot", Arch: "linux/amd64", SourceCommit: strings.Repeat("a", 40), Checksum: strings.Repeat("b", 64), Size: 1, URL: "/boot"}
		b := a
		b.Kind = "update"
		b.Name = "thin"
		b.URL = "/thin"
		artifactprotocol.Sign(key, &a)
		artifactprotocol.Sign(key, &b)
		r := &types.Release{ProductName: product, Version: a.Version, Checksum: a.Checksum, Signature: a.Signature, Manifest: types.Manifest{ArtifactVariants: []types.Artifact{a, b}}}
		info := releaseInfo(r, "1.0.0", true)
		if info.SelectedArtifactKind != "" || len(info.Artifacts) != 0 {
			t.Fatal("legacy response changed")
		}
		if !svc.DualOffer(r, info, a.Arch, artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: "1.0.0", Dependencies: []types.Dependency{}}) || info.SelectedArtifactKind != "update" || info.DownloadURL != "/thin" {
			t.Fatal("complete cache not selected")
		}
		r.Manifest.ArtifactVariants[1].SourceCommit = strings.Repeat("e", 40)
		info = releaseInfo(r, "1.0.0", true)
		if svc.DualOffer(r, info, a.Arch, artifactprotocol.Evidence{}) || info.SelectedArtifactKind != "" {
			t.Fatal("invalid metadata not legacy")
		}
	}
}
