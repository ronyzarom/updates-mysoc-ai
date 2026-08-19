package licensing

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

var operatorIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// NormalizeOperatorID turns a display name into a slug candidate.
func NormalizeOperatorID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// ValidateOperatorID reports whether id is an acceptable operator slug.
func ValidateOperatorID(id string) bool {
	return operatorIDPattern.MatchString(id)
}

// OperatorWithKey pairs an operator with its active platform license.
type OperatorWithKey struct {
	types.Operator
	License *types.License `json:"license,omitempty"`
}

// CreateOperator creates the operator entity plus its platform license key.
// The returned license contains the full key; it is shown once and never again.
func (s *Service) CreateOperator(ctx context.Context, id, name string, expiresAt time.Time) (*OperatorWithKey, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" {
		id = NormalizeOperatorID(name)
	}
	if !ValidateOperatorID(id) {
		return nil, fmt.Errorf("invalid operator id %q: use 3-64 lowercase letters, digits, hyphens", id)
	}
	if name == "" {
		return nil, fmt.Errorf("operator name is required")
	}

	now := time.Now()
	tag, err := s.db.Pool.Exec(ctx, `
		INSERT INTO operators (id, name, is_active, created_at, updated_at)
		VALUES ($1, $2, TRUE, $3, $3)
		ON CONFLICT (id) DO NOTHING
	`, id, name, now)
	if err != nil {
		return nil, fmt.Errorf("create operator: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("operator %q already exists", id)
	}

	license, err := s.issuePlatformKey(ctx, id, name, expiresAt)
	if err != nil {
		return nil, err
	}
	return &OperatorWithKey{
		Operator: types.Operator{ID: id, Name: name, IsActive: true, CreatedAt: now, UpdatedAt: now},
		License:  license,
	}, nil
}

// RotateOperatorKey deactivates the operator's current platform key(s) and
// issues a fresh one. Returns the new license with the full key.
func (s *Service) RotateOperatorKey(ctx context.Context, operatorID string) (*types.License, error) {
	op, err := s.GetOperator(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, fmt.Errorf("operator %q not found", operatorID)
	}

	license, err := s.issuePlatformKey(ctx, op.ID, op.Name, platformKeyExpiry(ctx, s, op.ID))
	if err != nil {
		return nil, err
	}

	// Deactivate every other platform key for this operator (grace handling is
	// the caller's concern; old keys stop validating immediately).
	_, err = s.db.Pool.Exec(ctx, `
		UPDATE licenses SET is_active = FALSE, updated_at = NOW()
		WHERE operator_ref = $1 AND product = 'mysoc' AND id <> $2
	`, op.ID, license.ID)
	if err != nil {
		return nil, fmt.Errorf("deactivate previous platform keys: %w", err)
	}
	return license, nil
}

// platformKeyExpiry carries over the current key's expiry on rotation, or
// defaults to one year out.
func platformKeyExpiry(ctx context.Context, s *Service, operatorID string) time.Time {
	var expires time.Time
	err := s.db.Pool.QueryRow(ctx, `
		SELECT expires_at FROM licenses
		WHERE operator_ref = $1 AND product = 'mysoc' AND is_active = TRUE
		ORDER BY created_at DESC LIMIT 1
	`, operatorID).Scan(&expires)
	if err != nil || expires.Before(time.Now()) {
		return time.Now().AddDate(1, 0, 0)
	}
	return expires
}

func (s *Service) issuePlatformKey(ctx context.Context, operatorID, operatorName string, expiresAt time.Time) (*types.License, error) {
	if expiresAt.IsZero() {
		expiresAt = time.Now().AddDate(1, 0, 0)
	}
	license := &types.License{
		LicenseKey:   GenerateLicenseKey("MYSOC"),
		CustomerID:   operatorID,
		CustomerName: operatorName,
		Type:         "mysoc-cloud",
		Product:      "mysoc",
		OperatorRef:  operatorID,
		Products:     []string{"mysoc", "siemcore", "swf"},
		IssuedAt:     time.Now(),
		ExpiresAt:    expiresAt,
		IsActive:     true,
	}
	if err := s.repo.Create(ctx, license); err != nil {
		return nil, fmt.Errorf("issue platform key: %w", err)
	}
	return license, nil
}

// GetOperator fetches one operator entity.
func (s *Service) GetOperator(ctx context.Context, id string) (*types.Operator, error) {
	var op types.Operator
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, name, is_active, created_at, updated_at FROM operators WHERE id = $1
	`, id).Scan(&op.ID, &op.Name, &op.IsActive, &op.CreatedAt, &op.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get operator: %w", err)
	}
	return &op, nil
}

// ListOperators returns all operators with their active platform license
// (key included; callers must mask before returning it to browsers).
func (s *Service) ListOperators(ctx context.Context) ([]OperatorWithKey, error) {
	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, name, is_active, created_at, updated_at FROM operators ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list operators: %w", err)
	}
	defer rows.Close()

	var out []OperatorWithKey
	for rows.Next() {
		var op types.Operator
		if err := rows.Scan(&op.ID, &op.Name, &op.IsActive, &op.CreatedAt, &op.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan operator: %w", err)
		}
		out = append(out, OperatorWithKey{Operator: op})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		license, err := s.activePlatformKey(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].License = license
	}
	return out, nil
}

func (s *Service) activePlatformKey(ctx context.Context, operatorID string) (*types.License, error) {
	license, err := scanLicense(s.db.Pool.QueryRow(ctx,
		`SELECT `+selectLicenseCols+` FROM licenses
		 WHERE operator_ref = $1 AND product = 'mysoc' AND is_active = TRUE
		 ORDER BY created_at DESC LIMIT 1`, operatorID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get platform key: %w", err)
	}
	return license, nil
}

// SetOperatorActive toggles an operator and, when deactivating, its platform
// keys — cutting the whole operator's update channel.
func (s *Service) SetOperatorActive(ctx context.Context, id string, active bool) error {
	tag, err := s.db.Pool.Exec(ctx, `
		UPDATE operators SET is_active = $2, updated_at = NOW() WHERE id = $1
	`, id, active)
	if err != nil {
		return fmt.Errorf("update operator: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("operator %q not found", id)
	}
	_, err = s.db.Pool.Exec(ctx, `
		UPDATE licenses SET is_active = $2, updated_at = NOW()
		WHERE operator_ref = $1 AND product = 'mysoc'
		  AND ($2 = FALSE OR id = (
			SELECT id FROM licenses WHERE operator_ref = $1 AND product = 'mysoc'
			ORDER BY created_at DESC LIMIT 1))
	`, id, active)
	if err != nil {
		return fmt.Errorf("update operator keys: %w", err)
	}
	return nil
}
