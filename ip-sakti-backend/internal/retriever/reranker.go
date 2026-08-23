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

func keywordBonus(text string) float64 {
	lower := strings.ToLower(text)
	bonus := 0.0
	if strings.Contains(lower, "section 3") || strings.Contains(lower, "3(p)") || strings.Contains(lower, "3(d)") || strings.Contains(lower, "3(e)") {
		bonus += 0.25
	}
	if strings.Contains(lower, "traditional knowledge") || strings.Contains(lower, "tkdl") {
		bonus += 0.15
	}
	if strings.Contains(lower, "benefit sharing") || strings.Contains(lower, "biological diversity") {
		bonus += 0.15
	}
	return bonus
}
