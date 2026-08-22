package retriever

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/genai"
)

// Embedder wraps the Google GenAI client to produce text embeddings using
// text-embedding-004. The model and task type MUST match what Teammate 2 uses
// during ingestion (model: text-embedding-004, ingestion task: RETRIEVAL_DOCUMENT).
// For queries we use RETRIEVAL_QUERY, which is the correct paired task type.
type Embedder struct {
	client *genai.Client
	logger *slog.Logger
}

// NewEmbedder creates an Embedder backed by the Google AI backend.
// apiKey must be a valid Gemini API key with embedding access.
func NewEmbedder(apiKey string, logger *slog.Logger) (*Embedder, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("embedder: failed to create genai client: %w", err)
	}
	return &Embedder{client: client, logger: logger}, nil
}

// Embed returns a 768-dimensional float32 vector for the given text.
// TaskType is RETRIEVAL_QUERY — this must match the query side of the
// retrieval pair (Teammate 2 uses RETRIEVAL_DOCUMENT during ingestion).
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := e.client.Models.EmbedContent(ctx,
		"text-embedding-004",
		genai.Text(text),
		&genai.EmbedContentConfig{
			TaskType: "RETRIEVAL_QUERY", // ⚠️ must be RETRIEVAL_QUERY for queries
		},
	)
	if err != nil {
		return nil, fmt.Errorf("embedder: embedding failed: %w", err)
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("embedder: empty embedding returned")
	}
	return result.Embeddings[0].Values, nil
}
