package synthesizer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
	"google.golang.org/genai"
)

const (
	synthesisModel   = "gemini-2.0-flash"
	synthesisTimeout = 45 * time.Second
	maxRawLogLen     = 500
)

// roadmapDisclaimer is set on every roadmap, unconditionally.
const roadmapDisclaimer = "This output is for informational purposes only and does not " +
	"constitute legal advice. IP-SAKTI is not a substitute for a registered patent agent, " +
	"IP attorney, or legal professional. Consult qualified professionals before taking any " +
	"formal IP or regulatory action."

// roadmapRaw is the intermediate struct the LLM JSON is unmarshalled into.
// It maps directly to the schema in the system prompt. The Citation type here
// only carries ChunkID — the Go code enriches the rest from the evidenceIndex.
type roadmapRaw struct {
	ProductSummary    string                    `json:"product_summary"`
	Classification    string                    `json:"classification"`
	Domains           []domainAnalysisRaw       `json:"domains"`
	JurisdictionNotes []domain.JurisdictionNote `json:"jurisdiction_notes"`
	NextSteps         []string                  `json:"next_steps"`
	HumanEscalation   []domain.EscalationItem   `json:"human_escalation"`
	OverallConfidence float64                   `json:"overall_confidence"`
	Disclaimer        string                    `json:"disclaimer"`
}

type domainAnalysisRaw struct {
	Domain          string          `json:"domain"`
	Status          string          `json:"status"`
	Finding         string          `json:"finding"`
	KeyRisks        []string        `json:"key_risks"`
	Citations       []citationRaw   `json:"citations"`
	Confidence      float64         `json:"confidence"`
	NeedsEscalation bool            `json:"needs_escalation"`
}

// citationRaw captures only the LLM-provided chunk_id. The rest of the fields
// are populated by Go from the evidenceIndex (Step 7).
type citationRaw struct {
	ChunkID string `json:"chunk_id"`
}

// Synthesizer generates a grounded IPRoadmap from a classification and evidence.
// It uses Gemini for synthesis, then validates and enriches citations from the
// actual evidence provided — ensuring no hallucinated chunk IDs survive to output.
type Synthesizer struct {
	client *genai.Client
	logger *slog.Logger
}

// NewSynthesizer constructs a Synthesizer with the supplied Gemini client.
// The client is shared with the Classifier — do not create a second one.
func NewSynthesizer(client *genai.Client, logger *slog.Logger) *Synthesizer {
	return &Synthesizer{client: client, logger: logger}
}

