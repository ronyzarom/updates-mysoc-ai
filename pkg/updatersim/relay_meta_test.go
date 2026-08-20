package updatersim

import (
	"encoding/json"
	"testing"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// The child-facing metadata payload must satisfy two consumers at once:
// leaf agents built against the guide's flat contract (product, size), and
// downstream relays that parse the upstream Release field names.
func TestReleaseMetaBodyIsSupersetOfBothContracts(t *testing.T) {
	meta := &platformtypes.Release{
		ID:           "rel-1",
		ProductName:  "swf",
		Version:      "2.2.0.29",
		Channel:      "stable",
		Checksum:     "94a6d3bc",
		Signature:    "c2ln",
		ArtifactSize: 12345,
	}

	body := releaseMetaBody(meta)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}

	// Guide contract (leaf agents).
	for key, want := range map[string]interface{}{
		"product":   "swf",
		"version":   "2.2.0.29",
		"checksum":  "94a6d3bc",
		"signature": "c2ln",
		"size":      float64(12345),
	} {
		if out[key] != want {
			t.Errorf("guide key %q = %v, want %v", key, out[key], want)
		}
	}

	// Upstream shape (downstream relays parsing the Release struct).
	var parsed platformtypes.Release
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ProductName != "swf" || parsed.Checksum != "94a6d3bc" ||
		parsed.Signature != "c2ln" || parsed.ArtifactSize != 12345 {
		t.Errorf("upstream contract broken: %+v", parsed)
	}
}
