package retriever

import "sort"

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
			(authorityScore(chunks[i].Authority) * authorityWeight)
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