// Synthesize generates an IPRoadmap from a classification and retrieved evidence.
//
// Steps:
//  1. Build evidenceIndex (ground truth for citation validation)
//  2. Build evidence context string
//  3. Build user message
//  4. Call Gemini with 45-second timeout
//  5. Parse JSON response
//  6. Validate citations (drop hallucinated IDs, downgrade empty domains)
//  7. Enrich surviving citations from evidenceIndex
//  8. Set disclaimer and return
func (s *Synthesizer) Synthesize(
	ctx context.Context,
	classification *domain.ClassificationResult,
	evidence []retriever.RetrievalResult,
) (*domain.IPRoadmap, error) {
	// ── Step 1: Build evidence index ─────────────────────────────────────────
	evidenceIndex := make(map[string]retriever.Chunk)
	totalChunks := 0
	for _, res := range evidence {
		for _, chunk := range res.Chunks {
			evidenceIndex[chunk.ID] = chunk
			totalChunks++
		}
	}

	s.logger.Info("synthesizing roadmap",
		"domains", len(evidence),
		"total_chunks", totalChunks,
		"formulation_type", string(classification.FormulationType),
	)

	if totalChunks == 0 {
		s.logger.Warn("empty evidence context",
			"formulation_type", string(classification.FormulationType),
		)
	}

	// ── Step 2: Build evidence context string ─────────────────────────────────
	evidenceContext := buildEvidenceContext(evidence)

	// ── Step 3: Build user message ───────────────────────────────────────────
	userMsg := buildSynthesisPrompt(classification, evidenceContext)

	// ── Step 4: Call Gemini ───────────────────────────────────────────────────
	callCtx, cancel := context.WithTimeout(ctx, synthesisTimeout)
	defer cancel()

	resp, err := s.client.Models.GenerateContent(
		callCtx,
		synthesisModel,
		genai.Text(userMsg),
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(synthesisSystemPrompt)},
			},
			Temperature:      genai.Ptr(float32(0)),
			MaxOutputTokens:  3000,
			ResponseMIMEType: "application/json",
		},
	)
	if err != nil {
		s.logger.Error("gemini synthesis failed", "error", err)
		return nil, fmt.Errorf("synthesizer: Gemini API call failed: %w", err)
	}

	// ── Step 5: Parse response ────────────────────────────────────────────────
	rawText := resp.Text()
	if strings.TrimSpace(rawText) == "" {
		return nil, fmt.Errorf("synthesizer: empty response from Gemini")
	}

	// Strip accidental markdown fences: find first '{' and last '}'.
	cleaned := extractJSON(rawText)

	var raw roadmapRaw
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		preview := rawText
		if len(preview) > maxRawLogLen {
			preview = preview[:maxRawLogLen]
		}
		s.logger.Error("roadmap JSON parse failed",
			"error", err,
			"raw_response_preview", preview,
		)
		return nil, fmt.Errorf("synthesizer: failed to parse roadmap JSON: %w", err)
	}

	// Map raw → strongly-typed domain struct.
	roadmap := mapRawToRoadmap(raw)

	// Validate structure before citation cleaning.
	if err := validateRoadmapStructure(roadmap); err != nil {
		s.logger.Error("roadmap structure invalid", "error", err)
		return nil, err
	}

	// ── Step 6: Validate citations ────────────────────────────────────────────
	// This is the critical invariant: every citation in the output must correspond
	// to a real chunk that was provided in the evidence context.
	validateAndCleanCitations(roadmap, evidenceIndex, s.logger)

	// ── Step 7: Enrich surviving citations ────────────────────────────────────
	// The LLM only outputs ChunkID. We populate the rest from the evidenceIndex
	// so the LLM cannot fabricate doc titles, section numbers, or URLs.
	for i := range roadmap.Domains {
		for j := range roadmap.Domains[i].Citations {
			cit := &roadmap.Domains[i].Citations[j]
			if chunk, ok := evidenceIndex[cit.ChunkID]; ok {
				cit.DocTitle    = chunk.DocTitle
				cit.Section     = chunk.Section
				cit.SourceURL   = chunk.SourceURL
				cit.RetrievedAt = chunk.RetrievedAt
			}
		}
	}

	// ── Step 8: Set mandatory fields ──────────────────────────────────────────
	roadmap.Disclaimer = roadmapDisclaimer

	// ── Log summary ──────────────────────────────────────────────────────────
	escalationCount := 0
	for _, d := range roadmap.Domains {
		if d.NeedsEscalation {
			escalationCount++
		}
	}
	s.logger.Info("roadmap synthesized",
		"overall_confidence", roadmap.OverallConfidence,
		"domains_analyzed", len(roadmap.Domains),
		"escalations", escalationCount,
	)

	// ── Step 9: Return ────────────────────────────────────────────────────────
	return roadmap, nil
}

// extractJSON strips leading/trailing content outside the outermost { } braces.
// Handles the case where Gemini wraps its JSON in a markdown code fence despite
// instructions not to.
func extractJSON(s string) string {
	first := strings.Index(s, "{")
	last := strings.LastIndex(s, "}")
	if first == -1 || last == -1 || last < first {
		return s
	}
	return s[first : last+1]
}

// mapRawToRoadmap converts the intermediate raw parsed struct to the strongly-typed
// domain.IPRoadmap. Citation fields other than ChunkID are left empty here — they
// are populated in Step 7 from the evidenceIndex.
func mapRawToRoadmap(raw roadmapRaw) *domain.IPRoadmap {
	domains := make([]domain.DomainAnalysis, 0, len(raw.Domains))
	for _, rd := range raw.Domains {
		citations := make([]domain.Citation, 0, len(rd.Citations))
		for _, rc := range rd.Citations {
			citations = append(citations, domain.Citation{
				ChunkID: rc.ChunkID,
			})
		}

		keyRisks := rd.KeyRisks
		if keyRisks == nil {
			keyRisks = []string{}
		}

		domains = append(domains, domain.DomainAnalysis{
			Domain:          domain.Domain(rd.Domain),
			Status:          domain.DomainStatus(rd.Status),
			Finding:         rd.Finding,
			KeyRisks:        keyRisks,
			Citations:       citations,
			Confidence:      rd.Confidence,
			NeedsEscalation: rd.NeedsEscalation,
		})
	}

	nextSteps := raw.NextSteps
	if nextSteps == nil {
		nextSteps = []string{}
	}

	humanEscalation := raw.HumanEscalation
	if humanEscalation == nil {
		humanEscalation = []domain.EscalationItem{}
	}

	jurisdictionNotes := raw.JurisdictionNotes
	if jurisdictionNotes == nil {
		jurisdictionNotes = []domain.JurisdictionNote{}
	}

	return &domain.IPRoadmap{
		ProductSummary:    raw.ProductSummary,
		Classification:    raw.Classification,
		Domains:           domains,
		JurisdictionNotes: jurisdictionNotes,
		NextSteps:         nextSteps,
		HumanEscalation:   humanEscalation,
		OverallConfidence: raw.OverallConfidence,
		// Disclaimer is set in Step 8 unconditionally.
	}
}
