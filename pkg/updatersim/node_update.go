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

const nodeUpdateProtocol = "pod-node-update-v1"

type NodeUpdateOperation struct {
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
type nodeTarget struct {
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}
type nodeRequest struct {
	Protocol    string      `json:"protocol"`
	OperationID string      `json:"operation_id,omitempty"`
	Target      *nodeTarget `json:"target,omitempty"`
}
type nodeResponse struct {
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
	NodeID                string         `json:"node_id,omitempty"`
	ServerType            string         `json:"server_type,omitempty"`
	Health                map[string]any `json:"health,omitempty"`
	Eligible              bool           `json:"eligible_for_security_upgrade,omitempty"`
	UICompliant           bool           `json:"ui_security_compliant,omitempty"`
}
type nodeBoundedBuffer struct{ bytes.Buffer }

func (b *nodeBoundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, fmt.Errorf("Node adapter output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func invokeNodeAdapter(ctx context.Context, action string, request nodeRequest) (nodeResponse, error) {
	var response nodeResponse
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
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", "/usr/local/sbin/siemcore-node-update", action)
	command.Stdin = bytes.NewReader(raw)
	command.WaitDelay = 5 * time.Second
	var output, diagnostic nodeBoundedBuffer
	command.Stdout = &output
	command.Stderr = &diagnostic
	if err = command.Run(); err != nil {
		return response, fmt.Errorf("Node adapter outcome uncertain: %w", err)
	}
	return parseNodeResponse(output.Bytes())
}

func parseNodeResponse(raw []byte) (nodeResponse, error) {
	var response nodeResponse
	if err := rejectObserverDuplicateKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return response, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return response, fmt.Errorf("invalid Node adapter response")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return response, fmt.Errorf("trailing Node adapter response")
	}
	if response.Protocol != nodeUpdateProtocol || time.Since(response.ObservedAt) > 40*time.Second || time.Until(response.ObservedAt) > 5*time.Second {
		return response, fmt.Errorf("stale or incompatible Node adapter")
	}
	return response, nil
}
func (s *Simulator) nodeCall(ctx context.Context, action string, q nodeRequest) (nodeResponse, error) {
	if s.nodeAdapterCall != nil {
		return s.nodeAdapterCall(ctx, action, q)
	}
	return invokeNodeAdapter(ctx, action, q)
}
func (s *Simulator) nodeUpdateReady(ctx context.Context) ([]string, error) {
	p, ok := s.config.Product("siemcore")
	if !ok || p.ServerType != "pod-node" || (p.NodeID != "1" && p.NodeID != "2") {
		return nil, fmt.Errorf("independent node identity required")
	}
	r, e := s.nodeCall(ctx, "readiness", nodeRequest{Protocol: nodeUpdateProtocol})
	if e != nil {
		return nil, e
	}
	if r.ServerType != "pod-node" || r.NodeID != p.NodeID || !nodeDigest(r.AdapterManifestSHA256) {
		return nil, fmt.Errorf("protected Node component not ready")
	}
	return r.Capabilities, nil
}

func (s *Simulator) applyIndependentNodeUpdate(ctx context.Context, u Update) error {
	operation := s.state.NodeUpdateOperation
	if operation != nil && operation.Phase == "accepted" && operation.TargetVersion == u.FromVersion && operation.TargetVersion != u.ToVersion {
		operation = nil // Root independently requires the previous accepted receipt.
	}
	if operation == nil {
		capabilities, err := s.nodeUpdateReady(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, c := range capabilities {
			if c == nodeUpdateProtocol {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("Node security upgrade capability missing")
		}
		operation = &NodeUpdateOperation{OperationID: uuid.NewString(), TargetVersion: u.ToVersion, SHA256: u.ArtifactSHA256, Signature: u.ArtifactSignature, Phase: "prepared", FromVersion: u.FromVersion, ArtifactPath: u.ArtifactPath, Channel: u.Channel}
		s.state.NodeUpdateOperation = operation
		if err = SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			return err
		}
	}
	if operation.FromVersion != u.FromVersion || operation.Channel != u.Channel || operation.TargetVersion != u.ToVersion || operation.SHA256 != u.ArtifactSHA256 || operation.Signature != u.ArtifactSignature {
		return fmt.Errorf("Node operation target changed; retained original transaction")
	}
	q := nodeRequest{Protocol: nodeUpdateProtocol, OperationID: operation.OperationID, Target: &nodeTarget{u.ToVersion, u.ArtifactSHA256, u.ArtifactSignature}}
	action := "apply"
	if operation.Phase != "prepared" && operation.Phase != "staged" {
		action = "status"
	}
	for step := 0; step < 3; step++ {
		r, err := s.nodeCall(ctx, action, q)
		if err != nil {
			operation.Phase = "uncertain"
			return errors.Join(err, SaveState(s.config.Simulation.StateFile, s.state))
		}
		if r.OperationID != operation.OperationID || (operation.OperationSHA256 != "" && operation.OperationSHA256 != r.OperationSHA256) || !nodeDigest(r.OperationSHA256) || r.TargetVersion != u.ToVersion || r.ArtifactSHA256 != u.ArtifactSHA256 {
			return fmt.Errorf("Node response binding mismatch")
		}
		operation.OperationSHA256 = r.OperationSHA256
		operation.Phase = r.Phase
		if err = SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			return err
		}
		switch r.Phase {
		case "accepted":
			return s.publishNodePointers(ctx, u)
		case "restored":
			return fmt.Errorf("Node target failed; predecessor restored")
		case "blocked":
			return fmt.Errorf("Node operation blocked: %s", r.ErrorCode)
		case "prepared", "staged":
			action = "apply"
		case "recovery_required", "restoring":
			action = "recover"
		case "switching", "verifying":
			action = "status"
		default:
			return fmt.Errorf("unknown Node transaction phase")
		}
	}
	return fmt.Errorf("Node transaction remains pending; reconcile retained operation")
}

// Only called after the protected root adapter proves acceptance. Product code
// is never executed through mutable updater-owned extraction during publication.
func (s *Simulator) publishNodePointers(ctx context.Context, u Update) error {
	e, ok := s.executor.(*FilesystemExecutor)
	if !ok {
		return fmt.Errorf("Node filesystem metadata executor required")
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
			return fmt.Errorf("Node accepted pointer metadata differs")
		}
		return nil
	}
	copyExecutor := *e
	copyExecutor.RestartCommand = nil
	copyExecutor.HealthCommand = nil
	return copyExecutor.Apply(ctx, u)
}

