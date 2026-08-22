package confidence

import (
	"math"

	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// Weights for the three confidence signals.
const (
	weightClassification = 0.35
	weightAuthority      = 0.40
	weightCoverage       = 0.25
)

// authorityScoreMap maps legal authority levels to normalized score values.
var authorityScoreMap = map[retriever.AuthorityLevel]float64{
	retriever.AuthorityStatute:   1.00,
	retriever.AuthorityRules:     0.75,
	retriever.AuthorityGuidance:  0.50,
	retriever.AuthoritySecondary: 0.25,
}

// ScoreInput contains the signals used to deterministically compute
// overall roadmap confidence.
type ScoreInput struct {
	ClassificationConfidence float64
	Evidence                 []retriever.RetrievalResult
	DomainAnalyses           []domain.DomainAnalysis
}

// Score computes the overall roadmap confidence score from three independent signals:
//  1. Classification Confidence (weight: 0.35)
//  2. Evidence Authority Quality (weight: 0.40)
//  3. Domain Coverage (weight: 0.25)
//
// Returns a value clamped strictly to [0.0, 1.0].
func Score(input ScoreInput) float64 {
	// ── Signal 1: Classification Confidence (35%) ────────────────────────────
	classConfidence := math.Min(math.Max(input.ClassificationConfidence, 0.0), 1.0)
	signalClassification := classConfidence * weightClassification

	// ── Signal 2: Evidence Authority Quality (40%) ───────────────────────────
	var totalAuthority float64
	var chunkCount int

	for _, result := range input.Evidence {
		for _, chunk := range result.Chunks {
			score, ok := authorityScoreMap[chunk.Authority]
			if !ok {
				score = 0.25 // default to secondary authority level
			}
			totalAuthority += score
			chunkCount++
		}
	}

	var avgAuthority float64
	if chunkCount > 0 {
		avgAuthority = totalAuthority / float64(chunkCount)
	}
	signalAuthority := avgAuthority * weightAuthority

	// ── Signal 3: Domain Coverage (25%) ──────────────────────────────────────
	var coveredCount int
	for _, result := range input.Evidence {
		if len(result.Chunks) >= 2 {
			coveredCount++
		}
	}

	var coverageScore float64
	if len(input.Evidence) > 0 {
		coverageScore = float64(coveredCount) / float64(len(input.Evidence))
	}
	signalCoverage := coverageScore * weightCoverage

	// ── Final Combined Score ─────────────────────────────────────────────────
	rawScore := signalClassification + signalAuthority + signalCoverage
	return math.Min(math.Max(rawScore, 0.0), 1.0)
}
