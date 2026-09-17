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

const DrainProtocol = "pod-maintenance-drain-recovery-v1"
const AckDrainProtocol = "pod-maintenance-ack-drain-recovery-v1"
const AckDrainSignatureDomain = "mysoc-pod-ack-drain-recovery-v1\n"

type AckBarrierProof struct {
	PreparedRevision   uint64 `json:"prepared_revision"`
	AssignmentRevision uint64 `json:"assignment_revision"`
}

func validAckProof(p *AckBarrierProof, generation uint64) bool {
	return p != nil && generation > 0 && p.PreparedRevision == generation && p.AssignmentRevision > 0
}

const DrainSignatureDomain = "mysoc-pod-drain-recovery-v1\n"

type CapturedOwner struct {
	NodeID             string `json:"node_id"`
	OwnerGeneration    uint64 `json:"owner_generation"`
	WatchdogGeneration uint64 `json:"watchdog_generation"`
}

func (o CapturedOwner) valid() bool {
	return (o.NodeID == "1" || o.NodeID == "2") && o.OwnerGeneration > 0 && o.WatchdogGeneration > 0
}

type DrainClaims struct {
	AckProof        *AckBarrierProof `json:"ack_proof,omitempty"`
	Protocol        string           `json:"protocol"`
	AuthorizationID string           `json:"authorization_id"`
	Binding         Binding          `json:"binding"`
	Generation      uint64           `json:"generation"`
	CapturedOwner   CapturedOwner    `json:"captured_owner"`
	Action          string           `json:"action"`
	IssuedAt        time.Time        `json:"issued_at"`
	ExpiresAt       time.Time        `json:"expires_at"`
}
type DrainRequest struct {
	AckProof      *AckBarrierProof       `json:"ack_proof,omitempty"`
	Protocol      string                 `json:"protocol"`
	Binding       Binding                `json:"binding"`
	Generation    uint64                 `json:"generation"`
	CapturedOwner *CapturedOwner         `json:"captured_owner,omitempty"`
	Authorization *RecoveryAuthorization `json:"authorization,omitempty"`
}
type DrainResponse struct {
	AckProof                *AckBarrierProof `json:"ack_proof,omitempty"`
	Protocol                string           `json:"protocol"`
	Binding                 Binding          `json:"binding"`
	Generation              uint64           `json:"generation"`
	CapturedOwner           CapturedOwner    `json:"captured_owner"`
	AuthorizationID         string           `json:"authorization_id,omitempty"`
	ObservedAt              time.Time        `json:"observed_at"`
	ValidUntil              time.Time        `json:"valid_until"`
	Phase                   string           `json:"phase"`
	OriginalDeadlineExpired *bool            `json:"original_deadline_expired,omitempty"`
	Authorized              *bool            `json:"authorized,omitempty"`
	PermissionExpires       *time.Time       `json:"permission_expires,omitempty"`
	ProcessingStopped       bool             `json:"processing_stopped,omitempty"`
	IngressStopped          bool             `json:"ingress_stopped,omitempty"`
	Quiescent               bool             `json:"quiescent,omitempty"`
	WatchdogRetired         bool             `json:"watchdog_retired,omitempty"`
	WatchdogExited          bool             `json:"watchdog_exited,omitempty"`
	LeaseReleased           bool             `json:"lease_released,omitempty"`
	NodeEvidenceObservedAt  *time.Time       `json:"node_evidence_observed_at,omitempty"`
	Reason                  string           `json:"reason,omitempty"`
}

// DrainAdapter is a trusted protected transport to the pinned, authenticated
// observer. JSON itself is not authentication. Non-2xx responses must be errors.
type DrainAdapter interface {
	CallDrain(context.Context, string, DrainRequest) (DrainResponse, error)
}

func (a CommandAdapter) CallDrain(ctx context.Context, action string, q DrainRequest) (DrainResponse, error) {
	var r DrainResponse
	if action != "status" && action != "authorize-drain-recovery" && action != "resume-drain" {
		return r, errors.New("invalid drain action")
	}
	if action != "status" && q.Generation == 0 {
		return r, errors.New("zero generation mutation refused")
	}
	e := a.invoke(ctx, action, q, &r)
	return r, e
}

