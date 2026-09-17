package updatersim

import (
	"context"
	"fmt"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

func (s *Simulator) podCapabilities(ctx context.Context, product string) (string, []string, error) {
	p, ok := s.config.Product(product)
	if !ok {
		return "", nil, fmt.Errorf("product not configured")
	}
	role := p.DeploymentRole
	if p.ServerType != "" {
		mapped, err := serverTypeRole(p.ServerType)
		if err != nil {
			return "", nil, err
		}
		if role != "" && role != mapped {
			return "", nil, fmt.Errorf("server type conflicts with deployment role")
		}
		role = mapped
	}
	cfg := s.config.Simulation.Filesystem.PodMaintenance
	if product != "siemcore" || cfg == nil {
		return role, nil, nil
	}
	if role != "" && role != "pod-node" {
		return "", nil, fmt.Errorf("pod maintenance configuration conflicts with deployment role")
	}
	role = "pod-node"
	if !cfg.AdvertiseCapabilities {
		return role, nil, nil
	}
	q := podmaintenance.ReadinessRequest{Protocol: podmaintenance.ReadinessProtocol, PodID: cfg.PodID, NodeID: cfg.NodeID, UpdaterID: s.config.Instance.ID}
	r, err := (podmaintenance.CommandAdapter{Command: cfg.AdapterCommand, Timeout: s.config.Simulation.Filesystem.CommandTimeout.Duration}).Readiness(ctx, q)
	if err != nil {
		return role, nil, err
	}
	caps := []string{}
	for _, cap := range r.Capabilities {
		if cap == podmaintenance.RecoveryProtocol {
			if cfg.Recovery == nil {
				continue
			}
			if cfg.Recovery.Protocol != podmaintenance.RecoveryProtocol {
				return role, nil, fmt.Errorf("recovery protocol not configured")
			}
			if _, err := signing.ParsePublicKeyHex(cfg.Recovery.ObserverPublicKey); err != nil {
				return role, nil, err
			}
		}
		caps = append(caps, cap)
	}
	return role, caps, nil
}
func (s *Simulator) verifyOfferRequirements(ctx context.Context, o *UpdateOffer) error {
	if o.UpdaterRequirements == nil {
		return nil
	}
	role, caps, err := s.podCapabilities(ctx, o.Product)
	if err != nil {
		return err
	}
	return o.UpdaterRequirements.Check(role, s.retryBuild(), caps)
}
