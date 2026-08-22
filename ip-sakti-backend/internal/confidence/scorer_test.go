package confidence

import (
	"log/slog"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// ── Test Score ───────────────────────────────────────────────────────────────

func TestScore(t *testing.T) {
	t.Parallel()

	const tolerance = 1e-6

	tests := []struct {
		name     string
		input    ScoreInput
		expected float64
	}{
		{
			name: "Case 1 — Perfect evidence",
			input: ScoreInput{
				ClassificationConfidence: 1.0,
				Evidence: []retriever.RetrievalResult{
					{
						Domain: "patent",
						Chunks: []retriever.Chunk{
							{Authority: retriever.AuthorityStatute},
							{Authority: retriever.AuthorityStatute},
							{Authority: retriever.AuthorityStatute},
						},
					},
					{
						Domain: "regulatory",
						Chunks: []retriever.Chunk{
							{Authority: retriever.AuthorityStatute},
							{Authority: retriever.AuthorityStatute},
							{Authority: retriever.AuthorityStatute},
						},
					},
				},
			},
			// (1.0*0.35) + (1.0*0.40) + (1.0*0.25) = 1.0
			expected: 1.0,
		},
		{
			name: "Case 2 — No evidence at all",
			input: ScoreInput{
				ClassificationConfidence: 0.80,
				Evidence:                 []retriever.RetrievalResult{},
			},
			// (0.80*0.35) + (0.0*0.40) + (0.0*0.25) = 0.28
			expected: 0.28,
		},
		{
			name: "Case 3 — Mixed authority",
			input: ScoreInput{
				ClassificationConfidence: 0.75,
				Evidence: []retriever.RetrievalResult{
					{
						Domain: "patent",
						Chunks: []retriever.Chunk{
							{Authority: retriever.AuthorityStatute},   // 1.0
							{Authority: retriever.AuthoritySecondary}, // 0.25
						},
					},
				},
			},
			// avgAuth = (1.0 + 0.25)/2 = 0.625
			// coverage = 1/1 = 1.0
			// (0.75*0.35) + (0.625*0.40) + (1.0*0.25) = 0.2625 + 0.25 + 0.25 = 0.7625
			expected: 0.7625,
		},
		{
			name: "Case 4 — Low coverage",
			input: ScoreInput{
				ClassificationConfidence: 0.85,
				Evidence: []retriever.RetrievalResult{
					{
						Domain: "patent",
						Chunks: []retriever.Chunk{
							{Authority: retriever.AuthorityStatute},
							{Authority: retriever.AuthorityStatute},
						},
					},
					{
						Domain: "regulatory",
						Chunks: []retriever.Chunk{
							{Authority: retriever.AuthorityStatute},
						},
					},
					{
						Domain: "biodiversity_abs",
						Chunks: []retriever.Chunk{},
					},
				},
			},
			// 3 chunks total across all domains, all statute (1.0) -> avgAuth = 1.0
			// coveredCount = 1 (only patent has >= 2 chunks) out of 3 domains -> coverage = 1/3
			// (0.85*0.35) + (1.0*0.40) + ((1.0/3.0)*0.25) = 0.2975 + 0.40 + 0.0833333 = 0.7808333
			expected: (0.85 * 0.35) + (1.0 * 0.40) + ((1.0 / 3.0) * 0.25),
		},
		{
			name: "Case 5 — Output clamped to [0,1]",
			input: ScoreInput{
				ClassificationConfidence: 2.0, // out of range high
				Evidence: []retriever.RetrievalResult{
					{
						Domain: "patent",
						Chunks: []retriever.Chunk{
							{Authority: retriever.AuthorityStatute},
							{Authority: retriever.AuthorityStatute},
						},
					},
				},
			},
			expected: 1.0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Score(tc.input)
			if math.Abs(got-tc.expected) > tolerance {
				t.Errorf("Score() = %v, expected %v (diff: %v)", got, tc.expected, math.Abs(got-tc.expected))
			}
			if got < 0.0 || got > 1.0 {
				t.Errorf("Score() = %v out of range [0.0, 1.0]", got)
			}
		})
	}
}

// ── Test DetermineEscalations ────────────────────────────────────────────────

