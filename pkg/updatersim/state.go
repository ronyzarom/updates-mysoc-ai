package updatersim

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// State is simulator state that must survive process restarts.
type State struct {
	InstanceID        string                       `json:"instance_id,omitempty"`
	APIKey            string                       `json:"api_key,omitempty"`
	ProductVersions   map[string]string            `json:"product_versions,omitempty"`
	LastUpdateAttempt *platformtypes.UpdateAttempt `json:"last_update_attempt,omitempty"`

	// Desired-state reconciliation tracking.
	SystemRelease     string            `json:"system_release,omitempty"`
	DBSchemaVersion   string            `json:"db_schema_version,omitempty"`
	ContainerVersions map[string]string `json:"container_versions,omitempty"`
	ConfigHashes      map[string]string `json:"config_hashes,omitempty"`
	UpdaterVersion    string            `json:"updater_version,omitempty"`
	PendingSelfUpdate *SelfUpdateState  `json:"pending_self_update,omitempty"`
	LastReconcile     *ReconcileStatus  `json:"last_reconcile,omitempty"`

	UpdatedAt time.Time `json:"updated_at"`
}

// SelfUpdateState marks an in-flight updater self-update. It is persisted before
// the handoff so a crash mid-swap is recoverable by a watchdog.
type SelfUpdateState struct {
	FromVersion string    `json:"from_version"`
	ToVersion   string    `json:"to_version"`
	StartedAt   time.Time `json:"started_at"`
}

// ReconcileStatus is the outcome of the most recent reconcile stage. It backs
// per-stage monitoring reported on the heartbeat and result endpoints.
type ReconcileStatus struct {
	Release   string    `json:"release"`
	Stage     string    `json:"stage"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// LoadState loads a state file. A missing file returns an empty state.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{
			ProductVersions:   make(map[string]string),
			ContainerVersions: make(map[string]string),
			ConfigHashes:      make(map[string]string),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read simulator state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse simulator state: %w", err)
	}
	if state.ProductVersions == nil {
		state.ProductVersions = make(map[string]string)
	}
	if state.ContainerVersions == nil {
		state.ContainerVersions = make(map[string]string)
	}
	if state.ConfigHashes == nil {
		state.ConfigHashes = make(map[string]string)
	}
	return &state, nil
}

// SaveState writes state with restrictive permissions using a temporary file.
func SaveState(path string, state *State) error {
	if path == "" {
		return fmt.Errorf("state file path is required")
	}
	if state == nil {
		return fmt.Errorf("state is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode simulator state: %w", err)
	}
	data = append(data, '\n')

	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create state temp file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0600); err != nil {
		file.Close()
		return fmt.Errorf("protect state temp file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write state temp file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync state temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close state temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace simulator state: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("protect simulator state: %w", err)
	}
	return nil
}

// ApplyState overlays server-issued identity and simulated product versions.
func ApplyState(cfg *Config, state *State) {
	if cfg == nil || state == nil {
		return
	}
	if state.InstanceID != "" {
		cfg.Instance.ID = state.InstanceID
	}
	if cfg.Server.APIKey == "" && state.APIKey != "" {
		cfg.Server.APIKey = state.APIKey
	}
	if state.UpdaterVersion != "" {
		cfg.Instance.UpdaterVersion = state.UpdaterVersion
	}
	for i := range cfg.Products {
		if version := state.ProductVersions[cfg.Products[i].Name]; version != "" {
			cfg.Products[i].CurrentVersion = version
		}
	}
}
