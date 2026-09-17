package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func TestDualMultiHopCacheSeparation(t *testing.T) {
	testArtifactMultiHopCacheSeparation(t, false)
}

func TestIndependentMultiHopCacheSeparation(t *testing.T) {
	testArtifactMultiHopCacheSeparation(t, true)
}

func testArtifactMultiHopCacheSeparation(t *testing.T, independent bool, serverTypes ...string) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	variants := []types.Artifact{}
	for _, kind := range []string{"bootstrap", "update"} {
		sum := sha256.Sum256([]byte(kind))
		a := types.Artifact{Product: "siemcore", Version: "1.1.0", Kind: kind, Name: kind, Arch: "linux/amd64", SourceCommit: strings.Repeat("a", 40), Checksum: hex.EncodeToString(sum[:]), Size: int64(len(kind))}
		artifactprotocol.Sign(key, &a)
		variants = append(variants, a)
	}
	meta := types.Release{ProductName: "siemcore", Version: "1.1.0", Checksum: variants[0].Checksum, Signature: variants[0].Signature, ArtifactSize: variants[0].Size, Manifest: types.Manifest{ArtifactVariants: variants}}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			kind := r.URL.Query().Get("artifact_kind")
			if kind == "" {
				kind = "bootstrap"
			}
			_, _ = w.Write([]byte(kind))
			return
		}
		if independent {
			kind := r.URL.Query().Get("artifact_kind")
			var selected *types.Artifact
			for i := range variants {
				if variants[i].Kind == kind {
					selected = &variants[i]
				}
			}
			if selected == nil {
				http.NotFound(w, r)
				return
			}
			row := meta
			row.Checksum, row.Signature, row.ArtifactSize = selected.Checksum, selected.Signature, selected.Size
			row.Manifest = types.Manifest{ArtifactKind: kind, ArtifactVariants: []types.Artifact{*selected}}
			writeTestJSON(t, w, row)
			return
		}
		writeTestJSON(t, w, meta)
	}))
	defer origin.Close()
	relayAt := func(url string) *Relay {
		cfg := newSimulatorTestConfig(t, url, ModeReal)
		if len(serverTypes) > 0 {
			cfg.Products[0].ServerType = serverTypes[0]
			if serverTypes[0] != "normal" && serverTypes[0] != "" {
				cfg.Products[0].PodID = "relay-pod"
				cfg.Products[0].NodeID = "1"
				if serverTypes[0] == "pod-observer" {
					cfg.Products[0].NodeID = "witness"
				}
			}
			// Exercise persisted role initialization used by the relay command.
			if _, err := NewSimulator(cfg, NoopExecutor{}, discardLogger()); err != nil {
				t.Fatal(err)
			}
		}
		cfg.Server.LicenseKey = "fixture"
		cfg.Relay.Enabled = true
		cfg.Relay.CacheDir = t.TempDir()
		cfg.Relay.MaxArtifactBytes = 1 << 20
		cfg.Signing.PublicKey = hex.EncodeToString(pub)
		cfg.Signing.Require = true
		client, err := NewClient(cfg.Server)
		if err != nil {
			t.Fatal(err)
		}
		relay, err := NewRelay(cfg, client, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		return relay
	}
	first := relayAt(origin.URL)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/releases/{product}/{version}", first.handleChildReleaseMeta)
	mux.HandleFunc("GET /api/v1/releases/{product}/{version}/download", first.handleChildDownload)
	hop := httptest.NewServer(mux)
	defer hop.Close()
	second := relayAt(hop.URL)
	paths := map[string]string{}
	kinds := []string{"bootstrap", "update", ""}
	if independent {
		kinds = kinds[:2]
	}
	for _, kind := range kinds {
		_, path, err := second.ensureCached(context.Background(), "siemcore", "1.1.0", kind)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		want := kind
		if want == "" {
			want = "bootstrap"
		}
		if string(data) != want {
			t.Fatalf("%s got %s", kind, data)
		}
		paths[kind] = path
	}
	if paths["bootstrap"] == paths["update"] {
		t.Fatal("variant cache collision")
	}
	if err := os.WriteFile(paths["update"], []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	_, path, err := second.ensureCached(context.Background(), "siemcore", "1.1.0", "update")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "update" {
		t.Fatal("corrupt cache served")
	}
}

// Relay delivery stays available for every host type, independent of the local
// product's readiness, drain barrier, or permission to process customer traffic.
func TestRelayArtifactDeliveryAllServerTypes(t *testing.T) {
	for _, kind := range []string{"", "normal", "pod-active", "pod-stby", "pod-observer"} {
		for _, independent := range []bool{false, true} {
			label := "paired"
			if independent {
				label = "independent"
			}
			t.Run(kind+"/"+label, func(t *testing.T) { testArtifactMultiHopCacheSeparation(t, independent, kind) })
		}
	}
}
