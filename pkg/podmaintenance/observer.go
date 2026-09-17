package podmaintenance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

const ObserverProtocol = "pod-observer-maintenance-v1"

type ObserverBinding Binding

func (b ObserverBinding) Validate() error {
	if b.Protocol != ObserverProtocol || b.NodeID != "witness" || b.Product != "siemcore" {
		return errors.New("explicit observer binding required")
	}
	base := Binding(b)
	base.Protocol = Protocol
	base.NodeID = "1"
	return base.Validate()
}

type ObserverRequest struct {
	Binding                 ObserverBinding `json:"binding"`
	Generation              uint64          `json:"generation"`
	AuthoritySnapshotSHA256 string          `json:"authority_snapshot_sha256,omitempty"`
}
type ObserverResponse struct {
	Binding                 ObserverBinding `json:"binding"`
	Generation              uint64          `json:"generation"`
	Phase                   string          `json:"phase"`
	PermissionExpires       time.Time       `json:"permission_expires"`
	Capabilities            []string        `json:"capabilities,omitempty"`
	LifecycleReady          bool            `json:"lifecycle_ready,omitempty"`
	AuthoritySnapshotSHA256 string          `json:"authority_snapshot_sha256,omitempty"`
	AuthorityPreserved      bool            `json:"authority_preserved"`
	Serialized              bool            `json:"serialized"`
	QuorumReady             bool            `json:"quorum_ready"`
	AuthenticationReady     bool            `json:"authentication_ready"`
	InstalledVersion        string          `json:"installed_version,omitempty"`
	InstalledSHA256         string          `json:"installed_sha256,omitempty"`
}
type ObserverAdapter interface {
	CallObserver(context.Context, string, ObserverRequest) (ObserverResponse, error)
}

func (a CommandAdapter) CallObserver(ctx context.Context, action string, q ObserverRequest) (ObserverResponse, error) {
	var r ObserverResponse
	switch action {
	case "capabilities", "prepare", "status", "apply", "reconcile", "health", "complete", "acceptance":
	default:
		return r, errors.New("invalid observer action")
	}
	e := a.invoke(ctx, action, q, &r)
	return r, e
}

type ObserverJournal struct {
	Binding                 ObserverBinding   `json:"binding"`
	Generation              uint64            `json:"generation"`
	AuthoritySnapshotSHA256 string            `json:"authority_snapshot_sha256,omitempty"`
	Phase                   string            `json:"phase"`
	Evidence                *ObserverResponse `json:"evidence,omitempty"`
}

func ReadObserverJournal(dir string) (ObserverJournal, error) {
	var j ObserverJournal
	e := readPrivate(filepath.Join(dir, "observer-operation.json"), &j)
	return j, e
}

type ObserverCoordinator struct {
	Directory              string
	Adapter                ObserverAdapter
	Now                    func() time.Time
	AfterDurableCheckpoint func(string)
}

