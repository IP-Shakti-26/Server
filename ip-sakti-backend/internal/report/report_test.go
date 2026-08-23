package report_test

import (
	"testing"

	"github.com/heythisissud/ip-sakti-backend/internal/report"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

func TestGeneratePDF_FullRoadmap(t *testing.T) {
	roadmap := &domain.IPRoadmap{
		ProductSummary:    "Ashwagandha & Shallaki joint-pain formulation",
		Classification:    "proprietary",
		OverallConfidence: 0.85,
		Disclaimer:        "Test disclaimer text.",
		Domains: []domain.DomainAnalysis{
			{
				Domain:          domain.DomainPatent,
				Status:          domain.StatusRelevant,
				Finding:         "Patent filing is possible for novel synergy.",
				KeyRisks:        []string{"Prior art in TKDL", "Section 3(p) objections"},
				Confidence:      0.80,
				NeedsEscalation: true,
				Citations: []domain.Citation{
					{
						DocTitle:  "Indian Patents Act 1970",
						Section:   "Section 3(p)",
						SourceURL: "https://ipindia.gov.in/patents-act",
					},
				},
			},
			{
				Domain:          domain.DomainABS,
				Status:          domain.StatusInsufficientEvidence,
				Finding:         "Requires NBA approval for Indian bio-resources.",
				Confidence:      0.60,
				NeedsEscalation: false,
			},
		},
		JurisdictionNotes: []domain.JurisdictionNote{
			{
				Market:                 "Germany",
				Note:                   "Must comply with EU Novel Food Regulation.",
				RequiresSeparateAnalysis: true,
			},
		},
		NextSteps: []string{
			"Conduct TKDL prior art search",
			"Apply for Form 1 NBA approval",
		},
		HumanEscalation: []domain.EscalationItem{
			{
				ProfType: "Patent Attorney",
				Reason:   "Drafting patent claims to avoid Section 3(p)",
				Urgency:  "before_filing",
			},
		},
	}

	pdfBytes, err := report.GeneratePDF("sess_1234567890", roadmap)
	if err != nil {
		t.Fatalf("GeneratePDF failed: %v", err)
	}
	if len(pdfBytes) == 0 {
		t.Fatal("GeneratePDF returned empty bytes")
	}
}

func TestGeneratePDF_EmptyOptionalFields(t *testing.T) {
	// Tests PDF generation when citations, key risks, jurisdiction notes, and human escalation are empty.
	roadmap := &domain.IPRoadmap{
		ProductSummary:    "Simple Ayurvedic Tea",
		Classification:    "classical",
		OverallConfidence: 0.90,
		Domains: []domain.DomainAnalysis{
			{
				Domain:          domain.DomainTK,
				Status:          domain.StatusNotApplicable,
				Finding:         "Classical formulation in public domain.",
				Confidence:      0.95,
				NeedsEscalation: false,
			},
		},
	}

	pdfBytes, err := report.GeneratePDF("sess_abc", roadmap)
	if err != nil {
		t.Fatalf("GeneratePDF failed with empty optional fields: %v", err)
	}
	if len(pdfBytes) == 0 {
		t.Fatal("GeneratePDF returned empty bytes")
	}
}

func TestGeneratePDF_NilRoadmap(t *testing.T) {
	_, err := report.GeneratePDF("sess_abc", nil)
	if err == nil {
		t.Fatal("Expected error for nil roadmap, got nil")
	}
}
