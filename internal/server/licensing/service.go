package licensing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Service handles license business logic
type Service struct {
	repo         *Repository
	instanceRepo *InstanceRepository
}

// NewService creates a new licensing service
func NewService(db *database.DB) *Service {
	return &Service{
		repo:         NewRepository(db),
		instanceRepo: NewInstanceRepository(db),
	}
}

// GenerateLicenseKey generates a new license key
func GenerateLicenseKey(prefix string) string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	hex := strings.ToUpper(hex.EncodeToString(bytes))
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		prefix,
		hex[0:4],
		hex[4:8],
		hex[8:12],
		hex[12:16])
}

// GenerateAPIKey generates an API key for an instance
func GenerateAPIKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return "sk_inst_" + hex.EncodeToString(bytes)
}

// HashAPIKey creates a hash of an API key for storage
func HashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// CreateLicense creates a new license
func (s *Service) CreateLicense(ctx context.Context, req CreateLicenseRequest) (*types.License, error) {
	licenseKey := GenerateLicenseKey(req.Prefix)

	license := &types.License{
		LicenseKey:   licenseKey,
		CustomerID:   req.CustomerID,
		CustomerName: req.CustomerName,
		Type:         req.Type,
		Products:     req.Products,
		Features:     req.Features,
		Limits:       req.Limits,
		IssuedAt:     time.Now(),
		ExpiresAt:    req.ExpiresAt,
		IsActive:     true,
	}

	if err := s.repo.Create(ctx, license); err != nil {
		return nil, fmt.Errorf("failed to create license: %w", err)
	}

	return license, nil
}

// ActivateLicense activates a license and creates an instance
func (s *Service) ActivateLicense(ctx context.Context, req types.LicenseActivationRequest) (*types.LicenseActivationResponse, error) {
	// Get the license
	license, err := s.repo.GetByKey(ctx, req.LicenseKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get license: %w", err)
	}
	if license == nil {
		return &types.LicenseActivationResponse{
			Success: false,
			Error:   "invalid license key",
		}, nil
	}

	// Check if license is active and not expired
	if !license.IsActive {
		return &types.LicenseActivationResponse{
			Success: false,
			Error:   "license is not active",
		}, nil
	}
	if license.ExpiresAt.Before(time.Now()) {
		return &types.LicenseActivationResponse{
			Success: false,
			Error:   "license has expired",
		}, nil
	}

	// Check binding if set
	if license.BoundTo != "" && license.BoundTo != req.MachineID {
		return &types.LicenseActivationResponse{
			Success: false,
			Error:   "license is bound to a different machine",
		}, nil
	}

	// Generate instance ID and API key
	instanceID := generateInstanceID(license, req.Hostname)
	apiKey := GenerateAPIKey()
	apiKeyHash := HashAPIKey(apiKey)

	// Create or update instance
	instance := &types.Instance{
		ID:           uuid.New().String(),
		InstanceID:   instanceID,
		InstanceType: license.Type,
		Hostname:     req.Hostname,
		LicenseID:    license.ID,
		APIKeyHash:   apiKeyHash,
		Status:       "online",
	}

	existingInstance, err := s.instanceRepo.GetByInstanceID(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing instance: %w", err)
	}

	if existingInstance != nil {
		// Update existing instance
		existingInstance.Hostname = req.Hostname
		existingInstance.APIKeyHash = apiKeyHash
		existingInstance.Status = "online"
		if err := s.instanceRepo.Update(ctx, existingInstance); err != nil {
			return nil, fmt.Errorf("failed to update instance: %w", err)
		}
		instance = existingInstance
	} else {
		// Create new instance
		if err := s.instanceRepo.Create(ctx, instance); err != nil {
			return nil, fmt.Errorf("failed to create instance: %w", err)
		}
	}

	// Bind license to machine if not already bound
	if license.BoundTo == "" && req.MachineID != "" {
		license.BoundTo = req.MachineID
		if err := s.repo.Update(ctx, license); err != nil {
			// Non-fatal, just log
			fmt.Printf("Warning: failed to bind license to machine: %v\n", err)
		}
	}

	// Build install manifest
	installManifest := buildInstallManifest(license)

	return &types.LicenseActivationResponse{
		Success: true,
		License: license,
		Instance: &types.InstanceInfo{
			ID:     instance.ID,
			Name:   instance.InstanceID,
			APIKey: apiKey, // Return the plain API key once
		},
		Install: installManifest,
	}, nil
}

