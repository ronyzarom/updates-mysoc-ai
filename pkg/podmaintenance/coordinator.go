// Package podmaintenance owns the durable outer update handshake, never pod authority.
package podmaintenance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

const Protocol = "pod-maintenance-v1"

type Binding struct {
	ArtifactSignature      string    `json:"artifact_signature"`
	ArtifactPath           string    `json:"artifact_path"`
	Protocol               string    `json:"protocol"`
	OperationID            string    `json:"operation_id"`
	PodID                  string    `json:"pod_id"`
	NodeID                 string    `json:"node_id"`
	UpdaterID              string    `json:"updater_id"`
	Product                string    `json:"product"`
	FromVersion            string    `json:"from_version"`
	TargetVersion          string    `json:"target_version"`
	ArtifactSHA256         string    `json:"artifact_sha256"`
	PreviousArtifactSHA256 string    `json:"previous_artifact_sha256"`
	Deadline               time.Time `json:"deadline"`
}
type Health struct {
	Version            string `json:"version"`
	SHA256             string `json:"sha256"`
	Role               string `json:"role"`
	ManagementReady    bool   `json:"management_ready"`
	ProcessingDisabled bool   `json:"processing_disabled"`
	TrafficDisabled    bool   `json:"traffic_disabled"`
	AuthorityValid     bool   `json:"authority_valid"`
	ReplicationStatus  string `json:"replication_status"`
}
type Acceptance struct {
	Health                   Health `json:"health"`
	ActiveIPOwned            bool   `json:"active_ip_owned"`
	ServiceHealthy           bool   `json:"service_healthy"`
	ProcessingAuthorityValid bool   `json:"processing_authority_valid"`
}
type Request struct {
	Binding    Binding `json:"binding"`
	Generation uint64  `json:"generation"`
	Health     *Health `json:"health,omitempty"`
}
type Response struct {
	Acceptance        *Acceptance `json:"acceptance,omitempty"`
	Capabilities      []string    `json:"capabilities,omitempty"`
	Binding           Binding     `json:"binding"`
	Generation        uint64      `json:"generation"`
	Phase             string      `json:"phase"`
	PermissionExpires time.Time   `json:"permission_expires"`
	Health            *Health     `json:"health,omitempty"`
}
type Adapter interface {
	Call(context.Context, string, Request) (Response, error)
}
type Journal struct {
	Acceptance *Acceptance `json:"acceptance,omitempty"`
	Binding    Binding     `json:"binding"`
	Generation uint64      `json:"generation"`
	Phase      string      `json:"phase"`
	Health     *Health     `json:"health,omitempty"`
}
type Coordinator struct {
	Directory string
	Adapter   Adapter
	Now       func() time.Time
}

func (c *Coordinator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}
func digest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && s == hex.EncodeToString(b)
}
func (b Binding) Validate() error {
	if b.ArtifactSignature == "" || !filepath.IsAbs(b.ArtifactPath) || b.Protocol != Protocol || b.OperationID == "" || b.PodID == "" || (b.NodeID != "1" && b.NodeID != "2") || b.UpdaterID == "" || b.Product == "" || b.FromVersion == "" || b.TargetVersion == "" || !digest(b.ArtifactSHA256) || !digest(b.PreviousArtifactSHA256) || b.Deadline.IsZero() {
		return errors.New("invalid maintenance identity")
	}
	return nil
}
func sameTarget(a, b Binding) bool {
	a.OperationID = ""
	b.OperationID = ""
	a.Deadline = time.Time{}
	b.Deadline = time.Time{}
	return reflect.DeepEqual(a, b)
}
func writeJournal(path string, j Journal) error {
	return writeDurableJSON(path, j)
}
func writeDurableJSON(path string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return e
	}
	return writeDurableBytes(path, raw)
}
func writeDurableBytes(path string, raw []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".maintenance-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = f.Chmod(0600); e != nil {
		return e
	}
	if _, e = f.Write(raw); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func validHealth(h *Health, b Binding) bool {
	return h != nil && h.Version == b.TargetVersion && h.SHA256 == b.ArtifactSHA256 && (h.Role == "ACTIVE" || h.Role == "STBY") && h.ManagementReady && h.ProcessingDisabled && h.TrafficDisabled && h.AuthorityValid && (h.ReplicationStatus == "ready" || h.ReplicationStatus == "degraded" || h.ReplicationStatus == "unknown")
}