func nodeDigest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}

// Reconcile local durable work before any network check or fresh offer admission.
// Recovery must still work when product management or the parent is unavailable.
func (s *Simulator) resumePendingIndependentNode(ctx context.Context) (bool, error) {
	op := s.state.NodeUpdateOperation
	if op == nil {
		return false, nil
	}
	p, ok := s.config.Product("siemcore")
	if !ok || p.ServerType != "pod-node" || !s.config.Simulation.Filesystem.IndependentNodeUpdate {
		return true, fmt.Errorf("retained Node operation requires original executor")
	}
	if err := s.validateSiemCoreExecution(); err != nil {
		return true, err
	}
	if op.Phase == "accepted" && p.CurrentVersion == op.TargetVersion {
		return false, nil
	}
	u := Update{Product: "siemcore", FromVersion: op.FromVersion, ToVersion: op.TargetVersion, ArtifactPath: op.ArtifactPath, ArtifactSHA256: op.SHA256, ArtifactSignature: op.Signature, Channel: op.Channel}
	err := s.applyIndependentNodeUpdate(ctx, u)
	message := ""
	if err != nil {
		message = err.Error()
	}
	s.recordAttempt(u, err == nil, message)
	return true, errors.Join(err, SaveState(s.config.Simulation.StateFile, s.state))
}
