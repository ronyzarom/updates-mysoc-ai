package updatersim

import (
	"context"
	"fmt"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"os"
	"time"
)

// resumePodDrain operates entirely from protected local configuration and the
// retained operation, before origin/offer work. It never reports product success.
func (s *Simulator) resumePodDrain(ctx context.Context, j podmaintenance.Journal) (bool, error) {
	cfg := s.config.Simulation.Filesystem.PodMaintenance
	prior, e := podmaintenance.ReadDrainJournal(cfg.JournalDirectory)
	if e != nil && !os.IsNotExist(e) {
		return true, e
	}
	hasPrior := e == nil
	// ACK-v2 prepares without draining. Its own coordinator must receive and
	// journal the prepared acknowledgment before any drain recovery is considered.
	if j.Binding.Protocol == podmaintenance.AckProtocol && !hasPrior && (cfg.Drain == nil || cfg.Drain.Protocol != podmaintenance.AckDrainProtocol) {
		return false, nil
	}
	if !hasPrior && j.Binding.Protocol == podmaintenance.AckProtocol && cfg.Drain != nil && cfg.Drain.Protocol == podmaintenance.AckDrainProtocol && cfg.Drain.AuthorizationFile == "" && time.Now().Before(j.Binding.Deadline) {
		return false, nil // A configured recovery transport does not preempt normal ACK-v2 progress.
	}
	if cfg.Drain == nil {
		if hasPrior {
			return true, fmt.Errorf("retained drain requires configured pinned drain transport")
		}
		return false, nil
	}
	if !hasPrior && j.Phase != "intent" && j.Phase != "acknowledged" && j.Phase != "barrier-acknowledged" && j.Phase != "draining" {
		return false, nil
	}
	d := cfg.Drain
	if d.Protocol != podmaintenance.DrainProtocol && d.Protocol != podmaintenance.AckDrainProtocol {
		return true, fmt.Errorf("unsupported drain protocol")
	}
	if e := s.validateSiemCoreExecution(); e != nil {
		return true, e
	}
	key, e := signing.ParsePublicKeyHex(d.ObserverPublicKey)
	if e != nil {
		return true, e
	}
	c := podmaintenance.DrainCoordinator{Protocol: d.Protocol, Directory: cfg.JournalDirectory, Adapter: podmaintenance.CommandAdapter{Command: d.AdapterCommand, Timeout: s.config.Simulation.Filesystem.CommandTimeout.Duration}, ObserverKey: key}
	// A v2 journal means a separately scoped recovery already began. Its own
	// coordinator checks the retained paused proof before doing anything.
	if hasPrior && prior.Phase == "paused" {
		if _, e := podmaintenance.ReadRecoveryJournal(cfg.JournalDirectory); e == nil {
			return false, nil
		} else if !os.IsNotExist(e) {
			return true, e
		}
	}
	if d.AuthorizationFile != "" {
		a, e := podmaintenance.ReadRecoveryAuthorization(d.AuthorizationFile)
		if e != nil {
			return true, e
		}
		prior, e = c.Run(ctx, a)
		if e != nil {
			return true, e
		}
	} else {
		prior, e = c.Discover(ctx)
		if e != nil {
			return true, e
		}
	}
	if prior.Phase == "paused" {
		if cfg.Recovery != nil && cfg.Recovery.AuthorizationFile != "" {
			original, err := podmaintenance.ReadJournal(cfg.JournalDirectory)
			if err != nil {
				return true, err
			}
			if handled, err := s.resumePodRecovery(ctx, original); handled {
				return true, err
			}
		}
		return true, fmt.Errorf("drain paused; separately authorized recovery-v2 handoff required")
	}
	return true, fmt.Errorf("drain %s; awaiting scoped drain authorization or verified termination", prior.Phase)
}
