package synthesizer

import (
	"fmt"
	"log/slog"

	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// validateAndCleanCitations walks every domain in the roadmap and removes any
// citation whose ChunkID is not present in evidenceIndex (hallucinated IDs).
//
// After dropping invalid citations, if a domain was marked "relevant" but now
// has zero citations, it is downgraded to "insufficient_evidence" and flagged
// for human escalation. This is the lightweight citation verifier — no separate
// service needed; the Go code enforces it, not the prompt.
func validateAndCleanCitations(
	roadmap *domain.IPRoadmap,
	evidenceIndex map[string]retriever.Chunk,
	logger *slog.Logger,
) {
	for i := range roadmap.Domains {
		d := &roadmap.Domains[i]

		validCitations := make([]domain.Citation, 0, len(d.Citations))
		droppedCount := 0

		for _, cit := range d.Citations {
			if _, exists := evidenceIndex[cit.ChunkID]; exists {
				validCitations = append(validCitations, cit)
			} else {
				droppedCount++
				logger.Warn("hallucinated citation dropped",
					"chunk_id", cit.ChunkID,
					"domain", d.Domain,
				)
			}
		}

		d.Citations = validCitations

		if droppedCount > 0 {
			logger.Info("citations cleaned",
				"domain", d.Domain,
				"dropped", droppedCount,
				"remaining", len(validCitations),
			)
		}

		// CRITICAL: If status was "relevant" but all citations were hallucinated
		// (or the LLM produced none), downgrade to insufficient_evidence.
		// This includes the case where the LLM returned an empty citations array
		// on a domain it marked "relevant" — if the evidence existed, it should
		// have cited it. We treat this as a soft failure and downgrade.
		if d.Status == domain.StatusRelevant && len(d.Citations) == 0 {
			d.Status = domain.StatusInsufficientEvidence
			d.NeedsEscalation = true
			d.Finding = "Insufficient authoritative evidence was retrieved for this domain. " +
				"The analysis cannot be grounded in specific legal provisions. " +
				"Consult a registered IP professional for this aspect of your situation."
			logger.Warn("domain downgraded to insufficient_evidence",
				"domain", d.Domain,
				"reason", "all citations were invalid",
			)
		}
	}
}

// validateRoadmapStructure checks that the LLM returned a structurally valid
// roadmap. It does NOT validate the semantic content — that is the LLM's job.
// If this returns an error, the synthesizer surfaces it as a 500.
func validateRoadmapStructure(roadmap *domain.IPRoadmap) error {
	if roadmap == nil {
		return fmt.Errorf("synthesizer: roadmap is nil")
	}
	if len(roadmap.Domains) == 0 {
		return fmt.Errorf("synthesizer: roadmap has no domain analyses")
	}
	if len(roadmap.NextSteps) == 0 {
		return fmt.Errorf("synthesizer: roadmap has no next steps")
	}

	validStatuses := map[domain.DomainStatus]bool{
		domain.StatusRelevant:             true,
		domain.StatusInsufficientEvidence: true,
		domain.StatusNotApplicable:        true,
	}

	for i, d := range roadmap.Domains {
		if !validStatuses[d.Status] {
			return fmt.Errorf("synthesizer: domain[%d] has invalid status: %s", i, d.Status)
		}
		if d.Domain == "" {
			return fmt.Errorf("synthesizer: domain[%d] has empty domain field", i)
		}
		if d.Confidence < 0 || d.Confidence > 1 {
			return fmt.Errorf("synthesizer: domain[%d] confidence out of range: %f", i, d.Confidence)
		}
	}

	return nil
}
