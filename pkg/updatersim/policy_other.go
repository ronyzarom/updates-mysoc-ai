//go:build !linux

package updatersim

import (
	"context"
	"errors"
)

func policyConsumerAvailable() bool                   { return false }
func (s *Simulator) policyCycleLock() (func(), error) { return func() {}, nil }
func (s *Simulator) applyPolicyGrant(ctx context.Context, offer *UpdateOffer) error {
	if len(offer.PolicyAuthorization) > 0 {
		return errors.New("policy consumer requires Linux")
	}
	return nil
}
