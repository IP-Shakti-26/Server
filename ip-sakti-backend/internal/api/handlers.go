package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/heythisissud/ip-sakti-backend/internal/classifier"
	"github.com/heythisissud/ip-sakti-backend/internal/confidence"
	"github.com/heythisissud/ip-sakti-backend/internal/report"
	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/internal/store"
	"github.com/heythisissud/ip-sakti-backend/internal/summary"
	"github.com/heythisissud/ip-sakti-backend/internal/synthesizer"
	"github.com/heythisissud/ip-sakti-backend/pkg/config"
	"github.com/heythisissud/ip-sakti-backend/pkg/respond"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler holds all injected dependencies for the HTTP layer.
type Handler struct {
	cfg         *config.Config
	pool        *pgxpool.Pool
	store       *store.Store
	classifier  *classifier.Classifier
	retriever   *retriever.Retriever
	synthesizer *synthesizer.Synthesizer
	summarizer  *summary.Summarizer
	logger      *slog.Logger
}

// NewHandler constructs a Handler with all required dependencies.
func NewHandler(cfg *config.Config, pool *pgxpool.Pool, st *store.Store, cl *classifier.Classifier, ret *retriever.Retriever, syn *synthesizer.Synthesizer, sum *summary.Summarizer, logger *slog.Logger) *Handler {
	return &Handler{
		cfg:         cfg,
		pool:        pool,
		store:       st,
		classifier:  cl,
		retriever:   ret,
		synthesizer: syn,
		summarizer:  sum,
		logger:      logger,
	}
}

// HealthHandler handles GET /api/v1/health.
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


