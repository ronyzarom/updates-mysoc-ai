package releases

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

// Seal statuses recorded on every release at upload.
const (
	SealSealed     = "sealed"
	SealUnsealed   = "unsealed"
	SealInvalid    = "invalid"
	SealUnknownKey = "unknown_key"
)

// Issuers that seal their own releases.
const (
	IssuerMySoc    = "mysoc"
	IssuerSiemCore = "siemcore"
	IssuerSWF      = "swf"
	IssuerUpdates  = "updates"
	IssuerOther    = "other"
)

// MaxActiveKeysPerScope allows the current and the next key during rotation.
const MaxActiveKeysPerScope = 2

// maxStoredSealLen bounds what an uploader can make us persist for a seal we
// could not verify; a valid ed25519 seal is 88 base64 characters.
const maxStoredSealLen = 256

// trustedKeysLockKey serializes trusted-key mutations so the active-key limit
// holds under concurrent admin requests.
const trustedKeysLockKey int64 = 0x7472757374656b79

var (
	ErrTrustedKeyExists   = errors.New("trusted key already registered")
	ErrTrustedKeyNotFound = errors.New("trusted key not found")
	ErrTooManyActiveKeys  = fmt.Errorf("at most %d active keys are allowed per issuer scope; retire one first", MaxActiveKeysPerScope)
	ErrInvalidIssuer      = errors.New("issuer must be empty (all issuers), mysoc, siemcore, swf or updates")
	ErrInvalidPublicKey   = errors.New("public_key must be a hex-encoded 32-byte ed25519 key")
)

