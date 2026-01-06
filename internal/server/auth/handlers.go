package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Handlers handles authentication HTTP requests
type Handlers struct {
	service *Service
}

// NewHandlers creates new auth handlers
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// HandleLogin handles POST /api/v1/auth/login
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req types.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	resp, err := h.service.Login(r.Context(), req.Email, req.Password, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid email or password")
		case errors.Is(err, ErrAccountLocked):
			writeError(w, http.StatusForbidden, "account is locked due to too many failed attempts")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleMFAVerify handles POST /api/v1/auth/mfa/verify
func (h *Handlers) HandleMFAVerify(w http.ResponseWriter, r *http.Request) {
	var req types.MFAVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MFAToken == "" || req.TOTPCode == "" {
		writeError(w, http.StatusBadRequest, "mfa_token and totp_code are required")
		return
	}

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	resp, err := h.service.VerifyMFA(r.Context(), req.MFAToken, req.TOTPCode, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrTokenExpired):
			writeError(w, http.StatusUnauthorized, "invalid or expired MFA token")
		case errors.Is(err, ErrInvalidMFACode):
			writeError(w, http.StatusUnauthorized, "invalid MFA code")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleRefresh handles POST /api/v1/auth/refresh
func (h *Handlers) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req types.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	resp, err := h.service.RefreshTokens(r.Context(), req.RefreshToken, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionExpired):
			writeError(w, http.StatusUnauthorized, "invalid or expired session")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleLogout handles POST /api/v1/auth/logout
func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req types.RefreshTokenRequest
	json.NewDecoder(r.Body).Decode(&req)

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	h.service.Logout(r.Context(), req.RefreshToken, user.ID, ip, userAgent)

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// HandleLogoutAll handles POST /api/v1/auth/logout-all
func (h *Handlers) HandleLogoutAll(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	h.service.LogoutAll(r.Context(), user.ID, ip, userAgent)

	writeJSON(w, http.StatusOK, map[string]string{"status": "all sessions logged out"})
}

// HandleGetProfile handles GET /api/v1/auth/profile
func (h *Handlers) HandleGetProfile(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	profile, err := h.service.GetProfile(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// HandleUpdateProfile handles PUT /api/v1/auth/profile
func (h *Handlers) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req types.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	profile, err := h.service.UpdateProfile(r.Context(), user.ID, req.Name, req.AvatarURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// HandleChangePassword handles POST /api/v1/auth/password
func (h *Handlers) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req types.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	err := h.service.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
		case errors.Is(err, ErrPasswordTooWeak):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

// HandleMFASetup handles GET /api/v1/auth/mfa/setup
func (h *Handlers) HandleMFASetup(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := h.service.SetupMFA(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, ErrMFAAlreadyEnabled) {
			writeError(w, http.StatusBadRequest, "MFA is already enabled")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleMFAEnable handles POST /api/v1/auth/mfa/enable
func (h *Handlers) HandleMFAEnable(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req types.MFAEnableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TOTPCode == "" {
		writeError(w, http.StatusBadRequest, "totp_code is required")
		return
	}

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	resp, err := h.service.EnableMFA(r.Context(), user.ID, req.TOTPCode, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrMFAAlreadyEnabled):
			writeError(w, http.StatusBadRequest, "MFA is already enabled")
		case errors.Is(err, ErrInvalidMFACode):
			writeError(w, http.StatusUnauthorized, "invalid MFA code")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleMFADisable handles POST /api/v1/auth/mfa/disable
func (h *Handlers) HandleMFADisable(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req types.MFADisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Password == "" || req.TOTPCode == "" {
		writeError(w, http.StatusBadRequest, "password and totp_code are required")
		return
	}

	ip := getClientIP(r)
	userAgent := r.UserAgent()

	err := h.service.DisableMFA(r.Context(), user.ID, req.Password, req.TOTPCode, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrMFANotEnabled):
			writeError(w, http.StatusBadRequest, "MFA is not enabled")
		case errors.Is(err, ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid password")
		case errors.Is(err, ErrInvalidMFACode):
			writeError(w, http.StatusUnauthorized, "invalid MFA code")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "MFA disabled"})
}

// HandleGetSessions handles GET /api/v1/auth/sessions
func (h *Handlers) HandleGetSessions(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sessions, err := h.service.GetSessions(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sessions)
}

// HandleGetAuditLog handles GET /api/v1/auth/audit
func (h *Handlers) HandleGetAuditLog(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	events, err := h.service.GetAuditLog(r.Context(), user.ID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, events)
}

// Admin handlers

// HandleListUsers handles GET /api/v1/admin/users
func (h *Handlers) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, users)
}

// HandleCreateUser handles POST /api/v1/admin/users
func (h *Handlers) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req types.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "email, password, and name are required")
		return
	}

	if req.Role == "" {
		req.Role = "viewer"
	}

	user, err := h.service.CreateUser(r.Context(), req.Email, req.Password, req.Name, req.Role)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserExists):
			writeError(w, http.StatusConflict, "user already exists")
		case errors.Is(err, ErrPasswordTooWeak):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

// HandleGetUser handles GET /api/v1/admin/users/{id}
func (h *Handlers) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	user, err := h.service.GetProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// HandleUpdateUser handles PUT /api/v1/admin/users/{id}
func (h *Handlers) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	var req types.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.service.UpdateUser(r.Context(), id, req.Name, req.Role, req.IsActive)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// HandleDeleteUser handles DELETE /api/v1/admin/users/{id}
func (h *Handlers) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	// Don't allow deleting yourself
	currentUser := GetUserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	err := h.service.DeleteUser(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Context key for user
type contextKey string

const userContextKey contextKey = "user"

// GetUserFromContext extracts the user from the request context
func GetUserFromContext(ctx context.Context) *types.User {
	user, ok := ctx.Value(userContextKey).(*types.User)
	if !ok {
		return nil
	}
	return user
}

// SetUserInContext sets the user in the request context
func SetUserInContext(ctx context.Context, user *types.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}
