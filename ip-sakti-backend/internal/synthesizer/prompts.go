package synthesizer

import (
	"fmt"
	"strings"

	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// synthesisSystemPrompt is the instruction given to Gemini on every synthesis call.
// It enforces citation discipline, plain-English findings, and the exact JSON schema
// the synthesizer expects to parse.
const synthesisSystemPrompt = `You are IP-SAKTI's roadmap synthesis engine. You produce structured legal/IP roadmaps for Ayurvedic innovators in India.

YOUR ROLE: You are NOT a lawyer. You do NOT give legal advice. You are an information synthesis and navigation tool that helps innovators understand what legal/IP domains are relevant to their situation and what they should investigate next.

━━━━━━━━━━━━━━━━━━
ABSOLUTE RULES — violating any of these is a critical failure
━━━━━━━━━━━━━━━━━━

RULE 1 — OUTPUT ONLY VALID JSON.
No preamble. No explanation. No markdown fences. No text before or after the JSON object. Your entire response must be parseable by json.Unmarshal.

RULE 2 — CITE ONLY CHUNK IDs FROM THE EVIDENCE PROVIDED.
Each evidence chunk has an ID like "chunk_001" or a UUID. In your citations array, use ONLY these exact IDs.
Never invent a chunk ID. Never cite a document not in the evidence.
If you cannot ground a claim in the provided evidence, set that domain's status to "insufficient_evidence" — do not speculate.

RULE 3 — FINDINGS MUST BE PLAIN ENGLISH.
No legal jargon. No Latin. No statute citations in the finding text itself.
The citations array handles source attribution.
A finding is 2-4 sentences a non-lawyer can understand.

RULE 4 — NEXT STEPS MUST BE SPECIFIC AND ACTIONABLE.
Bad: "Consult a patent professional."
Good: "Conduct a TKDL (Traditional Knowledge Digital Library) prior art search using keywords: Ashwagandha, Shallaki, joint pain formulation, before considering a patent application."

Bad: "Check regulatory requirements."
Good: "Determine whether your product will be classified as an Ayurvedic drug or Ayurveda-Aahar (food supplement) under the Drugs and Cosmetics Act, as this determines your licensing pathway with CDSCO."

RULE 5 — DO NOT MIX JURISDICTIONS.
If the evidence only covers India, do not make claims about Germany or EU law.
If German/EU market intent is detected, add a jurisdiction note explicitly stating that separate analysis is required for that jurisdiction.

RULE 6 — CONFIDENCE MUST REFLECT EVIDENCE QUALITY.
High confidence (0.80+): multiple statute-level chunks directly relevant.
Medium confidence (0.60-0.79): some relevant evidence but gaps exist.
Low confidence (below 0.60): thin or indirect evidence — set needs_escalation: true.

━━━━━━━━━━━━━━━━━━
DOMAIN ANALYSIS RULES
━━━━━━━━━━━━━━━━━━

Analyze ONLY the domains listed in the classification's relevant_domains.
Do not add domains that were not flagged by the classifier.

For each domain, determine:

"patent":
  - Is the formulation type potentially patentable?
    (proprietary/new_drug = potentially yes; classical = no)
  - Does Section 3(p) of the Patents Act (traditional knowledge exclusion) apply? Flag this if tk_involved is true.
  - What novelty/inventive step investigation is needed?
  - Cite relevant Patents Act chunks if available in evidence.

"traditional_knowledge":
  - Is there overlap with classical Ayurvedic preparations?
  - Should a TKDL search be recommended?
  - Could this be used as prior art against a future patent application?
  - Cite relevant TKDL/TK chunks if available.

"biodiversity_abs":
  - Are Indian biological resources involved?
  - Is NBA (National Biodiversity Authority) approval potentially required?
  - Does this apply to research use, commercialization, or export?
  - If commercialization + Indian bio resources: needs_escalation MUST be true.
  - Cite Biological Diversity Act chunks if available.

"regulatory":
  - What product classification applies?
    (Ayurvedic drug / new drug / phytopharmaceutical / food / cosmetic)
  - What licensing pathway is implied?
  - What regulatory body is relevant? (CDSCO, AYUSH, FSSAI)
  - Cite Drugs and Cosmetics Act / AYUSH rules chunks if available.

"trademark":
  - Is trademark registration relevant for the commercial brand?
  - Any distinctiveness issues with generic Ayurvedic terms?
  - Cite Trade Marks Act chunks if available.

━━━━━━━━━━━━━━━━━━
JURISDICTION NOTES RULES
━━━━━━━━━━━━━━━━━━

If target_markets contains only "india":
  No jurisdiction notes needed beyond the Indian analysis.

If target_markets contains any non-India market (germany, usa, uk, etc.):
  Add a jurisdiction note for each foreign market:
  {
    "market": "germany",
    "note": "German/EU regulatory and IP requirements differ significantly from Indian law. EU Traditional Herbal Medicinal Products Directive, REACH regulations, and EU trademark law all apply separately. A dedicated analysis by an EU-qualified IP and regulatory professional is required before market entry.",
    "requires_separate_analysis": true
  }
  Do NOT make specific claims about foreign law unless foreign-jurisdiction evidence chunks were provided.

━━━━━━━━━━━━━━━━━━
OUTPUT JSON SCHEMA — output exactly this structure
━━━━━━━━━━━━━━━━━━

{
  "product_summary": "1-2 sentence plain English summary of what the product is",
  "classification": "Human-readable classification label e.g. 'Proprietary non-classical Ayurvedic formulation'",
  "domains": [
    {
      "domain": "patent|traditional_knowledge|biodiversity_abs|regulatory|trademark",
      "status": "relevant|insufficient_evidence|not_applicable",
      "finding": "2-4 sentence plain English finding for this domain",
      "key_risks": ["risk 1", "risk 2"],
      "citations": [
        {"chunk_id": "exact_chunk_id_from_evidence"}
      ],
      "confidence": 0.00,
      "needs_escalation": true or false
    }
  ],
  "jurisdiction_notes": [
    {
      "market": "country name",
      "note": "explanation",
      "requires_separate_analysis": true
    }
  ],
  "next_steps": [
    "Specific actionable step 1",
    "Specific actionable step 2",
    "Specific actionable step 3",
    "Specific actionable step 4",
    "Specific actionable step 5"
  ],
  "human_escalation": [
    {
      "reason": "specific reason escalation is needed",
      "prof_type": "patent_agent|ip_attorney|nba_expert|regulatory_expert|trademark_agent",
      "urgency": "before_filing|before_commercialization|recommended"
    }
  ],
  "overall_confidence": 0.00,
  "disclaimer": ""
}

REMINDER: Output ONLY this JSON. Nothing else.`

// buildEvidenceContext formats all retrieved chunks into a structured string for
// the LLM to use as its grounding context. The [CHUNK ID: ...] format is
// load-bearing: the LLM copies these IDs verbatim into citations and the
// validator checks them against the evidenceIndex map.
//
// The total output is capped at ~12,000 characters. If the evidence would
// exceed this, lower-scoring chunks are dropped first.
func buildEvidenceContext(evidence []retriever.RetrievalResult) string {
	const (
		// 40K chars gives Gemini plenty of legal evidence while staying well within
		// its context window. This fixes regulatory/trademark being cut off when
		// patent + TK + biodiversity fill the old 12K budget.
		maxContextBytes = 40000
		maxChunkTextLen = 800
	)

	var sb strings.Builder

	sb.WriteString("=== EVIDENCE CONTEXT ===\n")
	sb.WriteString(fmt.Sprintf("Total domains with evidence: %d\n", len(evidence)))

	// Two-pass: first build all domain blocks, then trim if over budget.
	type chunkLine struct {
		domainHeader string
		chunkBlock   string
		finalScore   float64
		authority    int
	}

	var lines []chunkLine

	for _, res := range evidence {
		header := fmt.Sprintf("\n--- DOMAIN: %s ---\n", res.Domain)

		if len(res.Chunks) == 0 {
			lines = append(lines, chunkLine{
				domainHeader: header,
				chunkBlock:   "[NO EVIDENCE RETRIEVED FOR THIS DOMAIN]\n",
				finalScore:   -1, // always keep domain headers with no chunks
				authority:    -1,
			})
			continue
		}

		for _, chunk := range res.Chunks {
			text := chunk.Text
			if len(text) > maxChunkTextLen {
				text = text[:maxChunkTextLen] + "..."
			}

			block := fmt.Sprintf(
				"[CHUNK ID: %s]\nSource: %s | %s | Authority: %s\nText: %s\n\n",
				chunk.ID,
				chunk.DocTitle,
				chunk.Section,
				chunk.AuthorityStr,
				text,
			)

			lines = append(lines, chunkLine{
				domainHeader: header,
				chunkBlock:   block,
				finalScore:   chunk.FinalScore,
				authority:    int(chunk.Authority),
			})
		}
	}

	// Fair per-domain budget allocation.
	// Each domain gets an equal share of the total budget. This prevents
	// early domains (patent, TK) from monopolising the context window and
	// leaving later domains (regulatory, trademark) with no evidence.
	nDomains := len(evidence)
	if nDomains == 0 {
		nDomains = 1
	}
	perDomainBudget := maxContextBytes / nDomains

	// Group lines by domain header for fair allocation.
	type domainBlock struct {
		header string
		chunks []chunkLine
	}
	var domainBlocks []domainBlock
	domainIndex := make(map[string]int)
	for _, line := range lines {
		idx, ok := domainIndex[line.domainHeader]
		if !ok {
			domainIndex[line.domainHeader] = len(domainBlocks)
			domainBlocks = append(domainBlocks, domainBlock{header: line.domainHeader})
			idx = len(domainBlocks) - 1
		}
		domainBlocks[idx].chunks = append(domainBlocks[idx].chunks, line)
	}

	for _, db := range domainBlocks {
		sb.WriteString(db.header)
		domainRemaining := perDomainBudget - len(db.header)
		for _, line := range db.chunks {
			// Empty-domain marker: always include.
			if line.authority == -1 {
				sb.WriteString(line.chunkBlock)
				break
			}
			if domainRemaining > len(line.chunkBlock) {
				sb.WriteString(line.chunkBlock)
				domainRemaining -= len(line.chunkBlock)
			}
			// If over per-domain budget, skip remaining chunks for this domain.
		}
	}

	sb.WriteString("\n=== END EVIDENCE CONTEXT ===\n")
	return sb.String()
}

// buildSynthesisPrompt constructs the user message sent to the LLM alongside
// the system prompt. It embeds the classification and evidence context.
func buildSynthesisPrompt(
	classification *domain.ClassificationResult,
	evidenceContext string,
) string {
	var sb strings.Builder

	sb.WriteString("=== PRODUCT CLASSIFICATION ===\n")
	sb.WriteString(fmt.Sprintf("Formulation type: %s\n", classification.FormulationType))
	sb.WriteString(fmt.Sprintf("Indian biological resources: %v\n", classification.IndianBioResources))
	sb.WriteString(fmt.Sprintf("Traditional knowledge involved: %v\n", classification.TKInvolved))
	sb.WriteString(fmt.Sprintf("Target markets: %s\n", strings.Join(classification.TargetMarkets, ", ")))

	domains := make([]string, len(classification.RelevantDomains))
	for i, d := range classification.RelevantDomains {
		domains[i] = string(d)
	}
	sb.WriteString(fmt.Sprintf("Relevant domains to analyze: %s\n", strings.Join(domains, ", ")))
	sb.WriteString(fmt.Sprintf("Classifier confidence: %.2f\n", classification.Confidence))
	sb.WriteString(fmt.Sprintf("Raw description: %s\n", classification.RawDescription))

	sb.WriteString("\n=== LEGAL EVIDENCE ===\n")
	sb.WriteString(evidenceContext)

	sb.WriteString("\n=== INSTRUCTIONS ===\n")
	sb.WriteString("Based on the classification above and ONLY the evidence provided, ")
	sb.WriteString("produce the IP roadmap JSON. Analyze only the domains listed in ")
	sb.WriteString("\"Relevant domains to analyze\". For any domain with no evidence ")
	sb.WriteString("retrieved, set status to \"insufficient_evidence\". ")
	sb.WriteString("Cite only chunk IDs from the evidence context above. ")
	sb.WriteString("Do not speculate beyond what the evidence supports.\n")

	return sb.String()
}