// TrustedKey is a public key the server accepts issuer seals from. An empty
// Issuer means the key is trusted for every issuer.
type TrustedKey struct {
	ID        string     `json:"id"`
	KeyID     string     `json:"key_id"`
	PublicKey string     `json:"public_key"`
	Issuer    string     `json:"issuer"`
	Label     string     `json:"label"`
	Status    string     `json:"status"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	RetiredAt *time.Time `json:"retired_at,omitempty"`
}

func (k TrustedKey) appliesTo(issuer string) bool {
	return k.Status == "active" && (k.Issuer == "" || k.Issuer == issuer)
}

// KeyID is the stable identifier issuers send as issuer_key_id: the first 16
// hex characters of SHA-256 over the raw 32-byte public key.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// IssuerFor maps a release product to the team that seals it.
func IssuerFor(product string) string {
	p := strings.ToLower(strings.TrimSpace(product))
	switch {
	case strings.HasPrefix(p, "updater-"), strings.HasPrefix(p, "relay-"),
		strings.HasPrefix(p, "mysoc-updater"), strings.HasPrefix(p, "siemcore-cascade-updater"):
		return IssuerUpdates
	case p == "mysoc" || strings.HasPrefix(p, "mysoc-"):
		return IssuerMySoc
	case p == "siemcore" || strings.HasPrefix(p, "siemcore-"):
		return IssuerSiemCore
	case p == "swf" || strings.HasPrefix(p, "swf-"):
		return IssuerSWF
	default:
		return IssuerOther
	}
}

func validIssuerScope(issuer string) bool {
	switch issuer {
	case "", IssuerMySoc, IssuerSiemCore, IssuerSWF, IssuerUpdates:
		return true
	}
	return false
}

// SealResult is the outcome of checking an uploaded issuer seal.
type SealResult struct {
	Status    string
	Issuer    string
	KeyID     string
	Signature string
}

// EvaluateSeal checks an issuer seal over the canonical release message
// against the trusted keys. It never fails: the outcome is a status.
func EvaluateSeal(keys []TrustedKey, product, version, checksum, seal, keyID string) SealResult {
	res := SealResult{Status: SealUnsealed, Issuer: IssuerFor(product)}
	seal = strings.TrimSpace(seal)
	keyID = strings.ToLower(strings.TrimSpace(keyID))
	if seal == "" {
		return res
	}
	res.KeyID = truncate(keyID, 64)
	res.Signature = truncate(seal, maxStoredSealLen)

	var candidates []TrustedKey
	for _, k := range keys {
		if !k.appliesTo(res.Issuer) {
			continue
		}
		if keyID == "" || k.KeyID == keyID {
			candidates = append(candidates, k)
		}
	}
	if len(candidates) == 0 {
		res.Status = SealUnknownKey
		return res
	}
	for _, k := range candidates {
		pub, err := signing.ParsePublicKeyHex(k.PublicKey)
		if err != nil {
			continue
		}
		if signing.Verify(pub, product, version, checksum, seal) == nil {
			res.Status = SealSealed
			res.KeyID = k.KeyID
			return res
		}
	}
	res.Status = SealInvalid
	return res
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// KeyRepository persists the trusted key registry.
type KeyRepository struct {
	db *database.DB
}

// NewKeyRepository creates a trusted key repository.
func NewKeyRepository(db *database.DB) *KeyRepository {
	return &KeyRepository{db: db}
}

const trustedKeyColumns = `id, key_id, public_key, COALESCE(issuer, ''), label, status, created_by, created_at, retired_at`

func scanTrustedKey(row pgx.Row) (TrustedKey, error) {
	var k TrustedKey
	err := row.Scan(&k.ID, &k.KeyID, &k.PublicKey, &k.Issuer, &k.Label, &k.Status, &k.CreatedBy, &k.CreatedAt, &k.RetiredAt)
	return k, err
}

func (r *KeyRepository) query(ctx context.Context, where string) ([]TrustedKey, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT `+trustedKeyColumns+` FROM trusted_keys `+where+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query trusted keys: %w", err)
	}
	defer rows.Close()
	keys := []TrustedKey{}
	for rows.Next() {
		k, err := scanTrustedKey(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trusted key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// List returns every registered key, newest first.
func (r *KeyRepository) List(ctx context.Context) ([]TrustedKey, error) {
	return r.query(ctx, "")
}

// Active returns the keys seals are currently checked against.
func (r *KeyRepository) Active(ctx context.Context) ([]TrustedKey, error) {
	return r.query(ctx, "WHERE status = 'active'")
}

// Add registers a public key, enforcing the active-key limit per issuer scope.
func (r *KeyRepository) Add(ctx context.Context, publicKeyHex, issuer, label, createdBy string) (*TrustedKey, error) {
	issuer = strings.ToLower(strings.TrimSpace(issuer))
	if !validIssuerScope(issuer) {
		return nil, ErrInvalidIssuer
	}
	pub, err := signing.ParsePublicKeyHex(publicKeyHex)
	if err != nil {
		return nil, ErrInvalidPublicKey
	}
	pubHex := hex.EncodeToString(pub)
	keyID := KeyID(pub)

	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, trustedKeysLockKey); err != nil {
		return nil, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trusted_keys WHERE key_id = $1 OR public_key = $2)`, keyID, pubHex).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrTrustedKeyExists
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM trusted_keys WHERE status = 'active' AND issuer IS NOT DISTINCT FROM NULLIF($1, '')`, issuer).Scan(&active); err != nil {
		return nil, err
	}
	if active >= MaxActiveKeysPerScope {
		return nil, ErrTooManyActiveKeys
	}
	k, err := scanTrustedKey(tx.QueryRow(ctx, `
		INSERT INTO trusted_keys (id, key_id, public_key, issuer, label, created_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		RETURNING `+trustedKeyColumns,
		uuid.NewString(), keyID, pubHex, issuer, strings.TrimSpace(label), createdBy))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &k, nil
}

// Retire stops accepting seals from a key. Retired keys are kept for audit.
func (r *KeyRepository) Retire(ctx context.Context, id string) (*TrustedKey, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrTrustedKeyNotFound
	}
	k, err := scanTrustedKey(r.db.Pool.QueryRow(ctx, `
		UPDATE trusted_keys SET status = 'retired', retired_at = COALESCE(retired_at, NOW())
		WHERE id = $1
		RETURNING `+trustedKeyColumns, id))
	if err == pgx.ErrNoRows {
		return nil, ErrTrustedKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// EnsureKey registers the server's own release key on first start so the
// registry begins with the key every updater already trusts. A key that an
// admin later retired stays retired.
func (r *KeyRepository) EnsureKey(ctx context.Context, pub ed25519.PublicKey, label string) (bool, error) {
	tag, err := r.db.Pool.Exec(ctx, `
		INSERT INTO trusted_keys (id, key_id, public_key, label, created_by)
		VALUES ($1, $2, $3, $4, 'system')
		ON CONFLICT DO NOTHING
	`, uuid.NewString(), KeyID(pub), hex.EncodeToString(pub), label)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
