package podmaintenance

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

const NextOperationProtocol = "pod-maintenance-next-operation-v1"
const NextOperationDomain = "mysoc-pod-next-operation-v1\n"

type NextOperationClaims struct {
	Protocol           string    `json:"protocol"`
	AuthorizationID    string    `json:"authorization_id"`
	PreviousBinding    Binding   `json:"previous_binding"`
	PreviousGeneration uint64    `json:"previous_generation"`
	PreviousOutcome    string    `json:"previous_outcome"`
	NextBinding        Binding   `json:"next_binding"`
	IssuedAt           time.Time `json:"issued_at"`
	ExpiresAt          time.Time `json:"expires_at"`
}
type NextOperationRequest struct {
	Protocol      string                `json:"protocol"`
	Authorization RecoveryAuthorization `json:"authorization"`
}
type NextOperationResponse struct {
	Protocol           string    `json:"protocol"`
	AuthorizationID    string    `json:"authorization_id"`
	PreviousBinding    Binding   `json:"previous_binding"`
	PreviousGeneration uint64    `json:"previous_generation"`
	PreviousOutcome    string    `json:"previous_outcome"`
	NextBinding        Binding   `json:"next_binding"`
	Allowed            bool      `json:"allowed"`
	ValidUntil         time.Time `json:"valid_until"`
}
type NextOperationAdapter interface {
	AuthorizeNext(context.Context, NextOperationRequest) (NextOperationResponse, error)
}

func (a CommandAdapter) AuthorizeNext(ctx context.Context, q NextOperationRequest) (NextOperationResponse, error) {
	var r NextOperationResponse
	e := a.invoke(ctx, "authorize-next-operation", q, &r)
	return r, e
}

type TransitionReceipt struct {
	DrainJournalSHA256    string                `json:"drain_journal_sha256,omitempty"`
	Protocol              string                `json:"protocol"`
	Authorization         RecoveryAuthorization `json:"authorization"`
	Claims                NextOperationClaims   `json:"claims"`
	Approval              NextOperationResponse `json:"approval"`
	OldJournalSHA256      string                `json:"old_journal_sha256"`
	RecoveryJournalSHA256 string                `json:"recovery_journal_sha256,omitempty"`
	Phase                 string                `json:"phase"`
}
type TransitionCoordinator struct {
	// AfterDurableCheckpoint is optional instrumentation for isolated crash qualification.
	// Production callers leave it nil; it never supplies authorization.
	AfterDurableCheckpoint func(string)
	Directory              string
	Adapter                NextOperationAdapter
	ObserverKey            ed25519.PublicKey
	ReleaseKey             ed25519.PublicKey
	Now                    func() time.Time
	afterPrepare           func() error
}

