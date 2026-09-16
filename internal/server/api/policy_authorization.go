package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/policyauth"
	"github.com/go-chi/chi/v5"
)

func policyGrantFile(instance string) string {
	sum := sha256.Sum256([]byte(instance))
	return hex.EncodeToString(sum[:]) + ".json"
}

// An admin explicitly authorizes one transition. Ordinary release upload never
// creates grants. Storage is separate from artifacts, with no schema migration.
func (s *Server) handleIssuePolicyAuthorization(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Payload          policyauth.Payload `json:"payload"`
		PreviousPolicy   []byte             `json:"previous_policy"`
		PreviousRevision int64              `json:"previous_revision"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		writeError(w, 400, "invalid authorization request")
		return
	}
	repo := licensing.NewInstanceRepository(s.db)
	instance, err := repo.GetByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 404, "instance not found")
		return
	}
	p := request.Payload
	if instance.UpdateGroup != "alpha" || instance.Status == "decommissioned" || instance.InstanceID != "siemcore-testing-01" {
		writeError(w, 403, "policy qualification restricted to testing alpha")
		return
	}
	release, err := s.releaseService().GetRelease(r.Context(), "siemcore", p.TargetVersion)
	if err != nil {
		writeError(w, 404, "release not found")
		return
	}
	allowed := false
	for _, group := range release.TargetGroups {
		if group == "alpha" {
			allowed = true
		}
	}
	if !allowed || release.Channel != "stable" || release.Signature == "" || release.Manifest.ArtifactKind != "" || len(release.Manifest.ArtifactVariants) > 0 {
		writeError(w, 409, "signed stable alpha release required")
		return
	}
	current := ""
	if instance.LastHeartbeatData != nil {
		for _, product := range instance.LastHeartbeatData.Products {
			if product.Name == "siemcore" {
				current = product.Version
			}
		}
	}
	expected := policyauth.Expected{Hostname: instance.Hostname, FromVersion: current, TargetVersion: release.Version, ArtifactSHA256: release.Checksum, ArtifactSignature: release.Signature, SourceCommit: p.SourceCommit, Policy: request.PreviousPolicy, Revision: request.PreviousRevision}
	if err = policyauth.Validate(p, expected, time.Now()); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	envelope, err := policyauth.Sign(p, s.signingKey)
	if err != nil {
		writeError(w, 503, "policy signing unavailable")
		return
	}
	raw, _ := json.Marshal(envelope)
	if _, err = s.storage.Save("policy-authorizations", release.Version, policyGrantFile(instance.InstanceID), strings.NewReader(string(raw))); err != nil {
		writeError(w, 500, "grant persistence failed")
		return
	}
	writeJSON(w, http.StatusCreated, envelope)
}
func (s *Server) policyGrant(instance, version string) json.RawMessage {
	reader, err := s.storage.Get("policy-authorizations", version, policyGrantFile(instance))
	if err != nil {
		return nil
	}
	defer reader.Close()
	raw, err := io.ReadAll(io.LimitReader(reader, policyauth.MaxBytes+1))
	if err != nil || len(raw) > policyauth.MaxBytes {
		return nil
	}
	var envelope policyauth.Envelope
	if json.Unmarshal(raw, &envelope) != nil || envelope.Payload.ExpiresAt <= time.Now().Unix() {
		return nil
	}
	return raw
}
