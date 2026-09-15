package artifactprotocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func pair(product string) []types.Artifact {
	a := types.Artifact{Product: product, Version: "3.4.0.1", SourceCommit: strings.Repeat("a", 40), Arch: "linux/amd64", Size: 10, Checksum: strings.Repeat("b", 64), Kind: "bootstrap", Name: "bootstrap.tgz"}
	b := a
	b.Kind = "update"
	b.Name = "update.tgz"
	b.RequiredDependencies = []types.Dependency{{Reference: "postgres", Digest: "sha256:" + strings.Repeat("c", 64), Capabilities: []string{"postgres-16"}}}
	return []types.Artifact{a, b}
}
func TestSelectionAllProducts(t *testing.T) {
	for _, product := range []string{"siemcore", "mysoc", "swf"} {
		t.Run(product, func(t *testing.T) {
			variants := pair(product)
			deps := variants[1].RequiredDependencies
			for _, tc := range []struct {
				name         string
				e            Evidence
				kind, status string
			}{
				{"empty", Evidence{Lifecycle: "empty"}, "bootstrap", "missing"},
				{"complete", Evidence{"installed", "3.4.0.0", deps}, "update", "complete"},
				{"omitted", Evidence{"installed", "3.4.0.0", nil}, "bootstrap", "missing"},
				{"missing", Evidence{"installed", "3.4.0.0", []types.Dependency{}}, "bootstrap", "missing"},
				{"wrong digest", Evidence{"installed", "3.4.0.0", []types.Dependency{{Reference: "postgres", Digest: "sha256:" + strings.Repeat("d", 64)}}}, "bootstrap", "mismatch"},
				{"wrong capability", Evidence{"installed", "3.4.0.0", []types.Dependency{{Reference: "postgres", Digest: deps[0].Digest}}}, "bootstrap", "mismatch"},
				{"incomplete install", Evidence{"partial", "3.4.0.0", deps}, "bootstrap", "missing"},
				{"missing version", Evidence{"installed", "", deps}, "bootstrap", "missing"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					a, status := Select(variants, tc.e)
					if a.Kind != tc.kind || status != tc.status {
						t.Fatalf("got %s/%s", a.Kind, status)
					}
				})
			}
		})
	}
}
func TestSignedIdentityAndDependencies(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	for _, mutate := range []func(*types.Artifact){
		func(a *types.Artifact) { a.Kind = "bootstrap" }, func(a *types.Artifact) { a.Product = "mysoc" }, func(a *types.Artifact) { a.Version = "4.0" }, func(a *types.Artifact) { a.Arch = "windows/amd64" }, func(a *types.Artifact) { a.SourceCommit = strings.Repeat("d", 40) }, func(a *types.Artifact) { a.RequiredDependencies = nil }, func(a *types.Artifact) { a.Checksum = strings.Repeat("e", 64) },
	} {
		a := pair("siemcore")[1]
		Sign(key, &a)
		if err := Verify(pub, a); err != nil {
			t.Fatal(err)
		}
		mutate(&a)
		if Verify(pub, a) == nil {
			t.Fatal("accepted tampered metadata")
		}
	}
}
func TestPublicationValidation(t *testing.T) {
	for _, mutate := range []func([]types.Artifact){func(a []types.Artifact) { a[1].Version = "bad" }, func(a []types.Artifact) { a[1].Product = "mysoc" }, func(a []types.Artifact) { a[1].SourceCommit = strings.Repeat("f", 40) }, func(a []types.Artifact) { a[1].Arch = "arm64" }, func(a []types.Artifact) { a[1].Kind = "bootstrap" }, func(a []types.Artifact) { a[1].Name = "../x" }, func(a []types.Artifact) { a[1].RequiredDependencies[0].Digest = "postgres:latest" }} {
		a := pair("siemcore")
		mutate(a)
		if ValidatePair("siemcore", "3.4.0.1", a) == nil {
			t.Fatal("invalid pair accepted")
		}
	}
	if ValidatePair("siemcore", "3.4.0.1", nil) == nil {
		t.Fatal("missing pair accepted")
	}
	if Advertised("1", []string{Capability}) || Advertised(Version, nil) {
		t.Fatal("legacy/unknown client enabled")
	}
}

func TestCanonicalCrossLanguageVector(t *testing.T) {
	raw, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		PublicKey string         `json:"public_key"`
		Artifact  types.Artifact `json:"artifact"`
		Canonical string         `json:"canonical_metadata"`
		Digest    string         `json:"metadata_sha256"`
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	key, err := hex.DecodeString(vector.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if string(CanonicalMetadata(vector.Artifact)) != vector.Canonical || MetadataDigest(vector.Artifact) != vector.Digest {
		t.Fatal("canonical wire encoding changed")
	}
	if err := Verify(ed25519.PublicKey(key), vector.Artifact); err != nil {
		t.Fatal(err)
	}
}
