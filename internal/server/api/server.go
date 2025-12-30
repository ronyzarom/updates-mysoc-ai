package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/database"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/storage"
)

// Server represents the API server
type Server struct {
	config  *config.Config
	db      *database.DB
	storage storage.Storage
	router  *chi.Mux
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, db *database.DB, store storage.Storage) *Server {
	s := &Server{
		config:  cfg,
		db:      db,
		storage: store,
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

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// License endpoints
		r.Route("/license", func(r chi.Router) {
			r.Post("/activate", s.handleLicenseActivate)
			r.Post("/validate", s.handleLicenseValidate)
		})

		// Release endpoints
		r.Route("/releases", func(r chi.Router) {
			r.Get("/", s.handleListReleases)
			r.Post("/", s.handleUploadRelease) // Requires admin auth
			r.Get("/{product}", s.handleListProductReleases)
			r.Get("/{product}/latest", s.handleGetLatestRelease)
			r.Get("/{product}/{version}", s.handleGetRelease)
			r.Get("/{product}/{version}/download", s.handleDownloadRelease)
		})

		// Heartbeat endpoint
		r.Post("/heartbeat", s.handleHeartbeat)

		// Instance endpoints (admin)
		r.Route("/instances", func(r chi.Router) {
			r.Use(s.adminAuth)
			r.Get("/", s.handleListInstances)
			r.Get("/{id}", s.handleGetInstance)
			r.Delete("/{id}", s.handleDeleteInstance)
		})

		// Admin endpoints
		r.Route("/admin", func(r chi.Router) {
			r.Use(s.adminAuth)
			r.Get("/licenses", s.handleListLicenses)
			r.Post("/licenses", s.handleCreateLicense)
			r.Get("/licenses/{id}", s.handleGetLicense)
			r.Put("/licenses/{id}", s.handleUpdateLicense)
			r.Delete("/licenses/{id}", s.handleDeleteLicense)
		})
	})

	s.router = r
}

