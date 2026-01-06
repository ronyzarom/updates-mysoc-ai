package api

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/releases"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "ok",
		Version: "1.0.0",
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
	svc := releases.NewService(s.db, s.storage)
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

	svc := releases.NewService(s.db, s.storage)
	release, err := svc.CreateRelease(r.Context(), releases.CreateReleaseRequest{
		ProductName:  productName,
		Version:      version,
		Channel:      channel,
		ReleaseNotes: releaseNotes,
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

	svc := releases.NewService(s.db, s.storage)
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

	svc := releases.NewService(s.db, s.storage)
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

	svc := releases.NewService(s.db, s.storage)
	release, err := svc.GetRelease(r.Context(), product, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if release == nil {
		writeError(w, http.StatusNotFound, "release not found")
		return
	}

	writeJSON(w, http.StatusOK, release)
}

func (s *Server) handleDownloadRelease(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")

	svc := releases.NewService(s.db, s.storage)
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
		"status":   "uploaded",
		"product":  product,
		"version":  version,
		"filename": filename,
		"path":     path,
		"download_url": "/" + product + "/" + version + "/" + filename,
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
	svc := releases.NewService(s.db, s.storage)
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

	// Update instance heartbeat
	instanceRepo := licensing.NewInstanceRepository(s.db)
	if err := instanceRepo.UpdateHeartbeat(r.Context(), heartbeat.InstanceID, &heartbeat); err != nil {
		// Instance might not exist yet, that's ok
		// Just log and continue
	}

	// Check for available updates
	var updates []types.ReleaseInfo
	releaseSvc := releases.NewService(s.db, s.storage)

	for _, product := range heartbeat.Products {
		info, err := releaseSvc.GetLatestRelease(r.Context(), product.Name, product.Channel, product.Version)
		if err == nil && info != nil && info.UpdateAvailable {
			updates = append(updates, *info)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"updates": updates,
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

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo := licensing.NewInstanceRepository(s.db)
	instance, err := repo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if instance == nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}

	writeJSON(w, http.StatusOK, instance)
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo := licensing.NewInstanceRepository(s.db)
	if err := repo.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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
