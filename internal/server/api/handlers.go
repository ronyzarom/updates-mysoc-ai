package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/catalog"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/releases"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// getClientIP extracts client IP from request, respecting proxy headers
// NOTE: X-Forwarded-For is only trusted if you control the proxy chain
func getClientIP(r *http.Request) string {
	// Prefer X-Real-IP (typically set by nginx/proxy you control)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// X-Forwarded-For - take first IP (client), but be aware this is spoofable
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Fall back to direct connection
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // Return as-is if SplitHostPort fails
	}
	return ip
}

// Version is the running server build, injected from the main package at
// startup (populated via -ldflags). It defaults to "dev" for local builds.
var Version = "dev"

// requireLicense enforces a valid, active, unexpired X-License-Key on the
// agent data plane (heartbeat, update check/report, artifact download). On
// failure it writes the 401 response and returns nil.
func (s *Server) requireLicense(w http.ResponseWriter, r *http.Request) *types.License {
	licenseKey := strings.TrimSpace(r.Header.Get("X-License-Key"))
	if licenseKey == "" {
		writeError(w, http.StatusUnauthorized, "a valid X-License-Key is required")
		return nil
	}
	licenseSvc := licensing.NewService(s.db)
	license, err := licenseSvc.ValidateLicense(r.Context(), licenseKey)
	if err != nil || license == nil {
		writeError(w, http.StatusUnauthorized, "invalid license key")
		return nil
	}
	if !license.IsActive {
		writeError(w, http.StatusUnauthorized, "license is deactivated")
		return nil
	}
	if !license.ExpiresAt.IsZero() && license.ExpiresAt.Before(time.Now()) {
		writeError(w, http.StatusUnauthorized, "license has expired")
		return nil
	}
	return license
}

// licenseAuthorizesTier checks a claimed tier against the license. Only
// new-style keys (license.Product set) constrain the claim; legacy keys keep
// their pre-1.8.0 behavior.
func licenseAuthorizesTier(license *types.License, tier string) bool {
	if license == nil || license.Product == "" || tier == "" {
		return true
	}
	for _, p := range license.Products {
		if p == tier {
			return true
		}
	}
	return false
}

// Health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "ok",
		Version: Version,
	}
	writeJSON(w, http.StatusOK, resp)
}

// License handlers

