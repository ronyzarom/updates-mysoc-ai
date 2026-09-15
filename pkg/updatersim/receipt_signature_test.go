package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

// Exercise the actual offer -> verified download -> executor -> durable receipt
// boundary consumed by greenfield-hook.py, including the retained predecessor.
func TestSignedOfferRetainsVerifiableReceiptAndRollback(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	artifact := filepath.Join(t.TempDir(), "release.tar.gz")
	makeTarGz(t, artifact, map[string]string{"app": "fixture"})
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download":
			w.Write(data)
		case "/api/v1/updates/siemcore/report":
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "unexpected", 400)
		}
	}))
	defer server.Close()
	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	cfg.Signing = SigningConfig{PublicKey: hex.EncodeToString(pub), Require: true}
	cfg.Products[0].CurrentVersion = "3.3.152.18"
	executor := newFSExecutor(t, t.TempDir())
	sim, err := NewSimulator(cfg, executor, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	check := func(version string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(executor.versionDir("siemcore", version), ".updater-release.json"))
		if err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			Product   string `json:"product"`
			Version   string `json:"version"`
			SHA256    string `json:"sha256"`
			Signature string `json:"signature"`
		}
		if err = json.Unmarshal(raw, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Product != "siemcore" || receipt.Version != version || receipt.SHA256 != checksum {
			t.Fatalf("receipt identity mismatch: %#v", receipt)
		}
		if err = signing.Verify(pub, receipt.Product, receipt.Version, receipt.SHA256, receipt.Signature); err != nil {
			t.Fatalf("root consumer cannot verify saved receipt: %v", err)
		}
	}
	for _, version := range []string{"3.3.152.23", "3.3.152.24"} {
		offer := &UpdateOffer{Product: "siemcore", CurrentVersion: cfg.Products[0].CurrentVersion, LatestVersion: version, DownloadURL: "/download", Checksum: checksum, Signature: signing.Sign(priv, "siemcore", version, checksum)}
		if err = sim.processOffer(context.Background(), ModeReal, offer); err != nil {
			t.Fatal(err)
		}
		check(version)
	}
	if err = executor.Rollback(context.Background(), Update{Product: "siemcore"}); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(resolveCurrent(t, executor, "siemcore")) != "3.3.152.23" {
		t.Fatal("wrong rollback target")
	}
	check("3.3.152.23")
	bad := &UpdateOffer{Product: "siemcore", CurrentVersion: "3.3.152.23", LatestVersion: "3.3.152.25", DownloadURL: "/download", Checksum: checksum, Signature: signing.Sign(priv, "siemcore", "other", checksum)}
	if err = sim.processOffer(context.Background(), ModeReal, bad); err == nil {
		t.Fatal("accepted invalid signature")
	}
	if _, err = os.Stat(executor.versionDir("siemcore", "3.3.152.25")); !os.IsNotExist(err) {
		t.Fatal("invalid signature created release directory")
	}
}
