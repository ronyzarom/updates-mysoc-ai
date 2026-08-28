package api

import (
	"crypto/ed25519"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/auth"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/releases"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/security"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
)

// Server represents the API server
type Server struct {
	config      *config.Config
	db          *database.DB
	storage     storage.Storage
	router      *chi.Mux
	authService *auth.Service
	authHandler *auth.Handlers
	ipACL       *security.IPAllowlistRepository
	apiKeys     *security.APIKeyRepository
	signingKey  ed25519.PrivateKey // nil = release signing disabled

	// Check-plane relief (see checkplane.go): memoized release lookups and a
	// per-instance write throttle keep the O(fleet) update-check path
	// read-mostly at scale.
	releaseCache  *releaseLookupCache
	checkThrottle *checkWriteThrottle
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, db *database.DB, store storage.Storage) *Server {
	// Initialize auth
	authRepo := auth.NewRepository(db)
	authService := auth.NewService(authRepo, cfg.Auth.JWTSecret, cfg.Auth.Issuer)
	authHandlers := auth.NewHandlers(authService)

	s := &Server{
		config:      cfg,
		db:          db,
		storage:     store,
		authService: authService,
		authHandler: authHandlers,
		ipACL:       security.NewIPAllowlistRepository(db),
		apiKeys:     security.NewAPIKeyRepository(db),

		releaseCache:  newReleaseLookupCache(),
		checkThrottle: newCheckWriteThrottle(),
	}

	if seed := cfg.Server.SigningKeySeed; seed != "" {
		key, err := signing.ParseSeedHex(seed)
		if err != nil {
			log.Fatalf("invalid RELEASE_SIGNING_SEED: %v", err)
		}
		s.signingKey = key
		log.Printf("release signing enabled (public key %s)", signing.PublicKeyHex(key))
	} else {
		log.Printf("WARNING: RELEASE_SIGNING_SEED not set - releases will publish unsigned")
	}

	s.setupRoutes()
	return s
}

// releaseService builds the release service with the signing key attached.
func (s *Server) releaseService() *releases.Service {
	svc := releases.NewService(s.db, s.storage)
	if s.signingKey != nil {
		svc.SetSigningKey(s.signingKey)
	}
	return svc
}

