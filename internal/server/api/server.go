package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/auth"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
)

// Server represents the API server
type Server struct {
	config      *config.Config
	db          *database.DB
	storage     storage.Storage
	router      *chi.Mux
	authService *auth.Service
	authHandler *auth.Handlers
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
	}

	s.setupRoutes()
	return s
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

		// =====================
		// Release endpoints
		// =====================
		r.Route("/releases", func(r chi.Router) {
			r.Get("/", s.handleListReleases)
			r.Get("/{product}", s.handleListProductReleases)
			r.Get("/{product}/latest", s.handleGetLatestRelease)
			r.Get("/{product}/{version}", s.handleGetRelease)
			r.Get("/{product}/{version}/download", s.handleDownloadRelease)
			// Protected: upload releases
			r.With(s.adminAuth).Post("/", s.handleUploadRelease)
			r.With(s.adminAuth).Put("/{product}/{version}/{filename}", s.handleUploadBinary)
		})

		// =====================
		// Heartbeat endpoint (from updaters)
		// =====================
		r.Post("/heartbeat", s.handleHeartbeat)

		// =====================
		// Instance endpoints
		// =====================
		r.Route("/instances", func(r chi.Router) {
			// Read endpoints - require JWT auth for dashboard
			r.With(auth.OptionalJWTMiddleware(s.authService)).Get("/", s.handleListInstances)
			r.With(auth.OptionalJWTMiddleware(s.authService)).Get("/{id}", s.handleGetInstance)
			// Delete requires admin
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Delete("/{id}", s.handleDeleteInstance)
		})

		// =====================
		// Admin endpoints
		// =====================
		r.Route("/admin", func(r chi.Router) {
			// License management - read is public for dashboard, write requires JWT admin
			r.Get("/licenses", s.handleListLicenses)
			r.Get("/licenses/{id}", s.handleGetLicense)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Post("/licenses", s.handleCreateLicense)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Put("/licenses/{id}", s.handleUpdateLicense)
			r.With(auth.JWTMiddleware(s.authService), auth.RequireRole("admin")).Delete("/licenses/{id}", s.handleDeleteLicense)

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
