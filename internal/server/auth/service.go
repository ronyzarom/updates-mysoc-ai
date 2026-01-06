package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

const (
	AccessTokenDuration  = 15 * time.Minute
	RefreshTokenDuration = 7 * 24 * time.Hour
	MFATokenDuration     = 5 * time.Minute
	MaxLoginAttempts     = 5
	LockoutDuration      = 15 * time.Minute
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrMFARequired      = errors.New("MFA verification required")
	ErrInvalidMFACode   = errors.New("invalid MFA code")
	ErrMFANotEnabled    = errors.New("MFA is not enabled")
	ErrMFAAlreadyEnabled = errors.New("MFA is already enabled")
	ErrPasswordTooWeak  = errors.New("password must be at least 8 characters")
)

// Service handles authentication operations
type Service struct {
	repo      *Repository
	jwtSecret []byte
	issuer    string
}

// NewService creates a new auth service
func NewService(repo *Repository, jwtSecret, issuer string) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: []byte(jwtSecret),
		issuer:    issuer,
	}
}

// Login authenticates a user with email and password
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (*types.LoginResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check if account is locked
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		return nil, ErrAccountLocked
	}

	// Check if account is active
	if !user.IsActive {
		return nil, errors.New("account is disabled")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		// Increment failed attempts
		attempts, _ := s.repo.IncrementFailedAttempts(ctx, user.ID)
		s.repo.LogAuditEvent(ctx, user.ID, "failed_login", ip, userAgent, map[string]interface{}{
			"email":    email,
			"attempts": attempts,
		})
		return nil, ErrInvalidCredentials
	}

	// Reset failed attempts on successful password verification
	s.repo.ResetFailedAttempts(ctx, user.ID)

	// If MFA is enabled, return a temporary token for MFA verification
	if user.MFAEnabled {
		mfaToken, err := s.generateToken(user.ID, user.Email, user.Role, "mfa", MFATokenDuration)
		if err != nil {
			return nil, err
		}
		return &types.LoginResponse{
			RequiresMFA: true,
			MFAToken:    mfaToken,
		}, nil
	}

	// Generate full auth tokens
	return s.generateAuthTokens(ctx, &user.User, ip, userAgent)
}

// VerifyMFA verifies the TOTP code and completes login
func (s *Service) VerifyMFA(ctx context.Context, mfaToken, totpCode, ip, userAgent string) (*types.LoginResponse, error) {
	// Parse and validate MFA token
	claims, err := s.validateToken(mfaToken, "mfa")
	if err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		return nil, err
	}

	if !user.MFAEnabled || user.MFASecret == "" {
		return nil, ErrMFANotEnabled
	}

	// Verify TOTP code
	valid := totp.Validate(totpCode, user.MFASecret)
	if !valid {
		// Try backup codes
		used, err := s.repo.UseBackupCode(ctx, user.ID, totpCode)
		if err != nil || !used {
			s.repo.LogAuditEvent(ctx, user.ID, "failed_mfa", ip, userAgent, nil)
			return nil, ErrInvalidMFACode
		}
	}

	// Log successful MFA
	s.repo.LogAuditEvent(ctx, user.ID, "mfa_success", ip, userAgent, nil)

	// Generate full auth tokens
	return s.generateAuthTokens(ctx, &user.User, ip, userAgent)
}

// generateAuthTokens creates access and refresh tokens
func (s *Service) generateAuthTokens(ctx context.Context, user *types.User, ip, userAgent string) (*types.LoginResponse, error) {
	accessToken, err := s.generateToken(user.ID, user.Email, user.Role, "access", AccessTokenDuration)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken()
	if err != nil {
		return nil, err
	}

	// Hash refresh token for storage
	refreshTokenHash := hashToken(refreshToken)

	// Create session
	_, err = s.repo.CreateSession(ctx, user.ID, refreshTokenHash, userAgent, ip, time.Now().Add(RefreshTokenDuration))
	if err != nil {
		return nil, err
	}

	// Update last login
	s.repo.UpdateLastLogin(ctx, user.ID, ip)
	s.repo.LogAuditEvent(ctx, user.ID, "login", ip, userAgent, nil)

	return &types.LoginResponse{
		RequiresMFA:  false,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
		ExpiresIn:    int(AccessTokenDuration.Seconds()),
	}, nil
}

