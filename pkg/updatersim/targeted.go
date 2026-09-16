package updatersim

import (
	"context"
	"fmt"
	"net/url"
)

// RunTargetedRelease is an operator maintenance operation: it installs an
// explicitly selected signed release instead of the newest ring offer. The
// caller must authorize the target and exclude the ordinary daemon. Expected
// current version is a compare-and-set guard. It uses the same verification,
// filesystem rollback, health, durable state and reporting as normal updates.
// It does not change server policy or release targets.
func (s *Simulator) RunTargetedRelease(ctx context.Context, product, target, expected string) error {
	if !s.cycleMu.TryLock() {
		return ErrCycleInProgress
	}
	defer s.cycleMu.Unlock()
	releaseCycle, lockErr := s.policyCycleLock()
	if lockErr != nil {
		return lockErr
	}
	defer releaseCycle()
	p, ok := s.config.Product(product)
	if !ok || expected == "" || target == "" || p.CurrentVersion != expected {
		return fmt.Errorf("targeted release current-version guard failed")
	}
	if s.publicKey == nil || !s.config.Signing.Require {
		return fmt.Errorf("targeted release requires pinned signature verification")
	}
	if _, noop := s.executor.(NoopExecutor); noop {
		return fmt.Errorf("targeted release requires a real executor")
	}
	meta, err := s.client.GetReleaseMeta(ctx, product, target)
	if err != nil {
		return err
	}
	if meta.ProductName != product || meta.Version != target {
		return fmt.Errorf("targeted release metadata identity mismatch")
	}
	if _, err := s.SendHeartbeat(ctx); err != nil {
		return err
	}
	err = s.processOffer(ctx, ModeReal, &UpdateOffer{
		Product: product, CurrentVersion: expected, LatestVersion: target,
		UpdateAvailable: true, DownloadURL: "/api/v1/releases/" + url.PathEscape(product) + "/" + url.PathEscape(target) + "/download",
		Checksum: meta.Checksum, Signature: meta.Signature, Channel: meta.Channel,
		Source: "operator-targeted-maintenance",
	})
	if err != nil {
		return err
	}
	if s.state.ProductVersions[product] != target {
		return fmt.Errorf("targeted release was deferred; target %s is not installed", target)
	}
	_, err = s.SendHeartbeat(ctx)
	return err
}
