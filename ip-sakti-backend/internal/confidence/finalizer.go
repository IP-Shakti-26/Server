package confidence

import (
	"log/slog"
	"math"

	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

const canonicalDisclaimer = "This output is for informational purposes only and " +
	"does not constitute legal advice. IP-SAKTI is not a substitute for " +
	"a registered patent agent, IP attorney, or legal professional. " +
	"Consult qualified professionals before taking any formal IP or " +
	"regulatory action."

// Finalize applies post-processing overrides to the synthesized roadmap.
// It recomputes overall confidence, deterministically identifies and merges
// human escalations, syncs domain needs_escalation flags, clamps confidence values,
// ensures fallback next_steps, and enforces the canonical disclaimer.
func Finalize(
	roadmap *domain.IPRoadmap,
	classification *domain.ClassificationResult,
	evidence []retriever.RetrievalResult,
	logger *slog.Logger,
) {
	if roadmap == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	classConf := 0.0
	if classification != nil {
		classConf = classification.Confidence
	}

	// ── STEP 1: Recompute and overwrite overall_confidence ───────────────────
	score := Score(ScoreInput{
		ClassificationConfidence: classConf,
		Evidence:                 evidence,
		DomainAnalyses:           roadmap.Domains,
	})
	roadmap.OverallConfidence = score

	// ── STEP 2: Determine and merge escalations ──────────────────────────────
	ruleEscalations := DetermineEscalations(classification, roadmap, score)
	roadmap.HumanEscalation = mergeEscalations(roadmap.HumanEscalation, ruleEscalations)

	// ── STEP 3: Sync needs_escalation flags ──────────────────────────────────
	// Build lookup of professional types present in the final escalation list
	escalationProfTypes := make(map[string]bool)
	for _, item := range roadmap.HumanEscalation {
		escalationProfTypes[item.ProfType] = true
	}

	for i := range roadmap.Domains {
		d := &roadmap.Domains[i]
		matchingProf := profTypeForDomain(string(d.Domain))
		if escalationProfTypes[matchingProf] && !d.NeedsEscalation {
			d.NeedsEscalation = true
			logger.Debug("needs_escalation set by finalizer",
				"domain", d.Domain,
				"prof_type", matchingProf,
			)
		}
	}

	// ── STEP 4: Clamp all domain confidence values & apply penalty ───────────
	for i := range roadmap.Domains {
		d := &roadmap.Domains[i]
		d.Confidence = math.Min(math.Max(d.Confidence, 0.0), 1.0)

		if d.Status == domain.StatusInsufficientEvidence || string(d.Status) == "insufficient_evidence" {
			d.Confidence = math.Min(d.Confidence, 0.40)
		}
	}

	// ── STEP 5: Ensure next_steps is never empty ─────────────────────────────
	if len(roadmap.NextSteps) == 0 {
		roadmap.NextSteps = []string{
			"Consult a registered patent agent to assess IP protection options for your formulation.",
			"Conduct a prior art search on the TKDL (Traditional Knowledge Digital Library) at www.tkdl.res.in.",
			"Determine the regulatory classification of your product under the Drugs and Cosmetics Act with CDSCO or AYUSH.",
		}
		logger.Warn("next_steps was empty, applied fallback")
	}

	// ── STEP 6: Enforce disclaimer ───────────────────────────────────────────
	roadmap.Disclaimer = canonicalDisclaimer

	// ── STEP 7: Log final summary ────────────────────────────────────────────
	logger.Info("roadmap finalized",
		"overall_confidence", roadmap.OverallConfidence,
		"domains_analyzed", len(roadmap.Domains),
		"escalation_items", len(roadmap.HumanEscalation),
		"next_steps", len(roadmap.NextSteps),
	)
}
