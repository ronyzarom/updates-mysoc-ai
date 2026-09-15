package update

import (
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"testing"
)

func TestSelectArtifact(t *testing.T) {
	info := &types.ReleaseInfo{Artifacts: []types.Artifact{
		{Kind: "bootstrap", Checksum: "b"},
		{Kind: "update", Checksum: "u", RequiredDependencies: []types.Dependency{{Reference: "img", Digest: "sha256:x"}}},
	}}
	if a, v, _ := SelectArtifact(info, "empty", nil); a.Kind != "bootstrap" || v != "missing" {
		t.Fatalf("empty: %v %s", a, v)
	}
	if a, v, _ := SelectArtifact(info, "installed", []types.Dependency{{Reference: "img", Digest: "sha256:x"}}); a.Kind != "update" || v != "complete" {
		t.Fatalf("installed: %v %s", a, v)
	}
	if a, _, _ := SelectArtifact(info, "installed", nil); a.Kind != "bootstrap" {
		t.Fatalf("missing dependency: %v", a)
	}
	info.Artifacts = append(info.Artifacts, types.Artifact{Kind: "update"})
	if _, _, err := SelectArtifact(info, "empty", nil); err == nil {
		t.Fatal("ambiguous update accepted")
	}
}