// RefreshTokens generates new access and refresh tokens
func (s *Service) RefreshTokens(ctx context.Context, refreshToken, ip, userAgent string) (*types.RefreshTokenResponse, error) {
	refreshTokenHash := hashToken(refreshToken)

	session, err := s.repo.GetSessionByToken(ctx, refreshTokenHash)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	if !user.IsActive {
		s.repo.RevokeSession(ctx, session.ID)
		return nil, errors.New("account is disabled")
	}

	// Revoke old session
	s.repo.RevokeSession(ctx, session.ID)

	// Generate new tokens
	accessToken, err := s.generateToken(user.ID, user.Email, user.Role, "access", AccessTokenDuration)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.generateRefreshToken()
	if err != nil {
		return nil, err
	}

	// Create new session
	newRefreshTokenHash := hashToken(newRefreshToken)
	_, err = s.repo.CreateSession(ctx, user.ID, newRefreshTokenHash, userAgent, ip, time.Now().Add(RefreshTokenDuration))
	if err != nil {
		return nil, err
	}

	return &types.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int(AccessTokenDuration.Seconds()),
	}, nil
}

// Logout revokes a session
func (s *Service) Logout(ctx context.Context, refreshToken, userID, ip, userAgent string) error {
	refreshTokenHash := hashToken(refreshToken)
	session, err := s.repo.GetSessionByToken(ctx, refreshTokenHash)
	if err == nil {
		s.repo.RevokeSession(ctx, session.ID)
	}
	s.repo.LogAuditEvent(ctx, userID, "logout", ip, userAgent, nil)
	return nil
}

// LogoutAll revokes all sessions for a user
func (s *Service) LogoutAll(ctx context.Context, userID, ip, userAgent string) error {
	s.repo.RevokeAllUserSessions(ctx, userID)
	s.repo.LogAuditEvent(ctx, userID, "logout_all", ip, userAgent, nil)
	return nil
}

// ValidateAccessToken validates an access token and returns claims
func (s *Service) ValidateAccessToken(tokenString string) (*types.JWTClaims, error) {
	return s.validateToken(tokenString, "access")
}

// GetUserFromToken gets user from a valid access token
func (s *Service) GetUserFromToken(ctx context.Context, tokenString string) (*types.User, error) {
	claims, err := s.ValidateAccessToken(tokenString)
	if err != nil {
		return nil, err
	}
	return s.repo.GetUserByID(ctx, claims.UserID)
}

// SetupMFA generates a TOTP secret and QR code for MFA setup
func (s *Service) SetupMFA(ctx context.Context, userID string) (*types.MFASetupResponse, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.MFAEnabled {
		return nil, ErrMFAAlreadyEnabled
	}

	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer,
		AccountName: user.Email,
		SecretSize:  32,
	})
	if err != nil {
		return nil, err
	}

	// Store secret temporarily (will be verified before enabling)
	err = s.repo.UpdateMFASecret(ctx, userID, key.Secret())
	if err != nil {
		return nil, err
	}

	// Generate QR code PNG
	var qrBuf strings.Builder
	img, err := key.Image(200, 200)
	if err != nil {
		return nil, err
	}

	// Encode to base64 PNG
	pngBuf := &strings.Builder{}
	err = png.Encode(&writerWrapper{pngBuf}, img)
	if err != nil {
		return nil, err
	}
	qrBase64 := base64.StdEncoding.EncodeToString([]byte(pngBuf.String()))

	_ = qrBuf

	return &types.MFASetupResponse{
		Secret:     key.Secret(),
		QRCodeURL:  key.URL(),
		QRCodeData: "data:image/png;base64," + qrBase64,
	}, nil
}

// EnableMFA verifies the TOTP code and enables MFA
func (s *Service) EnableMFA(ctx context.Context, userID, totpCode, ip, userAgent string) (*types.MFABackupCodesResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, userID)
	if err != nil {
		// Try by ID
		userByID, err2 := s.repo.GetUserByID(ctx, userID)
		if err2 != nil {
			return nil, err
		}
		user, err = s.repo.GetUserByEmail(ctx, userByID.Email)
		if err != nil {
			return nil, err
		}
	}

	if user.MFAEnabled {
		return nil, ErrMFAAlreadyEnabled
	}

	if user.MFASecret == "" {
		return nil, errors.New("MFA setup not initiated")
	}

	// Verify TOTP code
	if !totp.Validate(totpCode, user.MFASecret) {
		return nil, ErrInvalidMFACode
	}

	// Generate backup codes
	backupCodes := make([]string, 10)
	backupCodeHashes := make([]string, 10)
	for i := 0; i < 10; i++ {
		code := generateBackupCode()
		backupCodes[i] = code
		hash := sha256.Sum256([]byte(code))
		backupCodeHashes[i] = hex.EncodeToString(hash[:])
	}

	// Enable MFA with backup codes
	err = s.repo.EnableMFA(ctx, user.ID, backupCodeHashes)
	if err != nil {
		return nil, err
	}

	s.repo.LogAuditEvent(ctx, user.ID, "mfa_enable", ip, userAgent, nil)

	return &types.MFABackupCodesResponse{
		BackupCodes: backupCodes,
	}, nil
}

