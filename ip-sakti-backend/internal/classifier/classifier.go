package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
	"google.golang.org/genai"
)

const (
	geminiModel     = "gemini-2.0-flash"
	classifyTimeout = 30 * time.Second
)

// classificationRaw is the intermediate struct used for JSON parsing before
// mapping to the strongly-typed domain.ClassificationResult.
type classificationRaw struct {
	FormulationType     string   `json:"formulation_type"`
	IndianBioResources  bool     `json:"indian_bio_resources"`
	TKInvolved          bool     `json:"tk_involved"`
	TargetMarkets       []string `json:"target_markets"`
	RelevantDomains     []string `json:"relevant_domains"`
	ClarifyingQuestions []string `json:"clarifying_questions"`
	Confidence          float64  `json:"confidence"`
}

// Classifier wraps the Gemini client and provides product classification.
// All dependencies are injected — no global state.
type Classifier struct {
	client *genai.Client
	logger *slog.Logger
}

// NewClassifier constructs a Classifier with the supplied Gemini client.
func NewClassifier(client *genai.Client, logger *slog.Logger) *Classifier {
	return &Classifier{client: client, logger: logger}
}

// Classify classifies an Ayurvedic product description using Gemini.
//
//   - description: raw text from the innovator
//   - clarifications: map of question_index → answer collected from /clarify;
//     pass an empty map on the first /classify call
func (c *Classifier) Classify(ctx context.Context, description string, clarifications map[string]string) (*domain.ClassificationResult, error) {
	// Step 1 — Validate input.
	if err := validateInput(description); err != nil {
		return nil, err
	}

	// Step 2 — Build user message.
	userMsg := buildClassificationPrompt(description, clarifications)

	// Step 3 — Call Gemini with a hard timeout.
	callCtx, cancel := context.WithTimeout(ctx, classifyTimeout)
	defer cancel()

	resp, err := c.client.Models.GenerateContent(
		callCtx,
		geminiModel,
		genai.Text(userMsg),
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(classificationSystemPrompt)},
			},
			Temperature:      genai.Ptr(float32(0)),
			MaxOutputTokens:  1024,
			ResponseMIMEType: "application/json",
		},
	)
	if err != nil {
		if callCtx.Err() != nil {
			return nil, fmt.Errorf("classification timed out: %w", callCtx.Err())
		}
		return nil, fmt.Errorf("classifier: Gemini API call failed: %w", err)
	}

	// Step 4 — Extract text from response.
	rawText := resp.Text()
	if strings.TrimSpace(rawText) == "" {
		return nil, fmt.Errorf("classifier: no text in Gemini response")
	}
	c.logger.Debug("gemini raw response", "text", rawText)

	// Step 5 — Strip any accidental markdown fences and parse JSON.
	cleaned := extractJSON(rawText)
	var raw classificationRaw
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		c.logger.Error("failed to parse Gemini response",
			"raw", rawText,
			"cleaned", cleaned,
			"error", err,
		)
		return nil, fmt.Errorf("classifier: failed to parse Gemini response: %w", err)
	}

	// Step 6 — Validate output semantics.
	if err := validateOutput(raw); err != nil {
		c.logger.Error("Gemini output failed validation", "error", err, "raw", rawText)
		return nil, fmt.Errorf("classifier: invalid Gemini output: %w", err)
	}

	// Step 7 — Confidence gate: inject a fallback clarifying question if needed.
	if raw.Confidence < 0.65 && len(raw.ClarifyingQuestions) == 0 {
		raw.ClarifyingQuestions = []string{
			"Could you describe what makes your formulation different from existing Ayurvedic preparations, and whether the ingredients are sourced from India?",
		}
	}

	// Step 8 — Map raw → domain type and return.
	result := mapToResult(raw, description)
	c.logger.Info("classified",
		"type", string(result.FormulationType),
		"confidence", result.Confidence,
		"needs_clarification", len(result.ClarifyingQuestions) > 0,
	)
	return result, nil
}

// extractJSON strips leading/trailing content outside the outermost { } braces.
// This handles the case where Gemini wraps its JSON in a markdown code fence
// despite instructions not to.
func extractJSON(s string) string {
	first := strings.Index(s, "{")
	last := strings.LastIndex(s, "}")
	if first == -1 || last == -1 || last < first {
		return s
	}
	return s[first : last+1]
}

// mapToResult converts the raw parsed struct to the strongly-typed domain result.
func mapToResult(raw classificationRaw, description string) *domain.ClassificationResult {
	domains := make([]domain.Domain, 0, len(raw.RelevantDomains))
	for _, d := range raw.RelevantDomains {
		domains = append(domains, domain.Domain(d))
	}

	questions := raw.ClarifyingQuestions
	if questions == nil {
		questions = []string{}
	}

	markets := raw.TargetMarkets
	if markets == nil {
		markets = []string{}
	}

	return &domain.ClassificationResult{
		FormulationType:     domain.FormulationType(raw.FormulationType),
		IndianBioResources:  raw.IndianBioResources,
		TKInvolved:          raw.TKInvolved,
		TargetMarkets:       markets,
		RelevantDomains:     domains,
		ClarifyingQuestions: questions,
		Confidence:          raw.Confidence,
		RawDescription:      description,
	}
}
