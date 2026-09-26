package releases

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDualPublicationDatabase(t *testing.T) {
	url := os.Getenv("UPDATES_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable UPDATES_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "dual_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, _ := pgxpool.ParseConfig(url)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE releases(id uuid PRIMARY KEY, product_name text, version text, channel text, manifest jsonb, artifact_path text, artifact_size bigint, checksum text, signature text, release_notes text, min_updater_version text, target_groups text[], released_at timestamptz, created_at timestamptz, UNIQUE(product_name,version))`)
	for _, file := range []string{"016_independent_artifacts.up.sql", "018_issuer_sealing.up.sql"} {
		if err != nil {
			break
		}
		migration, readErr := os.ReadFile("../../../migrations/" + file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		_, err = pool.Exec(ctx, string(migration))
	}
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store, err := storage.NewLocalStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(&database.DB{Pool: pool}, store)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	svc.SetSigningKey(key)
	uploads := func(product, version string) []VariantUpload {
		out := []VariantUpload{}
		for _, kind := range []string{"bootstrap", "update"} {
			payload := []byte(product + "-" + kind)
			sum := sha256.Sum256(payload)
			out = append(out, VariantUpload{Artifact: types.Artifact{Product: product, Version: version, Kind: kind, Name: kind + ".bin", Arch: "linux/amd64", SourceCommit: strings.Repeat("a", 40), Size: int64(len(payload)), Checksum: hex.EncodeToString(sum[:])}, File: bytes.NewReader(payload)})
		}
		return out
	}
	for _, product := range []string{"mysoc", "siemcore", "swf"} {
		t.Run(product, func(t *testing.T) {
			req := CreateReleaseRequest{ProductName: product, Version: "1.0.1", Channel: "stable", TargetGroups: []string{"alpha"}}
			release, err := svc.CreateDualRelease(ctx, req, uploads(product, req.Version))
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := svc.GetRelease(ctx, product, req.Version)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.Manifest.ArtifactVariants) != 2 || loaded.ID != release.ID {
				t.Fatal("pair not preserved")
			}
			for _, a := range loaded.Manifest.ArtifactVariants {
				if err := artifactprotocol.Verify(pub, a); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(store.GetPath(product, req.Version, a.Name))
				if err != nil || string(data) != product+"-"+a.Kind {
					t.Fatal("wrong stored variant")
				}
			}
			legacy := svc.ReleaseInfoFor(loaded, "1.0.0")
			if legacy.Checksum != loaded.Manifest.ArtifactVariants[0].Checksum || len(legacy.Artifacts) != 0 {
				t.Fatal("legacy compatibility failed")
			}
			if other, err := svc.HighestReleaseForGroup(ctx, product, "stable", "production"); err != nil || other != nil {
				t.Fatal("alpha leaked into production")
			}
			if _, err := svc.CreateDualRelease(ctx, req, uploads(product, req.Version)); err == nil {
				t.Fatal("duplicate accepted")
			}
			if err := svc.UpdateReleaseTargetGroups(ctx, loaded.ID, []string{"alpha", "beta"}); err != nil {
				t.Fatal(err)
			}
			promoted, _ := svc.GetRelease(ctx, product, req.Version)
			if promoted.Checksum != loaded.Checksum || promoted.Manifest.ArtifactVariants[1].Signature != loaded.Manifest.ArtifactVariants[1].Signature {
				t.Fatal("promotion rebuilt artifact")
			}
			// Invalid second stream leaves neither a row nor a visible artifact.
			bad := req
			bad.Version = "1.0.2"
			files := uploads(product, bad.Version)
			files[1].File = bytes.NewReader([]byte("corrupted"))
			if _, err := svc.CreateDualRelease(ctx, bad, files); err == nil {
				t.Fatal("bad bytes accepted")
			}
			if row, _ := svc.GetRelease(ctx, product, bad.Version); row != nil {
				t.Fatal("partial release published")
			}
			matches, _ := filepath.Glob(filepath.Join(dir, product, bad.Version, "*"))
			if len(matches) != 0 {
				t.Fatal("partial files remain")
			}
		})
	}
	for _, product := range []string{"mysoc", "siemcore", "swf"} {
		t.Run(product+"/independent", func(t *testing.T) {
			channel := "dual-alpha-" + product
			version := "4.0.0"
			files := uploads(product, version)
			var ids []string
			for _, file := range files {
				r, err := svc.CreateDualRelease(ctx, CreateReleaseRequest{ProductName: product, Version: version, Channel: channel, TargetGroups: []string{"alpha"}, ArtifactKind: file.Artifact.Kind}, []VariantUpload{file})
				if err != nil {
					t.Fatal(err)
				}
				ids = append(ids, r.ID)
				found, err := svc.GetRelease(ctx, product, version, file.Artifact.Kind)
				if err != nil || found == nil || found.ID != r.ID {
					t.Fatal("kind lookup failed", err)
				}
			}
			if ids[0] == ids[1] {
				t.Fatal("linked identity")
			}
			if r, _ := svc.GetRelease(ctx, product, version); r != nil {
				t.Fatal("legacy lookup exposed independent release")
			}
			if r, _ := svc.HighestReleaseForGroup(ctx, product, channel, "alpha"); r != nil {
				t.Fatal("old client sees independent artifact")
			}
			e := artifactprotocol.Evidence{Lifecycle: "empty"}
			r, info, err := svc.IndependentOffer(ctx, product, channel, "alpha", "", "linux/amd64", e)
			if err != nil || r == nil || info.SelectedArtifactKind != "bootstrap" {
				t.Fatal("empty host selection failed", err)
			}
			deps := files[1].Artifact.RequiredDependencies
			if deps == nil {
				deps = []types.Dependency{}
			}
			e = artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: "3.0.0", Dependencies: deps}
			_, info, err = svc.IndependentOffer(ctx, product, channel, "alpha", "3.0.0", "linux/amd64", e)
			if err != nil || info == nil || info.SelectedArtifactKind != "update" {
				t.Fatal("installed selection failed", err)
			}
			if r, _, _ := svc.IndependentOffer(ctx, product, "stable", "alpha", "3.0.0", "linux/amd64", e); r != nil {
				t.Fatal("channel leak")
			}
			if r, _, _ := svc.IndependentOffer(ctx, product, channel, "production", "3.0.0", "linux/amd64", e); r != nil {
				t.Fatal("ring leak")
			}
			if err := svc.UpdateReleaseTargetGroups(ctx, ids[0], []string{"beta"}); err != nil {
				t.Fatal(err)
			}
			u, _ := svc.GetRelease(ctx, product, version, "update")
			if len(u.TargetGroups) != 1 || u.TargetGroups[0] != "alpha" {
				t.Fatal("bootstrap promotion changed update")
			}
			// Once bootstrap leaves alpha, omitted dependency evidence cannot
			// turn an independent thin update into a legacy/full offer.
			_, _, prerequisiteErr := svc.IndependentOffer(ctx, product, channel, "alpha", "3.0.0", "linux/amd64", artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: "3.0.0"})
			if prerequisiteErr != ErrPrerequisites {
				t.Fatal("missing prerequisites did not fail closed", prerequisiteErr)
			}
			if err := svc.DeleteRelease(ctx, ids[0]); err != nil {
				t.Fatal(err)
			}
			if u, _ := svc.GetRelease(ctx, product, version, "update"); u == nil {
				t.Fatal("bootstrap delete removed update")
			}
		})
	}

	// Concurrent publication yields exactly one immutable winner and two files.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.CreateDualRelease(ctx, CreateReleaseRequest{ProductName: "swf", Version: "1.0.3", Channel: "stable", TargetGroups: []string{"alpha"}}, uploads("swf", "1.0.3"))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("got %d successful publishers", success)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "swf", "1.0.3", "*"))
	if len(matches) != 2 {
		t.Fatalf("winner artifacts changed: %v", matches)
	}
}
