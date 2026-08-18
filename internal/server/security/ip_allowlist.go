// Package security holds server-side controls that protect the updater
// data-plane channel, independent of licensing and release logic.
package security

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
)

// IPAllowlistEntry authorizes a source IP (or CIDR range) to reach the updater
// channel. An empty InstanceID is a global entry that applies to every instance
// and to instance-less endpoints such as artifact download.
type IPAllowlistEntry struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instance_id,omitempty"`
	CIDR       string    `json:"cidr"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// IPAllowlistRepository persists and evaluates allowlist entries.
type IPAllowlistRepository struct {
	db *database.DB
}

// NewIPAllowlistRepository constructs a repository over the given database.
func NewIPAllowlistRepository(db *database.DB) *IPAllowlistRepository {
	return &IPAllowlistRepository{db: db}
}

// NormalizeCIDR validates and canonicalizes an allowlist target. It accepts a
// bare host address (IPv4 or IPv6) or a CIDR range, and returns the string form
// that should be stored. A bare host is preserved as-is; a CIDR is returned in
// its masked canonical form (e.g. 10.0.0.5/8 -> 10.0.0.0/8).
func NormalizeCIDR(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("cidr is required")
	}
	if strings.Contains(trimmed, "/") {
		_, network, err := net.ParseCIDR(trimmed)
		if err != nil {
			return "", fmt.Errorf("invalid CIDR %q: %w", value, err)
		}
		return network.String(), nil
	}
	if ip := net.ParseIP(trimmed); ip != nil {
		return trimmed, nil
	}
	return "", fmt.Errorf("invalid IP or CIDR %q", value)
}

// matches reports whether ip falls within the stored target (host or CIDR).
func matches(target string, ip net.IP) bool {
	if ip == nil {
		return false
	}
	if strings.Contains(target, "/") {
		_, network, err := net.ParseCIDR(target)
		if err != nil {
			return false
		}
		return network.Contains(ip)
	}
	stored := net.ParseIP(target)
	return stored != nil && stored.Equal(ip)
}

// List returns every allowlist entry, global entries first, newest last within
// each scope.
func (r *IPAllowlistRepository) List(ctx context.Context) ([]IPAllowlistEntry, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, instance_id, cidr, note, created_at
		FROM ip_allowlist
		ORDER BY (instance_id IS NOT NULL), instance_id, created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list ip allowlist: %w", err)
	}
	defer rows.Close()

	var entries []IPAllowlistEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// Create validates the CIDR and inserts a new entry. An empty instanceID stores
// a global entry.
func (r *IPAllowlistRepository) Create(ctx context.Context, instanceID, cidr, note string) (*IPAllowlistEntry, error) {
	normalized, err := NormalizeCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var instancePtr *string
	if trimmed := strings.TrimSpace(instanceID); trimmed != "" {
		instancePtr = &trimmed
	}

	id := uuid.New().String()
	now := time.Now()
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO ip_allowlist (id, instance_id, cidr, note, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, id, instancePtr, normalized, note, now)
	if err != nil {
		return nil, fmt.Errorf("create ip allowlist entry: %w", err)
	}

	entry := &IPAllowlistEntry{ID: id, CIDR: normalized, Note: note, CreatedAt: now}
	if instancePtr != nil {
		entry.InstanceID = *instancePtr
	}
	return entry, nil
}

// ErrEntryNotFound is returned when deleting a non-existent entry.
var ErrEntryNotFound = fmt.Errorf("ip allowlist entry not found")

// Delete removes an entry by id.
func (r *IPAllowlistRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM ip_allowlist WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete ip allowlist entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEntryNotFound
	}
	return nil
}

// IsAllowed reports whether ipStr may reach the channel for the given instance.
// It considers global entries plus entries scoped to instanceID. An empty
// instanceID evaluates global entries only (used for instance-less endpoints).
func (r *IPAllowlistRepository) IsAllowed(ctx context.Context, instanceID, ipStr string) (bool, error) {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		// An unparseable source address can never match a concrete entry; deny.
		return false, nil
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT cidr FROM ip_allowlist
		WHERE instance_id IS NULL OR instance_id = $1
	`, strings.TrimSpace(instanceID))
	if err != nil {
		return false, fmt.Errorf("evaluate ip allowlist: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			return false, err
		}
		if matches(cidr, ip) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func scanEntry(rows pgx.Rows) (IPAllowlistEntry, error) {
	var entry IPAllowlistEntry
	var instanceID *string
	if err := rows.Scan(&entry.ID, &instanceID, &entry.CIDR, &entry.Note, &entry.CreatedAt); err != nil {
		return IPAllowlistEntry{}, fmt.Errorf("scan ip allowlist entry: %w", err)
	}
	if instanceID != nil {
		entry.InstanceID = *instanceID
	}
	return entry, nil
}