// ValidateLicense validates a license key
func (s *Service) ValidateLicense(ctx context.Context, licenseKey string) (*types.License, error) {
	license, err := s.repo.GetByKey(ctx, licenseKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get license: %w", err)
	}
	if license == nil {
		return nil, nil
	}

	return license, nil
}

// GetLicense retrieves a license by ID
func (s *Service) GetLicense(ctx context.Context, id string) (*types.License, error) {
	return s.repo.GetByID(ctx, id)
}

// ListLicenses retrieves all licenses
func (s *Service) ListLicenses(ctx context.Context) ([]types.License, error) {
	return s.repo.List(ctx)
}

// UpdateLicense updates a license
func (s *Service) UpdateLicense(ctx context.Context, license *types.License) error {
	return s.repo.Update(ctx, license)
}

// DeleteLicense deletes a license
func (s *Service) DeleteLicense(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// CreateLicenseRequest is the request to create a license
type CreateLicenseRequest struct {
	Prefix       string            `json:"prefix"`       // MYSOC or SIEM
	CustomerID   string            `json:"customer_id"`
	CustomerName string            `json:"customer_name"`
	Type         string            `json:"type"`         // mysoc-cloud, siemcore, siemcore-lite
	Products     []string          `json:"products"`
	Features     []string          `json:"features"`
	Limits       types.LicenseLimits `json:"limits"`
	ExpiresAt    time.Time         `json:"expires_at"`
}

// Helper functions

func generateInstanceID(license *types.License, hostname string) string {
	// Generate a readable instance ID
	prefix := strings.ToLower(license.Type)
	// Use hostname or generate random suffix
	if hostname != "" {
		// Clean hostname
		hostname = strings.ToLower(hostname)
		hostname = strings.ReplaceAll(hostname, ".", "-")
		return fmt.Sprintf("%s-%s", prefix, hostname)
	}
	// Generate random suffix
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(bytes))
}

func buildInstallManifest(license *types.License) *types.InstallManifest {
	var products []types.ProductInstall

	// Add products based on license type
	switch license.Type {
	case "siemcore", "siemcore-lite":
		products = []types.ProductInstall{
			{Name: "siemcore-api", Version: "latest", Channel: "stable"},
			{Name: "siemcore-collector", Version: "latest", Channel: "stable"},
			{Name: "siemcore-frontend", Version: "latest", Channel: "stable"},
			{Name: "detection-rules", Version: "latest", Channel: "stable"},
		}
	case "mysoc-cloud":
		products = []types.ProductInstall{
			{Name: "mysoc-api", Version: "latest", Channel: "stable"},
			{Name: "mysoc-frontend", Version: "latest", Channel: "stable"},
		}
	}

	// Also add any additional products from license
	for _, p := range license.Products {
		found := false
		for _, existing := range products {
			if existing.Name == p {
				found = true
				break
			}
		}
		if !found {
			products = append(products, types.ProductInstall{
				Name:    p,
				Version: "latest",
				Channel: "stable",
			})
		}
	}

	return &types.InstallManifest{
		Products:         products,
		ConfigTemplate:   getConfigTemplate(license.Type),
		SecurityBaseline: "cis-level1",
	}
}

func getConfigTemplate(licenseType string) string {
	switch licenseType {
	case "siemcore", "siemcore-lite":
		return "siemcore-standard"
	case "mysoc-cloud":
		return "mysoc-cloud"
	default:
		return "siemcore-standard"
	}
}

