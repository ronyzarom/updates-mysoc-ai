package updatersim

import (
	"os"
	"path/filepath"
	"testing"
)

func validTestManifest() *SystemTemplate {
	return &SystemTemplate{
		SchemaVersion:    1,
		Product:          "siemcore",
		Release:          "2.2.0",
		RequiredDBSchema: "2025.08.11-0007",
		Containers: []ContainerSpec{
			{Name: "siemcore-worker", Image: "img/worker", Version: "2.2.0", DependsOn: []string{"siemcore-api"}},
			{Name: "siemcore-api", Image: "img/api", Version: "2.2.0", DependsOn: []string{"siemcore-db"}},
			{Name: "siemcore-db", Image: "img/db", Version: "15.6", Strategy: StrategyRecreate},
		},
		ConfigTemplates: []ConfigTemplateSpec{
			{Path: "/opt/siemcore/config.yaml", SHA256: "abc123"},
		},
		SelfUpdate: &SelfUpdateSpec{Version: "updater/1.3.0.1"},
	}
}

func TestManifestValidateAcceptsValid(t *testing.T) {
	t.Parallel()
	if err := validTestManifest().Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestManifestValidateRejectsInvalid(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*SystemTemplate){
		"zero schema version": func(m *SystemTemplate) { m.SchemaVersion = 0 },
		"bad product":         func(m *SystemTemplate) { m.Product = "bad name" },
		"empty release":       func(m *SystemTemplate) { m.Release = "" },
		"missing image":       func(m *SystemTemplate) { m.Containers[0].Image = "" },
		"missing version":     func(m *SystemTemplate) { m.Containers[0].Version = "" },
		"bad strategy":        func(m *SystemTemplate) { m.Containers[0].Strategy = "sideways" },
		"duplicate container": func(m *SystemTemplate) { m.Containers[0].Name = m.Containers[1].Name },
		"unknown dependency":  func(m *SystemTemplate) { m.Containers[0].DependsOn = []string{"ghost"} },
		"missing config sha":  func(m *SystemTemplate) { m.ConfigTemplates[0].SHA256 = "" },
		"self update no ver":  func(m *SystemTemplate) { m.SelfUpdate.Version = "" },
	}

	for name, mutate := range cases {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			manifest := validTestManifest()
			mutate(manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatalf("expected validation error for %q", name)
			}
		})
	}
}

func TestManifestValidateDetectsCycle(t *testing.T) {
	t.Parallel()
	manifest := validTestManifest()
	// db -> api creates a cycle with the existing api -> db dependency.
	for i := range manifest.Containers {
		if manifest.Containers[i].Name == "siemcore-db" {
			manifest.Containers[i].DependsOn = []string{"siemcore-api"}
		}
	}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestManifestOrderIsDependencyRespecting(t *testing.T) {
	t.Parallel()
	order, err := validTestManifest().order()
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	position := make(map[string]int, len(order))
	for i, name := range order {
		position[name] = i
	}
	if position["siemcore-db"] > position["siemcore-api"] {
		t.Fatalf("db must precede api: %v", order)
	}
	if position["siemcore-api"] > position["siemcore-worker"] {
		t.Fatalf("api must precede worker: %v", order)
	}
}

func TestLoadManifestReadsAndValidates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "system-template.json")
	data := `{
      "schema_version": 1,
      "product": "siemcore",
      "release": "2.2.0",
      "containers": [{"name": "api", "image": "img/api", "version": "2.2.0"}]
    }`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.Product != "siemcore" || len(manifest.Containers) != 1 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestReconcileFreshInstallPlansEverythingInOrder(t *testing.T) {
	t.Parallel()
	plan, err := Reconcile(CurrentState{}, validTestManifest())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !plan.Has(ActionMigrate) {
		t.Fatal("expected a migrate action")
	}
	if !plan.Has(ActionRenderConfig) || !plan.Has(ActionSelfUpdate) {
		t.Fatalf("expected config and self-update actions: %#v", plan.Actions)
	}

	// Migrate must be first; container adds must follow dependency order.
	if plan.Actions[0].Kind != ActionMigrate {
		t.Fatalf("migrate must be first: %#v", plan.Actions)
	}
	containerOrder := make([]string, 0)
	for _, action := range plan.Actions {
		if action.Kind == ActionContainerAdd {
			containerOrder = append(containerOrder, action.Target)
		}
	}
	want := []string{"siemcore-db", "siemcore-api", "siemcore-worker"}
	for i := range want {
		if containerOrder[i] != want[i] {
			t.Fatalf("container add order = %v, want %v", containerOrder, want)
		}
	}
}

func TestReconcileUpgradeRemoveAndNoop(t *testing.T) {
	t.Parallel()
	manifest := validTestManifest()

	current := CurrentState{
		Containers: map[string]string{
			"siemcore-db":     "15.6",
			"siemcore-api":    "2.1.0", // upgrade
			"siemcore-worker": "2.2.0",
			"legacy-shipper":  "1.0.0", // removed
		},
		DBSchema:       manifest.RequiredDBSchema,
		ConfigHashes:   map[string]string{"/opt/siemcore/config.yaml": "abc123"},
		UpdaterVersion: "updater/1.3.0.1",
	}
	plan, err := Reconcile(current, manifest)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var upgrade, remove bool
	for _, action := range plan.Actions {
		if action.Kind == ActionContainerUpgrade && action.Target == "siemcore-api" {
			upgrade = true
			if action.FromVersion != "2.1.0" || action.ToVersion != "2.2.0" {
				t.Fatalf("unexpected upgrade action: %#v", action)
			}
		}
		if action.Kind == ActionContainerRemove && action.Target == "legacy-shipper" {
			remove = true
		}
	}
	if !upgrade || !remove {
		t.Fatalf("expected upgrade and remove actions: %#v", plan.Actions)
	}
	if plan.Has(ActionMigrate) || plan.Has(ActionRenderConfig) || plan.Has(ActionSelfUpdate) {
		t.Fatalf("did not expect migrate/config/self-update: %#v", plan.Actions)
	}

	// Now apply the same manifest against fully-current state: no work.
	current.Containers = map[string]string{
		"siemcore-db":     "15.6",
		"siemcore-api":    "2.2.0",
		"siemcore-worker": "2.2.0",
	}
	settled, err := Reconcile(current, manifest)
	if err != nil {
		t.Fatalf("reconcile settled: %v", err)
	}
	if settled.HasWork() {
		t.Fatalf("expected no work when settled: %#v", settled.Actions)
	}
}