type DrainGrantRecord struct {
	PayloadSHA256 string `json:"payload_sha256"`
	Signature     string `json:"signature"`
	Superseded    bool   `json:"superseded"`
}
type DrainJournal struct {
	AckProof              *AckBarrierProof            `json:"ack_proof,omitempty"`
	Protocol              string                      `json:"protocol"`
	Binding               Binding                     `json:"binding"`
	Generation            uint64                      `json:"generation"`
	CapturedOwner         CapturedOwner               `json:"captured_owner"`
	Phase                 string                      `json:"phase"`
	ActiveAuthorizationID string                      `json:"active_authorization_id,omitempty"`
	Grants                map[string]DrainGrantRecord `json:"grants"`
	Evidence              *DrainResponse              `json:"evidence,omitempty"`
}

func ReadDrainJournal(dir string) (DrainJournal, error) {
	var j DrainJournal
	e := readPrivate(filepath.Join(dir, "drain-v1.json"), &j)
	return j, e
}
func DecodeDrainAuthorization(a RecoveryAuthorization, key ed25519.PublicKey) (DrainClaims, string, error) {
	var c DrainClaims
	p, e := base64.StdEncoding.DecodeString(a.PayloadBase64)
	if e != nil {
		return c, "", e
	}
	if len(p) > 32768 {
		return c, "", errors.New("drain authorization too large")
	}
	if e = decode(p, &c); e != nil {
		return c, "", e
	}
	domain := DrainSignatureDomain
	if c.Protocol == AckDrainProtocol {
		domain = AckDrainSignatureDomain
	}
	sig, e := base64.StdEncoding.DecodeString(a.Signature)
	if e != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, append([]byte(domain), p...), sig) {
		return c, "", errors.New("invalid drain signature")
	}
	if e = decode(p, &c); e != nil {
		return c, "", e
	}
	if (c.Protocol != DrainProtocol && c.Protocol != AckDrainProtocol) || c.AuthorizationID == "" || c.Binding.Validate() != nil || c.Generation == 0 || !c.CapturedOwner.valid() || c.Action != "resume-drain" || c.IssuedAt.IsZero() || !c.ExpiresAt.After(c.IssuedAt) || c.ExpiresAt.Sub(c.IssuedAt) > 15*time.Minute {
		return c, "", errors.New("invalid drain authorization")
	}
	if c.Protocol == AckDrainProtocol {
		if c.Binding.Protocol != AckProtocol || !validAckProof(c.AckProof, c.Generation) {
			return c, "", errors.New("invalid ACK drain scope")
		}
	} else if c.Binding.Protocol != Protocol || c.AckProof != nil {
		return c, "", errors.New("legacy drain scope mismatch")
	}
	h := sha256.Sum256(p)
	return c, hex.EncodeToString(h[:]), nil
}

// DrainCoordinator never creates a maintenance operation, clears its barrier,
// executes an artifact, or rewrites the original deadline. It is opt-in only.
type DrainCoordinator struct {
	Protocol               string
	Directory              string
	Adapter                DrainAdapter
	ObserverKey            ed25519.PublicKey
	Now                    func() time.Time
	AfterDurableCheckpoint func(string)
}

