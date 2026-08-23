package summary

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
	"google.golang.org/genai"
)

const (
	summaryModel   = "gemini-3.5-flash-lite"
	summaryTimeout = 4 * time.Second
)

type SummaryOutput struct {
	Headline        string  `json:"headline"`
	Summary         string  `json:"summary"`
	TopAction       string  `json:"top_action"`
	ConfidenceLabel string  `json:"confidence_label"`
	ConfidenceValue float64 `json:"confidence_value"`
}

type llmSummaryResult struct {
	Headline  string `json:"headline"`
	Summary   string `json:"summary"`
	TopAction string `json:"top_action"`
}

type Summarizer struct {
	client *genai.Client
	logger *slog.Logger
}

func NewSummarizer(client *genai.Client, logger *slog.Logger) *Summarizer {
	return &Summarizer{
		client: client,
		logger: logger,
	}
}

func (s *Summarizer) Summarize(ctx context.Context, roadmap *domain.IPRoadmap) *SummaryOutput {
	// Compute confidence label and value
	confVal := roadmap.OverallConfidence
	confLabel := "Low"
	if confVal >= 0.75 {
		confLabel = "High"
	} else if confVal >= 0.50 {
		confLabel = "Medium"
	}

	// Attempt LLM summary with timeout
	llmRes, err := s.callGemini(ctx, roadmap)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Gemini summary failed or timed out, using fallback", "error", err)
		}
		llmRes = s.computeFallback(roadmap)
	}

	return &SummaryOutput{
		Headline:        llmRes.Headline,
		Summary:         llmRes.Summary,
		TopAction:       llmRes.TopAction,
		ConfidenceLabel: confLabel,
		ConfidenceValue: confVal,
	}
}

func (s *Summarizer) callGemini(ctx context.Context, roadmap *domain.IPRoadmap) (*llmSummaryResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("gemini client is nil")
	}

	callCtx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()

	relevantCount := 0
	needsEscalation := false
	for _, d := range roadmap.Domains {
		if d.Status == domain.StatusRelevant || d.Status == "relevant" {
			relevantCount++
		}
		if d.NeedsEscalation {
			needsEscalation = true
		}
	}

	mostUrgentEsc := "None"
	if len(roadmap.HumanEscalation) > 0 {
		mostUrgentEsc = fmt.Sprintf("%s (%s)", roadmap.HumanEscalation[0].Reason, roadmap.HumanEscalation[0].ProfType)
	}

	userPrompt := fmt.Sprintf(
		"Product: %s\n"+
			"Classification: %s\n"+
			"Domains analyzed: %d\n"+
			"Domains with sufficient evidence: %d\n"+
			"Needs escalation: %t\n"+
			"Most urgent escalation: %s\n"+
			"Next steps count: %d\n"+
			"Overall confidence: %.2f\n\n"+
			"Produce:\n"+
			"{\n"+
			"  \"headline\": \"One sentence — the most critical finding for this innovator\",\n"+
			"  \"summary\": \"2-3 plain English sentences. No jargon. No statute names. What does this person need to understand right now?\",\n"+
			"  \"top_action\": \"The single most important thing to do next, specific and actionable\"\n"+
			"}",
		roadmap.ProductSummary,
		roadmap.Classification,
		len(roadmap.Domains),
		relevantCount,
		needsEscalation,
		mostUrgentEsc,
		len(roadmap.NextSteps),
		roadmap.OverallConfidence,
	)

	sysPrompt := "You are a plain-English translator for legal/IP analysis results.\n" +
		"Your job is to summarize a complex IP roadmap in 2-3 sentences that\n" +
		"a non-lawyer small business owner can immediately understand.\n" +
		"Output ONLY valid JSON. No preamble. No markdown."

	resp, err := s.client.Models.GenerateContent(
		callCtx,
		summaryModel,
		genai.Text(userPrompt),
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(sysPrompt)},
			},
			Temperature:      genai.Ptr(float32(0)),
			MaxOutputTokens:  500,
			ResponseMIMEType: "application/json",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("generate content error: %w", err)
	}

	rawText := resp.Text()
	cleaned := extractJSON(rawText)

	var res llmSummaryResult
	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return nil, fmt.Errorf("unmarshal summary json error: %w (raw: %s)", err, rawText)
	}

	if res.Headline == "" || res.Summary == "" || res.TopAction == "" {
		return nil, fmt.Errorf("incomplete json output from LLM")
	}

	return &res, nil
}

func (s *Summarizer) computeFallback(roadmap *domain.IPRoadmap) *llmSummaryResult {
	headline := roadmap.ProductSummary
	if len(headline) > 100 {
		headline = headline[:100]
	}

	n := len(roadmap.Domains)
	m := 0
	for _, d := range roadmap.Domains {
		if d.NeedsEscalation {
			m++
		}
	}

	summaryText := "Your formulation has been analyzed across " + strconv.Itoa(n) + " legal domains. " +
		strconv.Itoa(m) + " domains require professional review."

	topAction := "Consult a registered IP professional"
	if len(roadmap.NextSteps) > 0 && strings.TrimSpace(roadmap.NextSteps[0]) != "" {
		topAction = roadmap.NextSteps[0]
	}

	return &llmSummaryResult{
		Headline:  headline,
		Summary:   summaryText,
		TopAction: topAction,
	}
}

func extractJSON(s string) string {
	first := strings.Index(s, "{")
	last := strings.LastIndex(s, "}")
	if first == -1 || last == -1 || last < first {
		return s
	}
	return s[first : last+1]
}