func (s *Server) handleLicenseActivate(w http.ResponseWriter, r *http.Request) {
	var req types.LicenseActivationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.LicenseKey == "" {
		writeError(w, http.StatusBadRequest, "license_key is required")
		return
	}

	svc := licensing.NewService(s.db)
	resp, err := svc.ActivateLicense(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !resp.Success {
		writeJSON(w, http.StatusBadRequest, resp)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLicenseValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LicenseKey string `json:"license_key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := licensing.NewService(s.db)
	license, err := svc.ValidateLicense(r.Context(), req.LicenseKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if license == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"valid": false,
			"error": "license not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":      license.IsActive,
		"license":    license,
		"expires_at": license.ExpiresAt,
	})
}

// Release handlers

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	svc := s.releaseService()
	releaseList, err := svc.ListReleases(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, releaseList)
}

func (s *Server) handleUploadRelease(w http.ResponseWriter, r *http.Request) {
	// Check admin auth via middleware already

	// Parse multipart form (max 500MB)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	productName := r.FormValue("product")
	version := r.FormValue("version")
	channel := r.FormValue("channel")
	if channel == "" {
		channel = "stable"
	}
	releaseNotes := r.FormValue("release_notes")

	// Parse target groups (comma-separated or multiple form values)
	var targetGroups []string
	rawTargetGroups := r.FormValue("target_groups")
	if rawTargetGroups != "" {
		// Support comma-separated values: "alpha,beta,stable,production"
		for _, g := range strings.Split(rawTargetGroups, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				targetGroups = append(targetGroups, g)
			}
		}
	}
	// Also support multiple form values: target_groups[]=alpha&target_groups[]=beta
	if tgs := r.Form["target_groups[]"]; len(tgs) > 0 {
		targetGroups = append(targetGroups, tgs...)
	}

	// Validate target group names
	if len(targetGroups) > 0 {
		validGroups := map[string]bool{"alpha": true, "beta": true, "stable": true, "production": true}
		for _, g := range targetGroups {
			if !validGroups[g] {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid target group: %q (must be alpha, beta, stable, or production). Raw form value: %q", g, rawTargetGroups))
				return
			}
		}
	}

	// Log parsed groups for debugging
	fmt.Printf("[upload-release] product=%s version=%s raw_target_groups=%q parsed_groups=%v\n", productName, version, rawTargetGroups, targetGroups)

	if productName == "" || version == "" {
		writeError(w, http.StatusBadRequest, "product and version are required")
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("artifact")
	if err != nil {
		writeError(w, http.StatusBadRequest, "artifact file is required")
		return
	}
	defer file.Close()

	svc := s.releaseService()
	release, err := svc.CreateRelease(r.Context(), releases.CreateReleaseRequest{
		ProductName:  productName,
		Version:      version,
		Channel:      channel,
		ReleaseNotes: releaseNotes,
		TargetGroups: targetGroups,
		Filename:     header.Filename,
		FileSize:     header.Size,
		File:         file,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, release)
}

func (s *Server) handleListProductReleases(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")

	svc := s.releaseService()
	releaseList, err := svc.ListProductReleases(r.Context(), product)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, releaseList)
}

func (s *Server) handleGetLatestRelease(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = "stable"
	}
	currentVersion := r.URL.Query().Get("current_version")

	svc := s.releaseService()
	releaseInfo, err := svc.GetLatestRelease(r.Context(), product, channel, currentVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if releaseInfo == nil {
		writeError(w, http.StatusNotFound, "no releases found for product")
		return
	}

	writeJSON(w, http.StatusOK, releaseInfo)
}

func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")

	svc := s.releaseService()
	release, err := svc.GetRelease(r.Context(), product, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if release == nil {
		writeError(w, http.StatusNotFound, "release not found")
		return
	}

	// Serve a superset of the release object: the child-facing guide contract
	// guarantees flat "product" and "size" keys at every hop of the cascade
	// (relays add the same aliases), while the original field names remain for
	// consumers that parse the Release struct.
	body := map[string]interface{}{}
	if raw, err := json.Marshal(release); err == nil {
		_ = json.Unmarshal(raw, &body)
	}
	body["product"] = release.ProductName
	body["size"] = release.ArtifactSize

	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handleDownloadRelease(w http.ResponseWriter, r *http.Request) {
	// Artifact download has no instance identity, so it is gated by the global
	// allowlist entries only.
	if !s.enforceIPAllowed(w, r, "") {
		return
	}

	// Artifacts are not world-downloadable: a valid license is required (1.8.0).
	if s.requireLicense(w, r) == nil {
		return
	}

	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")

	svc := s.releaseService()
	release, err := svc.GetRelease(r.Context(), product, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if release == nil {
		writeError(w, http.StatusNotFound, "release not found")
		return
	}

	// Get the artifact file
	reader, err := s.storage.Get(product, version, filepath.Base(release.ArtifactPath))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get artifact")
		return
	}
	defer reader.Close()

	// Set headers for download
	filename := filepath.Base(release.ArtifactPath)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(release.ArtifactSize, 10))
	w.Header().Set("X-Checksum-SHA256", release.Checksum)
	if release.Signature != "" {
		w.Header().Set("X-Signature-Ed25519", release.Signature)
	}

	io.Copy(w, reader)
}

// handleUploadBinary handles uploading a specific binary file
// PUT /api/v1/releases/{product}/{version}/{filename}
// This allows uploading multiple architecture-specific binaries for a single release
func (s *Server) handleUploadBinary(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")
	filename := chi.URLParam(r, "filename")

	// Read the binary from request body
	defer r.Body.Close()

	// Save to storage
	path, err := s.storage.Save(product, version, filename, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save binary: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "uploaded",
		"product":      product,
		"version":      version,
		"filename":     filename,
		"path":         path,
		"download_url": "/" + product + "/" + version + "/" + filename,
	})
}

// UpdateReleaseTargetGroupsRequest is the request to update release target groups
type UpdateReleaseTargetGroupsRequest struct {
	TargetGroups []string `json:"target_groups"` // alpha, beta, stable, production
}

// UpdateReleaseRequest is the request to update a release (notes and/or groups)
type UpdateReleaseRequest struct {
	ReleaseNotes *string  `json:"release_notes,omitempty"`
	TargetGroups []string `json:"target_groups,omitempty"`
}

// handleUpdateRelease updates release notes and/or target groups
// PUT /api/v1/releases/{product}/{version}
func (s *Server) handleUpdateRelease(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")

	var req UpdateReleaseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate groups if provided
	if len(req.TargetGroups) > 0 {
		validGroups := map[string]bool{"alpha": true, "beta": true, "stable": true, "production": true}
		for _, g := range req.TargetGroups {
			if !validGroups[g] {
				writeError(w, http.StatusBadRequest, "invalid group: "+g)
				return
			}
		}
	}

	svc := s.releaseService()

	// Get the release first
	release, err := svc.GetRelease(r.Context(), product, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if release == nil {
		writeError(w, http.StatusNotFound, "release not found")
		return
	}

	// Update the release
	if err := svc.UpdateRelease(r.Context(), release.ID, req.ReleaseNotes, req.TargetGroups); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return updated info
	response := map[string]interface{}{
		"status":  "updated",
		"product": product,
		"version": version,
	}
	if req.ReleaseNotes != nil {
		response["release_notes"] = *req.ReleaseNotes
	}
	if len(req.TargetGroups) > 0 {
		response["target_groups"] = req.TargetGroups
	}

	writeJSON(w, http.StatusOK, response)
}

// handleUpdateReleaseTargetGroups updates which groups can receive a release
// PUT /api/v1/releases/{product}/{version}/target-groups
func (s *Server) handleUpdateReleaseTargetGroups(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")

	var req UpdateReleaseTargetGroupsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate groups
	validGroups := map[string]bool{"alpha": true, "beta": true, "stable": true, "production": true}
	for _, g := range req.TargetGroups {
		if !validGroups[g] {
			writeError(w, http.StatusBadRequest, "invalid group: "+g+" (must be alpha, beta, stable, or production)")
			return
		}
	}

	svc := s.releaseService()

	// Get the release first
	release, err := svc.GetRelease(r.Context(), product, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if release == nil {
		writeError(w, http.StatusNotFound, "release not found")
		return
	}

	// Update target groups
	if err := svc.UpdateReleaseTargetGroups(r.Context(), release.ID, req.TargetGroups); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "updated",
		"product":       product,
		"version":       version,
		"target_groups": req.TargetGroups,
	})
}

// handleDeleteRelease deletes a release
// DELETE /api/v1/releases/{product}/{version}
func (s *Server) handleDeleteRelease(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")

	svc := s.releaseService()

	// Get the release first to find its ID
	release, err := svc.GetRelease(r.Context(), product, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if release == nil {
		writeError(w, http.StatusNotFound, "release not found")
		return
	}

	// Delete the release
	if err := svc.DeleteRelease(r.Context(), release.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "deleted",
		"product": product,
		"version": version,
	})
}

// handleDirectDownload serves binaries directly at /{product}/{version}/{filename}
// This supports the Siemcore installer format:
// GET /siemcore/v1.5.0/siemcore-linux-amd64
// GET /siemcore/v1.5.0/siemcore-linux-arm64
func (s *Server) handleDirectDownload(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")
	filename := chi.URLParam(r, "filename")

	// Skip if this looks like an API route
	if product == "api" || product == "health" {
		http.NotFound(w, r)
		return
	}

	// Direct download has no instance identity, so it is gated by the global
	// allowlist entries only.
	if !s.enforceIPAllowed(w, r, "") {
		return
	}

	// Artifacts are not world-downloadable: a valid license is required (1.8.0).
	if s.requireLicense(w, r) == nil {
		return
	}

	// Check if file exists in storage
	if !s.storage.Exists(product, version, filename) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}

	// Get the artifact file
	reader, err := s.storage.Get(product, version, filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get artifact")
		return
	}
	defer reader.Close()

	// Try to get release info for checksum
	svc := s.releaseService()
	release, _ := svc.GetRelease(r.Context(), product, version)

	// Set headers for download
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Type", "application/octet-stream")

	// Add checksum if available from release record
	if release != nil && release.Checksum != "" {
		w.Header().Set("X-Checksum-SHA256", release.Checksum)
	}

	io.Copy(w, reader)
}

// Heartbeat handler

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var heartbeat types.Heartbeat
	if err := decodeJSON(r, &heartbeat); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if heartbeat.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "instance_id is required")
		return
	}

	// Enforce the allowlist-only IP control before touching any state.
	if !s.enforceIPAllowed(w, r, heartbeat.InstanceID) {
		return
	}

	// A valid license is mandatory on the agent plane (1.8.0).
	license := s.requireLicense(w, r)
	if license == nil {
		return
	}
	licenseID := license.ID

	// Normalize and validate the self-reported product hierarchy.
	heartbeat.ProductTier = catalog.Normalize(heartbeat.ProductTier)
	heartbeat.ParentInstanceID = strings.TrimSpace(heartbeat.ParentInstanceID)
	heartbeat.CustomerID = strings.TrimSpace(heartbeat.CustomerID)
	heartbeat.CustomerName = strings.TrimSpace(heartbeat.CustomerName)
	if heartbeat.ProductTier == "" && license.Product != "" {
		heartbeat.ProductTier = license.Product
	}
	if !licenseAuthorizesTier(license, heartbeat.ProductTier) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("license does not authorize product tier %q", heartbeat.ProductTier))
		return
	}
	if msg, ok := s.validateHierarchy(r.Context(), heartbeat.ProductTier, heartbeat.ParentInstanceID); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	// Upsert instance (create if not exists, update if exists)
	instanceRepo := licensing.NewInstanceRepository(s.db)
	clientIP := getClientIP(r)
	if err := instanceRepo.UpsertFromHeartbeat(r.Context(), heartbeat.InstanceID, &heartbeat, licenseID, clientIP); err != nil {
		// Log error but continue - don't fail the heartbeat
		// fmt.Printf("Warning: failed to upsert instance: %v\n", err)
	}

	// Ingest the cascaded fleet rollup (relays report their whole subtree).
	reportedNodes := 0
	if len(heartbeat.Children) > 0 {
		n, truncated, err := instanceRepo.UpsertReportedChildren(r.Context(), heartbeat.InstanceID, licenseID, heartbeat.Children)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid fleet rollup: %v", err))
			return
		}
		reportedNodes = n
		if truncated {
			// A truncated rollup is ingested (partial fleet view beats a
			// rejected one); surface it so an oversized subtree is visible.
			log.Printf("rollup from %s truncated at %d nodes", heartbeat.InstanceID, reportedNodes)
		}
	}

	// Ingest the change-only delta stream (Fleet Scalability 1.12). A relay
	// sends deltas OR the full rollup above, never both, so this is additive
	// and cannot double-count. The ack cursor tells the relay it may prune the
	// acknowledged entries from its forwarding queue.
	var ackCursor uint64
	if heartbeat.Delta != nil {
		n, err := instanceRepo.IngestDelta(r.Context(), heartbeat.InstanceID, licenseID, heartbeat.Delta)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid fleet delta: %v", err))
			return
		}
		reportedNodes += n
		ackCursor = heartbeat.Delta.Cursor
	}

	// Check for available updates
	var updates []types.ReleaseInfo
	releaseSvc := s.releaseService()

	for _, product := range heartbeat.Products {
		info, err := releaseSvc.GetLatestRelease(r.Context(), product.Name, product.Channel, product.Version)
		if err == nil && info != nil && info.UpdateAvailable {
			updates = append(updates, *info)
		}
	}

	resp := map[string]interface{}{
		"status":         "ok",
		"updates":        updates,
		"reported_nodes": reportedNodes,
	}
	if ackCursor > 0 {
		resp["ack_cursor"] = ackCursor
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleDecommission lets a directly-connected node announce its own clean
// removal (contract 1.11.0 Item B). Authenticated exactly like a heartbeat
// (license required); idempotent by design — repeat calls and unknown
// instance ids all ack, because an uninstall's goodbye must never fail.
// Cascaded children use the same endpoint on their relay instead; their mark
// arrives here through the rollup.
func (s *Server) handleDecommission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InstanceID string `json:"instance_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.InstanceID = strings.TrimSpace(req.InstanceID)
	if req.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "instance_id is required")
		return
	}
	if !s.enforceIPAllowed(w, r, req.InstanceID) {
		return
	}
	if s.requireLicense(w, r) == nil {
		return
	}

	instanceRepo := licensing.NewInstanceRepository(s.db)
	if err := instanceRepo.MarkDecommissioned(r.Context(), req.InstanceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record decommission")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "decommissioned"})
}

// Update check handler (siemcore-updater format)
// Accepts the format sent by siemcore-updater and creates/updates instances

type UpdateCheckRequest struct {
	InstanceID       string `json:"instance_id"`
	CurrentVersion   string `json:"current_version"`
	UpdaterVersion   string `json:"updater_version"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	Hostname         string `json:"hostname"`
	Channel          string `json:"channel"`
	ProductTier      string `json:"product_tier,omitempty"`       // canonical tier (defaults to {product} when it is a tier)
	ParentInstanceID string `json:"parent_instance_id,omitempty"` // parent node's instance_id
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	if product == "" {
		writeError(w, http.StatusBadRequest, "product is required")
		return
	}

	var req UpdateCheckRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "instance_id is required")
		return
	}

	// Enforce the allowlist-only IP control before touching any state.
	if !s.enforceIPAllowed(w, r, req.InstanceID) {
		return
	}

	// Resolve the product tier: prefer the explicit field, else fall back to the
	// {product} path segment when it names a canonical tier.
	productTier := catalog.Normalize(req.ProductTier)
	if productTier == "" && catalog.IsValidTier(catalog.Normalize(product)) {
		productTier = catalog.Normalize(product)
	}
	parentInstanceID := strings.TrimSpace(req.ParentInstanceID)
	if msg, ok := s.validateHierarchy(r.Context(), productTier, parentInstanceID); !ok {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	// A valid license is mandatory on the agent plane (1.8.0).
	license := s.requireLicense(w, r)
	if license == nil {
		return
	}
	licenseID := license.ID
	if !licenseAuthorizesTier(license, productTier) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("license does not authorize product tier %q", productTier))
		return
	}

	// Convert to heartbeat format for upsert
	heartbeat := &types.Heartbeat{
		InstanceID:       req.InstanceID,
		InstanceType:     product,
		ProductTier:      productTier,
		ParentInstanceID: parentInstanceID,
		Hostname:         req.Hostname,
		UpdaterVersion:   req.UpdaterVersion,
		Products: []types.ProductStatus{
			{
				Name:    product,
				Version: req.CurrentVersion,
				Channel: req.Channel,
			},
		},
	}

	// Record the check-in without impersonating a direct heartbeat: relays
	// forward child checks upstream, so this must not clobber rollup state.
	// At fleet scale the write is throttled to one per instance per interval
	// (unless the reported version changed) — checks in between touch nothing.
	instanceRepo := licensing.NewInstanceRepository(s.db)
	clientIP := getClientIP(r)
	if s.checkThrottle.shouldWrite(req.InstanceID, req.CurrentVersion) {
		if err := instanceRepo.TouchFromCheck(r.Context(), req.InstanceID, heartbeat, licenseID, clientIP); err != nil {
			// Log but don't fail
		}
	}

	// The auto-update toggle gates PRODUCT updates only. The updater's own
	// binary ("updater-<os>-<arch>" products) is fleet infrastructure and must
	// stay current even on nodes whose product installs are an explicit
	// opt-in, so self-update checks bypass the gate.
	selfUpdateProduct := strings.HasPrefix(product, "updater-")
	instance, _ := instanceRepo.GetByInstanceID(r.Context(), req.InstanceID)
	if instance != nil && !instance.AutoUpdateEnabled && !selfUpdateProduct {
		// Auto-update disabled - don't notify of updates
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"update_available": false,
			"current_version":  req.CurrentVersion,
			"auto_update":      false,
		})
		return
	}

	// Check for available updates
	channel := req.Channel
	if channel == "" {
		channel = "stable"
	}

	// Determine update group (default to stable if not set)
	updateGroup := "stable"
	if instance != nil && instance.UpdateGroup != "" {
		updateGroup = instance.UpdateGroup
	}

	// Memoize the release lookup per (product, channel, group) so a burst of
	// identical checks costs one DB query, not one each. The version compare
	// stays per-request.
	releaseSvc := s.releaseService()
	cacheKey := product + "|" + channel + "|" + updateGroup
	release, err := s.releaseCache.getOrLoad(cacheKey, func() (*types.Release, error) {
		return releaseSvc.HighestReleaseForGroup(r.Context(), product, channel, updateGroup)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check for updates")
		return
	}
	info := releaseSvc.ReleaseInfoFor(release, req.CurrentVersion)

	if info != nil && info.UpdateAvailable {
		// Build absolute download URL
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			if r.TLS != nil {
				scheme = "https"
			} else {
				scheme = "http"
			}
		}
		host := r.Host
		absoluteURL := fmt.Sprintf("%s://%s%s", scheme, host, info.DownloadURL)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"update_available": true,
			"latest_version":   info.LatestVersion,
			"download_url":     absoluteURL,
			"update_url":       absoluteURL, // Alias for compatibility with siemcore-updater
			"sha256":           info.Checksum,
			"signature":        info.Signature,
			"release_notes":    info.ReleaseNotes,
			"channel":          info.Channel,
			"update_group":     updateGroup,
		})
	} else {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"update_available": false,
			"current_version":  req.CurrentVersion,
			"update_group":     updateGroup,
		})
	}
}

// Update report handler - reports update success/failure
type UpdateReportRequest struct {
	InstanceID  string `json:"instance_id"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

func (s *Server) handleUpdateReport(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	if product == "" {
		writeError(w, http.StatusBadRequest, "product is required")
		return
	}

	var req UpdateReportRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Enforce the allowlist-only IP control (report carries the instance_id).
	if !s.enforceIPAllowed(w, r, req.InstanceID) {
		return
	}

	// A valid license is mandatory on the agent plane (1.8.0).
	if s.requireLicense(w, r) == nil {
		return
	}

	// For now, just acknowledge the report
	// TODO: Store update reports in database for analytics
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"message": "update report received",
	})
}

