// Package retriever implements domain-filtered, authority-weighted retrieval
// from Qdrant for the IP-SAKTI pipeline. It is the evidence engine for /analyze.
package retriever

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	qdrantpb "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/metadata"
)

// qdrantCollection is the single Qdrant collection that stores all IP-SAKTI
// document chunks. All domains share one collection; domain filtering is done
// via Qdrant payload filters.
const qdrantCollection = "ipsakti_docs"

// domainOrder defines consistent output ordering for []RetrievalResult.
// Synthesizer expects a stable domain order for structured output generation.
var domainOrder = map[string]int{
	"patent":                0,
	"traditional_knowledge": 1,
	"biodiversity_abs":      2,
	"regulatory":            3,
	"trademark":             4,
}

// Retriever is the evidence engine. It embeds a query, filters by domain and
// jurisdiction in Qdrant, and reranks by legal authority.
type Retriever struct {
	qdrant         qdrantpb.PointsClient
	embedder       *Embedder
	apiKey         string // Qdrant API key — empty for local/unauthenticated deployments
	collectionName string // Qdrant collection name
	logger         *slog.Logger
}

// NewRetriever constructs a Retriever. qdrantClient must already be connected.
// apiKey is the Qdrant Cloud API key; pass "" for local Qdrant (no auth).
func NewRetriever(qdrantClient qdrantpb.PointsClient, embedder *Embedder, apiKey string, logger *slog.Logger, collectionName ...string) *Retriever {
	coll := qdrantCollection
	if len(collectionName) > 0 && collectionName[0] != "" {
		coll = collectionName[0]
	}
	return &Retriever{
		qdrant:         qdrantClient,
		embedder:       embedder,
		apiKey:         apiKey,
		collectionName: coll,
		logger:         logger,
	}
}

// qdrantCtx appends the Qdrant API key and ngrok-skip-browser-warning header as gRPC metadata.
// For ngrok free tier, this bypasses the browser warning page on non-browser requests.
func (r *Retriever) qdrantCtx(ctx context.Context) context.Context {
	ctx = metadata.AppendToOutgoingContext(ctx, "ngrok-skip-browser-warning", "true")
	if r.apiKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "api-key", r.apiKey)
	}
	return ctx
}

// qdrantDomain maps internal domain constants to the values stored in Qdrant
func qdrantDomain(domain string) string {
	switch domain {
	case "biodiversity_abs":
		return "biodiversity"
	case "traditional_knowledge":
		return "patent"
	default:
		return domain
	}
}

