package releases

import (
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// TestSelfUpdateOfferIsFrozenAndSigned proves the self-update offer the whole
// fleet depends on is built from a fixed, signed field set that is independent
// of any protocol/capability/ring gate. The oldest updater sends a minimal
// check (no protocol_version, no capabilities); ReleaseInfoFor must still return
// a complete signed offer a relay can forward field-for-field (invariant 8).
func TestSelfUpdateOfferIsFrozenAndSigned(t *testing.T) {
	svc := &Service{} // ReleaseInfoFor is pure; no repository needed.
	release := &types.Release{
		ProductName:  "updater-linux-amd64",
		Version:      "1.18.0.1",
		Channel:      "stable",
		Checksum:     "deadbeef",
		Signature:    "c2lnbmF0dXJl", // non-empty base64; presence is what matters
		ArtifactSize: 4096,
		TargetGroups: []string{"alpha"}, // narrow targeting must not blank the offer shape
	}

	// An ancient current version — the oldest self-update check shape.
	info := svc.ReleaseInfoFor(release, "1.8.3.1")
	if info == nil {
		t.Fatal("ReleaseInfoFor returned nil for a self-update release")
	}
	if !info.UpdateAvailable {
		t.Fatal("self-update offer must be available to an older updater")
	}
	// The frozen field set a relay forwards must be fully populated.
	if info.Signature == "" {
		t.Error("self-update offer must carry a signature")
	}
	if info.Checksum == "" {
		t.Error("self-update offer must carry a checksum")
	}
	if info.LatestVersion != "1.18.0.1" {
		t.Errorf("latest_version = %q, want 1.18.0.1", info.LatestVersion)
	}
	if info.DownloadURL != "/api/v1/releases/updater-linux-amd64/1.18.0.1/download" {
		t.Errorf("download_url = %q, want the relay-rewritable form", info.DownloadURL)
	}

	// The offer shape does not depend on the caller advertising a protocol:
	// the same release yields the same signed offer regardless of current
	// version, so a version-mix cascade always resolves to a bootable offer.
	again := svc.ReleaseInfoFor(release, "1.17.1.1")
	if !again.UpdateAvailable || again.Signature != info.Signature || again.Checksum != info.Checksum {
		t.Fatal("self-update offer must be stable across caller versions")
	}

	// And the product must be classified as a self-update product so the server
	// bypasses the auto-update/ring gates for it.
	if !isSelfUpdateProduct(release.ProductName) {
		t.Fatalf("%q must be recognized as a self-update product", release.ProductName)
	}
}
