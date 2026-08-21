package retriever

import (
	"context"

	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// Chunk is a unit of text retrieved from the vector store.
type Chunk struct {
	ID      string
	Content string
	Score   float64
}

// RetrievalResult groups retrieved chunks for a specific domain.
type RetrievalResult struct {
	Domain domain.Domain
	Chunks []Chunk
}

// RetrieveForDomains queries the vector store for evidence relevant to each
// supplied domain and returns one RetrievalResult per domain.
// STUB: returns empty slices — real Qdrant retrieval wired in a later deliverable.
func RetrieveForDomains(_ context.Context, _ string, domains []domain.Domain, _ string) ([]RetrievalResult, error) {
	results := make([]RetrievalResult, 0, len(domains))
	for _, d := range domains {
		results = append(results, RetrievalResult{
			Domain: d,
			Chunks: []Chunk{},
		})
	}
	return results, nil
}