// RetrieveForDomains runs one Qdrant search per domain concurrently and returns
// a []RetrievalResult ordered by canonical domain sequence. It is the only
// public method the /analyze handler calls.
//
// Error policy: if one domain fails (embedding or Qdrant error), an empty result
// for that domain is recorded and the rest continue. A partial roadmap is better than no roadmap.
func (r *Retriever) RetrieveForDomains(ctx context.Context, req RetrieveRequest) ([]RetrievalResult, error) {
	// Step 1 — Apply defaults.
	if req.TopK == 0 {
		req.TopK = 8
	}
	if req.Jurisdiction == "" {
		req.Jurisdiction = "india"
	}

	// Step 2 — Retrieve concurrently, one goroutine per domain.
	var (
		mu      sync.Mutex
		results []RetrievalResult
	)

	var wg sync.WaitGroup
	for _, domain := range req.Domains {
		domain := domain // capture loop variable
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := r.retrieveDomain(ctx, req, domain)
			if err != nil {
				r.logger.Error("domain retrieval failed",
					"domain", domain,
					"error", err,
				)
				mu.Lock()
				results = append(results, RetrievalResult{
					Domain:    domain,
					Chunks:    nil,
					QueryUsed: "",
				})
				mu.Unlock()
				return
			}
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Step 3 — Sort into canonical domain order.
	sort.Slice(results, func(i, j int) bool {
		oi := domainSortOrder(results[i].Domain)
		oj := domainSortOrder(results[j].Domain)
		return oi < oj
	})

	// Step 4 — Return results (may be empty if all domains failed; caller handles).
	return results, nil
}

// retrieveDomain executes the full retrieval pipeline for a single domain:
// build query → embed → Qdrant search → map chunks → rerank.
func (r *Retriever) retrieveDomain(ctx context.Context, req RetrieveRequest, domain string) (RetrievalResult, error) {
	// Step 1 — Build a semantically richer domain-specific query.
	domainQuery := buildDomainQuery(req.BaseQuery, domain)

	// Step 2 — Embed the query with a 10-second hard timeout.
	embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	embedding, err := r.embedder.Embed(embedCtx, domainQuery)
	if err != nil {
		return RetrievalResult{}, fmt.Errorf("retrieveDomain %s: embed: %w", domain, err)
	}

	// Step 3 — Build Qdrant filter.
	//
	// CONFIRMED via debug logs: The TK guidance document chunks do NOT have a
	// "subdomain" payload field in Qdrant. Using subdomain=traditional_knowledge
	// returns 0 results. The jurisdiction field is also absent on guidance docs.
	//
	// Solution for traditional_knowledge:
	//   - Filter by domain=patent ONLY (no jurisdiction, no subdomain)
	//   - The TK-enriched query text provides semantic precision
	//   - The reranker will surface the most relevant TK chunks
	//
	// All other domains: use domain + jurisdiction (both fields present).
	var filter *qdrantpb.Filter

	if domain == "traditional_knowledge" {
		// TK: filter only by domain=patent. Jurisdiction and subdomain fields
		// are absent on the TK guidance chunks stored in Qdrant.
		r.logger.Info("TK domain: using domain-only filter (no jurisdiction/subdomain — fields not present in Qdrant payload)",
			"qdrant_domain", qdrantDomain(domain), // "patent"
		)
		filter = &qdrantpb.Filter{
			Must: []*qdrantpb.Condition{
				{
					ConditionOneOf: &qdrantpb.Condition_Field{
						Field: &qdrantpb.FieldCondition{
							Key: "domain",
							Match: &qdrantpb.Match{
								MatchValue: &qdrantpb.Match_Keyword{
									Keyword: qdrantDomain(domain), // "patent"
								},
							},
						},
					},
				},
			},
		}
	} else {
		// All other domains: domain + jurisdiction must both match.
		filter = &qdrantpb.Filter{
			Must: []*qdrantpb.Condition{
				{
					ConditionOneOf: &qdrantpb.Condition_Field{
						Field: &qdrantpb.FieldCondition{
							Key: "domain",
							Match: &qdrantpb.Match{
								MatchValue: &qdrantpb.Match_Keyword{
									Keyword: qdrantDomain(domain),
								},
							},
						},
					},
				},
				{
					ConditionOneOf: &qdrantpb.Condition_Field{
						Field: &qdrantpb.FieldCondition{
							Key: "jurisdiction",
							Match: &qdrantpb.Match{
								MatchValue: &qdrantpb.Match_Keyword{
									Keyword: req.Jurisdiction,
								},
							},
						},
					},
				},
			},
		}
	}

	// Step 4 — Search Qdrant. Retrieve up to 50 candidates; reranker trims to TopK.
	// NOTE: Gemini text-embedding-004 produces cosine similarities in the 0.0–0.4 range
	// for most legal document queries. A threshold of 0.15 rejects valid matches.
	// Set to 0.05 to capture all semantically relevant chunks above noise floor.
	// Step 4 — Search Qdrant.
	//
	// For traditional_knowledge: fetch top-100 WITH NO score threshold.
	// The TK Guidelines doc chunks have low cosine similarity scores for general
	// wellness supplement queries (~0.03-0.04) but contain the exact legal text
	// we need. Without threshold, all domain=patent chunks enter the candidate set
	// and the keyword reranker (Section 3(p), TKDL bonuses) surfaces them correctly.
	//
	// For all other domains: use scoreThreshold=0.05 to cut irrelevant noise.
	var searchResult *qdrantpb.SearchResponse

	if domain == "traditional_knowledge" {
		// No score threshold — let the reranker do the work via keyword bonuses.
		searchResult, err = r.qdrant.Search(r.qdrantCtx(ctx), &qdrantpb.SearchPoints{
			CollectionName: r.collectionName,
			Vector:         toFloat32Slice(embedding),
			Filter:         filter,
			Limit:          uint64(100), // larger candidate pool for TK
			WithPayload: &qdrantpb.WithPayloadSelector{
				SelectorOptions: &qdrantpb.WithPayloadSelector_Enable{Enable: true},
			},
			// ScoreThreshold intentionally nil for TK — reranker handles filtering
		})
	} else {
		scoreThreshold := float32(0.05)
		searchResult, err = r.qdrant.Search(r.qdrantCtx(ctx), &qdrantpb.SearchPoints{
			CollectionName: r.collectionName,
			Vector:         toFloat32Slice(embedding),
			Filter:         filter,
			Limit:          uint64(50),
			WithPayload: &qdrantpb.WithPayloadSelector{
				SelectorOptions: &qdrantpb.WithPayloadSelector_Enable{Enable: true},
			},
			ScoreThreshold: &scoreThreshold,
		})
	}
	if err != nil {
		return RetrievalResult{}, fmt.Errorf("retrieveDomain %s: qdrant search: %w", domain, err)
	}
	r.logger.Info("qdrant search result",
		"domain", domain,
		"filter_type", func() string {
			if domain == "traditional_knowledge" {
				return "domain-only(no-threshold)"
			}
			return "domain+jurisdiction"
		}(),
		"hits", len(searchResult.Result),
	)

	// Fallback Step — If filtered search returns 0 results, retry WITHOUT filters to verify vector retrieval.
	const fallbackThreshold = float32(0.05)
	if len(searchResult.Result) == 0 {
		r.logger.Warn("filtered search returned 0 results; retrying search without domain/jurisdiction filters",
			"domain", domain,
		)
		fallbackThresholdVal := fallbackThreshold
		searchResult, err = r.qdrant.Search(r.qdrantCtx(ctx), &qdrantpb.SearchPoints{
			CollectionName: r.collectionName,
			Vector:         toFloat32Slice(embedding),
			Filter:         nil, // no payload filters
			Limit:          uint64(50),
			WithPayload: &qdrantpb.WithPayloadSelector{
				SelectorOptions: &qdrantpb.WithPayloadSelector_Enable{Enable: true},
			},
			ScoreThreshold: &fallbackThresholdVal,
		})
		if err != nil {
			return RetrievalResult{}, fmt.Errorf("retrieveDomain %s (fallback): qdrant search: %w", domain, err)
		}
		r.logger.Info("filterless fallback result", "domain", domain, "hits", len(searchResult.Result))
	}

	// Last-resort — if threshold is still killing results, fetch top-5 with NO threshold
	// to get raw scores and confirm Qdrant is populated.
	if len(searchResult.Result) == 0 {
		r.logger.Warn("threshold fallback also 0; probing Qdrant with no score threshold", "domain", domain)
		probeResult, probeErr := r.qdrant.Search(r.qdrantCtx(ctx), &qdrantpb.SearchPoints{
			CollectionName: r.collectionName,
			Vector:         toFloat32Slice(embedding),
			Filter:         nil,
			Limit:          uint64(5),
			WithPayload: &qdrantpb.WithPayloadSelector{
				SelectorOptions: &qdrantpb.WithPayloadSelector_Enable{Enable: true},
			},
			// ScoreThreshold intentionally nil — get raw top-5 regardless of score
		})
		if probeErr == nil && len(probeResult.Result) > 0 {
			// Log the actual scores so we can calibrate the threshold correctly
			scores := make([]float32, 0, len(probeResult.Result))
			for _, sp := range probeResult.Result {
				scores = append(scores, sp.GetScore())
			}
			r.logger.Warn("Qdrant has vectors but all scores are below threshold — using raw top results",
				"domain", domain,
				"raw_scores", scores,
				"current_threshold", fallbackThreshold,
			)
			// Use these results rather than returning empty
			searchResult = probeResult
		} else if probeErr != nil {
			r.logger.Error("probe search failed", "domain", domain, "error", probeErr)
		} else {
			r.logger.Error("Qdrant collection appears empty", "domain", domain, "collection", r.collectionName)
		}
	}

	// Step 5 — Map ScoredPoints → []Chunk.
	chunks := make([]Chunk, 0, len(searchResult.Result))
	for _, sp := range searchResult.Result {
		chunk := r.mapScoredPoint(sp, domain)
		chunks = append(chunks, chunk)
	}

	// Step 6 — Rerank by authority and trim to topK.
	reranked := rerank(chunks, req.TopK)

	// Step 7 — Return result.
	return RetrievalResult{
		Domain:    domain,
		Chunks:    reranked,
		QueryUsed: domainQuery,
	}, nil
}

// mapScoredPoint converts a Qdrant ScoredPoint to a Chunk.
// Missing payload fields are zero/empty — never panics.
func (r *Retriever) mapScoredPoint(sp *qdrantpb.ScoredPoint, fallbackDomain string) Chunk {
	payload := sp.GetPayload()

	text := getPayloadString(payload, "text")
	docTitle := getPayloadString(payload, "doc_title")

	if text == "" {
		r.logger.Warn("qdrant point missing 'text' field",
			"point_id", pointID(sp),
			"domain", fallbackDomain,
		)
	}
	if docTitle == "" {
		r.logger.Warn("qdrant point missing 'doc_title' field",
			"point_id", pointID(sp),
			"domain", fallbackDomain,
		)
	}

	authorityStr := getPayloadString(payload, "authority")
	authority := authorityFromString(authorityStr)

	domainStr := getPayloadString(payload, "domain")
	if domainStr == "" {
		domainStr = fallbackDomain
	}

	return Chunk{
		ID:           pointID(sp),
		Text:         text,
		DocTitle:     docTitle,
		Section:      getPayloadString(payload, "section_ref"),
		Domain:       domainStr,
		Jurisdiction: getPayloadString(payload, "jurisdiction"),
		Authority:    authority,
		AuthorityStr: authorityStr,
		SourceURL:    getPayloadString(payload, "source_url"),
		RetrievedAt:  getPayloadString(payload, "retrieved_at"),
		VectorScore:  float64(sp.GetScore()),
	}
}

// ── Helper functions ──────────────────────────────────────────────────────────

// buildDomainQuery augments the base product description with domain-specific
// legal terminology. This dramatically improves vector retrieval quality
// compared to submitting the raw description alone.
func buildDomainQuery(baseQuery string, domain string) string {
	switch domain {
	case "patent":
		// Fix 3: Enriched with TK-specific terms so patent+TK chunk scoring improves.
		return baseQuery + " traditional knowledge Section 3p TKDL " +
			"prior art patent eligibility Ayurvedic formulation " +
			"not patentable aggregation traditionally known components " +
			"Guidelines Processing Patent Applications TK biological material"
	case "traditional_knowledge":
		return baseQuery + " traditional knowledge TKDL prior art classical text " +
			"Ayurveda Charaka Samhita Section 3p Section 25 Patents Act opposition " +
			"Ashwagandha Withania somnifera Ayurvedic formulation not patentable " +
			"aggregation traditionally known components Guidelines Processing Patent Applications"
	case "biodiversity_abs":
		return baseQuery + " biological diversity access benefit sharing NBA approval " +
			"biological resources India commercialization ABS"
	case "regulatory":
		return baseQuery + " AYUSH drug licensing Drugs Cosmetics Act regulatory " +
			"approval Ayurvedic medicine classification new drug"
	case "trademark":
		return baseQuery + " trademark registration brand name AYUSH product " +
			"distinctiveness Trade Marks Act"
	default:
		return baseQuery
	}
}

// toFloat32Slice is an explicit identity pass-through for readability at the Qdrant call site.
func toFloat32Slice(f []float32) []float32 {
	return f
}

// floatPtr returns a pointer to a float32 literal for use in proto fields.
func floatPtr(f float32) *float32 { //nolint:unused // retained for clarity, may be used in tests
	return &f
}

// getPayloadString safely extracts a string value from a Qdrant payload map.
// Returns "" if the key is absent, nil, or not a string kind — never panics.
func getPayloadString(payload map[string]*qdrantpb.Value, key string) string {
	if payload == nil {
		return ""
	}
	v, ok := payload[key]
	if !ok || v == nil {
		return ""
	}
	sv, ok := v.Kind.(*qdrantpb.Value_StringValue)
	if !ok {
		return ""
	}
	return sv.StringValue
}

// pointID extracts a stable string ID from a ScoredPoint.
// Qdrant IDs are either UUIDs or uint64 numerics.
func pointID(sp *qdrantpb.ScoredPoint) string {
	if sp.GetId() == nil {
		return ""
	}
	if uid := sp.GetId().GetUuid(); uid != "" {
		return uid
	}
	return strconv.FormatUint(sp.GetId().GetNum(), 10)
}

// domainSortOrder returns a canonical sort priority for a domain string.
// Unknown domains sort last (99) so they don't displace known ones.
func domainSortOrder(domain string) int {
	if v, ok := domainOrder[domain]; ok {
		return v
	}
	return 99
}
