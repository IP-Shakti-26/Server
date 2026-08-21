package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/heythisissud/ip-sakti-backend/internal/classifier"
	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/internal/store"
	"github.com/heythisissud/ip-sakti-backend/internal/synthesizer"
	"github.com/heythisissud/ip-sakti-backend/pkg/config"
	"github.com/heythisissud/ip-sakti-backend/pkg/respond"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler holds all dependencies needed by the HTTP handlers. All fields are
// injected at construction — no global state.
type Handler struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	store  *store.Store
	logger *slog.Logger
}

// NewHandler constructs a Handler with all required dependencies.
func NewHandler(cfg *config.Config, pool *pgxpool.Pool, st *store.Store, logger *slog.Logger) *Handler {
	return &Handler{
		cfg:    cfg,
		pool:   pool,
		store:  st,
		logger: logger,
	}
}

// HealthHandler handles GET /api/v1/health.
// Pings Postgres and returns 503 if the database is unreachable.
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if err := store.Ping(r.Context(), h.pool); err != nil {
		h.logger.Warn("health check: database unreachable", "error", err)
		respond.JSON(w, http.StatusServiceUnavailable, HealthResponse{
			Status:    "database_unavailable",
			Env:       h.cfg.Env,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	respond.JSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Env:       h.cfg.Env,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ClassifyHandler handles POST /api/v1/classify.
// Validates the description, creates or loads a session, runs classification
// (stub), persists the result, and returns a ClassifyResponse.
func (h *Handler) ClassifyHandler(w http.ResponseWriter, r *http.Request) {
	var req ClassifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Input validation — before any business logic.
	if len(req.Description) < 20 {
		respond.Error(w, http.StatusBadRequest, "description too short", "min_length", 20)
		return
	}
	if len(req.Description) > 2000 {
		respond.Error(w, http.StatusBadRequest, "description too long", "max_length", 2000)
		return
	}

	ctx := r.Context()

	// Create or reuse session.
	var sess *store.Session
	var err error
	if req.SessionID != "" {
		sess, err = h.store.GetSession(ctx, req.SessionID)
		if errors.Is(err, store.ErrSessionNotFound) {
			respond.NotFound(w, "session")
			return
		}
		if err != nil {
			respond.InternalError(w, err, h.logger)
			return
		}
	} else {
		sess, err = h.store.CreateSession(ctx, req.Description)
		if err != nil {
			respond.InternalError(w, err, h.logger)
			return
		}
	}

	// Run classification (stub).
	result, err := classifier.Classify(ctx, req.Description)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	// Persist classification result.
	if err := h.store.UpdateClassification(ctx, sess.ID, result); err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	needsClarification := len(result.ClarifyingQuestions) > 0
	questions := result.ClarifyingQuestions
	if questions == nil {
		questions = []string{}
	}

	respond.JSON(w, http.StatusOK, ClassifyResponse{
		SessionID:           sess.ID,
		Classification:      result,
		NeedsClarification:  needsClarification,
		ClarifyingQuestions: questions,
	})
}

// ClarifyHandler handles POST /api/v1/clarify.
// Loads the session, appends clarification answers, re-runs classification (stub),
// and returns the same shape as /classify.
func (h *Handler) ClarifyHandler(w http.ResponseWriter, r *http.Request) {
	var req ClarifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SessionID == "" {
		respond.Error(w, http.StatusBadRequest, "session_id is required")
		return
	}

	ctx := r.Context()

	sess, err := h.store.GetSession(ctx, req.SessionID)
	if errors.Is(err, store.ErrSessionNotFound) {
		respond.NotFound(w, "session")
		return
	}
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	// Persist the new clarification answers.
	if len(req.Answers) > 0 {
		if err := h.store.UpdateClarificationAnswers(ctx, sess.ID, req.Answers); err != nil {
			respond.InternalError(w, err, h.logger)
			return
		}
	}

	// Re-run classification with the enriched context (stub).
	result, err := classifier.Classify(ctx, sess.RawDescription)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	if err := h.store.UpdateClassification(ctx, sess.ID, result); err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	needsClarification := len(result.ClarifyingQuestions) > 0
	questions := result.ClarifyingQuestions
	if questions == nil {
		questions = []string{}
	}

	respond.JSON(w, http.StatusOK, ClassifyResponse{
		SessionID:           sess.ID,
		Classification:      result,
		NeedsClarification:  needsClarification,
		ClarifyingQuestions: questions,
	})
}

// AnalyzeHandler handles POST /api/v1/analyze.
// Loads the session, verifies classification is done, runs the retrieve →
// synthesize → score pipeline (all stubs), persists and returns the roadmap.
func (h *Handler) AnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SessionID == "" {
		respond.Error(w, http.StatusBadRequest, "session_id is required")
		return
	}

	ctx := r.Context()

	sess, err := h.store.GetSession(ctx, req.SessionID)
	if errors.Is(err, store.ErrSessionNotFound) {
		respond.NotFound(w, "session")
		return
	}
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	// Guard: classification must be complete before analysis.
	if !sess.ClassificationDone || sess.Classification == nil {
		respond.Error(w, http.StatusConflict,
			"classification not complete",
			"hint", "call /classify first",
		)
		return
	}

	// Retrieve evidence (stub — returns empty slices).
	evidence, err := retriever.RetrieveForDomains(
		ctx,
		sess.RawDescription,
		sess.Classification.RelevantDomains,
		"IN",
	)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	// Synthesise roadmap (stub — returns hardcoded demo data).
	roadmap, err := synthesizer.Synthesize(ctx, sess.Classification, evidence)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	// Persist roadmap.
	if err := h.store.UpdateRoadmap(ctx, sess.ID, roadmap); err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	respond.JSON(w, http.StatusOK, AnalyzeResponse{
		SessionID:   sess.ID,
		Roadmap:     roadmap,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
