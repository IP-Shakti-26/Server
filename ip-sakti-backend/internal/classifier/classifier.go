package classifier

import (
	"context"

	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// Classify analyses a product description and returns a ClassificationResult.
// STUB: returns a hardcoded result representing a proprietary Ayurvedic
// formulation with Indian bio-resources and traditional knowledge involvement.
// Real AI classification will be wired here in a later deliverable.
func Classify(_ context.Context, description string) (*domain.ClassificationResult, error) {
	return &domain.ClassificationResult{
		FormulationType:    domain.FormulationProprietary,
		IndianBioResources: true,
		TKInvolved:         true,
		TargetMarkets:      []string{"India", "Germany"},
		RelevantDomains: []domain.Domain{
			domain.DomainPatent,
			domain.DomainTK,
			domain.DomainABS,
			domain.DomainRegulatory,
			domain.DomainTrademark,
		},
		ClarifyingQuestions: []string{},
		Confidence:          0.82,
		RawDescription:      description,
	}, nil
}