func (c *ObserverCoordinator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}
func observerTargetEqual(a, b ObserverBinding) bool {
	a.OperationID = ""
	b.OperationID = ""
	a.Deadline = time.Time{}
	b.Deadline = time.Time{}
	return reflect.DeepEqual(a, b)
}
func (c *ObserverCoordinator) Run(ctx context.Context, target ObserverBinding) error {
	if c.Adapter == nil || !filepath.IsAbs(c.Directory) {
		return errors.New("protected observer adapter and directory required")
	}
	if err := os.MkdirAll(c.Directory, 0700); err != nil {
		return err
	}
	st, err := os.Lstat(c.Directory)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("observer journal directory must be private")
	}
	unlock, err := lock(c.Directory)
	if err != nil {
		return err
	}
	defer unlock()
	for _, name := range []string{"operation.json", "drain-v1.json", "recovery-v2.json"} {
		if _, err := os.Lstat(filepath.Join(c.Directory, name)); !os.IsNotExist(err) {
			return errors.New("data-node journal cannot be adopted by observer")
		}
	}
	j, err := ReadObserverJournal(c.Directory)
	if err == nil {
		if j.Binding.Validate() != nil {
			return errors.New("invalid observer journal binding")
		}
		if j.Phase != "intent" && (j.Generation == 0 || !digest(j.AuthoritySnapshotSHA256)) {
			return errors.New("missing durable observer acknowledgment")
		}
		if j.Phase == "accepted" && observerTargetEqual(j.Binding, target) {
			return nil
		}
		if j.Phase != "accepted" && !observerTargetEqual(j.Binding, target) {
			return errors.New("another observer operation is pending")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	save := func(phase string) error {
		j.Phase = phase
		err := writeDurableJSON(filepath.Join(c.Directory, "observer-operation.json"), j)
		if err == nil && c.AfterDurableCheckpoint != nil {
			c.AfterDurableCheckpoint(phase)
		}
		return err
	}
	if os.IsNotExist(err) || j.Phase == "accepted" {
		id := make([]byte, 16)
		if _, err = rand.Read(id); err != nil {
			return err
		}
		target.OperationID = hex.EncodeToString(id)
		if target.Deadline.IsZero() {
			target.Deadline = c.now().Add(30 * time.Minute)
		}
		if err = target.Validate(); err != nil {
			return err
		}
		j = ObserverJournal{Binding: target}
		if err = save("intent"); err != nil {
			return err
		}
	}
	request := func() ObserverRequest {
		return ObserverRequest{Binding: j.Binding, Generation: j.Generation, AuthoritySnapshotSHA256: j.AuthoritySnapshotSHA256}
	}
	cap, err := c.Adapter.CallObserver(ctx, "capabilities", request())
	if err != nil {
		return err
	}
	supported := false
	for _, v := range cap.Capabilities {
		if v == ObserverProtocol {
			supported = true
		}
	}
	if !supported || !cap.LifecycleReady {
		return errors.New("qualified observer lifecycle capability required; no fallback")
	}
	call := func(action string, mutation bool) (ObserverResponse, error) {
		if mutation && !c.now().Before(j.Binding.Deadline) {
			return ObserverResponse{}, errors.New("observer deadline expired; protection retained")
		}
		r, e := c.Adapter.CallObserver(ctx, action, request())
		if e != nil {
			return r, e
		}
		if !reflect.DeepEqual(r.Binding, j.Binding) || r.Generation == 0 || (j.Generation != 0 && r.Generation != j.Generation) || !digest(r.AuthoritySnapshotSHA256) || (j.AuthoritySnapshotSHA256 != "" && r.AuthoritySnapshotSHA256 != j.AuthoritySnapshotSHA256) || !r.AuthorityPreserved {
			return r, errors.New("observer authority identity or preservation mismatch")
		}
		if action != "status" && !c.now().Before(r.PermissionExpires) {
			return r, errors.New("observer acknowledgment expired")
		}
		if r.Phase != "completed" && !r.Serialized {
			return r, errors.New("observer serialization protection missing")
		}
		return r, nil
	}
	ready := func(r ObserverResponse) bool {
		return r.InstalledVersion == j.Binding.TargetVersion && r.InstalledSHA256 == j.Binding.ArtifactSHA256 && r.QuorumReady && r.AuthenticationReady && r.AuthorityPreserved && c.now().Before(r.PermissionExpires)
	}
	if j.Phase == "intent" {
		r, e := call("prepare", true)
		if e != nil {
			return e
		}
		if r.Phase != "prepared" {
			return errors.New("observer prepare not acknowledged")
		}
		j.Generation = r.Generation
		j.AuthoritySnapshotSHA256 = r.AuthoritySnapshotSHA256
		j.Evidence = &r
		if e = save("acknowledged"); e != nil {
			return e
		}
	}
	switch j.Phase {
	case "acknowledged":
		r, e := call("status", false)
		if e != nil {
			return e
		}
		if r.Phase != "prepared" || !c.now().Before(r.PermissionExpires) {
			return errors.New("observer protection not prepared")
		}
		if e = save("applying"); e != nil {
			return e
		}
		r, e = call("apply", true)
		if e != nil {
			return e
		}
		if r.Phase != "ready" || !ready(r) {
			return errors.New("observer apply not verified")
		}
		j.Evidence = &r
		if e = save("applied"); e != nil {
			return e
		}
	case "applying":
		r, e := call("reconcile", true)
		if e != nil {
			return e
		}
		if r.Phase != "ready" || !ready(r) {
			return errors.New("observer restart reconciliation incomplete")
		}
		j.Evidence = &r
		if e = save("applied"); e != nil {
			return e
		}
	case "applied", "completing", "completed":
	default:
		return errors.New("invalid observer journal phase")
	}
	if j.Phase == "completing" || j.Phase == "completed" {
		r, e := call("status", false)
		if e != nil {
			return e
		}
		if r.Phase == "completed" {
			if j.Evidence == nil || !readyHistorical(*j.Evidence, j.Binding) {
				return errors.New("missing observer completion evidence")
			}
			if e = save("completed"); e != nil {
				return e
			}
		} else if j.Phase == "completed" || r.Phase != "ready" {
			return errors.New("observer completion regressed")
		}
	}
	if j.Phase != "completed" {
		r, e := call("health", false)
		if e != nil {
			return e
		}
		if r.Phase != "ready" || !ready(r) {
			return errors.New("observer health not ready")
		}
		j.Evidence = &r
		if e = save("completing"); e != nil {
			return e
		}
		r, e = call("complete", true)
		if e != nil {
			return e
		}
		if r.Phase != "completed" || !ready(r) {
			return errors.New("observer completion not verified")
		}
		if e = save("completed"); e != nil {
			return e
		}
	}
	r, err := call("acceptance", false)
	if err != nil {
		return err
	}
	if r.Phase != "completed" || !ready(r) {
		return errors.New("observer acceptance incomplete")
	}
	j.Evidence = &r
	return save("accepted")
}
func readyHistorical(r ObserverResponse, b ObserverBinding) bool {
	return r.InstalledVersion == b.TargetVersion && r.InstalledSHA256 == b.ArtifactSHA256 && r.QuorumReady && r.AuthenticationReady && r.AuthorityPreserved
}