// Instance handlers (admin)

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	repo := licensing.NewInstanceRepository(s.db)
	instances, err := repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, instances)
}

// parseIntQueryParam parses an integer query parameter with bounds checking
func parseIntQueryParam(r *http.Request, name string, defaultVal, minVal, maxVal int) int {
	valStr := r.URL.Query().Get(name)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	if val < minVal {
		return minVal
	}
	if maxVal > 0 && val > maxVal {
		return maxVal
	}
	return val
}

// instanceFilterFromQuery reads the shared filter/search/sort query params.
func instanceFilterFromQuery(r *http.Request) licensing.InstanceListFilter {
	q := r.URL.Query()
	return licensing.InstanceListFilter{
		Status:   q.Get("status"),
		Tier:     q.Get("tier"),
		Customer: q.Get("customer"),
		Operator: q.Get("operator"),
		Parent:   q.Get("parent"),
		Search:   q.Get("search"),
		Sort:     q.Get("sort"),
		SortDir:  q.Get("dir"),
	}
}

func (s *Server) handleListInstancesPaged(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQueryParam(r, "limit", 50, 1, 200)
	offset := parseIntQueryParam(r, "offset", 0, 0, -1)

	repo := licensing.NewInstanceRepository(s.db)
	result, err := repo.ListPagedFiltered(r.Context(), instanceFilterFromQuery(r), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleFleetStats returns SQL-aggregated fleet counts (same filters as the
// paged list). No per-node rows leave the database.
func (s *Server) handleFleetStats(w http.ResponseWriter, r *http.Request) {
	repo := licensing.NewInstanceRepository(s.db)
	stats, err := repo.FleetStatsSummary(r.Context(), instanceFilterFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleCustomerDirectory returns the SQL-aggregated, paged, exceptions-first
// customer directory that replaces the full fleet tree at scale.
func (s *Server) handleCustomerDirectory(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQueryParam(r, "limit", 50, 1, 200)
	offset := parseIntQueryParam(r, "offset", 0, 0, -1)
	search := r.URL.Query().Get("search")
	sort := r.URL.Query().Get("sort")

	repo := licensing.NewInstanceRepository(s.db)
	items, total, err := repo.CustomerDirectory(r.Context(), search, sort, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"limit":  limit,
		"offset": offset,
		"total":  total,
	})
}

// handleSecurityStats returns the SQL-aggregated fleet security posture.
func (s *Server) handleSecurityStats(w http.ResponseWriter, r *http.Request) {
	repo := licensing.NewInstanceRepository(s.db)
	stats, err := repo.SecurityStatsSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleSecurityPaged returns reporting nodes' security posture, paged and
// worst-score-first.
func (s *Server) handleSecurityPaged(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQueryParam(r, "limit", 50, 1, 200)
	offset := parseIntQueryParam(r, "offset", 0, 0, -1)

	repo := licensing.NewInstanceRepository(s.db)
	rows, total, err := repo.ListSecurityPaged(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  rows,
		"limit":  limit,
		"offset": offset,
		"total":  total,
	})
}

// handleInstanceParents resolves a node's ancestor chain server-side.
func (s *Server) handleInstanceParents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo := licensing.NewInstanceRepository(s.db)

	// The path uses the UUID id like the other detail routes; resolve it to
	// the instance_id the chain walk keys on.
	node, err := repo.GetByID(r.Context(), id)
	if errors.Is(err, licensing.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	chain, err := repo.ParentChain(r.Context(), node.InstanceID)
	if err != nil && !errors.Is(err, licensing.ErrInstanceNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"parents": chain})
}

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo := licensing.NewInstanceRepository(s.db)
	instance, err := repo.GetByID(r.Context(), id)
	if errors.Is(err, licensing.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, instance)
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo := licensing.NewInstanceRepository(s.db)
	if err := repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, licensing.ErrInstanceNotFound) {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// UpdateInstanceRequest is the request to update instance settings
type UpdateInstanceRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	AutoUpdate  *bool   `json:"auto_update_enabled,omitempty"`
	UpdateGroup *string `json:"update_group,omitempty"`
}

func (s *Server) handleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateInstanceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	repo := licensing.NewInstanceRepository(s.db)

	// UpdateInstance now uses pk `id` directly, validates, and returns updated instance
	instance, err := repo.UpdateInstance(r.Context(), id, req.DisplayName, req.AutoUpdate, req.UpdateGroup)
	if errors.Is(err, licensing.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if errors.Is(err, licensing.ErrNoFieldsToUpdate) {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}
	if err != nil {
		// Validation errors from repo (e.g., invalid update_group) return here
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, instance)
}

// SetAutoUpdateRequest is the request to enable/disable auto-update
type SetAutoUpdateRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleSetAutoUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req SetAutoUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	repo := licensing.NewInstanceRepository(s.db)

	// SetAutoUpdate is now a thin wrapper around UpdateInstance
	if err := repo.SetAutoUpdate(r.Context(), id, req.Enabled); err != nil {
		if errors.Is(err, licensing.ErrInstanceNotFound) {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":              "updated",
		"auto_update_enabled": req.Enabled,
	})
}

// SetUpdateGroupRequest is the request to set an instance's update group
type SetUpdateGroupRequest struct {
	Group string `json:"group"` // alpha, beta, stable, production
}

func (s *Server) handleSetUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req SetUpdateGroupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	repo := licensing.NewInstanceRepository(s.db)

	// SetUpdateGroup is now a thin wrapper around UpdateInstance (validation in repo)
	if err := repo.SetUpdateGroup(r.Context(), id, req.Group); err != nil {
		if errors.Is(err, licensing.ErrInstanceNotFound) {
			writeError(w, http.StatusNotFound, "instance not found")
			return
		}
		// Validation errors (invalid group) come back as regular errors
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "updated",
		"update_group": req.Group,
	})
}