func (h *Handler) DebugRetrieveHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	evidence, err := h.retriever.RetrieveForDomains(ctx, retriever.RetrieveRequest{
		BaseQuery:    "Ashwagandha joint pain formulation patent India brand name commercial sale",
		Domains:      []string{"patent", "biodiversity_abs", "traditional_knowledge", "regulatory", "trademark"},
		Jurisdiction: "india",
		TopK:         3,
	})
	if err != nil {
		respond.JSON(w, 500, map[string]any{"step": "retrieve_failed", "error": err.Error()})
		return
	}

	respond.JSON(w, 200, map[string]any{
		"status":   "success",
		"evidence": evidence,
	})
}
// ClassifyHandler handles POST /api/v1/classify.
func (h *Handler) ClassifyHandler(w http.ResponseWriter, r *http.Request) {
	var req ClassifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Input validation before any business logic.
	if len(req.Description) < 20 {
		respond.Error(w, http.StatusBadRequest, "description too short", "min_length", 20)
		return
	}
	if len(req.Description) > 2000 {
		respond.Error(w, http.StatusBadRequest, "description too long", "max_length", 2000)
		return
	}

	ctx := r.Context()

	// Create or load session.
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

	// Merge existing clarification answers from session.
	clarifications := sess.ClarificationAnswers
	if clarifications == nil {
		clarifications = map[string]string{}
	}

	// Run real classification via Gemini.
	result, err := h.classifier.Classify(ctx, req.Description, clarifications)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	// Persist classification. Mark as done ONLY when no clarification is needed.
	needsClarification := len(result.ClarifyingQuestions) > 0
	if err := h.store.SaveClassification(ctx, sess.ID, result, !needsClarification); err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

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

	// Reload session to get merged clarification answers.
	sess, err = h.store.GetSession(ctx, sess.ID)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	clarifications := sess.ClarificationAnswers
	if clarifications == nil {
		clarifications = map[string]string{}
	}

	// Re-run classification with enriched context.
	result, err := h.classifier.Classify(ctx, sess.RawDescription, clarifications)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	needsClarification := len(result.ClarifyingQuestions) > 0
	if err := h.store.SaveClassification(ctx, sess.ID, result, !needsClarification); err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

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

	// Guard: classification must be marked done.
	if !sess.ClassificationDone || sess.Classification == nil {
		respond.Error(w, http.StatusConflict,
			"classification not complete",
			"hint", "call /classify first",
		)
		return
	}

	// Step 4 — Build retrieval request.
	// Convert []domain.Domain to []string for the retriever.
	domainStrs := make([]string, len(sess.Classification.RelevantDomains))
	for i, d := range sess.Classification.RelevantDomains {
		domainStrs[i] = string(d)
	}

	// Step 5 — Retrieve evidence from Qdrant.
	// Partial failure is tolerated: one domain failing does not abort the call.
	evidence, err := h.retriever.RetrieveForDomains(ctx, retriever.RetrieveRequest{
		BaseQuery:    sess.Classification.RawDescription,
		Domains:      domainStrs,
		Jurisdiction: "india",
		TopK:         8,
	})
	if err != nil {
		// RetrieveForDomains only returns a non-nil error for catastrophic failures;
		// per-domain errors are logged and swallowed inside the method.
		respond.InternalError(w, err, h.logger)
		return
	}

	// Step 6 — Log retrieval counts per domain at INFO level.
	for _, result := range evidence {
		h.logger.Info("retrieved evidence",
			"domain", result.Domain,
			"chunks", len(result.Chunks),
			"session_id", req.SessionID,
		)
	}

	// Step 6 — Synthesize the roadmap via Gemini.
	roadmap, err := h.synthesizer.Synthesize(ctx, sess.Classification, evidence)
	if err != nil {
		// Check specifically for synthesis timeout — return 504, not 500.
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "504") || strings.Contains(err.Error(), "DEADLINE_EXCEEDED") {
			h.logger.Error("synthesis timed out", "session_id", req.SessionID, "error", err)
			respond.Error(w, http.StatusGatewayTimeout, "synthesis timed out", "hint", "retry the request")
			return
		}
		h.logger.Error("gemini synthesis failed", "session_id", req.SessionID, "error", err)
		respond.InternalError(w, err, h.logger)
		return
	}

	// Step 7 — Finalize (confidence scoring, escalation, consistency overrides).
	// This is the D5 addition. Mutates roadmap in-place.
	confidence.Finalize(roadmap, sess.Classification, evidence, h.logger)

	// Step 8 — Persist roadmap. This is best-effort: if the DB write fails,
	// the roadmap is still returned to the client. Log at ERROR but do not 500.
	if err := h.store.UpdateRoadmap(ctx, sess.ID, roadmap); err != nil {
		h.logger.Error("failed to persist roadmap",
			"session_id", req.SessionID,
			"error", err,
		)
		// Intentionally continue — do not fail the request.
	}

	// Step 8 — Respond.
	respond.JSON(w, http.StatusOK, AnalyzeResponse{
		SessionID:   sess.ID,
		Roadmap:     roadmap,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// ReportHandler handles GET /api/v1/report/{session_id}.
func (h *Handler) ReportHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	if sessionID == "" {
		respond.Error(w, http.StatusBadRequest, "session_id is required")
		return
	}

	ctx := r.Context()
	sess, err := h.store.GetSession(ctx, sessionID)
	if errors.Is(err, store.ErrSessionNotFound) {
		respond.NotFound(w, "session")
		return
	}
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	if sess.Roadmap == nil {
		respond.Error(w, http.StatusConflict, "analysis not complete", "hint", "call /analyze first")
		return
	}

	pdfBytes, err := report.GeneratePDF(sessionID, sess.Roadmap)
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	sessShort := sessionID
	if len(sessShort) > 8 {
		sessShort = sessShort[:8]
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=ipsakti-report-%s.pdf", sessShort))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// SummaryHandler handles GET /api/v1/summary/{session_id}.
func (h *Handler) SummaryHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	if sessionID == "" {
		respond.Error(w, http.StatusBadRequest, "session_id is required")
		return
	}

	ctx := r.Context()
	sess, err := h.store.GetSession(ctx, sessionID)
	if errors.Is(err, store.ErrSessionNotFound) {
		respond.NotFound(w, "session")
		return
	}
	if err != nil {
		respond.InternalError(w, err, h.logger)
		return
	}

	if sess.Roadmap == nil {
		respond.Error(w, http.StatusConflict, "analysis not complete", "hint", "call /analyze first")
		return
	}

	sumOut := h.summarizer.Summarize(ctx, sess.Roadmap)

	respond.JSON(w, http.StatusOK, SummaryResponse{
		SessionID:       sessionID,
		Headline:        sumOut.Headline,
		Summary:         sumOut.Summary,
		TopAction:       sumOut.TopAction,
		ConfidenceLabel: sumOut.ConfidenceLabel,
		ConfidenceValue: sumOut.ConfidenceValue,
	})
}

// ExamplesHandler handles GET /api/v1/examples.
func (h *Handler) ExamplesHandler(w http.ResponseWriter, r *http.Request) {
	examples := []Example{
		{
			ID:          "ex_01",
			Title:       "Joint Pain Formulation (India + Germany)",
			Description: "I created a new Ayurvedic joint-pain formulation using Ashwagandha and Shallaki. The ingredients are sourced from India. It is not directly copied from a classical formulation. I want to sell it in India and later Germany.",
			Tags:        []string{"proprietary", "international", "abs", "patent"},
			Complexity:  "high",
		},
		{
			ID:          "ex_02",
			Title:       "Classical Dashamoola Preparation",
			Description: "I am preparing Dashamoola Kwatha exactly as described in Charaka Samhita. All ten roots, traditional preparation method, same proportions. Herbs sourced from Indian forests. Planning to sell only within India to Ayurvedic practitioners.",
			Tags:        []string{"classical", "traditional_knowledge", "india_only"},
			Complexity:  "medium",
		},
		{
			ID:          "ex_03",
			Title:       "Herbal Sleep Supplement",
			Description: "I have developed a herbal sleep supplement using Ashwagandha root extract and Brahmi. It is in capsule form, not a traditional preparation. I am marketing it as a natural food supplement for stress and sleep. Sourced from certified organic farms in India. Selling online in India only.",
			Tags:        []string{"food_nutraceutical", "regulatory", "trademark"},
			Complexity:  "medium",
		},
		{
			ID:          "ex_04",
			Title:       "Novel Extraction Process",
			Description: "I have developed a new cold-press extraction process for Turmeric that preserves curcumin levels 40% higher than standard methods. The formulation itself uses classical ingredients but the process is entirely new. I want to patent the process and sell the product globally including USA and EU.",
			Tags:        []string{"process_patent", "international", "proprietary"},
			Complexity:  "high",
		},
	}
	respond.JSON(w, http.StatusOK, map[string]any{"examples": examples})
}

