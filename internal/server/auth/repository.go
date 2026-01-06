package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrUserExists       = errors.New("user already exists")
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionExpired   = errors.New("session expired")
	ErrAccountLocked    = errors.New("account locked due to failed attempts")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

// Repository handles auth database operations
type Repository struct {
	db *database.DB
}

// NewRepository creates a new auth repository
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateUser creates a new user
func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, name, role string) (*types.User, error) {
	var user types.User
	var lastLoginAt, mfaVerifiedAt, emailVerifiedAt, lockedUntil pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, name, role, avatar_url, mfa_enabled, is_active, email_verified,
				  last_login_at, password_changed_at, created_at, updated_at
	`, email, passwordHash, name, role).Scan(
		&user.ID, &user.Email, &user.Name, &user.Role, &user.AvatarURL,
		&user.MFAEnabled, &user.IsActive, &user.EmailVerified,
		&lastLoginAt, &user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "ERROR: duplicate key value violates unique constraint \"users_email_key\" (SQLSTATE 23505)" {
			return nil, ErrUserExists
		}
		return nil, err
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	_ = mfaVerifiedAt
	_ = emailVerifiedAt
	_ = lockedUntil

	return &user, nil
}

// GetUserByEmail retrieves a user by email (includes password hash for auth)
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*types.UserWithPassword, error) {
	var user types.UserWithPassword
	var lastLoginAt, lockedUntil pgtype.Timestamptz
	var avatarURL, mfaSecret sql.NullString
	var backupCodes []string

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, email, password_hash, name, role, avatar_url, 
			   mfa_enabled, mfa_secret, mfa_backup_codes, is_active, email_verified,
			   last_login_at, password_changed_at, failed_login_attempts, locked_until,
			   created_at, updated_at
		FROM users
		WHERE email = $1
	`, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role, &avatarURL,
		&user.MFAEnabled, &mfaSecret, &backupCodes, &user.IsActive, &user.EmailVerified,
		&lastLoginAt, &user.PasswordChangedAt, &user.FailedLoginAttempts, &lockedUntil,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	if mfaSecret.Valid {
		user.MFASecret = mfaSecret.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	if lockedUntil.Valid {
		user.LockedUntil = &lockedUntil.Time
	}
	user.MFABackupCodes = backupCodes

	return &user, nil
}

// GetUserByID retrieves a user by ID
func (r *Repository) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	var user types.User
	var lastLoginAt pgtype.Timestamptz
	var avatarURL sql.NullString

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, email, name, role, avatar_url, mfa_enabled, is_active, email_verified,
			   last_login_at, password_changed_at, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.Role, &avatarURL,
		&user.MFAEnabled, &user.IsActive, &user.EmailVerified,
		&lastLoginAt, &user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}

// ListUsers returns all users
func (r *Repository) ListUsers(ctx context.Context) ([]types.User, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, email, name, role, avatar_url, mfa_enabled, is_active, email_verified,
			   last_login_at, password_changed_at, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []types.User
	for rows.Next() {
		var user types.User
		var lastLoginAt pgtype.Timestamptz
		var avatarURL sql.NullString

		if err := rows.Scan(
			&user.ID, &user.Email, &user.Name, &user.Role, &avatarURL,
			&user.MFAEnabled, &user.IsActive, &user.EmailVerified,
			&lastLoginAt, &user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if avatarURL.Valid {
			user.AvatarURL = avatarURL.String
		}
		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}

		users = append(users, user)
	}

	return users, nil
}