func TestDetermineEscalations(t *testing.T) {
	t.Parallel()

	t.Run("Case 1 — ABS + commercial intent triggers nba_expert", func(t *testing.T) {
		t.Parallel()
		classification := &domain.ClassificationResult{
			IndianBioResources: true,
			TargetMarkets:      []string{"india"},
		}
		items := DetermineEscalations(classification, nil, 0.85)

		var found bool
		for _, it := range items {
			if it.ProfType == "nba_expert" && it.Urgency == "before_commercialization" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected nba_expert escalation before_commercialization, got %v", items)
		}
	})

	t.Run("Case 2 — International market triggers ip_attorney", func(t *testing.T) {
		t.Parallel()
		classification := &domain.ClassificationResult{
			TargetMarkets: []string{"india", "germany"},
		}
		items := DetermineEscalations(classification, nil, 0.85)

		var found bool
		for _, it := range items {
			if it.ProfType == "ip_attorney" && strings.Contains(it.Reason, "germany") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected ip_attorney escalation for germany, got %v", items)
		}
	})

	t.Run("Case 3 — new_drug triggers regulatory_expert", func(t *testing.T) {
		t.Parallel()
		classification := &domain.ClassificationResult{
			FormulationType: domain.FormulationNewDrug,
		}
		items := DetermineEscalations(classification, nil, 0.85)

		var found bool
		for _, it := range items {
			if it.ProfType == "regulatory_expert" && it.Urgency == "before_commercialization" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected regulatory_expert escalation for new_drug, got %v", items)
		}
	})

	t.Run("Case 4 — TK + patent triggers patent_agent before_filing", func(t *testing.T) {
		t.Parallel()
		classification := &domain.ClassificationResult{
			TKInvolved:      true,
			RelevantDomains: []domain.Domain{domain.DomainPatent},
		}
		items := DetermineEscalations(classification, nil, 0.85)

		var found bool
		for _, it := range items {
			if it.ProfType == "patent_agent" && it.Urgency == "before_filing" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected patent_agent before_filing for TK + patent, got %v", items)
		}
	})

	t.Run("Case 5 — Low confidence triggers patent_agent recommended", func(t *testing.T) {
		t.Parallel()
		classification := &domain.ClassificationResult{}
		items := DetermineEscalations(classification, nil, 0.45)

		var found bool
		for _, it := range items {
			if it.ProfType == "patent_agent" && it.Urgency == "recommended" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected patent_agent recommended for confidence < 0.60, got %v", items)
		}
	})

	t.Run("Case 6 — High confidence, no special flags", func(t *testing.T) {
		t.Parallel()
		classification := &domain.ClassificationResult{
			FormulationType:    domain.FormulationProprietary,
			IndianBioResources: false,
			TKInvolved:         false,
			TargetMarkets:      []string{"india"},
			RelevantDomains:    []domain.Domain{domain.DomainRegulatory},
		}
		roadmap := &domain.IPRoadmap{
			Domains: []domain.DomainAnalysis{
				{
					Domain:          domain.DomainRegulatory,
					Status:          domain.StatusRelevant,
					NeedsEscalation: false,
				},
			},
		}
		items := DetermineEscalations(classification, roadmap, 0.85)
		if len(items) != 0 {
			t.Errorf("expected 0 escalations for clean scenario, got %v", items)
		}
	})
}

// ── Test mergeEscalations ────────────────────────────────────────────────────

func TestMergeEscalations(t *testing.T) {
	t.Parallel()

	t.Run("Case 1 — No overlap", func(t *testing.T) {
		t.Parallel()
		ruleItems := []domain.EscalationItem{
			{ProfType: "nba_expert", Urgency: "before_commercialization", Reason: "Rule NBA"},
		}
		llmItems := []domain.EscalationItem{
			{ProfType: "patent_agent", Urgency: "before_filing", Reason: "LLM Patent"},
		}

		merged := mergeEscalations(llmItems, ruleItems)
		if len(merged) != 2 {
			t.Fatalf("expected 2 items, got %d", len(merged))
		}
		// Priority sort: before_filing (0) comes before before_commercialization (1)
		if merged[0].ProfType != "patent_agent" || merged[0].Urgency != "before_filing" {
			t.Errorf("expected first item to be patent_agent before_filing, got %+v", merged[0])
		}
		if merged[1].ProfType != "nba_expert" || merged[1].Urgency != "before_commercialization" {
			t.Errorf("expected second item to be nba_expert before_commercialization, got %+v", merged[1])
		}
	})

	t.Run("Case 2 — Duplicate by (ProfType, Urgency)", func(t *testing.T) {
		t.Parallel()
		ruleItems := []domain.EscalationItem{
			{ProfType: "patent_agent", Urgency: "before_filing", Reason: "rule reason"},
		}
		llmItems := []domain.EscalationItem{
			{ProfType: "patent_agent", Urgency: "before_filing", Reason: "llm reason"},
		}

		merged := mergeEscalations(llmItems, ruleItems)
		if len(merged) != 1 {
			t.Fatalf("expected 1 item after deduplication, got %d", len(merged))
		}
		if merged[0].Reason != "rule reason" {
			t.Errorf("expected rule reason to take priority, got %s", merged[0].Reason)
		}
	})

	t.Run("Case 3 — Ordering by urgency priority", func(t *testing.T) {
		t.Parallel()
		items := []domain.EscalationItem{
			{ProfType: "trademark_agent", Urgency: "recommended"},
			{ProfType: "nba_expert", Urgency: "before_commercialization"},
			{ProfType: "patent_agent", Urgency: "before_filing"},
		}

		merged := mergeEscalations(items, nil)
		if len(merged) != 3 {
			t.Fatalf("expected 3 items, got %d", len(merged))
		}
		if merged[0].Urgency != "before_filing" {
			t.Errorf("expected 1st item urgency before_filing, got %s", merged[0].Urgency)
		}
		if merged[1].Urgency != "before_commercialization" {
			t.Errorf("expected 2nd item urgency before_commercialization, got %s", merged[1].Urgency)
		}
		if merged[2].Urgency != "recommended" {
			t.Errorf("expected 3rd item urgency recommended, got %s", merged[2].Urgency)
		}
	})
}

