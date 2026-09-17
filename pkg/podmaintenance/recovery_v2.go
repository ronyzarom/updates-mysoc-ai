package podmaintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

const RecoveryProtocol = "pod-maintenance-recovery-v2"
const RecoveryAuthorizationDomain = "mysoc-pod-recovery-authorization-v2\n"
const TargetInstalled = "target-installed"
const PredecessorRestored = "predecessor-restored"

type RetainedArtifact struct {
	Product   string `json:"product"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	Path      string `json:"path"`
}
type RecoveryAuthorization struct {
	PayloadBase64 string `json:"payload_base64"`
	Signature     string `json:"signature"`
}
type RecoveryClaims struct {
	Protocol        string    `json:"protocol"`
	AuthorizationID string    `json:"authorization_id"`
	Binding         Binding   `json:"binding"`
	Generation      uint64    `json:"generation"`
	Action          string    `json:"action"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// DecodeRecoveryAuthorization verifies exact signed bytes before strict decoding.
// Time validity for mutation is checked separately; expired receipts may prove
// the identity of an already completed operation, never authorize new mutation.
func DecodeRecoveryAuthorization(a RecoveryAuthorization, key ed25519.PublicKey) (RecoveryClaims, error) {
	var c RecoveryClaims
	raw, e := base64.StdEncoding.DecodeString(a.PayloadBase64)
	if e != nil || len(raw) > 32768 {
		return c, errors.New("invalid authorization payload")
	}
	sig, e := base64.StdEncoding.DecodeString(a.Signature)
	if e != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, append([]byte(RecoveryAuthorizationDomain), raw...), sig) {
		return c, errors.New("invalid observer authorization signature")
	}
	if e = decode(raw, &c); e != nil {
		return c, e
	}
	if c.Protocol != RecoveryProtocol || c.AuthorizationID == "" || len(c.AuthorizationID) > 128 || c.Generation == 0 || (c.Action != "resume-target" && c.Action != "restore-predecessor") || c.IssuedAt.IsZero() || !c.ExpiresAt.After(c.IssuedAt) || c.ExpiresAt.Sub(c.IssuedAt) > 15*time.Minute {
		return c, errors.New("invalid recovery authorization scope")
	}
	return c, c.Binding.Validate()
}

type RecoveryRequest struct {
	Protocol      string                `json:"protocol"`
	Binding       Binding               `json:"binding"`
	Generation    uint64                `json:"generation"`
	Outcome       string                `json:"outcome"`
	Authorization RecoveryAuthorization `json:"authorization"`
	Artifact      RetainedArtifact      `json:"artifact"`
	Health        *Health               `json:"health,omitempty"`
}
type RecoveryResponse struct {
	Protocol          string      `json:"protocol"`
	Capabilities      []string    `json:"capabilities,omitempty"`
	Binding           Binding     `json:"binding"`
	Generation        uint64      `json:"generation"`
	Outcome           string      `json:"outcome"`
	AuthorizationID   string      `json:"authorization_id"`
	Phase             string      `json:"phase"`
	PermissionExpires time.Time   `json:"permission_expires"`
	Health            *Health     `json:"health,omitempty"`
	Acceptance        *Acceptance `json:"acceptance,omitempty"`
}
type RecoveryAdapter interface {
	CallRecovery(context.Context, string, RecoveryRequest) (RecoveryResponse, error)
}
type RecoveryJournal struct {
	Protocol            string                `json:"protocol"`
	Binding             Binding               `json:"binding"`
	Generation          uint64                `json:"generation"`
	Outcome             string                `json:"outcome"`
	Artifact            RetainedArtifact      `json:"artifact"`
	Authorization       RecoveryAuthorization `json:"authorization"`
	AuthorizationHashes map[string]string     `json:"authorization_hashes"`
	Phase               string                `json:"phase"`
	Health              *Health               `json:"health,omitempty"`
	Acceptance          *Acceptance           `json:"acceptance,omitempty"`
}
type RecoveryCoordinator struct {
	Directory   string
	Adapter     RecoveryAdapter
	ObserverKey ed25519.PublicKey
	ReleaseKey  ed25519.PublicKey
	Now         func() time.Time
}