// UpdateUser updates user profile
func (r *Repository) UpdateUser(ctx context.Context, id, name, avatarURL string) (*types.User, error) {
	var user types.User
	var lastLoginAt pgtype.Timestamptz
	var avatar sql.NullString

	err := r.db.Pool.QueryRow(ctx, `
		UPDATE users
		SET name = COALESCE(NULLIF($2, ''), name),
			avatar_url = COALESCE(NULLIF($3, ''), avatar_url),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, email, name, role, avatar_url, mfa_enabled, is_active, email_verified,
				  last_login_at, password_changed_at, created_at, updated_at
	`, id, name, avatarURL).Scan(
		&user.ID, &user.Email, &user.Name, &user.Role, &avatar,
		&user.MFAEnabled, &user.IsActive, &user.EmailVerified,
		&lastLoginAt, &user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if avatar.Valid {
		user.AvatarURL = avatar.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}

// UpdatePassword updates user password
func (r *Repository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	result, err := r.db.Pool.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, password_changed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id, passwordHash)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateMFASecret sets the MFA secret for a user
func (r *Repository) UpdateMFASecret(ctx context.Context, id, secret string) error {
	result, err := r.db.Pool.Exec(ctx, `
		UPDATE users
		SET mfa_secret = $2, updated_at = NOW()
		WHERE id = $1
	`, id, secret)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// EnableMFA enables MFA for a user and sets backup codes
func (r *Repository) EnableMFA(ctx context.Context, id string, backupCodes []string) error {
	result, err := r.db.Pool.Exec(ctx, `
		UPDATE users
		SET mfa_enabled = true, mfa_backup_codes = $2, mfa_verified_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id, backupCodes)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// DisableMFA disables MFA for a user
func (r *Repository) DisableMFA(ctx context.Context, id string) error {
	result, err := r.db.Pool.Exec(ctx, `
		UPDATE users
		SET mfa_enabled = false, mfa_secret = NULL, mfa_backup_codes = NULL, 
			mfa_verified_at = NULL, updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// IncrementFailedAttempts increments failed login attempts
func (r *Repository) IncrementFailedAttempts(ctx context.Context, id string) (int, error) {
	var attempts int
	err := r.db.Pool.QueryRow(ctx, `
		UPDATE users
		SET failed_login_attempts = failed_login_attempts + 1,
			locked_until = CASE 
				WHEN failed_login_attempts >= 4 THEN NOW() + INTERVAL '15 minutes'
				ELSE locked_until
			END,
			updated_at = NOW()
		WHERE id = $1
		RETURNING failed_login_attempts
	`, id).Scan(&attempts)
	return attempts, err
}

// ResetFailedAttempts resets failed login attempts
func (r *Repository) ResetFailedAttempts(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE users
		SET failed_login_attempts = 0, locked_until = NULL, updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

// UpdateLastLogin updates last login time and IP
func (r *Repository) UpdateLastLogin(ctx context.Context, id, ip string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE users
		SET last_login_at = NOW(), last_login_ip = $2, updated_at = NOW()
		WHERE id = $1
	`, id, ip)
	return err
}

// UseBackupCode uses a backup code and removes it
func (r *Repository) UseBackupCode(ctx context.Context, id, code string) (bool, error) {
	// Hash the code to compare
	codeHash := sha256.Sum256([]byte(code))
	codeHashStr := hex.EncodeToString(codeHash[:])

	result, err := r.db.Pool.Exec(ctx, `
		UPDATE users
		SET mfa_backup_codes = array_remove(mfa_backup_codes, $2),
			updated_at = NOW()
		WHERE id = $1 AND $2 = ANY(mfa_backup_codes)
	`, id, codeHashStr)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}

// DeleteUser deletes a user
func (r *Repository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateUserAdmin updates user (admin operation)
func (r *Repository) UpdateUserAdmin(ctx context.Context, id string, name, role string, isActive *bool) (*types.User, error) {
	var user types.User
	var lastLoginAt pgtype.Timestamptz
	var avatar sql.NullString

	query := `
		UPDATE users
		SET name = COALESCE(NULLIF($2, ''), name),
			role = COALESCE(NULLIF($3, ''), role),
			is_active = COALESCE($4, is_active),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, email, name, role, avatar_url, mfa_enabled, is_active, email_verified,
				  last_login_at, password_changed_at, created_at, updated_at
	`

	err := r.db.Pool.QueryRow(ctx, query, id, name, role, isActive).Scan(
		&user.ID, &user.Email, &user.Name, &user.Role, &avatar,
		&user.MFAEnabled, &user.IsActive, &user.EmailVerified,
		&lastLoginAt, &user.PasswordChangedAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if avatar.Valid {
		user.AvatarURL = avatar.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}

// Session operations

// CreateSession creates a new session
func (r *Repository) CreateSession(ctx context.Context, userID, refreshTokenHash, userAgent, ip string, expiresAt time.Time) (*types.Session, error) {
	var session types.Session
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, user_agent, ip_address, expires_at, created_at
	`, userID, refreshTokenHash, userAgent, ip, expiresAt).Scan(
		&session.ID, &session.UserID, &session.UserAgent, &session.IPAddress,
		&session.ExpiresAt, &session.CreatedAt,
	)
	return &session, err
}

// GetSessionByToken retrieves a session by refresh token hash
func (r *Repository) GetSessionByToken(ctx context.Context, refreshTokenHash string) (*types.Session, error) {
	var session types.Session
	var revokedAt pgtype.Timestamptz
	var userAgent, ipAddress sql.NullString

	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, user_id, user_agent, ip_address, expires_at, revoked_at, created_at
		FROM sessions
		WHERE refresh_token_hash = $1
	`, refreshTokenHash).Scan(
		&session.ID, &session.UserID, &userAgent, &ipAddress,
		&session.ExpiresAt, &revokedAt, &session.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	if userAgent.Valid {
		session.UserAgent = userAgent.String
	}
	if ipAddress.Valid {
		session.IPAddress = ipAddress.String
	}
	if revokedAt.Valid {
		session.RevokedAt = &revokedAt.Time
	}

	// Check if expired
	if session.ExpiresAt.Before(time.Now()) {
		return nil, ErrSessionExpired
	}
	// Check if revoked
	if session.RevokedAt != nil {
		return nil, ErrSessionExpired
	}

	return &session, nil
}

// RevokeSession revokes a session
func (r *Repository) RevokeSession(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = NOW() WHERE id = $1
	`, id)
	return err
}

// RevokeAllUserSessions revokes all sessions for a user
func (r *Repository) RevokeAllUserSessions(ctx context.Context, userID string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}

// GetUserSessions returns all active sessions for a user
func (r *Repository) GetUserSessions(ctx context.Context, userID string) ([]types.Session, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, user_id, user_agent, ip_address, expires_at, created_at
		FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []types.Session
	for rows.Next() {
		var session types.Session
		var userAgent, ipAddress sql.NullString

		if err := rows.Scan(
			&session.ID, &session.UserID, &userAgent, &ipAddress,
			&session.ExpiresAt, &session.CreatedAt,
		); err != nil {
			return nil, err
		}

		if userAgent.Valid {
			session.UserAgent = userAgent.String
		}
		if ipAddress.Valid {
			session.IPAddress = ipAddress.String
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// CleanupExpiredSessions removes expired sessions
func (r *Repository) CleanupExpiredSessions(ctx context.Context) error {
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM sessions WHERE expires_at < NOW() OR revoked_at IS NOT NULL
	`)
	return err
}

// Audit logging

// LogAuditEvent logs an authentication event
func (r *Repository) LogAuditEvent(ctx context.Context, userID, eventType, ip, userAgent string, details map[string]interface{}) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO auth_audit_log (user_id, event_type, ip_address, user_agent, details)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, eventType, ip, userAgent, details)
	return err
}

// GetAuditLog returns audit events for a user
func (r *Repository) GetAuditLog(ctx context.Context, userID string, limit int) ([]types.AuthAuditLog, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, user_id, event_type, ip_address, user_agent, details, created_at
		FROM auth_audit_log
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []types.AuthAuditLog
	for rows.Next() {
		var event types.AuthAuditLog
		var ipAddress, userAgent sql.NullString
		var userIDStr sql.NullString

		if err := rows.Scan(
			&event.ID, &userIDStr, &event.EventType, &ipAddress, &userAgent,
			&event.Details, &event.CreatedAt,
		); err != nil {
			return nil, err
		}

		if userIDStr.Valid {
			event.UserID = userIDStr.String
		}
		if ipAddress.Valid {
			event.IPAddress = ipAddress.String
		}
		if userAgent.Valid {
			event.UserAgent = userAgent.String
		}

		events = append(events, event)
	}

	return events, nil
}