// ── Test Finalize ────────────────────────────────────────────────────────────

func TestFinalize(t *testing.T) {
	t.Parallel()

	t.Run("Case 1 — Disclaimer always overwritten", func(t *testing.T) {
		t.Parallel()
		roadmap := &domain.IPRoadmap{
			Disclaimer: "custom text from LLM",
			Domains: []domain.DomainAnalysis{
				{Domain: domain.DomainPatent, Status: domain.StatusRelevant, Confidence: 0.8},
			},
			NextSteps: []string{"Step 1"},
		}
		Finalize(roadmap, &domain.ClassificationResult{}, nil, testLogger())

		if roadmap.Disclaimer != canonicalDisclaimer {
			t.Errorf("expected canonical disclaimer, got %q", roadmap.Disclaimer)
		}
	})

	t.Run("Case 2 — Empty next_steps gets fallback", func(t *testing.T) {
		t.Parallel()
		roadmap := &domain.IPRoadmap{
			NextSteps: []string{},
			Domains: []domain.DomainAnalysis{
				{Domain: domain.DomainPatent, Status: domain.StatusRelevant, Confidence: 0.8},
			},
		}
		Finalize(roadmap, &domain.ClassificationResult{}, nil, testLogger())

		if len(roadmap.NextSteps) != 3 {
			t.Fatalf("expected 3 fallback next steps, got %d", len(roadmap.NextSteps))
		}
	})

	t.Run("Case 3 — Insufficient evidence domain confidence clamped", func(t *testing.T) {
		t.Parallel()
		roadmap := &domain.IPRoadmap{
			Domains: []domain.DomainAnalysis{
				{
					Domain:     domain.DomainPatent,
					Status:     domain.StatusInsufficientEvidence,
					Confidence: 0.85,
				},
			},
			NextSteps: []string{"Step 1"},
		}
		Finalize(roadmap, &domain.ClassificationResult{}, nil, testLogger())

		if roadmap.Domains[0].Confidence > 0.40 {
			t.Errorf("expected confidence <= 0.40 for insufficient_evidence domain, got %v", roadmap.Domains[0].Confidence)
		}
	})

	t.Run("Case 4 — overall_confidence is overwritten", func(t *testing.T) {
		t.Parallel()
		roadmap := &domain.IPRoadmap{
			OverallConfidence: 0.99, // LLM was overconfident
			Domains: []domain.DomainAnalysis{
				{Domain: domain.DomainPatent, Status: domain.StatusRelevant, Confidence: 0.8},
			},
			NextSteps: []string{"Step 1"},
		}
		classification := &domain.ClassificationResult{Confidence: 0.60}

		Finalize(roadmap, classification, []retriever.RetrievalResult{}, testLogger())

		// With empty evidence and 0.60 classification conf, score = 0.60 * 0.35 = 0.21
		if roadmap.OverallConfidence >= 0.99 {
			t.Errorf("expected overall_confidence to be overwritten, got %v", roadmap.OverallConfidence)
		}
		const expectedScore = 0.60 * 0.35
		if math.Abs(roadmap.OverallConfidence-expectedScore) > 1e-6 {
			t.Errorf("expected score %v, got %v", expectedScore, roadmap.OverallConfidence)
		}
	})
}
