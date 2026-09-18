package updatersim

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const observerUpdateProtocol = "observer-unlinked-update-v1"

type ObserverUpdateOperation struct {
	OperationID     string `json:"operation_id"`
	OperationSHA256 string `json:"operation_sha256,omitempty"`
	TargetVersion   string `json:"target_version"`
	SHA256          string `json:"sha256"`
	Signature       string `json:"signature"`
	Phase           string `json:"phase"`
	FromVersion     string `json:"from_version"`
	ArtifactPath    string `json:"artifact_path"`
	Channel         string `json:"channel"`
}
type observerTarget struct {
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}
type observerRequest struct {
	Protocol    string          `json:"protocol"`
	OperationID string          `json:"operation_id,omitempty"`
	Target      *observerTarget `json:"target,omitempty"`
}
type observerResponse struct {
	Protocol              string         `json:"protocol"`
	OperationID           string         `json:"operation_id,omitempty"`
	OperationSHA256       string         `json:"operation_sha256,omitempty"`
	Phase                 string         `json:"phase,omitempty"`
	ObservedAt            time.Time      `json:"observed_at"`
	TargetVersion         string         `json:"target_version,omitempty"`
	ArtifactSHA256        string         `json:"artifact_sha256,omitempty"`
	ErrorCode             string         `json:"error_code,omitempty"`
	Capabilities          []string       `json:"capabilities,omitempty"`
	AdapterManifestSHA256 string         `json:"adapter_manifest_sha256,omitempty"`
	ServerType            string         `json:"server_type,omitempty"`
	Health                map[string]any `json:"health,omitempty"`
	Eligible              bool           `json:"eligible_for_security_upgrade,omitempty"`
	UICompliant           bool           `json:"ui_security_compliant,omitempty"`
}
type observerBoundedBuffer struct{ bytes.Buffer }

