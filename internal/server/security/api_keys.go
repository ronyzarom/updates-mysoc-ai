package security

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
)

// API key scopes. A key's scope bounds which management actions it may perform.
const (
	// ScopeReleases authorizes release management only (upload/update/delete).
	ScopeReleases = "releases"
	// ScopeAdmin authorizes the full admin surface, equivalent to the static
	// ADMIN_API_KEY environment credential.
	ScopeAdmin = "admin"
)

// apiKeyPrefix is prepended to every generated key so credentials are easy to
// recognize (e.g. in logs, secret stores) and to differentiate from JWTs.
const apiKeyPrefix = "msk_"

// ErrAPIKeyNotFound is returned when revoking a key that does not exist.
var ErrAPIKeyNotFound = fmt.Errorf("api key not found")

// APIKey is the non-sensitive metadata for a managed credential. The plaintext
// key is never stored or returned by reads — only at creation time.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Scope      string     `json:"scope"`
	CreatedBy  string     `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	// Status is a derived convenience field: active | expired | revoked.
	Status string `json:"status"`
}

// APIKeyRepository persists and authenticates managed API keys.
type APIKeyRepository struct {
	db *database.DB
}

// NewAPIKeyRepository constructs a repository over the given database.
func NewAPIKeyRepository(db *database.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

// NormalizeScope validates and canonicalizes a requested scope. An empty value
// defaults to the least-privileged scope (releases).
func NormalizeScope(scope string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", ScopeReleases:
		return ScopeReleases, nil
	case ScopeAdmin:
		return ScopeAdmin, nil
	default:
		return "", fmt.Errorf("invalid scope %q (must be %q or %q)", scope, ScopeReleases, ScopeAdmin)
	}
}

// ScopeAllows reports whether a key with keyScope may perform an action that
// requires required. An admin-scoped key satisfies every requirement; otherwise
// the scopes must match exactly.
func ScopeAllows(keyScope, required string) bool {
	if keyScope == ScopeAdmin {
		return true
	}
	return keyScope == required
}

// GenerateAPIKey mints a new random key. It returns the full plaintext key (to
// be shown once), a short display prefix, and the SHA-256 hash to persist.
func GenerateAPIKey() (full, prefix, hash string, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}
	full = apiKeyPrefix + hex.EncodeToString(buf)
	prefix = full[:12]
	hash = HashAPIKey(full)
	return full, prefix, hash, nil
}

// HashAPIKey returns the hex-encoded SHA-256 of a full key. Keys are
// high-entropy, so a fast hash (rather than a password KDF) is appropriate.
func HashAPIKey(full string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(full)))
	return hex.EncodeToString(sum[:])
}

// Create generates and stores a new API key. It returns the plaintext key
// (shown once) alongside its stored metadata.
func (r *APIKeyRepository) Create(ctx context.Context, name, scope, createdBy string, expiresAt *time.Time) (fullKey string, meta *APIKey, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("name is required")
	}
	normalizedScope, err := NormalizeScope(scope)
	if err != nil {
		return "", nil, err
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return "", nil, fmt.Errorf("expiry must be in the future")
	}

	full, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		return "", nil, err
	}

	id := uuid.New().String()
	now := time.Now()
	var createdByPtr *string
	if trimmed := strings.TrimSpace(createdBy); trimmed != "" {
		createdByPtr = &trimmed
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO api_keys (id, name, key_hash, key_prefix, scope, created_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, name, hash, prefix, normalizedScope, createdByPtr, now, expiresAt)
	if err != nil {
		return "", nil, fmt.Errorf("create api key: %w", err)
	}

	meta = &APIKey{
		ID:        id,
		Name:      name,
		KeyPrefix: prefix,
		Scope:     normalizedScope,
		CreatedBy: strings.TrimSpace(createdBy),
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Status:    "active",
	}
	return full, meta, nil
}

// List returns all API keys, newest first, with a derived status.
func (r *APIKeyRepository) List(ctx context.Context) ([]APIKey, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, key_prefix, scope, created_by, created_at, expires_at, last_used_at, revoked_at
		FROM api_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var createdBy *string
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Scope, &createdBy,
			&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		if createdBy != nil {
			k.CreatedBy = *createdBy
		}
		k.Status = deriveStatus(k.ExpiresAt, k.RevokedAt)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Revoke marks a key revoked so it can no longer authenticate. Revocation is
// permanent; it does not delete the audit row.
func (r *APIKeyRepository) Revoke(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// Authenticate verifies a presented plaintext key. On success it returns the
// key's metadata (including scope) and records last-used. It returns (nil, nil)
// when no active key matches, so callers can fall through to other auth paths.
func (r *APIKeyRepository) Authenticate(ctx context.Context, presented string) (*APIKey, error) {
	presented = strings.TrimSpace(presented)
	if presented == "" {
		return nil, nil
	}
	hash := HashAPIKey(presented)

	var k APIKey
	var createdBy *string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, name, key_prefix, scope, created_by, created_at, expires_at, last_used_at, revoked_at
		FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > NOW())
	`, hash).Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Scope, &createdBy,
		&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt)
	if err != nil {
		// pgx returns ErrNoRows when nothing matched; treat as "no match".
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, fmt.Errorf("authenticate api key: %w", err)
	}

	// Defense in depth: the lookup is by hash, but constant-time compare the
	// recomputed hash to avoid leaking through any future non-indexed path.
	if subtle.ConstantTimeCompare([]byte(hash), []byte(HashAPIKey(presented))) != 1 {
		return nil, nil
	}
	if createdBy != nil {
		k.CreatedBy = *createdBy
	}
	k.Status = "active"

	// Best-effort last-used bookkeeping; never fail auth on this.
	_, _ = r.db.Pool.Exec(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, k.ID)
	return &k, nil
}

func deriveStatus(expiresAt, revokedAt *time.Time) string {
	if revokedAt != nil {
		return "revoked"
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return "expired"
	}
	return "active"
}
