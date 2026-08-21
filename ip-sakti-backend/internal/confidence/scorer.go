package confidence

import (
	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// Score combines the classifier confidence with evidence quality to produce an
// overall confidence score for the roadmap.
// STUB: passes through classificationConf as-is.
// Real scoring logic (evidence density, citation quality) added in a later deliverable.
func Score(classificationConf float64, _ []retriever.RetrievalResult) float64 {
	return classificationConf
}

// DetermineEscalations reviews the classification, roadmap and overall
// confidence to produce a list of human-escalation recommendations.
// STUB: returns empty slice — real heuristics added in a later deliverable.
func DetermineEscalations(_ *domain.ClassificationResult, _ *domain.IPRoadmap, _ float64) []domain.EscalationItem {
	return []domain.EscalationItem{}
}
