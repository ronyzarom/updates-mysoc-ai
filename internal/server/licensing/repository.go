package licensing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Repository handles license database operations
type Repository struct {
	db *database.DB
}

// NewRepository creates a new license repository
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new license
func (r *Repository) Create(ctx context.Context, license *types.License) error {
	license.ID = uuid.New().String()
	license.CreatedAt = time.Now()
	license.UpdatedAt = time.Now()

	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO licenses (id, license_key, customer_id, customer_name, license_type, products, features, limits, issued_at, expires_at, bound_to, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, license.ID, license.LicenseKey, license.CustomerID, license.CustomerName, license.Type,
		license.Products, license.Features, license.Limits, license.IssuedAt, license.ExpiresAt,
		license.BoundTo, license.IsActive, license.CreatedAt, license.UpdatedAt)

	return err
}

// GetByKey retrieves a license by its key
func (r *Repository) GetByKey(ctx context.Context, licenseKey string) (*types.License, error) {
	var license types.License
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, license_key, customer_id, customer_name, license_type, products, features, limits, issued_at, expires_at, bound_to, is_active, created_at, updated_at
		FROM licenses
		WHERE license_key = $1
	`, licenseKey).Scan(
		&license.ID, &license.LicenseKey, &license.CustomerID, &license.CustomerName,
		&license.Type, &license.Products, &license.Features, &license.Limits,
		&license.IssuedAt, &license.ExpiresAt, &license.BoundTo, &license.IsActive,
		&license.CreatedAt, &license.UpdatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get license: %w", err)
	}

	return &license, nil
}

// GetByID retrieves a license by ID
func (r *Repository) GetByID(ctx context.Context, id string) (*types.License, error) {
	var license types.License
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, license_key, customer_id, customer_name, license_type, products, features, limits, issued_at, expires_at, bound_to, is_active, created_at, updated_at
		FROM licenses
		WHERE id = $1
	`, id).Scan(
		&license.ID, &license.LicenseKey, &license.CustomerID, &license.CustomerName,
		&license.Type, &license.Products, &license.Features, &license.Limits,
		&license.IssuedAt, &license.ExpiresAt, &license.BoundTo, &license.IsActive,
		&license.CreatedAt, &license.UpdatedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get license: %w", err)
	}

	return &license, nil
}

// List retrieves all licenses
func (r *Repository) List(ctx context.Context) ([]types.License, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, license_key, customer_id, customer_name, license_type, products, features, limits, issued_at, expires_at, bound_to, is_active, created_at, updated_at
		FROM licenses
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list licenses: %w", err)
	}
	defer rows.Close()

	var licenses []types.License
	for rows.Next() {
		var license types.License
		err := rows.Scan(
			&license.ID, &license.LicenseKey, &license.CustomerID, &license.CustomerName,
			&license.Type, &license.Products, &license.Features, &license.Limits,
			&license.IssuedAt, &license.ExpiresAt, &license.BoundTo, &license.IsActive,
			&license.CreatedAt, &license.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan license: %w", err)
		}
		licenses = append(licenses, license)
	}

	return licenses, nil
}

// Update updates a license
func (r *Repository) Update(ctx context.Context, license *types.License) error {
	license.UpdatedAt = time.Now()

	_, err := r.db.Pool.Exec(ctx, `
		UPDATE licenses
		SET customer_name = $2, products = $3, features = $4, limits = $5, expires_at = $6, bound_to = $7, is_active = $8, updated_at = $9
		WHERE id = $1
	`, license.ID, license.CustomerName, license.Products, license.Features, license.Limits,
		license.ExpiresAt, license.BoundTo, license.IsActive, license.UpdatedAt)

	return err
}

// Delete deletes a license
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM licenses WHERE id = $1`, id)
	return err
}

