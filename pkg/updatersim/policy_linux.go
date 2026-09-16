//go:build linux

package updatersim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/policyauth"
)

const policyRequestPath = "/var/lib/siemcore-cascade-updater/policy-request.json"
const policyConsumerPath = "/usr/local/lib/siemcore-policy-consumer/1.0.0.1/consumer.py"

func policyConsumerAvailable() bool {
	for p := policyConsumerPath; ; p = filepath.Dir(p) {
		st, err := os.Lstat(p)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0022 != 0 {
			return false
		}
		owner, ok := st.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != 0 {
			return false
		}
		if p == "/" {
			break
		}
	}
	return true
}
func (s *Simulator) policyCycleLock() (func(), error) {
	// Same lock covers targeted and ordinary cycles, including policy staging.
	path := s.config.Simulation.StateFile + ".cycle-lock"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrCycleInProgress
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func (s *Simulator) applyPolicyGrant(ctx context.Context, offer *UpdateOffer) error {
	if len(offer.PolicyAuthorization) == 0 {
		if _, err := os.Lstat(policyRequestPath); err == nil {
			return errors.New("pending policy request blocks application")
		}
		return nil
	}
	if offer.Product != "siemcore" || s.config.Instance.ID != "siemcore-testing-01" || offer.UpdateGroup != "alpha" || !policyConsumerAvailable() {
		return errors.New("installed policy consumer and testing alpha scope required")
	}
	var envelope policyauth.Envelope
	if err := json.Unmarshal(offer.PolicyAuthorization, &envelope); err != nil {
		return err
	}
	p := envelope.Payload
	// The root consumer checks the actual protected policy; the unprivileged
	// client independently binds host, current version, signed artifact and time.
	expected := policyauth.Expected{Hostname: s.config.Instance.Hostname, FromVersion: offer.CurrentVersion, TargetVersion: offer.LatestVersion, ArtifactSHA256: offer.Checksum, ArtifactSignature: offer.Signature, SourceCommit: p.SourceCommit, Revision: p.PolicyRevision - 1, PolicyDigest: p.OldPolicySHA256}
	operation := policyauth.Operation{RequestPath: policyRequestPath, PublicKey: s.publicKey, Expected: expected, Invoke: func(ctx context.Context) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 800*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "sudo", "-n", "/usr/local/sbin/siemcore-apply-update", "apply").CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("root policy maintenance failed: %w", err)
		}
		return output, nil
	}}
	return operation.Run(ctx, offer.PolicyAuthorization, time.Now())
}