func (c *RecoveryCoordinator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}
func verifyRetained(a RetainedArtifact, key ed25519.PublicKey) error {
	if !filepath.IsAbs(a.Path) || !digest(a.SHA256) || len(key) != ed25519.PublicKeySize {
		return errors.New("invalid retained artifact receipt")
	}
	if e := signing.Verify(key, a.Product, a.Version, a.SHA256, a.Signature); e != nil {
		return e
	}
	f, e := os.Open(a.Path)
	if e != nil {
		return e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() {
		return errors.New("retained artifact is not a regular file")
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return errors.New("retained artifact digest mismatch")
	}
	return nil
}
func readPrivate(path string, v any) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 1<<20 {
		return errors.New("expected bounded private regular file")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return decode(raw, v)
}
func ReadRecoveryJournal(dir string) (RecoveryJournal, error) {
	var j RecoveryJournal
	e := readPrivate(filepath.Join(dir, "recovery-v2.json"), &j)
	return j, e
}
func ReadRecoveryAuthorization(path string) (RecoveryAuthorization, error) {
	var a RecoveryAuthorization
	e := readPrivate(path, &a)
	return a, e
}
func writeRecovery(path string, j RecoveryJournal) error { return writeDurableJSON(path, j) }

// Run resumes exactly one existing maintenance operation. It never creates a
// maintenance barrier or alters the original binding/deadline/generation.
func (c *RecoveryCoordinator) Run(ctx context.Context, authorization RecoveryAuthorization, artifact RetainedArtifact) (string, error) {
	if c.Adapter == nil || !filepath.IsAbs(c.Directory) {
		return "", errors.New("recovery adapter and absolute directory required")
	}
	st, e := os.Lstat(c.Directory)
	if e != nil {
		return "", e
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return "", errors.New("recovery directory must be private")
	}
	unlock, e := lock(c.Directory)
	if e != nil {
		return "", e
	}
	defer unlock()
	original, e := ReadJournal(c.Directory)
	if e != nil {
		return "", e
	}
	if original.Generation == 0 {
		return "", errors.New("recovery requires acknowledged original generation")
	}
	// A drain grant never authorizes artifact recovery. Even a separately
	// signed v2 grant must wait for a retained paused drain receipt.
	if drain, err := ReadDrainJournal(c.Directory); err == nil {
		if drain.Protocol != DrainProtocol || drain.Phase != "paused" || drain.Evidence == nil || !paused(*drain.Evidence) || !reflect.DeepEqual(drain.Binding, original.Binding) || drain.Generation != original.Generation {
			return "", errors.New("drain has not reached a verified paused state")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	claims, e := DecodeRecoveryAuthorization(authorization, c.ObserverKey)
	if e != nil {
		return "", e
	}
	if !reflect.DeepEqual(claims.Binding, original.Binding) || claims.Generation != original.Generation {
		return "", errors.New("authorization does not match original operation")
	}
	outcome := TargetInstalled
	if claims.Action == "restore-predecessor" {
		outcome = PredecessorRestored
	}
	expectedVersion, expectedDigest := original.Binding.TargetVersion, original.Binding.ArtifactSHA256
	if outcome == PredecessorRestored {
		expectedVersion = original.Binding.FromVersion
		expectedDigest = original.Binding.PreviousArtifactSHA256
	}
	if artifact.Product != original.Binding.Product || artifact.Version != expectedVersion || artifact.SHA256 != expectedDigest {
		return "", errors.New("recovery artifact identity mismatch")
	}
	if e = verifyRetained(artifact, c.ReleaseKey); e != nil {
		return "", e
	}
	path := filepath.Join(c.Directory, "recovery-v2.json")
	j, e := ReadRecoveryJournal(c.Directory)
	if os.IsNotExist(e) {
		switch original.Phase {
		case "acknowledged", "applying", "applied", "completing":
		default:
			return "", errors.New("original phase cannot enter recovery")
		}
		j = RecoveryJournal{Protocol: RecoveryProtocol, Binding: original.Binding, Generation: original.Generation, Outcome: outcome, Artifact: artifact, Phase: "intent", AuthorizationHashes: map[string]string{}}
	} else if e != nil {
		return "", e
	}
	if j.Protocol != RecoveryProtocol || !reflect.DeepEqual(j.Binding, original.Binding) || j.Generation != original.Generation || j.Outcome != outcome || j.Artifact != artifact {
		return "", errors.New("conflicting retained recovery operation")
	}
	hash := sha256.Sum256([]byte(authorization.PayloadBase64 + "\n" + authorization.Signature))
	fingerprint := hex.EncodeToString(hash[:])
	if prior, ok := j.AuthorizationHashes[claims.AuthorizationID]; ok && prior != fingerprint {
		return "", errors.New("authorization ID reused with changed bytes")
	}
	if j.AuthorizationHashes == nil {
		return "", errors.New("missing authorization history")
	}
	changed := j.Authorization.PayloadBase64 != authorization.PayloadBase64 || j.Authorization.Signature != authorization.Signature
	if _, seen := j.AuthorizationHashes[claims.AuthorizationID]; seen && changed {
		return "", errors.New("superseded authorization ID cannot be replayed")
	}

	if changed && (j.Phase == "completing" || j.Phase == "completed" || j.Phase == "accepted") {
		old, e := DecodeRecoveryAuthorization(j.Authorization, c.ObserverKey)
		if e != nil {
			return "", e
		}
		r, e := c.Adapter.CallRecovery(ctx, "status", RecoveryRequest{Protocol: RecoveryProtocol, Binding: j.Binding, Generation: j.Generation, Outcome: j.Outcome, Authorization: j.Authorization, Artifact: j.Artifact, Health: j.Health})
		if e != nil {
			return "", e
		}
		if r.Protocol != RecoveryProtocol || !reflect.DeepEqual(r.Binding, j.Binding) || r.Generation != j.Generation || r.Outcome != j.Outcome || r.AuthorizationID != old.AuthorizationID {
			return "", errors.New("replacement reconciliation mismatch")
		}
		if r.Phase == "completed" {
			authorization = j.Authorization
			claims = old
			changed = false
			hash = sha256.Sum256([]byte(authorization.PayloadBase64 + "\n" + authorization.Signature))
			fingerprint = hex.EncodeToString(hash[:])
		} else if r.Phase != "paused" || j.Phase != "completing" {
			return "", errors.New("cannot replace authorization for completed operation")
		}
	}

	if changed && (!c.now().Before(claims.ExpiresAt) || c.now().Before(claims.IssuedAt)) {
		return "", errors.New("replacement authorization is not current")
	}
	j.AuthorizationHashes[claims.AuthorizationID] = fingerprint
	j.Authorization = authorization
	if e = writeRecovery(path, j); e != nil {
		return "", e
	}
	save := func(p string) error { j.Phase = p; return writeRecovery(path, j) }
	request := func() RecoveryRequest {
		return RecoveryRequest{Protocol: RecoveryProtocol, Binding: j.Binding, Generation: j.Generation, Outcome: j.Outcome, Authorization: j.Authorization, Artifact: j.Artifact, Health: j.Health}
	}
	call := func(action string, mutation bool) (RecoveryResponse, error) {
		if mutation && (c.now().Before(claims.IssuedAt) || !c.now().Before(claims.ExpiresAt)) {
			return RecoveryResponse{}, errors.New("recovery authorization expired or not yet valid")
		}
		r, e := c.Adapter.CallRecovery(ctx, action, request())
		if e != nil {
			return r, e
		}
		if r.Protocol != RecoveryProtocol || !reflect.DeepEqual(r.Binding, j.Binding) || r.Generation != j.Generation || r.Outcome != j.Outcome || r.AuthorizationID != claims.AuthorizationID {
			return r, errors.New("recovery acknowledgement identity mismatch")
		}
		if r.Phase != "paused" && r.Phase != "completed" {
			return r, errors.New("invalid recovery phase")
		}
		if action != "status" && action != "capabilities" && !c.now().Before(r.PermissionExpires) {
			return r, errors.New("recovery authority is stale")
		}
		return r, nil
	}
	cap, e := call("capabilities", false)
	if e != nil {
		return "", e
	}
	supported := false
	for _, v := range cap.Capabilities {
		if v == RecoveryProtocol {
			supported = true
		}
	}
	if !supported {
		return "", errors.New("recovery-v2 capability missing; no v1 fallback")
	}
	r, e := call("status", false)
	if e != nil {
		return "", e
	}
	expected := j.Binding
	expected.TargetVersion = expectedVersion
	expected.ArtifactSHA256 = expectedDigest
	if r.Phase == "completed" {
		if (j.Phase != "completing" && j.Phase != "completed" && j.Phase != "accepted") || !validHealth(j.Health, expected) {
			return "", errors.New("unexpected recovery completion")
		}
		if e = save("completed"); e != nil {
			return "", e
		}
	} else {
		if j.Phase == "completed" || j.Phase == "accepted" {
			return "", errors.New("observer completion regressed")
		}
		if j.Phase != "intent" && j.Phase != "recovering" && j.Phase != "recovered" && j.Phase != "completing" {
			return "", errors.New("unknown recovery phase")
		}
		// A fresh observer authorization acknowledgement checks revocation, drain,
		// watchdog, fencing and readiness before any recovery mutation.
		r, e = call("authorize-recovery", true)
		if e != nil {
			return "", e
		}
		if r.Phase != "paused" || r.PermissionExpires.After(claims.ExpiresAt) {
			return "", errors.New("invalid scoped recovery permission")
		}
		if j.Phase == "intent" || j.Phase == "recovering" {
			if e = save("recovering"); e != nil {
				return "", e
			}
			r, e = call("recover", true)
			if e != nil {
				return "", e
			}
			if r.Phase != "paused" {
				return "", errors.New("recovery escaped barrier")
			}
			if e = save("recovered"); e != nil {
				return "", e
			}
		}
		r, e = call("health", false)
		if e != nil {
			return "", e
		}
		if r.Phase != "paused" || !validHealth(r.Health, expected) {
			return "", errors.New("exact recovery outcome paused health not proven")
		}
		j.Health = r.Health
		if e = save("completing"); e != nil {
			return "", e
		}
		r, e = call("authorize-recovery", true)
		if e != nil {
			return "", e
		}
		if r.Phase != "paused" || r.PermissionExpires.After(claims.ExpiresAt) {
			return "", errors.New("invalid completion authorization")
		}
		r, e = call("complete", true)
		if e != nil {
			return "", e
		}
		if r.Phase != "completed" {
			return "", errors.New("recovery completion not acknowledged")
		}
		if e = save("completed"); e != nil {
			return "", e
		}
	}
	r, e = call("acceptance", false)
	if e != nil {
		return "", e
	}
	if r.Phase != "completed" || r.Acceptance == nil {
		return "", errors.New("missing recovery acceptance")
	}
	a := r.Acceptance
	h := a.Health
	if h.Version != expectedVersion || h.SHA256 != expectedDigest || !h.ManagementReady || !h.AuthorityValid || !a.ServiceHealthy {
		return "", errors.New("recovery installed identity/health mismatch")
	}
	switch h.Role {
	case "ACTIVE":
		if !a.ActiveIPOwned || !a.ProcessingAuthorityValid || h.ProcessingDisabled || h.TrafficDisabled {
			return "", errors.New("active recovery acceptance failed")
		}
	case "STBY":
		if a.ActiveIPOwned || a.ProcessingAuthorityValid || !h.ProcessingDisabled || !h.TrafficDisabled {
			return "", errors.New("standby recovery acceptance failed")
		}
	default:
		return "", errors.New("unknown recovery role")
	}
	j.Acceptance = a
	if e = save("accepted"); e != nil {
		return "", e
	}
	original.Health = j.Health
	original.Acceptance = a
	original.Phase = "accepted"
	if outcome == PredecessorRestored {
		original.Phase = "rolled-back"
	}
	if e = writeJournal(filepath.Join(c.Directory, "operation.json"), original); e != nil {
		return "", e
	}
	return outcome, nil
}

func ReadRetainedArtifact(path string) (RetainedArtifact, error) {
	var a RetainedArtifact
	e := readPrivate(path, &a)
	return a, e
}