// Admin license handlers

func (s *Server) handleListLicenses(w http.ResponseWriter, r *http.Request) {
	svc := licensing.NewService(s.db)
	licenses, err := svc.ListLicenses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, licenses)
}

func (s *Server) handleCreateLicense(w http.ResponseWriter, r *http.Request) {
	var req licensing.CreateLicenseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CustomerID == "" || req.CustomerName == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "customer_id, customer_name, and type are required")
		return
	}

	// Ownership metadata: platform (mysoc-cloud) licenses belong to the SOC
	// operator itself; customer licenses point at their operator.
	req.OperatorID = strings.TrimSpace(req.OperatorID)
	req.ResellerID = strings.TrimSpace(req.ResellerID)
	req.ResellerName = strings.TrimSpace(req.ResellerName)
	if req.Type == "mysoc-cloud" && req.OperatorID == "" {
		req.OperatorID = req.CustomerID
	}

	// Set prefix based on type
	if req.Prefix == "" {
		if req.Type == "mysoc-cloud" {
			req.Prefix = "MYSOC"
		} else {
			req.Prefix = "SIEM"
		}
	}

	svc := licensing.NewService(s.db)
	license, err := svc.CreateLicense(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, license)
}

func (s *Server) handleGetLicense(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	svc := licensing.NewService(s.db)
	license, err := svc.GetLicense(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if license == nil {
		writeError(w, http.StatusNotFound, "license not found")
		return
	}

	writeJSON(w, http.StatusOK, license)
}

func (s *Server) handleUpdateLicense(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	svc := licensing.NewService(s.db)
	license, err := svc.GetLicense(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if license == nil {
		writeError(w, http.StatusNotFound, "license not found")
		return
	}

	// Decode updates
	var updates map[string]interface{}
	if err := decodeJSON(r, &updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Apply updates
	if name, ok := updates["customer_name"].(string); ok {
		license.CustomerName = name
	}
	if active, ok := updates["is_active"].(bool); ok {
		license.IsActive = active
	}
	if op, ok := updates["operator_id"].(string); ok {
		license.OperatorID = strings.TrimSpace(op)
	}
	if rid, ok := updates["reseller_id"].(string); ok {
		license.ResellerID = strings.TrimSpace(rid)
	}
	if rname, ok := updates["reseller_name"].(string); ok {
		license.ResellerName = strings.TrimSpace(rname)
	}

	if err := svc.UpdateLicense(r.Context(), license); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, license)
}

func (s *Server) handleDeleteLicense(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	svc := licensing.NewService(s.db)
	if err := svc.DeleteLicense(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
