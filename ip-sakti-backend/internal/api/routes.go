package api

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/heythisissud/ip-sakti-backend/pkg/config"
)

// NewRouter builds and returns the chi router with all middleware and routes wired.
// Middleware order follows the spec exactly:
//  1. RequestID
//  2. RealIP
//  3. requestLogger (custom structured)
//  4. Recoverer
//  5. cors.Handler
func NewRouter(h *Handler, cfg *config.Config, logger *slog.Logger) *chi.Mux {
	r := chi.NewRouter()

	// ── Global middleware ──────────────────────────────────────────────────
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(requestLogger(logger))
	r.Use(chimiddleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: cfg.AllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         300,
	}))

	// ── Routes ────────────────────────────────────────────────────────────
	// Top-level aliases
	r.Get("/health", h.HealthHandler)
	r.Post("/classify", h.ClassifyHandler)
	r.Post("/clarify", h.ClarifyHandler)
	r.Post("/analyze", h.AnalyzeHandler)
	r.Get("/debug/retrieve", h.DebugRetrieveHandler)

	// Versioned API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", h.HealthHandler)
		r.Post("/classify", h.ClassifyHandler)
		r.Post("/clarify", h.ClarifyHandler)
		r.Post("/analyze", h.AnalyzeHandler)
		r.Get("/debug/retrieve", h.DebugRetrieveHandler)
	})

	return r
}
