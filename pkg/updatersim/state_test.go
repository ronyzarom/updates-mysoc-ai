package updatersim

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveStateIsAtomicAndPrivate(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state", "simulator.json")
	state := &State{
		InstanceID:      "sim-siemcore",
		APIKey:          "test-api-key",
		ProductVersions: map[string]string{"siemcore": "1.2.3"},
	}
	if err := SaveState(path, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loaded.InstanceID != state.InstanceID ||
		loaded.APIKey != state.APIKey ||
		loaded.ProductVersions["siemcore"] != "1.2.3" {
		t.Fatalf("unexpected loaded state: %#v", loaded)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat state: %v", err)
		}
		if permissions := info.Mode().Perm(); permissions != 0600 {
			t.Fatalf("state permissions = %o, want 600", permissions)
		}
	}
}
