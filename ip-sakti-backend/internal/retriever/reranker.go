package retriever

import (
	"sort"
	"strings"
)

const (
	vectorWeight    = 0.55
	authorityWeight = 0.45
)

// authorityScore normalises an AuthorityLevel to a [0, 1] float64.
// AuthorityStatute (4) → 1.00 (max weight)
// AuthoritySecondary (1) → 0.25 (min weight)
func authorityScore(level AuthorityLevel) float64 {
	return float64(level) / float64(AuthorityStatute)
}

// rerank computes a FinalScore for each chunk that blends vector similarity
// and legal authority, then returns the top topK chunks sorted descending.
//
// Weights are intentionally authority-heavy (0.45) because in legal retrieval
// a statute at 0.60 vector similarity must outrank a blog at 0.85 similarity.
// This is a feature, not a bug.
func rerank(chunks []Chunk, topK int) []Chunk {
	if len(chunks) == 0 {
		return chunks
	}

	// Compute FinalScore for each chunk.
	for i := range chunks {
		chunks[i].FinalScore = (chunks[i].VectorScore * vectorWeight) +
			(authorityScore(chunks[i].Authority) * authorityWeight) +
			keywordBonus(chunks[i].Text)
	}

	// Sort descending by FinalScore.
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].FinalScore > chunks[j].FinalScore
	})

	// Trim to topK — never pad with empty chunks.
	if len(chunks) <= topK {
		return chunks
	}
	return chunks[:topK]
}

// keywordBonus returns extra FinalScore weight for domain-critical legal terms.
// Bonuses are calibrated to overcome the authority gap between guidance docs (0.5)
// and statute docs (1.0), which is 0.225 at authorityWeight=0.45.
// A TK-specific chunk from the Guidelines doc must outscore a generic Patents Act chunk.
func keywordBonus(text string) float64 {
	lower := strings.ToLower(text)
	bonus := 0.0

	// ── Section 3 patent exclusions (highest priority) ────────────────────────
	// A chunk with Section 3(p) directly addresses TK patent exclusion.
	if strings.Contains(lower, "3(p)") || strings.Contains(lower, "section 3p") {
		bonus += 0.40 // large boost — this IS the legal basis for TK exclusion
	}
	if strings.Contains(lower, "section 3") || strings.Contains(lower, "3(d)") || strings.Contains(lower, "3(e)") {
		bonus += 0.15
	}

	// ── Traditional Knowledge specific terms ──────────────────────────────────
	if strings.Contains(lower, "traditional knowledge") {
		bonus += 0.20
	}
	if strings.Contains(lower, "tkdl") {
		bonus += 0.20
	}
	if strings.Contains(lower, "not patentable") || strings.Contains(lower, "aggregation") && strings.Contains(lower, "traditionally known") {
		bonus += 0.20
	}
	if strings.Contains(lower, "prior art") && (strings.Contains(lower, "traditional") || strings.Contains(lower, "ayurvedic")) {
		bonus += 0.15
	}

	// ── Ayurvedic ingredient terms ────────────────────────────────────────────
	if strings.Contains(lower, "ashwagandha") || strings.Contains(lower, "withania somnifera") {
		bonus += 0.10
	}
	if strings.Contains(lower, "brahmi") || strings.Contains(lower, "bacopa") {
		bonus += 0.10
	}
	if strings.Contains(lower, "ayurvedic formulation") || strings.Contains(lower, "ayurvedic preparation") {
		bonus += 0.10
	}

	// ── Biodiversity / ABS terms ──────────────────────────────────────────────
	if strings.Contains(lower, "benefit sharing") || strings.Contains(lower, "biological diversity") {
		bonus += 0.15
	}
	if strings.Contains(lower, "national biodiversity authority") || strings.Contains(lower, "nba") {
		bonus += 0.10
	}

	// ── TK Guidelines document title ──────────────────────────────────────────
	// This ensures the Guidelines for Processing Patent Applications doc always
	// surfaces when it appears in the TK domain results.
	if strings.Contains(lower, "guidelines for processing patent applications") {
		bonus += 0.30
	}
	if strings.Contains(lower, "biological material") && strings.Contains(lower, "traditional knowledge") {
		bonus += 0.20
	}

	return bonus
}