// Run retains the operation on every failure. It never clears an observer barrier.
func (c *Coordinator) Run(ctx context.Context, target Binding) error {
	target.Protocol = Protocol
	if c.Adapter == nil || !filepath.IsAbs(c.Directory) {
		return errors.New("maintenance adapter and absolute journal directory required")
	}
	if e := os.MkdirAll(c.Directory, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(c.Directory)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("maintenance directory must be private")
	}
	unlock, e := lock(c.Directory)
	if e != nil {
		return e
	}
	defer unlock()
	if _, err := os.Lstat(filepath.Join(c.Directory, "recovery-v2.json")); err == nil {
		return errors.New("v2 recovery journal requires v2 reconciliation; no v1 fallback")
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(filepath.Join(c.Directory, "drain-v1.json")); !os.IsNotExist(err) {
		return errors.New("drain recovery requires separate reconciliation; no v1 fallback")
	}
	path := filepath.Join(c.Directory, "operation.json")
	j := Journal{}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("journal must be a private regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	raw, e := os.ReadFile(path)
	if e == nil {
		if e = decode(raw, &j); e != nil {
			return fmt.Errorf("invalid retained journal: %w", e)
		}
		if e = j.Binding.Validate(); e != nil {
			return e
		}
		if j.Phase != "accepted" && !sameTarget(j.Binding, target) {
			return errors.New("unfinished operation binds another release")
		}
		if j.Phase == "accepted" && sameTarget(j.Binding, target) {
			return nil
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if os.IsNotExist(e) || j.Phase == "accepted" {
		id := make([]byte, 16)
		if _, e = rand.Read(id); e != nil {
			return e
		}
		target.Protocol = Protocol
		target.OperationID = hex.EncodeToString(id)
		if target.Deadline.IsZero() {
			target.Deadline = c.now().Add(30 * time.Minute)
		}
		if e = target.Validate(); e != nil {
			return e
		}
		j = Journal{Binding: target, Phase: "intent"}
		if e = writeJournal(path, j); e != nil {
			return e
		}
	}

	cap, e := c.Adapter.Call(ctx, "capabilities", Request{Binding: j.Binding})
	if e != nil {
		return e
	}
	supported := false
	for _, s := range cap.Capabilities {
		if s == Protocol {
			supported = true
		}
	}
	if !supported {
		return errors.New("adapter lacks pod-maintenance-v1; no legacy fallback")
	}
	call := func(action string) (Response, error) {
		if action != "status" && !c.now().Before(j.Binding.Deadline) {
			return Response{}, errors.New("maintenance deadline expired; explicit recovery required")
		}
		r, e := c.Adapter.Call(ctx, action, Request{Binding: j.Binding, Generation: j.Generation, Health: j.Health})
		if e != nil {
			return r, e
		}
		if !reflect.DeepEqual(r.Binding, j.Binding) || r.Generation == 0 || (j.Generation != 0 && r.Generation != j.Generation) {
			return r, errors.New("maintenance acknowledgement identity/generation mismatch")
		}
		if r.Phase != "paused" && r.Phase != "completed" {
			return r, errors.New("invalid observer phase")
		}
		if (r.Phase == "paused" || action == "complete") && !c.now().Before(r.PermissionExpires) {
			return r, errors.New("maintenance permission expired")
		}
		return r, nil
	}
	save := func(phase string) error { j.Phase = phase; return writeJournal(path, j) }
	if j.Phase == "intent" {
		r, e := call("begin-or-resume")
		if e != nil {
			return e
		}
		if r.Phase != "paused" {
			return errors.New("begin did not acknowledge paused barrier")
		}
		j.Generation = r.Generation
		if e = save("acknowledged"); e != nil {
			return e
		}
	}
	r, e := call("status")
	if e != nil {
		return e
	}
	if r.Phase == "completed" {
		if (j.Phase != "completing" && j.Phase != "completed") || !validHealth(j.Health, j.Binding) {
			return errors.New("unexpected completed operation")
		}
		if e = save("completed"); e != nil {
			return e
		}
		return c.accept(ctx, &j, path)
	}
	switch j.Phase {
	case "acknowledged":
		if e = save("applying"); e != nil {
			return e
		}
		r, e = call("apply")
		if e != nil {
			return e
		}
		if r.Phase != "paused" {
			return errors.New("apply escaped maintenance")
		}
		if e = save("applied"); e != nil {
			return e
		}
	case "applying":
		r, e = call("recover")
		if e != nil {
			return e
		}
		if r.Phase != "paused" {
			return errors.New("recovery escaped maintenance")
		}
		if e = save("applied"); e != nil {
			return e
		}
	case "applied", "completing":
	default:
		return errors.New("unknown journal phase; recovery required")
	}
	r, e = call("health")
	if e != nil {
		return e
	}
	if r.Phase != "paused" || !validHealth(r.Health, j.Binding) {
		return errors.New("role health did not prove exact installed artifact and paused processing")
	}
	j.Health = r.Health
	if e = save("completing"); e != nil {
		return e
	}
	r, e = call("complete")
	if e != nil {
		return e
	}
	if r.Phase != "completed" {
		return errors.New("completion not acknowledged")
	}
	if e = save("completed"); e != nil {
		return e
	}
	return c.accept(ctx, &j, path)
}

func (c *Coordinator) accept(ctx context.Context, j *Journal, path string) error {
	r, e := c.Adapter.Call(ctx, "acceptance", Request{Binding: j.Binding, Generation: j.Generation})
	if e != nil {
		return e
	}
	a := r.Acceptance
	if !reflect.DeepEqual(r.Binding, j.Binding) || r.Generation != j.Generation || r.Phase != "completed" || a == nil {
		return errors.New("invalid post-completion identity")
	}
	h := a.Health
	if h.Version != j.Binding.TargetVersion || h.SHA256 != j.Binding.ArtifactSHA256 || !h.ManagementReady || !h.AuthorityValid || !a.ServiceHealthy || !c.now().Before(r.PermissionExpires) {
		return errors.New("post-completion installed identity/health/authority failure")
	}
	switch h.Role {
	case "ACTIVE":
		if !a.ActiveIPOwned || !a.ProcessingAuthorityValid || h.ProcessingDisabled || h.TrafficDisabled {
			return errors.New("ACTIVE has no proven processing/traffic authority")
		}
	case "STBY":
		if a.ActiveIPOwned || a.ProcessingAuthorityValid || !h.ProcessingDisabled || !h.TrafficDisabled {
			return errors.New("STBY not paused")
		}
	default:
		return errors.New("invalid accepted role")
	}
	j.Acceptance = a
	j.Phase = "accepted"
	return writeJournal(path, *j)
}

// ReadJournal is read-only; Run revalidates the binding under the operation lock.
func ReadJournal(directory string) (Journal, error) {
	var j Journal
	p := filepath.Join(directory, "operation.json")
	info, e := os.Lstat(p)
	if e != nil {
		return j, e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return j, errors.New("journal must be private regular file")
	}
	raw, e := os.ReadFile(p)
	if e != nil {
		return j, e
	}
	if e = decode(raw, &j); e != nil {
		return j, e
	}
	return j, j.Binding.Validate()
}