// DisableMFA disables MFA after verifying password and TOTP
func (s *Service) DisableMFA(ctx context.Context, userID, password, totpCode, ip, userAgent string) error {
	user, err := s.repo.GetUserByEmail(ctx, userID)
	if err != nil {
		// Try by ID
		userByID, err2 := s.repo.GetUserByID(ctx, userID)
		if err2 != nil {
			return err
		}
		user, err = s.repo.GetUserByEmail(ctx, userByID.Email)
		if err != nil {
			return err
		}
	}

	if !user.MFAEnabled {
		return ErrMFANotEnabled
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}

	// Verify TOTP
	if !totp.Validate(totpCode, user.MFASecret) {
		return ErrInvalidMFACode
	}

	err = s.repo.DisableMFA(ctx, user.ID)
	if err != nil {
		return err
	}

	s.repo.LogAuditEvent(ctx, user.ID, "mfa_disable", ip, userAgent, nil)

	return nil
}

// ChangePassword changes the user's password
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, ip, userAgent string) error {
	if len(newPassword) < 8 {
		return ErrPasswordTooWeak
	}

	user, err := s.repo.GetUserByEmail(ctx, userID)
	if err != nil {
		// Try by ID
		userByID, err2 := s.repo.GetUserByID(ctx, userID)
		if err2 != nil {
			return err
		}
		user, err = s.repo.GetUserByEmail(ctx, userByID.Email)
		if err != nil {
			return err
		}
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	err = s.repo.UpdatePassword(ctx, user.ID, string(hash))
	if err != nil {
		return err
	}

	// Revoke all sessions
	s.repo.RevokeAllUserSessions(ctx, user.ID)
	s.repo.LogAuditEvent(ctx, user.ID, "password_change", ip, userAgent, nil)

	return nil
}

// CreateUser creates a new user (admin only)
func (s *Service) CreateUser(ctx context.Context, email, password, name, role string) (*types.User, error) {
	if len(password) < 8 {
		return nil, ErrPasswordTooWeak
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return s.repo.CreateUser(ctx, email, string(hash), name, role)
}

// UpdateProfile updates user profile
func (s *Service) UpdateProfile(ctx context.Context, userID, name, avatarURL string) (*types.User, error) {
	return s.repo.UpdateUser(ctx, userID, name, avatarURL)
}

// GetProfile gets user profile
func (s *Service) GetProfile(ctx context.Context, userID string) (*types.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

// ListUsers lists all users (admin only)
func (s *Service) ListUsers(ctx context.Context) ([]types.User, error) {
	return s.repo.ListUsers(ctx)
}

// UpdateUser updates a user (admin only)
func (s *Service) UpdateUser(ctx context.Context, userID, name, role string, isActive *bool) (*types.User, error) {
	return s.repo.UpdateUserAdmin(ctx, userID, name, role, isActive)
}

// DeleteUser deletes a user (admin only)
func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	return s.repo.DeleteUser(ctx, userID)
}

// GetSessions returns active sessions for a user
func (s *Service) GetSessions(ctx context.Context, userID string) ([]types.Session, error) {
	return s.repo.GetUserSessions(ctx, userID)
}

// GetAuditLog returns audit events for a user
func (s *Service) GetAuditLog(ctx context.Context, userID string, limit int) ([]types.AuthAuditLog, error) {
	return s.repo.GetAuditLog(ctx, userID, limit)
}

// Helper functions

func (s *Service) generateToken(userID, email, role, tokenType string, duration time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"role":    role,
		"type":    tokenType,
		"iat":     now.Unix(),
		"exp":     now.Add(duration).Unix(),
		"iss":     s.issuer,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) validateToken(tokenString, expectedType string) (*types.JWTClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Check token type
	tokenType, _ := claims["type"].(string)
	if tokenType != expectedType {
		return nil, ErrInvalidToken
	}

	return &types.JWTClaims{
		UserID: claims["user_id"].(string),
		Email:  claims["email"].(string),
		Role:   claims["role"].(string),
		Type:   tokenType,
	}, nil
}

func (s *Service) generateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func generateBackupCode() string {
	bytes := make([]byte, 5)
	rand.Read(bytes)
	code := base32.StdEncoding.EncodeToString(bytes)
	// Format as XXXX-XXXX
	return code[:4] + "-" + code[4:8]
}

// writerWrapper wraps strings.Builder to implement io.Writer
type writerWrapper struct {
	builder *strings.Builder
}

func (w *writerWrapper) Write(p []byte) (n int, err error) {
	return w.builder.Write(p)
}
