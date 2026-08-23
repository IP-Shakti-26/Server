package api

import "github.com/heythisissud/ip-sakti-backend/pkg/domain"

// ── Input types ──────────────────────────────────────────────────────────────

// ClassifyRequest is the body accepted by POST /api/v1/classify.
type ClassifyRequest struct {
	Description string `json:"description"`
	SessionID   string `json:"session_id,omitempty"`
}

// ClarifyRequest is the body accepted by POST /api/v1/clarify.
type ClarifyRequest struct {
	SessionID string            `json:"session_id"`
	Answers   map[string]string `json:"answers"`
}

// AnalyzeRequest is the body accepted by POST /api/v1/analyze.
type AnalyzeRequest struct {
	SessionID string `json:"session_id"`
}

// ── Response wrappers ─────────────────────────────────────────────────────────

// ClassifyResponse is returned by both /classify and /clarify.
type ClassifyResponse struct {
	SessionID           string                       `json:"session_id"`
	Classification      *domain.ClassificationResult `json:"classification"`
	NeedsClarification  bool                         `json:"needs_clarification"`
	ClarifyingQuestions []string                     `json:"clarifying_questions"`
}

// AnalyzeResponse is returned by /analyze.
type AnalyzeResponse struct {
	SessionID   string          `json:"session_id"`
	Roadmap     *domain.IPRoadmap `json:"roadmap"`
	GeneratedAt string          `json:"generated_at"`
}

// HealthResponse is returned by /health.
type HealthResponse struct {
	Status    string `json:"status"`
	Env       string `json:"env"`
	Timestamp string `json:"timestamp"`
}

// SummaryResponse is returned by /summary/:session_id.
type SummaryResponse struct {
	SessionID       string  `json:"session_id"`
	Headline        string  `json:"headline"`
	Summary         string  `json:"summary"`
	TopAction       string  `json:"top_action"`
	ConfidenceLabel string  `json:"confidence_label"`
	ConfidenceValue float64 `json:"confidence_value"`
}

// Example represents a demo sample formulation.
type Example struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Complexity  string   `json:"complexity"`
}

// ExamplesResponse is returned by /examples.
type ExamplesResponse struct {
	Examples []Example `json:"examples"`
}