func (b *observerBoundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, fmt.Errorf("Observer adapter output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func invokeObserverAdapter(ctx context.Context, action string, request observerRequest) (observerResponse, error) {
	var response observerResponse
	duration := 16 * time.Minute
	if action == "readiness" || action == "status" {
		duration = 40 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	raw, err := json.Marshal(request)
	if err != nil {
		return response, err
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", "/usr/local/sbin/siemcore-observer-update", action)
	command.Stdin = bytes.NewReader(raw)
	command.WaitDelay = 5 * time.Second
	var output, diagnostic observerBoundedBuffer
	command.Stdout = &output
	command.Stderr = &diagnostic
	if err = command.Run(); err != nil {
		return response, fmt.Errorf("Observer adapter outcome uncertain: %w", err)
	}
	return parseObserverResponse(output.Bytes())
}

func parseObserverResponse(raw []byte) (observerResponse, error) {
	var response observerResponse
	if err := rejectObserverDuplicateKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return response, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return response, fmt.Errorf("invalid Observer adapter response")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return response, fmt.Errorf("trailing Observer adapter response")
	}
	if response.Protocol != observerUpdateProtocol || time.Since(response.ObservedAt) > 40*time.Second || time.Until(response.ObservedAt) > 5*time.Second {
		return response, fmt.Errorf("stale or incompatible Observer adapter")
	}
	return response, nil
}
func (s *Simulator) observerCall(ctx context.Context, action string, q observerRequest) (observerResponse, error) {
	if s.observerAdapterCall != nil {
		return s.observerAdapterCall(ctx, action, q)
	}
	return invokeObserverAdapter(ctx, action, q)
}
func (s *Simulator) observerUpdateReady(ctx context.Context) ([]string, error) {
	r, e := s.observerCall(ctx, "readiness", observerRequest{Protocol: observerUpdateProtocol})
	if e != nil {
		return nil, e
	}
	if r.ServerType != "observer-unlinked" || !observerDigest(r.AdapterManifestSHA256) {
		return nil, fmt.Errorf("protected Observer component not ready")
	}
	return r.Capabilities, nil
}

func (s *Simulator) applyObserverSecurityUpdate(ctx context.Context, u Update) error {
	operation := s.state.ObserverUpdateOperation
	if operation != nil && operation.Phase == "accepted" && operation.TargetVersion == u.FromVersion && operation.TargetVersion != u.ToVersion {
		operation = nil // Root independently requires the previous accepted receipt.
	}
	if operation == nil {
		capabilities, err := s.observerUpdateReady(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, c := range capabilities {
			if c == observerUpdateProtocol {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("Observer security upgrade capability missing")
		}
		operation = &ObserverUpdateOperation{OperationID: uuid.NewString(), TargetVersion: u.ToVersion, SHA256: u.ArtifactSHA256, Signature: u.ArtifactSignature, Phase: "prepared", FromVersion: u.FromVersion, ArtifactPath: u.ArtifactPath, Channel: u.Channel}
		s.state.ObserverUpdateOperation = operation
		if err = SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			return err
		}
	}
	if operation.TargetVersion != u.ToVersion || operation.SHA256 != u.ArtifactSHA256 || operation.Signature != u.ArtifactSignature {
		return fmt.Errorf("Observer operation target changed; retained original transaction")
	}
	q := observerRequest{Protocol: observerUpdateProtocol, OperationID: operation.OperationID, Target: &observerTarget{u.ToVersion, u.ArtifactSHA256, u.ArtifactSignature}}
	action := "apply"
	if operation.Phase != "prepared" && operation.Phase != "staged" {
		action = "status"
	}
	for step := 0; step < 3; step++ {
		r, err := s.observerCall(ctx, action, q)
		if err != nil {
			operation.Phase = "uncertain"
			return errors.Join(err, SaveState(s.config.Simulation.StateFile, s.state))
		}
		if r.OperationID != operation.OperationID || (operation.OperationSHA256 != "" && operation.OperationSHA256 != r.OperationSHA256) || !observerDigest(r.OperationSHA256) || r.TargetVersion != u.ToVersion || r.ArtifactSHA256 != u.ArtifactSHA256 {
			return fmt.Errorf("Observer response binding mismatch")
		}
		operation.OperationSHA256 = r.OperationSHA256
		operation.Phase = r.Phase
		if err = SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			return err
		}
		switch r.Phase {
		case "accepted":
			return s.publishObserverPointers(ctx, u)
		case "restored":
			return fmt.Errorf("Observer target failed; predecessor restored")
		case "blocked":
			return fmt.Errorf("Observer operation blocked: %s", r.ErrorCode)
		case "prepared", "staged":
			action = "apply"
		case "recovery_required", "restoring":
			action = "recover"
		case "switching", "verifying":
			action = "status"
		default:
			return fmt.Errorf("unknown Observer transaction phase")
		}
	}
	return fmt.Errorf("Observer transaction remains pending; reconcile retained operation")
}

// Only called after the protected root adapter proves acceptance. Product code
// is never executed through mutable updater-owned extraction during publication.
func (s *Simulator) publishObserverPointers(ctx context.Context, u Update) error {
	e, ok := s.executor.(*FilesystemExecutor)
	if !ok {
		return fmt.Errorf("Observer filesystem metadata executor required")
	}
	target, err := filepath.EvalSymlinks(e.currentLink(u.Product))
	expected := e.versionDir(u.Product, u.ToVersion)
	if actual, resolveErr := filepath.EvalSymlinks(expected); resolveErr == nil && err == nil && actual == target {
		raw, readErr := os.ReadFile(filepath.Join(target, ".updater-release.json"))
		if readErr != nil {
			return readErr
		}
		var m releaseMetadata
		if json.Unmarshal(raw, &m) != nil || m.Version != u.ToVersion || m.Product != u.Product || m.SHA256 != u.ArtifactSHA256 || m.Signature != u.ArtifactSignature {
			return fmt.Errorf("Observer accepted pointer metadata differs")
		}
		return nil
	}
	copyExecutor := *e
	copyExecutor.RestartCommand = nil
	copyExecutor.HealthCommand = nil
	return copyExecutor.Apply(ctx, u)
}

func observerDigest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}

// JSON duplicate keys must not change the meaning of a privileged response.
func rejectObserverDuplicateKeys(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid Observer JSON key")
			}
			seen[name] = true
			if err = rejectObserverDuplicateKeys(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := rejectObserverDuplicateKeys(d); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid Observer JSON")
	}
	_, err = d.Token()
	return err
}

// Reconcile local durable work before any network check or fresh offer admission.
// Recovery must still work when product management or the parent is unavailable.
func (s *Simulator) resumePendingIndependentObserver(ctx context.Context) (bool, error) {
	op := s.state.ObserverUpdateOperation
	if op == nil {
		return false, nil
	}
	p, ok := s.config.Product("siemcore")
	if !ok || p.ServerType != "observer-unlinked" || !s.config.Simulation.Filesystem.ObserverUnlinkedUpdate {
		return true, fmt.Errorf("retained Observer operation requires original executor")
	}
	if err := s.validateSiemCoreExecution(); err != nil {
		return true, err
	}
	if op.Phase == "accepted" && p.CurrentVersion == op.TargetVersion {
		return false, nil
	}
	u := Update{Product: "siemcore", FromVersion: op.FromVersion, ToVersion: op.TargetVersion, ArtifactPath: op.ArtifactPath, ArtifactSHA256: op.SHA256, ArtifactSignature: op.Signature, Channel: op.Channel}
	err := s.applyObserverSecurityUpdate(ctx, u)
	message := ""
	if err != nil {
		message = err.Error()
	}
	s.recordAttempt(u, err == nil, message)
	return true, errors.Join(err, SaveState(s.config.Simulation.StateFile, s.state))
}