// Router returns the HTTP router
func (s *Server) Router() http.Handler {
	return s.router
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	// Inflate gzipped request bodies (cascade rollup heartbeats) before any
	// handler reads them.
	r.Use(decompressRequest)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.config.Server.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check (no auth)
	r.Get("/health", s.handleHealth)

	// Direct binary download routes (Siemcore installer format)
	// Supports: /{product}/{version}/{filename}
	// Example: /siemcore/v1.5.0/siemcore-linux-amd64
	r.Get("/{product}/{version}/{filename}", s.handleDirectDownload)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// =====================
		// Authentication routes (public)
		// =====================
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.authHandler.HandleLogin)
			r.Post("/mfa/verify", s.authHandler.HandleMFAVerify)
			r.Post("/refresh", s.authHandler.HandleRefresh)

			// Protected auth routes
			r.Group(func(r chi.Router) {
				r.Use(auth.JWTMiddleware(s.authService))
				r.Post("/logout", s.authHandler.HandleLogout)
				r.Post("/logout-all", s.authHandler.HandleLogoutAll)
				r.Get("/profile", s.authHandler.HandleGetProfile)
				r.Put("/profile", s.authHandler.HandleUpdateProfile)
				r.Post("/password", s.authHandler.HandleChangePassword)
				r.Get("/mfa/setup", s.authHandler.HandleMFASetup)
				r.Post("/mfa/enable", s.authHandler.HandleMFAEnable)
				r.Post("/mfa/disable", s.authHandler.HandleMFADisable)
				r.Get("/sessions", s.authHandler.HandleGetSessions)
				r.Get("/audit", s.authHandler.HandleGetAuditLog)
			})
		})

		// =====================
		// License endpoints (public for activation)
		// =====================
		r.Route("/license", func(r chi.Router) {
			r.Post("/activate", s.handleLicenseActivate)
			r.Post("/validate", s.handleLicenseValidate)
		})

		// Canonical product-tier catalog (mysoc > siemcore > swf) for
		// dashboard dropdowns and agent self-configuration. Static data.
		r.Get("/products", s.handleListProducts)

		// Release-signing public key so updaters can pin it at provisioning.
		r.Get("/signing-key", s.handleSigningKey)

		// =====================
		// Release endpoints
		// =====================
		r.Route("/releases", func(r chi.Router) {
			r.Get("/", s.handleListReleases)
			r.Get("/{product}", s.handleListProductReleases)
			r.Get("/{product}/latest", s.handleGetLatestRelease)
			r.Get("/{product}/{version}", s.handleGetRelease)
			r.Get("/{product}/{version}/download", s.handleDownloadRelease)
			// Protected: upload releases and manage target groups. These accept
			// a releases-scoped managed key (or full-admin key / admin JWT).
			r.With(s.requireScope(security.ScopeReleases)).Post("/", s.handleUploadRelease)
			r.With(s.requireScope(security.ScopeReleases)).Put("/{product}/{version}/{filename}", s.handleUploadBinary)
			r.With(s.requireScope(security.ScopeReleases)).Put("/{product}/{version}/target-groups", s.handleUpdateReleaseTargetGroups)
			r.With(s.requireScope(security.ScopeReleases)).Put("/{product}/{version}", s.handleUpdateRelease)
			r.With(s.requireScope(security.ScopeReleases)).Delete("/{product}/{version}", s.handleDeleteRelease)
		})

		// =====================
		// Heartbeat endpoint (from updaters)
		// =====================
		r.Post("/heartbeat", s.handleHeartbeat)

		// Decommission: a direct node announces its own clean removal
		// (state change, not deletion; contract 1.11.0 Item B).
		r.Post("/decommission", s.handleDecommission)

		// =====================
		// Updates endpoint (siemcore-updater format)
		// =====================
		r.Route("/updates", func(r chi.Router) {
			r.Post("/{product}/check", s.handleUpdateCheck)
			r.Post("/{product}/report", s.handleUpdateReport)
		})

		// =====================
		// Instance endpoints
		// =====================
		r.Route("/instances", func(r chi.Router) {
			// Read endpoints - require an authenticated dashboard user.
			r.With(auth.JWTMiddleware(s.authService)).Get("/", s.handleListInstances)
			r.With(auth.JWTMiddleware(s.authService)).Get("/paged", s.handleListInstancesPaged)
			// SQL-aggregated fleet summary for the dashboard headline cards.
			r.With(auth.JWTMiddleware(s.authService)).Get("/stats", s.handleFleetStats)
			// SQL-aggregated + paged security posture (exceptions first).
			r.With(auth.JWTMiddleware(s.authService)).Get("/security", s.handleSecurityPaged)
			r.With(auth.JWTMiddleware(s.authService)).Get("/security/stats", s.handleSecurityStats)
			// Fleet as a per-customer tier tree (mysoc > siemcore > swf).
			r.With(auth.JWTMiddleware(s.authService)).Get("/tree", s.handleInstanceTree)
			// Exceptions-first, paged, SQL-aggregated customer directory
			// (the scale replacement for rendering the whole tree).
			r.With(auth.JWTMiddleware(s.authService)).Get("/customers", s.handleCustomerDirectory)
			r.With(auth.JWTMiddleware(s.authService)).Get("/{id}", s.handleGetInstance)
			// Ancestor chain resolved server-side (no full-fleet fetch).
			r.With(auth.JWTMiddleware(s.authService)).Get("/{id}/parents", s.handleInstanceParents)
			// Mutations require admin authorization.
			r.With(s.adminAuth).Put("/{id}", s.handleUpdateInstance)
			r.With(s.adminAuth).Delete("/{id}", s.handleDeleteInstance)
			r.With(s.adminAuth).Put("/{id}/auto-update", s.handleSetAutoUpdate)
			r.With(s.adminAuth).Put("/{id}/update-group", s.handleSetUpdateGroup)
		})

		// =====================
		// Admin endpoints
		// =====================
		r.Route("/admin", func(r chi.Router) {
			// License management - reads and writes require admin authorization.
			r.With(s.adminAuth).Get("/licenses", s.handleListLicenses)
			r.With(s.adminAuth).Get("/licenses/{id}", s.handleGetLicense)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Post("/licenses", s.handleCreateLicense)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Put("/licenses/{id}", s.handleUpdateLicense)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Delete("/licenses/{id}", s.handleDeleteLicense)

			// IP allowlist management (updater channel firewall). Reads and
			// writes require admin authorization (API key or admin JWT).
			r.With(s.adminAuth).Get("/ip-allowlist", s.handleListIPAllowlist)
			r.With(s.adminAuth).Post("/ip-allowlist", s.handleCreateIPAllowlist)
			r.With(s.adminAuth).Delete("/ip-allowlist/{id}", s.handleDeleteIPAllowlist)

			// Managed API keys (named, scoped, revocable credentials for
			// automation/upload). Managing keys is a full-admin action.
			r.With(s.adminAuth).Get("/api-keys", s.handleListAPIKeys)
			r.With(s.adminAuth).Post("/api-keys", s.handleCreateAPIKey)
			r.With(s.adminAuth).Delete("/api-keys/{id}", s.handleRevokeAPIKey)

			// Operators: the licensing surface of the cascade model. Issuing
			// an operator mints its single platform key (returned once).
			r.With(s.adminAuth).Get("/operators", s.handleListOperators)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Post("/operators", s.handleCreateOperator)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Post("/operators/{id}/rotate-key", s.handleRotateOperatorKey)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Put("/operators/{id}", s.handleUpdateOperator)

			// User management - requires JWT admin
			r.Group(func(r chi.Router) {
				r.Use(auth.JWTMiddleware(s.authService))
				r.Use(auth.RequireRole("admin"))
				r.Get("/users", s.authHandler.HandleListUsers)
				r.Post("/users", s.authHandler.HandleCreateUser)
				r.Get("/users/{id}", s.authHandler.HandleGetUser)
				r.Put("/users/{id}", s.authHandler.HandleUpdateUser)
				r.Delete("/users/{id}", s.authHandler.HandleDeleteUser)
			})
		})
	})

	s.router = r
}
