package updatersim

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const maxProductRetryDelay = 15 * time.Minute

// ProductRetry is a durable attempt reservation. Persisting before execution
// prevents a process restart from turning a failed install into a tight loop.
type ProductRetry struct {
	TargetVersion  string    `json:"target_version"`
	ArtifactDigest string    `json:"artifact_digest"`
	UpdaterVersion string    `json:"updater_version"`
	Attempts       int       `json:"attempts"`
	NextRetryAt    time.Time `json:"next_retry_at"`
	LastError      string    `json:"last_error,omitempty"`
}

func (s *Simulator) retryNow() time.Time {
	if s.nowFn != nil {
		return s.nowFn().UTC()
	}
	return time.Now().UTC()
}

func retryDelay(attempt int) time.Duration {
	delay := time.Minute
	for i := 1; i < attempt && delay < maxProductRetryDelay; i++ {
		delay *= 2
	}
	if delay > maxProductRetryDelay {
		return maxProductRetryDelay
	}
	return delay
}

func (s *Simulator) retryBuild() string {
	if s.binaryVersion != "" {
		return s.binaryVersion
	}
	return s.config.Instance.UpdaterVersion
}

func (s *Simulator) processOffer(ctx context.Context, mode Mode, offer *UpdateOffer) error {
	if mode != ModeReal {
		return s.processOfferAttempt(ctx, mode, offer)
	}
	now := s.retryNow()
	if s.state.ProductRetries == nil {
		s.state.ProductRetries = make(map[string]*ProductRetry)
	}
	previous := s.state.ProductRetries[offer.Product]
	same := previous != nil && previous.TargetVersion == offer.LatestVersion && previous.ArtifactDigest == offer.Checksum && previous.UpdaterVersion == s.retryBuild()
	if same && now.Before(previous.NextRetryAt) {
		s.logger.Info("product retry deferred", "product", offer.Product, "target_version", offer.LatestVersion, "next_retry_at", previous.NextRetryAt, "attempt", previous.Attempts)
		return nil
	}
	count := 1
	if same {
		count = previous.Attempts + 1
		if count > 32 {
			count = 32
		}
	}
	retry := &ProductRetry{TargetVersion: offer.LatestVersion, ArtifactDigest: offer.Checksum, UpdaterVersion: s.retryBuild(), Attempts: count, NextRetryAt: now.Add(retryDelay(count))}
	s.state.ProductRetries[offer.Product] = retry
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		s.state.ProductRetries[offer.Product] = previous
		return fmt.Errorf("persist retry reservation before execution: %w", err)
	}
	priorAttempt := s.state.LastUpdateAttempt
	err := s.processOfferAttempt(ctx, mode, offer)
	// Reporting transport can fail after a successful application. Do not turn
	// that into another install attempt or a false application failure.
	if err == nil || s.state.ProductVersions[offer.Product] == offer.LatestVersion {
		delete(s.state.ProductRetries, offer.Product)
	} else {
		retry.LastError = err.Error()
		retry.NextRetryAt = s.retryNow().Add(retryDelay(count))
		// Also record failures that occurred before executor entry (e.g. download).
		if attempt := s.state.LastUpdateAttempt; attempt != nil && attempt != priorAttempt {
			// Preserve the executor's validation and rollback failure evidence.
			attempt.RetryAttempt = count
			deadline := retry.NextRetryAt
			attempt.NextRetryAt = &deadline
		} else {
			s.recordAttempt(Update{Product: offer.Product, FromVersion: offer.CurrentVersion, ToVersion: offer.LatestVersion, ArtifactSHA256: offer.Checksum, SelectedArtifactKind: offer.SelectedArtifactKind, DependencyValidation: offer.DependencyValidation}, false, err.Error())
		}
	}
	return errors.Join(err, SaveState(s.config.Simulation.StateFile, s.state))
}

// RetryProduct clears a matching delay in this running simulator. A caller must
// authorize operator access; it does not bypass signature or native checks.
// This is an in-process API, not permission to edit a live daemon's state file.
func (s *Simulator) RetryProduct(product, target, digest string) error {
	if !s.cycleMu.TryLock() {
		return ErrCycleInProgress
	}
	defer s.cycleMu.Unlock()
	r := s.state.ProductRetries[product]
	if r == nil || r.TargetVersion != target || r.ArtifactDigest != digest {
		return fmt.Errorf("retry identity does not match pending failure")
	}
	prior := r.NextRetryAt
	r.NextRetryAt = time.Time{}
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		r.NextRetryAt = prior
		return err
	}
	return nil
}
