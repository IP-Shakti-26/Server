package retriever

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/genai"
)

type Embedder struct {
	client *genai.Client
	logger *slog.Logger
}

func NewEmbedder(client *genai.Client, logger *slog.Logger) (*Embedder, error) {
	if client == nil {
		return nil, fmt.Errorf("embedder: Gemini client is required")
	}
	return &Embedder{client: client, logger: logger}, nil
}

func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	res, err := e.client.Models.EmbedContent(ctx,
		"gemini-embedding-001",
		genai.Text(text),
		&genai.EmbedContentConfig{
			TaskType:             "RETRIEVAL_QUERY",
			OutputDimensionality: genai.Ptr(int32(768)),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("embedder: failed to generate embedding: %w", err)
	}

	if len(res.Embeddings) == 0 || len(res.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("embedder: empty embedding returned from Gemini")
	}

	return res.Embeddings[0].Values, nil
}