func (c *DrainCoordinator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}
func (c *DrainCoordinator) save(j DrainJournal) error {
	if e := writeDurableJSON(filepath.Join(c.Directory, "drain-v1.json"), j); e != nil {
		return e
	}
	if c.AfterDurableCheckpoint != nil {
		c.AfterDurableCheckpoint(j.Phase)
	}
	return nil
}
func (c *DrainCoordinator) Discover(ctx context.Context) (DrainJournal, error) {
	return c.run(ctx, nil)
}
func (c *DrainCoordinator) Run(ctx context.Context, a RecoveryAuthorization) (DrainJournal, error) {
	return c.run(ctx, &a)
}
func (c *DrainCoordinator) validResponse(r DrainResponse, j DrainJournal) bool {
	now := c.now()
	return r.Protocol == j.Protocol && (j.Protocol != AckDrainProtocol || (validAckProof(r.AckProof, r.Generation) && (j.AckProof == nil || reflect.DeepEqual(r.AckProof, j.AckProof)))) && reflect.DeepEqual(r.Binding, j.Binding) && r.Generation > 0 && (j.Generation == 0 || r.Generation == j.Generation) && r.CapturedOwner.valid() && (j.Generation == 0 || r.CapturedOwner == j.CapturedOwner) && !r.ObservedAt.After(now) && now.Before(r.ValidUntil) && r.ValidUntil.After(r.ObservedAt) && r.ValidUntil.Sub(r.ObservedAt) <= 60*time.Second && (r.Phase == "draining" || r.Phase == "paused" || r.Phase == "blocked" || (j.Protocol == AckDrainProtocol && r.Phase == "prepared"))
}
func paused(r DrainResponse) bool {
	return r.Phase == "paused" && r.ProcessingStopped && r.IngressStopped && r.WatchdogRetired && r.WatchdogExited && r.LeaseReleased
}
func (c *DrainCoordinator) run(ctx context.Context, a *RecoveryAuthorization) (j DrainJournal, err error) {
	if c.Adapter == nil || !filepath.IsAbs(c.Directory) {
		return j, errors.New("protected drain transport and absolute directory required")
	}
	st, e := os.Lstat(c.Directory)
	if e != nil {
		return j, e
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return j, errors.New("private drain directory required")
	}
	unlock, e := lock(c.Directory)
	if e != nil {
		return j, e
	}
	defer unlock()
	original, e := ReadJournal(c.Directory)
	if e != nil {
		return j, e
	}
	if original.Binding.Validate() != nil {
		return j, errors.New("invalid original binding")
	}
	protocol := c.Protocol
	if protocol == "" {
		protocol = DrainProtocol
	}
	if protocol == AckDrainProtocol {
		if original.Binding.Protocol != AckProtocol || original.Generation == 0 || (original.Phase != "barrier-acknowledged" && original.Phase != "draining" && original.Phase != "acknowledged") {
			return j, errors.New("ACK drain requires durable original acknowledgment")
		}
	} else if protocol != DrainProtocol || original.Binding.Protocol != Protocol {
		return j, errors.New("explicit matching drain protocol required")
	}
	// Never reopen an operation after artifact execution or completion.
	if protocol == DrainProtocol && original.Phase != "intent" && original.Phase != "acknowledged" {
		return j, errors.New("original phase cannot enter drain recovery")
	}
	if _, e = os.Lstat(filepath.Join(c.Directory, "recovery-v2.json")); !os.IsNotExist(e) {
		return j, errors.New("v2 recovery already started or unavailable")
	}
	j, e = ReadDrainJournal(c.Directory)
	if os.IsNotExist(e) {
		j = DrainJournal{Protocol: protocol, Binding: original.Binding, Phase: "discovering", Grants: map[string]DrainGrantRecord{}}
		if e = c.save(j); e != nil {
			return j, e
		}
	} else if e != nil {
		return j, e
	}
	if j.Protocol != protocol || !reflect.DeepEqual(j.Binding, original.Binding) || (original.Generation != 0 && j.Generation != 0 && original.Generation != j.Generation) || (j.Generation != 0 && !j.CapturedOwner.valid()) || j.Grants == nil {
		return j, errors.New("retained drain scope mismatch")
	}
	if protocol == AckDrainProtocol && j.Generation != 0 && !validAckProof(j.AckProof, j.Generation) {
		return j, errors.New("retained ACK epoch proof missing or invalid")
	}
	q := DrainRequest{Protocol: protocol, Binding: j.Binding, Generation: j.Generation, AckProof: j.AckProof}
	// If the original begin reply was retained, constrain even initial lookup.
	if q.Generation == 0 {
		q.Generation = original.Generation
	}
	r, e := c.Adapter.CallDrain(ctx, "status", q)
	if e != nil {
		return j, e
	}
	if !c.validResponse(r, j) || (original.Generation != 0 && r.Generation != original.Generation) || r.Authorized != nil || r.PermissionExpires != nil || r.AuthorizationID != "" {
		return j, errors.New("invalid authenticated drain discovery")
	}
	j.Generation = r.Generation
	j.AckProof = r.AckProof
	j.CapturedOwner = r.CapturedOwner
	j.Evidence = &r
	j.Phase = r.Phase
	if r.Phase == "paused" && (!paused(r) || c.now().Sub(r.ObservedAt) > 5*time.Second) {
		return j, errors.New("paused evidence incomplete")
	}
	if protocol == AckDrainProtocol && r.Phase == "paused" && !freshAckPaused(r, c.now()) {
		return j, errors.New("ACK paused evidence incomplete or stale")
	}
	if e = c.save(j); e != nil {
		return j, e
	}
	// Sidecar proof is committed first; a crash before this write repeats status
	// with the discovered positive generation and can never allocate a new one.
	if original.Generation == 0 {
		original.Generation = j.Generation
		if e = writeJournal(filepath.Join(c.Directory, "operation.json"), original); e != nil {
			return j, e
		}
	}
	if a == nil || paused(r) {
		return j, nil
	}
	claims, hash, e := DecodeDrainAuthorization(*a, c.ObserverKey)
	if e != nil {
		return j, e
	}
	if claims.Protocol != protocol || !reflect.DeepEqual(claims.AckProof, j.AckProof) || !reflect.DeepEqual(claims.Binding, j.Binding) || claims.Generation != j.Generation || claims.CapturedOwner != j.CapturedOwner {
		return j, errors.New("drain grant scope mismatch")
	}
	validGrant := func() bool { now := c.now(); return !claims.IssuedAt.After(now) && now.Before(claims.ExpiresAt) }
	if !validGrant() {
		return j, errors.New("drain authorization expired or future")
	}
	old, exists := j.Grants[claims.AuthorizationID]
	if exists && (old.PayloadSHA256 != hash || old.Signature != a.Signature || old.Superseded) {
		return j, errors.New("drain authorization replay refused")
	}
	if j.ActiveAuthorizationID != "" && j.ActiveAuthorizationID != claims.AuthorizationID {
		old := j.Grants[j.ActiveAuthorizationID]
		old.Superseded = true
		j.Grants[j.ActiveAuthorizationID] = old
	}
	j.ActiveAuthorizationID = claims.AuthorizationID
	j.Grants[claims.AuthorizationID] = DrainGrantRecord{PayloadSHA256: hash, Signature: a.Signature}
	j.Phase = "authorizing"
	if e = c.save(j); e != nil {
		return j, e
	}
	q.Generation = j.Generation
	q.AckProof = j.AckProof
	q.CapturedOwner = &j.CapturedOwner
	q.Authorization = a
	// Observer checks its durable revocation ledger on EVERY mutation. Transport
	// errors (including generic HTTP 409) never imply permission or revocation.
	grantCtx, cancel := context.WithDeadline(ctx, claims.ExpiresAt)
	defer cancel()
	r, e = c.Adapter.CallDrain(grantCtx, "authorize-drain-recovery", q)
	if e != nil {
		return j, e
	}
	if !validGrant() || !c.validResponse(r, j) || r.AuthorizationID != claims.AuthorizationID || r.Authorized == nil || !*r.Authorized || r.PermissionExpires == nil || !c.now().Before(*r.PermissionExpires) || r.PermissionExpires.After(claims.ExpiresAt) || r.Phase == "blocked" {
		return j, errors.New("drain permission refused")
	}
	j.Phase = "resuming"
	j.Evidence = &r
	if e = c.save(j); e != nil {
		return j, e
	}
	if !validGrant() || !c.now().Before(*r.PermissionExpires) {
		return j, errors.New("drain permission expired")
	}
	r, e = c.Adapter.CallDrain(grantCtx, "resume-drain", q)
	if e != nil {
		return j, e
	}
	if !c.validResponse(r, j) || r.AuthorizationID != claims.AuthorizationID || !paused(r) || !r.Quiescent || r.NodeEvidenceObservedAt == nil || r.NodeEvidenceObservedAt.After(r.ObservedAt) || c.now().Sub(*r.NodeEvidenceObservedAt) > 5*time.Second {
		return j, errors.New("drain did not prove paused")
	}
	if protocol == AckDrainProtocol && (!validGrant() || !freshAckPaused(r, c.now())) {
		return j, errors.New("ACK drain result expired or incomplete")
	}
	j.Phase = "paused"
	j.Evidence = &r
	err = c.save(j)
	return
}

