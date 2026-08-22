package retriever

import "strings"

// AuthorityLevel represents the legal weight of a source document.
// Higher = more authoritative = ranked higher regardless of vector score.
type AuthorityLevel int

const (
	AuthoritySecondary AuthorityLevel = 1 // legal blogs, academic papers
	AuthorityGuidance  AuthorityLevel = 2 // government guidance docs, FAQs
	AuthorityRules     AuthorityLevel = 3 // statutory rules, regulations
	AuthorityStatute   AuthorityLevel = 4 // Acts of Parliament, primary legislation
)

// authorityFromString maps the string stored in Qdrant payload to AuthorityLevel.
// This must match exactly what Teammate 2 stores during ingestion.
func authorityFromString(s string) AuthorityLevel {
	switch strings.ToLower(s) {
	case "statute":
		return AuthorityStatute
	case "rules":
		return AuthorityRules
	case "guidance":
		return AuthorityGuidance
	default:
		return AuthoritySecondary
	}
}

// Chunk is a single retrieved document segment with full provenance metadata.
type Chunk struct {
	ID           string         `json:"id"`
	Text         string         `json:"text"`
	DocTitle     string         `json:"doc_title"`
	Section      string         `json:"section"`
	Domain       string         `json:"domain"`
	Jurisdiction string         `json:"jurisdiction"`
	Authority    AuthorityLevel `json:"authority"`
	AuthorityStr string         `json:"authority_str"` // raw string from Qdrant
	SourceURL    string         `json:"source_url"`
	RetrievedAt  string         `json:"retrieved_at"`
	VectorScore  float64        `json:"vector_score"` // raw cosine similarity from Qdrant
	FinalScore   float64        `json:"final_score"`  // after authority reranking
}

// RetrievalResult holds all chunks retrieved for a single domain.
type RetrievalResult struct {
	Domain    string  `json:"domain"`
	Chunks    []Chunk `json:"chunks"`     // ordered by FinalScore descending
	QueryUsed string  `json:"query_used"` // the actual query sent to Qdrant (for debugging)
}

// RetrieveRequest is the input to RetrieveForDomains.
type RetrieveRequest struct {
	BaseQuery    string   // raw product description from classification
	Domains      []string // from ClassificationResult.RelevantDomains
	Jurisdiction string   // "india" for MVP — always lowercase
	TopK         int      // chunks per domain, default 5
}
