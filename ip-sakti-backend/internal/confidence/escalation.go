package confidence

import (
	"fmt"
	"sort"
	"strings"

	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// DetermineEscalations runs deterministic rule-based checks against the
// classification and roadmap to identify mandatory human professional
// review items that must be present in the final roadmap.
func DetermineEscalations(
	classification *domain.ClassificationResult,
	roadmap *domain.IPRoadmap,
	overallConfidence float64,
) []domain.EscalationItem {
	var items []domain.EscalationItem

	if classification == nil {
		return items
	}

	// ── RULE 1: Low overall confidence ───────────────────────────────────────
	if overallConfidence < 0.60 {
		items = append(items, domain.EscalationItem{
			Reason:   "Overall analysis confidence is below threshold. The roadmap may be incomplete due to limited authoritative evidence.",
			ProfType: "patent_agent",
			Urgency:  "recommended",
		})
	}

	// ── RULE 2: ABS + commercialization intent ───────────────────────────────
	if classification.IndianBioResources && len(classification.TargetMarkets) > 0 {
		items = append(items, domain.EscalationItem{
			Reason:   "Indian biological resources are involved and commercial intent is detected. National Biodiversity Authority (NBA) approval or intimation may be required before commercialization.",
			ProfType: "nba_expert",
			Urgency:  "before_commercialization",
		})
	}

	// ── RULE 3: International market detected ────────────────────────────────
	for _, market := range classification.TargetMarkets {
		if strings.ToLower(strings.TrimSpace(market)) != "india" {
			items = append(items, domain.EscalationItem{
				Reason:   fmt.Sprintf("Export or sale in %s detected. Indian IP and regulatory conclusions do not automatically apply. Separate jurisdiction-specific analysis is required.", market),
				ProfType: "ip_attorney",
				Urgency:  "before_filing",
			})
		}
	}

	// ── RULE 4: New drug classification ─────────────────────────────────────
	if classification.FormulationType == domain.FormulationNewDrug || string(classification.FormulationType) == "new_drug" {
		items = append(items, domain.EscalationItem{
			Reason:   "Product may be classified as a new drug under the Drugs and Cosmetics Act. CDSCO approval process and clinical trial requirements may apply before commercialization.",
			ProfType: "regulatory_expert",
			Urgency:  "before_commercialization",
		})
	}

	// ── RULE 5: Traditional knowledge + patent domain active ─────────────────
	hasPatent := false
	for _, d := range classification.RelevantDomains {
		if d == domain.DomainPatent || string(d) == "patent" {
			hasPatent = true
			break
		}
	}
	if classification.TKInvolved && hasPatent {
		items = append(items, domain.EscalationItem{
			Reason:   "Traditional knowledge is involved alongside a potential patent application. Section 3(p) of the Patents Act excludes traditional knowledge from patentability. A registered patent agent must assess the novelty and inventive step carefully.",
			ProfType: "patent_agent",
			Urgency:  "before_filing",
		})
	}

	// ── RULE 6: Synthesizer-flagged domain escalation missing coverage ──────
	if roadmap != nil {
		for _, d := range roadmap.Domains {
			if d.NeedsEscalation {
				expectedProf := profTypeForDomain(string(d.Domain))
				covered := false
				for _, item := range items {
					if item.ProfType == expectedProf {
						covered = true
						break
					}
				}
				if !covered {
					items = append(items, domain.EscalationItem{
						Reason:   fmt.Sprintf("The %s analysis requires professional review — evidence was insufficient for a reliable automated assessment.", d.Domain),
						ProfType: expectedProf,
						Urgency:  "recommended",
					})
				}
			}
		}
	}

	return items
}

// profTypeForDomain maps an IP/regulatory domain to the appropriate professional type.
func profTypeForDomain(d string) string {
	switch d {
	case "patent":
		return "patent_agent"
	case "traditional_knowledge":
		return "patent_agent"
	case "biodiversity_abs":
		return "nba_expert"
	case "regulatory":
		return "regulatory_expert"
	case "trademark":
		return "trademark_agent"
	default:
		return "ip_attorney"
	}
}

// urgencyRank assigns a numeric priority to urgency levels for sorting.
// Lower number = higher priority (shown first).
func urgencyRank(urgency string) int {
	switch urgency {
	case "before_filing":
		return 0
	case "before_commercialization":
		return 1
	case "recommended":
		return 2
	default:
		return 3
	}
}

// mergeEscalations merges rule-based escalation items with LLM-generated escalation items,
// giving priority to rule-based items and deduplicating by (ProfType, Urgency).
// The resulting slice is sorted by urgency priority (before_filing -> before_commercialization -> recommended).
func mergeEscalations(
	llmItems []domain.EscalationItem,
	ruleItems []domain.EscalationItem,
) []domain.EscalationItem {
	type key struct {
		profType string
		urgency  string
	}

	seen := make(map[key]bool)
	result := make([]domain.EscalationItem, 0, len(ruleItems)+len(llmItems))

	// 1. Rule items take absolute priority.
	for _, item := range ruleItems {
		k := key{profType: item.ProfType, urgency: item.Urgency}
		if !seen[k] {
			seen[k] = true
			result = append(result, item)
		}
	}

	// 2. Append LLM items only if not already covered by (ProfType, Urgency).
	for _, item := range llmItems {
		k := key{profType: item.ProfType, urgency: item.Urgency}
		if !seen[k] {
			seen[k] = true
			result = append(result, item)
		}
	}

	// 3. Sort by urgency priority (stable sort to preserve insertion order within same urgency).
	sort.SliceStable(result, func(i, j int) bool {
		return urgencyRank(result[i].Urgency) < urgencyRank(result[j].Urgency)
	})

	return result
}