// Fresh terminal proof is required at discovery and explicit artifact recovery entry.
func freshAckPaused(r DrainResponse, now time.Time) bool {
	return r.Protocol == AckDrainProtocol && validAckProof(r.AckProof, r.Generation) && paused(r) && r.Quiescent && !r.ObservedAt.After(now) && now.Sub(r.ObservedAt) <= 5*time.Second && now.Before(r.ValidUntil) && r.ValidUntil.After(r.ObservedAt) && r.ValidUntil.Sub(r.ObservedAt) <= 60*time.Second && r.NodeEvidenceObservedAt != nil && !r.NodeEvidenceObservedAt.After(r.ObservedAt) && now.Sub(*r.NodeEvidenceObservedAt) <= 5*time.Second
}
func ackDrainHandoff(d DrainJournal, original Journal) bool {
	return d.Protocol == AckDrainProtocol && original.Binding.Protocol == AckProtocol && d.Phase == "paused" && d.Binding == original.Binding && d.Generation == original.Generation && validAckProof(d.AckProof, d.Generation) && d.Evidence != nil && d.Evidence.Binding == d.Binding && d.Evidence.Generation == d.Generation && d.CapturedOwner.valid() && d.Evidence.CapturedOwner == d.CapturedOwner && reflect.DeepEqual(d.Evidence.AckProof, d.AckProof) && freshAckPaused(*d.Evidence, d.Evidence.ObservedAt)
}
