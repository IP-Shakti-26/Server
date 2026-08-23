package summary_test

import (
	"context"
	"testing"

	"github.com/heythisissud/ip-sakti-backend/internal/summary"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

func TestSummarizer_Fallback(t *testing.T) {
	// With a nil Gemini client, Summarize should fall back smoothly without panicking.
	sum := summary.NewSummarizer(nil, nil)

	roadmap := &domain.IPRoadmap{
		ProductSummary:    "Novel herbal formulation combining Ashwagandha and Shallaki for joint pain relief",
		Classification:    "proprietary",
		OverallConfidence: 0.82,
		Domains: []domain.DomainAnalysis{
			{Domain: domain.DomainPatent, Status: domain.StatusRelevant, NeedsEscalation: true},
			{Domain: domain.DomainABS, Status: domain.StatusRelevant, NeedsEscalation: false},
		},
		NextSteps: []string{
			"Conduct a patentability search",
			"Submit NBA Form 1",
		},
	}

	out := sum.Summarize(context.Background(), roadmap)
	if out == nil {
		t.Fatal("Expected non-nil summary output")
	}

	if out.ConfidenceLabel != "High" {
		t.Errorf("Expected ConfidenceLabel 'High', got '%s'", out.ConfidenceLabel)
	}

	if out.ConfidenceValue != 0.82 {
		t.Errorf("Expected ConfidenceValue 0.82, got %f", out.ConfidenceValue)
	}

	if out.TopAction != "Conduct a patentability search" {
		t.Errorf("Expected top action from NextSteps[0], got '%s'", out.TopAction)
	}
}

func TestSummarizer_ConfidenceLabels(t *testing.T) {
	sum := summary.NewSummarizer(nil, nil)

	tests := []struct {
		confidence float64
		expected   string
	}{
		{0.85, "High"},
		{0.75, "High"},
		{0.74, "Medium"},
		{0.50, "Medium"},
		{0.49, "Low"},
		{0.10, "Low"},
	}

	for _, tt := range tests {
		roadmap := &domain.IPRoadmap{
			ProductSummary:    "Test product",
			OverallConfidence: tt.confidence,
		}
		out := sum.Summarize(context.Background(), roadmap)
		if out.ConfidenceLabel != tt.expected {
			t.Errorf("For confidence %.2f, expected label '%s', got '%s'", tt.confidence, tt.expected, out.ConfidenceLabel)
		}
	}
}
