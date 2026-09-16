// Command pod-maintenance-qualification runs real coordination against an isolated
// product adapter. It is a test driver, never an installer or release command.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	p "github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	FaultCheckpoint string                  `json:"fault_checkpoint,omitempty"`
	EvidenceTier    string                  `json:"evidence_tier"`
	Isolated        bool                    `json:"isolated"`
	TestID          string                  `json:"test_id"`
	Mode            string                  `json:"mode"`
	Directory       string                  `json:"directory"`
	Adapter         []string                `json:"adapter_command"`
	TimeoutSeconds  int                     `json:"timeout_seconds"`
	Binding         p.Binding               `json:"binding"`
	Authorization   p.RecoveryAuthorization `json:"authorization"`
	Artifact        p.RetainedArtifact      `json:"artifact"`
	ObserverKey     string                  `json:"observer_public_key"`
	ReleaseKey      string                  `json:"release_public_key"`
}

func execute(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	var c Config
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return e
	}
	if c.EvidenceTier != "component" && c.EvidenceTier != "native-with-synthetic-host" && c.EvidenceTier != "executable-real-host" {
		return fmt.Errorf("explicit evidence_tier required")
	}
	if !c.Isolated || c.TestID == "" || !filepath.IsAbs(c.Directory) || len(c.Adapter) == 0 {
		return fmt.Errorf("explicit isolated test config required")
	}
	if c.TimeoutSeconds <= 0 || c.TimeoutSeconds > 60 {
		return fmt.Errorf("qualification command timeout must be 1..60 seconds")
	}
	os.Setenv("POD_QUALIFICATION_DRIVER_PID", strconv.Itoa(os.Getpid()))
	os.Setenv("POD_QUALIFICATION_TEST_ID", c.TestID)
	adapter := p.CommandAdapter{Command: c.Adapter, Timeout: time.Duration(c.TimeoutSeconds) * time.Second}
	ctx := context.Background()
	outcome := ""
	switch c.Mode {
	case "readiness":
		_, e = adapter.Readiness(ctx, p.ReadinessRequest{Protocol: p.ReadinessProtocol, PodID: c.Binding.PodID, NodeID: c.Binding.NodeID, UpdaterID: c.Binding.UpdaterID})
	case "maintenance-v1":
		key, err := signing.ParsePublicKeyHex(c.ReleaseKey)
		if err != nil {
			return err
		}
		if err = signing.Verify(key, c.Binding.Product, c.Binding.TargetVersion, c.Binding.ArtifactSHA256, c.Binding.ArtifactSignature); err != nil {
			return err
		}
		artifact, err := os.Open(c.Binding.ArtifactPath)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, artifact)
		artifact.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != c.Binding.ArtifactSHA256 {
			return fmt.Errorf("qualification artifact checksum mismatch")
		}
		e = (&p.Coordinator{Directory: c.Directory, Adapter: adapter}).Run(ctx, c.Binding)
	case "recovery-v2", "next-operation":
		observer, err := signing.ParsePublicKeyHex(c.ObserverKey)
		if err != nil {
			return err
		}
		release, err := signing.ParsePublicKeyHex(c.ReleaseKey)
		if err != nil {
			return err
		}
		if c.Mode == "recovery-v2" {
			outcome, e = (&p.RecoveryCoordinator{Directory: c.Directory, Adapter: adapter, ObserverKey: observer, ReleaseKey: release}).Run(ctx, c.Authorization, c.Artifact)
		} else {
			transition := &p.TransitionCoordinator{Directory: c.Directory, Adapter: adapter, ObserverKey: observer, ReleaseKey: release}
			if c.FaultCheckpoint != "" {
				switch c.FaultCheckpoint {
				case "prepared", "recovery-archived", "next-intent-written":
				default:
					return fmt.Errorf("invalid archival fault checkpoint")
				}
				transition.AfterDurableCheckpoint = func(point string) {
					if point != c.FaultCheckpoint {
						return
					}
					marker := filepath.Join(filepath.Dir(path), "injected-"+point)
					f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
					if os.IsExist(err) {
						return
					}
					if err != nil {
						panic(err)
					}
					f.WriteString(point)
					f.Sync()
					f.Close()
					dir, err := os.Open(filepath.Dir(marker))
					if err != nil {
						panic(err)
					}
					dir.Sync()
					dir.Close()
					self, _ := os.FindProcess(os.Getpid())
					self.Kill()
				}
			}
			e = transition.Advance(ctx, c.Authorization)
		}
	default:
		return fmt.Errorf("unsupported qualification mode")
	}
	files := map[string]string{}
	for _, name := range []string{"operation.json", "recovery-v2.json", "next-operation.json"} {
		if raw, err := os.ReadFile(filepath.Join(c.Directory, name)); err == nil {
			h := sha256.Sum256(raw)
			files[name] = hex.EncodeToString(h[:])
		}
	}
	message := ""
	if e != nil {
		message = e.Error()
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"test_id": c.TestID, "evidence_tier": c.EvidenceTier, "mode": c.Mode, "success": e == nil, "outcome": outcome, "error": message, "journal_sha256": files})
	return e
}
func main() {
	path := flag.String("config", "", "isolated qualification configuration")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "--config is required")
		os.Exit(2)
	}
	if e := execute(*path); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
