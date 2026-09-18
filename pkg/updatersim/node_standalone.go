package updatersim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/google/uuid"
)

const nodeStandaloneProtocol = "pod-node-standalone-v1"

type NodeStandaloneOperation struct {
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
type standaloneRequest struct {
	Protocol    string      `json:"protocol"`
	OperationID string      `json:"operation_id,omitempty"`
	Target      *nodeTarget `json:"target,omitempty"`
}
type standaloneResponse struct {
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
	Mutation              string         `json:"mutation,omitempty"`
}

func invokeStandaloneAdapter(ctx context.Context, action string, request standaloneRequest) (standaloneResponse, error) {
	var response standaloneResponse
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
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", "/usr/local/sbin/siemcore-node-standalone", action)
	command.Stdin = bytes.NewReader(raw)
	command.WaitDelay = 5 * time.Second
	var output, diagnostic nodeBoundedBuffer
	command.Stdout = &output
	command.Stderr = &diagnostic
	if err = command.Run(); err != nil {
		return response, fmt.Errorf("Node adapter outcome uncertain: %w", err)
	}
	return parseStandaloneResponse(output.Bytes())
}

func parseStandaloneResponse(raw []byte) (standaloneResponse, error) {
	var response standaloneResponse
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
	if response.Protocol != nodeStandaloneProtocol || time.Since(response.ObservedAt) > 40*time.Second || time.Until(response.ObservedAt) > 5*time.Second {
		return response, fmt.Errorf("stale or incompatible Node adapter")
	}
	return response, nil
}
func (s *Simulator) standaloneCall(ctx context.Context, action string, q standaloneRequest) (standaloneResponse, error) {
	if s.standaloneAdapterCall != nil {
		return s.standaloneAdapterCall(ctx, action, q)
	}
	return invokeStandaloneAdapter(ctx, action, q)
}
func (s *Simulator) standaloneReady(ctx context.Context) (standaloneResponse, error) {
	p, ok := s.config.Product("siemcore")
	if !ok || p.ServerType != "pod-node" || (p.NodeID != "1" && p.NodeID != "2") {
		return standaloneResponse{}, fmt.Errorf("independent node identity required")
	}
	r, e := s.standaloneCall(ctx, "readiness", standaloneRequest{Protocol: nodeStandaloneProtocol})
	if e != nil {
		return standaloneResponse{}, e
	}
	if r.ServerType != "pod-node" || r.NodeID != p.NodeID || !nodeDigest(r.AdapterManifestSHA256) {
		return standaloneResponse{}, fmt.Errorf("protected Node component not ready")
	}
	if _, err := uuid.Parse(r.OperationID); err != nil {
		return standaloneResponse{}, fmt.Errorf("root standalone operation ID required")
	}
	return r, nil
}

func (s *Simulator) applyIndependentStandalone(ctx context.Context, u Update) error {
	operation := s.state.NodeStandaloneOperation
	if operation != nil && operation.Phase == "accepted" && operation.TargetVersion == u.FromVersion && operation.TargetVersion != u.ToVersion {
		return fmt.Errorf("routine standalone updates require a qualified executor")
	}
	if operation == nil {
		ready, err := s.standaloneReady(ctx)
		if err != nil {
			return err
		}
		if ready.TargetVersion != u.ToVersion || ready.ArtifactSHA256 != u.ArtifactSHA256 {
			return fmt.Errorf("standalone offer differs from protected transition")
		}
		found := false
		for _, c := range ready.Capabilities {
			if c == nodeStandaloneProtocol {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("standalone transition capability missing")
		}
		operation = &NodeStandaloneOperation{OperationID: ready.OperationID, TargetVersion: u.ToVersion, SHA256: u.ArtifactSHA256, Signature: u.ArtifactSignature, Phase: "prepared", FromVersion: u.FromVersion, ArtifactPath: u.ArtifactPath, Channel: u.Channel}
		s.state.NodeStandaloneOperation = operation
		if err = SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			return err
		}
	}
	if operation.FromVersion != u.FromVersion || operation.Channel != u.Channel || operation.TargetVersion != u.ToVersion || operation.SHA256 != u.ArtifactSHA256 || operation.Signature != u.ArtifactSignature {
		return fmt.Errorf("Node operation target changed; retained original transaction")
	}
	q := standaloneRequest{Protocol: nodeStandaloneProtocol, OperationID: operation.OperationID, Target: &nodeTarget{u.ToVersion, u.ArtifactSHA256, u.ArtifactSignature}}
	action := "apply"
	if operation.Phase != "prepared" && operation.Phase != "staged" {
		action = "status"
	}
	for step := 0; step < 3; step++ {
		r, err := s.standaloneCall(ctx, action, q)
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

// Reconcile local durable work before any network check or fresh offer admission.
// Recovery must still work when product management or the parent is unavailable.
func (s *Simulator) resumePendingIndependentStandalone(ctx context.Context) (bool, error) {
	op := s.state.NodeStandaloneOperation
	if op == nil {
		return false, nil
	}
	p, ok := s.config.Product("siemcore")
	if !ok || p.ServerType != "pod-node" || !s.config.Simulation.Filesystem.IndependentNodeStandalone {
		return true, fmt.Errorf("retained Node operation requires original executor")
	}
	if err := s.validateSiemCoreExecution(); err != nil {
		return true, err
	}
	if op.Phase == "accepted" && p.CurrentVersion == op.TargetVersion {
		return false, nil
	}
	u := Update{Product: "siemcore", FromVersion: op.FromVersion, ToVersion: op.TargetVersion, ArtifactPath: op.ArtifactPath, ArtifactSHA256: op.SHA256, ArtifactSignature: op.Signature, Channel: op.Channel}
	err := s.applyIndependentStandalone(ctx, u)
	message := ""
	if err != nil {
		message = err.Error()
	}
	s.recordAttempt(u, err == nil, message)
	return true, errors.Join(err, SaveState(s.config.Simulation.StateFile, s.state))
}
