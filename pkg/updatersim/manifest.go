package updatersim

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Container rollout strategies.
const (
	StrategyRolling  = "rolling"
	StrategyRecreate = "recreate"
)

// SystemTemplate is the declarative desired state (system-template.json) that
// the agent reconciles a machine toward. It unifies the code, container,
// configuration, database schema, and self-update targets of one release into a
// single signed unit so the agent applies them through one gated pipeline
// rather than eight ad hoc code paths.
type SystemTemplate struct {
	SchemaVersion    int                  `json:"schema_version"`
	Product          string               `json:"product"`
	Release          string               `json:"release"`
	RequiredDBSchema string               `json:"required_db_schema,omitempty"`
	Containers       []ContainerSpec      `json:"containers"`
	ConfigTemplates  []ConfigTemplateSpec `json:"config_templates,omitempty"`
	SelfUpdate       *SelfUpdateSpec      `json:"self_update,omitempty"`
	Signature        string               `json:"signature,omitempty"`
}

// ContainerSpec is one desired container in the deployment set.
type ContainerSpec struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Version   string   `json:"version"`
	DependsOn []string `json:"depends_on,omitempty"`
	Health    string   `json:"health,omitempty"`
	Strategy  string   `json:"strategy,omitempty"`
}

// ConfigTemplateSpec is a rendered configuration file with an expected digest.
type ConfigTemplateSpec struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// SelfUpdateSpec describes a new updater agent version bundled with the release.
type SelfUpdateSpec struct {
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

// LoadManifest reads, decodes, and validates a system-template.json file.
func LoadManifest(path string) (*SystemTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var template SystemTemplate
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := template.Validate(); err != nil {
		return nil, err
	}
	return &template, nil
}

// Signed reports whether the manifest carries a detached signature. Signature
// verification against trusted keys is a target requirement and is not yet
// performed here.
func (t *SystemTemplate) Signed() bool {
	return t.Signature != ""
}

// Validate checks structural and referential integrity of the manifest.
func (t *SystemTemplate) Validate() error {
	if t == nil {
		return fmt.Errorf("manifest is required")
	}
	if t.SchemaVersion <= 0 {
		return fmt.Errorf("manifest schema_version must be positive")
	}
	if !productNamePattern.MatchString(t.Product) {
		return fmt.Errorf("invalid manifest product %q", t.Product)
	}
	if t.Release == "" {
		return fmt.Errorf("manifest release is required")
	}

	seen := make(map[string]struct{}, len(t.Containers))
	for _, container := range t.Containers {
		if !productNamePattern.MatchString(container.Name) {
			return fmt.Errorf("invalid container name %q", container.Name)
		}
		if _, ok := seen[container.Name]; ok {
			return fmt.Errorf("duplicate container %q", container.Name)
		}
		seen[container.Name] = struct{}{}
		if container.Image == "" {
			return fmt.Errorf("container %q is missing an image", container.Name)
		}
		if container.Version == "" {
			return fmt.Errorf("container %q is missing a version", container.Name)
		}
		switch container.Strategy {
		case "", StrategyRolling, StrategyRecreate:
		default:
			return fmt.Errorf("container %q has invalid strategy %q", container.Name, container.Strategy)
		}
	}
	for _, container := range t.Containers {
		for _, dependency := range container.DependsOn {
			if _, ok := seen[dependency]; !ok {
				return fmt.Errorf("container %q depends on unknown container %q", container.Name, dependency)
			}
			if dependency == container.Name {
				return fmt.Errorf("container %q cannot depend on itself", container.Name)
			}
		}
	}
	if _, err := t.order(); err != nil {
		return err
	}

	configPaths := make(map[string]struct{}, len(t.ConfigTemplates))
	for _, template := range t.ConfigTemplates {
		if template.Path == "" {
			return fmt.Errorf("config template path is required")
		}
		if _, ok := configPaths[template.Path]; ok {
			return fmt.Errorf("duplicate config template %q", template.Path)
		}
		configPaths[template.Path] = struct{}{}
		if template.SHA256 == "" {
			return fmt.Errorf("config template %q is missing a sha256", template.Path)
		}
	}

	if t.SelfUpdate != nil && t.SelfUpdate.Version == "" {
		return fmt.Errorf("self_update requires a version")
	}
	return nil
}

// order returns container names in dependency order using a deterministic
// Kahn topological sort. It returns an error when the dependency graph contains
// a cycle.
func (t *SystemTemplate) order() ([]string, error) {
	indegree := make(map[string]int, len(t.Containers))
	dependents := make(map[string][]string, len(t.Containers))
	names := make([]string, 0, len(t.Containers))
	for _, container := range t.Containers {
		names = append(names, container.Name)
		if _, ok := indegree[container.Name]; !ok {
			indegree[container.Name] = 0
		}
	}
	for _, container := range t.Containers {
		for _, dependency := range container.DependsOn {
			indegree[container.Name]++
			dependents[dependency] = append(dependents[dependency], container.Name)
		}
	}
	sort.Strings(names)

	var ready []string
	for _, name := range names {
		if indegree[name] == 0 {
			ready = append(ready, name)
		}
	}

	ordered := make([]string, 0, len(names))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		ordered = append(ordered, name)

		next := append([]string(nil), dependents[name]...)
		sort.Strings(next)
		for _, dependent := range next {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if len(ordered) != len(names) {
		return nil, fmt.Errorf("container dependency graph contains a cycle")
	}
	return ordered, nil
}

// ActionKind enumerates the reconcile actions the agent may take.
type ActionKind string

const (
	ActionMigrate          ActionKind = "migrate"
	ActionContainerAdd     ActionKind = "container_add"
	ActionContainerUpgrade ActionKind = "container_upgrade"
	ActionContainerRemove  ActionKind = "container_remove"
	ActionRenderConfig     ActionKind = "render_config"
	ActionSelfUpdate       ActionKind = "self_update"
)

// PlanAction is a single unit of reconcile work.
type PlanAction struct {
	Kind        ActionKind `json:"kind"`
	Target      string     `json:"target"`
	FromVersion string     `json:"from_version,omitempty"`
	ToVersion   string     `json:"to_version,omitempty"`
}

// Plan is the ordered set of actions that moves current state to desired state.
type Plan struct {
	Product string
	Release string
	Order   []string
	Actions []PlanAction
}

// HasWork reports whether the plan contains any actions.
func (p Plan) HasWork() bool {
	return len(p.Actions) > 0
}

// Has reports whether the plan contains at least one action of the given kind.
func (p Plan) Has(kind ActionKind) bool {
	for _, action := range p.Actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}

// CurrentState is the agent's view of what is installed on the machine.
type CurrentState struct {
	Containers     map[string]string
	DBSchema       string
	ConfigHashes   map[string]string
	UpdaterVersion string
}

// Reconcile diffs current state against a desired manifest and produces an
// ordered plan: database migration first, then containers in dependency order,
// then configuration renders, then updater self-update.
func Reconcile(current CurrentState, desired *SystemTemplate) (Plan, error) {
	if desired == nil {
		return Plan{}, fmt.Errorf("desired manifest is required")
	}
	order, err := desired.order()
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{Product: desired.Product, Release: desired.Release, Order: order}

	if desired.RequiredDBSchema != "" && desired.RequiredDBSchema != current.DBSchema {
		plan.Actions = append(plan.Actions, PlanAction{
			Kind:        ActionMigrate,
			Target:      "db",
			FromVersion: current.DBSchema,
			ToVersion:   desired.RequiredDBSchema,
		})
	}

	desiredByName := make(map[string]ContainerSpec, len(desired.Containers))
	for _, container := range desired.Containers {
		desiredByName[container.Name] = container
	}
	for _, name := range order {
		container := desiredByName[name]
		installed, ok := current.Containers[name]
		switch {
		case !ok:
			plan.Actions = append(plan.Actions, PlanAction{
				Kind:      ActionContainerAdd,
				Target:    name,
				ToVersion: container.Version,
			})
		case installed != container.Version:
			plan.Actions = append(plan.Actions, PlanAction{
				Kind:        ActionContainerUpgrade,
				Target:      name,
				FromVersion: installed,
				ToVersion:   container.Version,
			})
		}
	}

	var removed []string
	for name := range current.Containers {
		if _, ok := desiredByName[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(removed)
	for _, name := range removed {
		plan.Actions = append(plan.Actions, PlanAction{
			Kind:        ActionContainerRemove,
			Target:      name,
			FromVersion: current.Containers[name],
		})
	}

	for _, template := range desired.ConfigTemplates {
		if current.ConfigHashes[template.Path] != template.SHA256 {
			plan.Actions = append(plan.Actions, PlanAction{
				Kind:      ActionRenderConfig,
				Target:    template.Path,
				ToVersion: template.SHA256,
			})
		}
	}

	if desired.SelfUpdate != nil && desired.SelfUpdate.Version != current.UpdaterVersion {
		plan.Actions = append(plan.Actions, PlanAction{
			Kind:        ActionSelfUpdate,
			Target:      "updater",
			FromVersion: current.UpdaterVersion,
			ToVersion:   desired.SelfUpdate.Version,
		})
	}

	return plan, nil
}