func (c *TransitionCoordinator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}
func parseNext(a RecoveryAuthorization, key ed25519.PublicKey) (NextOperationClaims, error) {
	var x NextOperationClaims
	raw, e := base64.StdEncoding.DecodeString(a.PayloadBase64)
	if e != nil || len(raw) > 32768 {
		return x, errors.New("invalid next-operation payload")
	}
	sig, e := base64.StdEncoding.DecodeString(a.Signature)
	if e != nil || len(key) != 32 || !ed25519.Verify(key, append([]byte(NextOperationDomain), raw...), sig) {
		return x, errors.New("invalid next-operation signature")
	}
	if e = decode(raw, &x); e != nil {
		return x, e
	}
	if x.Protocol != NextOperationProtocol || x.AuthorizationID == "" || x.PreviousGeneration == 0 || x.NextBinding.OperationID == x.PreviousBinding.OperationID || !x.ExpiresAt.After(x.IssuedAt) || x.ExpiresAt.Sub(x.IssuedAt) > 15*time.Minute {
		return x, errors.New("invalid next-operation scope")
	}
	if e = x.PreviousBinding.Validate(); e != nil {
		return x, e
	}
	if e = x.NextBinding.Validate(); e != nil {
		return x, e
	}
	p, n := x.PreviousBinding, x.NextBinding
	if p.PodID != n.PodID || p.NodeID != n.NodeID || p.UpdaterID != n.UpdaterID || p.Product != n.Product {
		return x, errors.New("next-operation identity changed")
	}
	v, d := p.TargetVersion, p.ArtifactSHA256
	if x.PreviousOutcome == PredecessorRestored {
		v, d = p.FromVersion, p.PreviousArtifactSHA256
	} else if x.PreviousOutcome != TargetInstalled {
		return x, errors.New("invalid prior outcome")
	}
	if n.FromVersion != v || n.PreviousArtifactSHA256 != d {
		return x, errors.New("next predecessor does not match accepted outcome")
	}
	return x, nil
}
func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (c *TransitionCoordinator) Advance(ctx context.Context, a RecoveryAuthorization) error {
	if c.Adapter == nil || !filepath.IsAbs(c.Directory) {
		return errors.New("transition configuration missing")
	}
	st, e := os.Lstat(c.Directory)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("transition directory not private")
	}
	unlock, e := lock(c.Directory)
	if e != nil {
		return e
	}
	defer unlock()
	claims, e := parseNext(a, c.ObserverKey)
	if e != nil {
		return e
	}
	receiptPath := filepath.Join(c.Directory, "next-operation.json")
	var receipt TransitionReceipt
	e = readPrivate(receiptPath, &receipt)
	if e == nil && receipt.Claims.AuthorizationID == claims.AuthorizationID && receipt.Authorization != a {
		return errors.New("authorization ID reused with changed next-operation bytes")
	}
	if e == nil && receipt.Authorization == a {
		return c.finalize(receiptPath, &receipt)
	}
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if e == nil && receipt.Phase != "activated" {
		return errors.New("unfinished archival transition")
	}
	if c.now().Before(claims.IssuedAt) || !c.now().Before(claims.ExpiresAt) || !c.now().Before(claims.NextBinding.Deadline) {
		return errors.New("next-operation authorization expired")
	}
	original, e := ReadJournal(c.Directory)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(original.Binding, claims.PreviousBinding) || original.Generation != claims.PreviousGeneration {
		return errors.New("terminal operation identity mismatch")
	}
	expectedPhase := "accepted"
	if claims.PreviousOutcome == PredecessorRestored {
		expectedPhase = "rolled-back"
	}
	if original.Phase != expectedPhase || original.Acceptance == nil {
		return errors.New("prior operation not accepted terminal outcome")
	}
	expected := original.Binding
	if claims.PreviousOutcome == PredecessorRestored {
		expected.TargetVersion = expected.FromVersion
		expected.ArtifactSHA256 = expected.PreviousArtifactSHA256
	}
	if !validHealth(original.Health, expected) || original.Acceptance.Health.Version != expected.TargetVersion || original.Acceptance.Health.SHA256 != expected.ArtifactSHA256 {
		return errors.New("terminal evidence identity invalid")
	}
	recovery, e := ReadRecoveryJournal(c.Directory)
	hasRecovery := e == nil
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if hasRecovery && (recovery.Phase != "accepted" || !reflect.DeepEqual(recovery.Binding, original.Binding) || recovery.Generation != original.Generation || recovery.Outcome != claims.PreviousOutcome) {
		return errors.New("recovery evidence not terminal")
	}
	if claims.PreviousOutcome == PredecessorRestored && !hasRecovery {
		return errors.New("rollback acceptance evidence missing")
	}
	drain, drainErr := ReadDrainJournal(c.Directory)
	hasDrain := drainErr == nil
	if drainErr != nil && !os.IsNotExist(drainErr) {
		return drainErr
	}
	if hasDrain && (drain.Protocol != DrainProtocol || drain.Phase != "paused" || drain.Binding != original.Binding || drain.Generation != original.Generation || drain.Evidence == nil || !paused(*drain.Evidence)) {
		return errors.New("drain evidence not terminal")
	}
	n := claims.NextBinding
	if e = verifyRetained(RetainedArtifact{Product: n.Product, Version: n.TargetVersion, SHA256: n.ArtifactSHA256, Signature: n.ArtifactSignature, Path: n.ArtifactPath}, c.ReleaseKey); e != nil {
		return e
	}
	approval, e := c.Adapter.AuthorizeNext(ctx, NextOperationRequest{Protocol: NextOperationProtocol, Authorization: a})
	if e != nil {
		return e
	}
	if approval.Protocol != NextOperationProtocol || approval.AuthorizationID != claims.AuthorizationID || !reflect.DeepEqual(approval.PreviousBinding, claims.PreviousBinding) || approval.PreviousGeneration != claims.PreviousGeneration || approval.PreviousOutcome != claims.PreviousOutcome || !reflect.DeepEqual(approval.NextBinding, n) || !approval.Allowed || !c.now().Before(approval.ValidUntil) || approval.ValidUntil.After(claims.ExpiresAt) {
		return errors.New("observer did not authorize exact next operation")
	}
	archive := filepath.Join(c.Directory, "archive", hashBytes([]byte(original.Binding.OperationID)))
	if e = os.MkdirAll(archive, 0700); e != nil {
		return e
	}
	st, e = os.Lstat(archive)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("archive not private directory")
	}
	saveEvidence := func(name string) (string, error) {
		raw, e := os.ReadFile(filepath.Join(c.Directory, name))
		if e != nil {
			return "", e
		}
		dest := filepath.Join(archive, name)
		if old, e := os.ReadFile(dest); e == nil {
			if string(old) != string(raw) {
				return "", errors.New("archived evidence differs")
			}
		} else if os.IsNotExist(e) {
			if e = writeDurableBytes(dest, raw); e != nil {
				return "", e
			}
			stored, e := os.ReadFile(dest)
			if e != nil {
				return "", e
			}
			return hashBytes(stored), nil
		} else {
			return "", e
		}
		return hashBytes(raw), nil
	}
	oldHash, e := saveEvidence("operation.json")
	if e != nil {
		return e
	}
	recoveryHash := ""
	if hasRecovery {
		recoveryHash, e = saveEvidence("recovery-v2.json")
		if e != nil {
			return e
		}
	}
	drainHash := ""
	if hasDrain {
		drainHash, e = saveEvidence("drain-v1.json")
		if e != nil {
			return e
		}
	}
	// Preserve and verify exact archived journal bytes.
	var oldJournal Journal
	if e = readPrivate(filepath.Join(archive, "operation.json"), &oldJournal); e != nil {
		return e
	}
	if !reflect.DeepEqual(oldJournal, original) {
		return errors.New("archival verification failed")
	}
	receipt = TransitionReceipt{Protocol: NextOperationProtocol, Authorization: a, Claims: claims, Approval: approval, OldJournalSHA256: oldHash, RecoveryJournalSHA256: recoveryHash, DrainJournalSHA256: drainHash, Phase: "prepared"}
	if e = writeDurableJSON(filepath.Join(archive, "transition.json"), receipt); e != nil {
		return e
	}
	if e = writeDurableJSON(receiptPath, receipt); e != nil {
		return e
	}
	if c.AfterDurableCheckpoint != nil {
		c.AfterDurableCheckpoint("prepared")
	}
	if c.afterPrepare != nil {
		if e = c.afterPrepare(); e != nil {
			return e
		}
	}
	return c.finalize(receiptPath, &receipt)
}
func (c *TransitionCoordinator) finalize(path string, r *TransitionReceipt) error {
	claims, e := parseNext(r.Authorization, c.ObserverKey)
	if e != nil {
		return e
	}
	if r.Protocol != NextOperationProtocol || !reflect.DeepEqual(claims, r.Claims) || (r.Phase != "prepared" && r.Phase != "activated") {
		return errors.New("invalid retained transition")
	}
	approval := r.Approval
	if !approval.Allowed || approval.Protocol != NextOperationProtocol || approval.AuthorizationID != claims.AuthorizationID || !reflect.DeepEqual(approval.PreviousBinding, claims.PreviousBinding) || approval.PreviousGeneration != claims.PreviousGeneration || approval.PreviousOutcome != claims.PreviousOutcome || !reflect.DeepEqual(approval.NextBinding, claims.NextBinding) || approval.ValidUntil.After(claims.ExpiresAt) || !approval.ValidUntil.After(claims.IssuedAt) {
		return errors.New("invalid retained transition approval")
	}
	archive := filepath.Join(c.Directory, "archive", hashBytes([]byte(claims.PreviousBinding.OperationID)))
	raw, e := os.ReadFile(filepath.Join(archive, "operation.json"))
	if e != nil || hashBytes(raw) != r.OldJournalSHA256 {
		return errors.New("archived operation evidence missing or changed")
	}
	if r.RecoveryJournalSHA256 != "" {
		raw, e = os.ReadFile(filepath.Join(archive, "recovery-v2.json"))
		if e != nil || hashBytes(raw) != r.RecoveryJournalSHA256 {
			return errors.New("archived recovery evidence missing or changed")
		}
	}
	if r.DrainJournalSHA256 != "" {
		raw, e = os.ReadFile(filepath.Join(archive, "drain-v1.json"))
		if e != nil || hashBytes(raw) != r.DrainJournalSHA256 {
			return errors.New("archived drain evidence missing or changed")
		}
	}
	current, e := ReadJournal(c.Directory)
	if e != nil {
		return e
	}
	if r.Phase == "activated" {
		if !reflect.DeepEqual(current.Binding, claims.NextBinding) {
			return errors.New("consumed transition cannot be replayed")
		}
		return nil
	}
	if !reflect.DeepEqual(current.Binding, claims.PreviousBinding) && !reflect.DeepEqual(current.Binding, claims.NextBinding) {
		return errors.New("journal changed during archival")
	}
	if reflect.DeepEqual(current.Binding, claims.PreviousBinding) {
		var archived Journal
		if e = readPrivate(filepath.Join(archive, "operation.json"), &archived); e != nil || !reflect.DeepEqual(current, archived) {
			return errors.New("live terminal evidence changed after preparation")
		}
	}
	// Local finalization resumes after crashes without re-authorizing product work.
	// The new operation must still satisfy its original deadline and fresh begin.
	if recovery, e := ReadRecoveryJournal(c.Directory); e == nil {
		archived := RecoveryJournal{}
		if e = readPrivate(filepath.Join(archive, "recovery-v2.json"), &archived); e != nil || !reflect.DeepEqual(recovery, archived) {
			return errors.New("live recovery evidence differs")
		}
		if e = os.Remove(filepath.Join(c.Directory, "recovery-v2.json")); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if drain, err := ReadDrainJournal(c.Directory); err == nil {
		var archived DrainJournal
		if r.DrainJournalSHA256 == "" {
			return errors.New("unarchived drain evidence")
		}
		if err = readPrivate(filepath.Join(archive, "drain-v1.json"), &archived); err != nil || !reflect.DeepEqual(drain, archived) {
			return errors.New("live drain evidence differs")
		}
		if err = os.Remove(filepath.Join(c.Directory, "drain-v1.json")); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if c.AfterDurableCheckpoint != nil {
		c.AfterDurableCheckpoint("recovery-archived")
	}
	if reflect.DeepEqual(current.Binding, claims.PreviousBinding) {
		if e = writeJournal(filepath.Join(c.Directory, "operation.json"), Journal{Binding: claims.NextBinding, Phase: "intent"}); e != nil {
			return e
		}
	}
	if c.AfterDurableCheckpoint != nil {
		c.AfterDurableCheckpoint("next-intent-written")
	}
	r.Phase = "activated"
	if e = writeDurableJSON(filepath.Join(archive, "transition.json"), r); e != nil {
		return e
	}
	return writeDurableJSON(path, r)
}
