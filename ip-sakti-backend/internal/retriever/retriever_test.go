package retriever

import (
	"strings"
	"testing"
)

// ── rerank tests ──────────────────────────────────────────────────────────────

func TestRerank_AuthorityBeatsVector(t *testing.T) {
	// chunk A: high vector similarity but low authority (blog post)
	chunkA := Chunk{
		ID:          "a",
		VectorScore: 0.90,
		Authority:   AuthoritySecondary, // score = 0.25
	}
	// chunk B: lower vector similarity but statute-level authority
	chunkB := Chunk{
		ID:          "b",
		VectorScore: 0.65,
		Authority:   AuthorityStatute, // score = 1.00
	}

	// Expected final scores:
	//   A: (0.90 * 0.55) + (0.25 * 0.45) = 0.495 + 0.1125 = 0.6075
	//   B: (0.65 * 0.55) + (1.00 * 0.45) = 0.3575 + 0.45   = 0.8075
	// chunk B must rank first.

	got := rerank([]Chunk{chunkA, chunkB}, 5)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if got[0].ID != "b" {
		t.Errorf("expected chunk B first (statute), got chunk %q (FinalScore=%.4f vs B=%.4f)",
			got[0].ID, got[0].FinalScore, got[1].FinalScore)
	}

	// Verify exact FinalScore values (tolerance 1e-6).
	const eps = 1e-6
	expectedA := (0.90 * vectorWeight) + (authorityScore(AuthoritySecondary) * authorityWeight)
	expectedB := (0.65 * vectorWeight) + (authorityScore(AuthorityStatute) * authorityWeight)

	if abs64(got[1].FinalScore-expectedA) > eps {
		t.Errorf("chunk A final score: got %.6f, want %.6f", got[1].FinalScore, expectedA)
	}
	if abs64(got[0].FinalScore-expectedB) > eps {
		t.Errorf("chunk B final score: got %.6f, want %.6f", got[0].FinalScore, expectedB)
	}
}

func TestRerank_TopKTruncation(t *testing.T) {
	chunks := makeChunks(8)
	got := rerank(chunks, 5)
	if len(got) != 5 {
		t.Errorf("expected 5 chunks after truncation to topK=5, got %d", len(got))
	}
	// Verify ordering: each FinalScore must be >= next.
	for i := 1; i < len(got); i++ {
		if got[i].FinalScore > got[i-1].FinalScore {
			t.Errorf("chunks not sorted: got[%d].FinalScore=%.4f > got[%d].FinalScore=%.4f",
				i, got[i].FinalScore, i-1, got[i-1].FinalScore)
		}
	}
}

func TestRerank_FewerChunksThanTopK(t *testing.T) {
	chunks := makeChunks(3)
	got := rerank(chunks, 5)
	if len(got) != 3 {
		t.Errorf("expected 3 chunks (no padding), got %d", len(got))
	}
}

func TestRerank_EmptyInput(t *testing.T) {
	got := rerank([]Chunk{}, 5)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d chunks", len(got))
	}
}

func TestRerank_NilInput(t *testing.T) {
	// Should not panic.
	var chunks []Chunk
	got := rerank(chunks, 5)
	if got != nil && len(got) != 0 {
		t.Errorf("expected nil/empty result, got %d chunks", len(got))
	}
}

// ── buildDomainQuery tests ────────────────────────────────────────────────────

func TestBuildDomainQuery_PatentContainsPatentsAct(t *testing.T) {
	q := buildDomainQuery("Ashwagandha formulation", "patent")
	// The patent query is enriched with TK/Section 3(p) terms for better retrieval.
	if !strings.Contains(q, "Section 3p") {
		t.Errorf("patent query missing 'Section 3p': %q", q)
	}
	if !strings.Contains(q, "patent eligibility") {
		t.Errorf("patent query missing 'patent eligibility': %q", q)
	}
}

func TestBuildDomainQuery_ABSContainsNBA(t *testing.T) {
	q := buildDomainQuery("Ashwagandha formulation", "biodiversity_abs")
	if !strings.Contains(q, "NBA") {
		t.Errorf("abs query missing 'NBA': %q", q)
	}
}

func TestBuildDomainQuery_UnknownDomainUnchanged(t *testing.T) {
	base := "some product description"
	q := buildDomainQuery(base, "unknown_domain")
	if q != base {
		t.Errorf("unknown domain should return base query unchanged; got %q", q)
	}
}

func TestBuildDomainQuery_AllKnownDomainsLongerThanBase(t *testing.T) {
	base := "Ashwagandha Shallaki joint pain proprietary formulation"
	domains := []string{"patent", "traditional_knowledge", "biodiversity_abs", "regulatory", "trademark"}
	for _, d := range domains {
		q := buildDomainQuery(base, d)
		if len(q) <= len(base) {
			t.Errorf("domain %q: augmented query must be longer than base; base=%d chars, got=%d chars",
				d, len(base), len(q))
		}
	}
}

func TestBuildDomainQuery_TKContainsTKDL(t *testing.T) {
	q := buildDomainQuery("base", "traditional_knowledge")
	if !strings.Contains(q, "TKDL") {
		t.Errorf("TK query missing 'TKDL': %q", q)
	}
}

func TestBuildDomainQuery_RegulatoryContainsAYUSH(t *testing.T) {
	q := buildDomainQuery("base", "regulatory")
	if !strings.Contains(q, "AYUSH") {
		t.Errorf("regulatory query missing 'AYUSH': %q", q)
	}
}

func TestBuildDomainQuery_TrademarkContainsTradeMarksAct(t *testing.T) {
	q := buildDomainQuery("base", "trademark")
	if !strings.Contains(q, "Trade Marks Act") {
		t.Errorf("trademark query missing 'Trade Marks Act': %q", q)
	}
}

// ── authorityFromString tests ─────────────────────────────────────────────────

func TestAuthorityFromString(t *testing.T) {
	cases := []struct {
		input    string
		expected AuthorityLevel
	}{
		{"statute", AuthorityStatute},
		{"rules", AuthorityRules},
		{"guidance", AuthorityGuidance},
		{"secondary", AuthoritySecondary},
		{"STATUTE", AuthorityStatute},    // case-insensitive
		{"RULES", AuthorityRules},        // case-insensitive
		{"blog", AuthoritySecondary},     // unknown → secondary
		{"", AuthoritySecondary},         // empty → secondary
		{"academic", AuthoritySecondary}, // unknown → secondary
	}

	for _, tc := range cases {
		got := authorityFromString(tc.input)
		if got != tc.expected {
			t.Errorf("authorityFromString(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

// ── authorityScore tests ──────────────────────────────────────────────────────

func TestAuthorityScore(t *testing.T) {
	cases := []struct {
		level    AuthorityLevel
		expected float64
	}{
		{AuthoritySecondary, 0.25},
		{AuthorityGuidance, 0.50},
		{AuthorityRules, 0.75},
		{AuthorityStatute, 1.00},
	}
	const eps = 1e-9
	for _, tc := range cases {
		got := authorityScore(tc.level)
		if abs64(got-tc.expected) > eps {
			t.Errorf("authorityScore(%d) = %.4f, want %.4f", tc.level, got, tc.expected)
		}
	}
}

// ── domainSortOrder tests ─────────────────────────────────────────────────────

func TestDomainSortOrder_KnownDomains(t *testing.T) {
	// Validate the canonical ordering: patent < TK < ABS < regulatory < trademark.
	cases := []struct {
		domain string
		order  int
	}{
		{"patent", 0},
		{"traditional_knowledge", 1},
		{"biodiversity_abs", 2},
		{"regulatory", 3},
		{"trademark", 4},
	}
	for _, tc := range cases {
		got := domainSortOrder(tc.domain)
		if got != tc.order {
			t.Errorf("domainSortOrder(%q) = %d, want %d", tc.domain, got, tc.order)
		}
	}
}

func TestDomainSortOrder_UnknownDomain(t *testing.T) {
	got := domainSortOrder("mystery_domain")
	if got != 99 {
		t.Errorf("unknown domain should sort last (99), got %d", got)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// makeChunks creates n test chunks with varying VectorScore and AuthorityLevel.
func makeChunks(n int) []Chunk {
	chunks := make([]Chunk, n)
	for i := range chunks {
		chunks[i] = Chunk{
			ID:          string(rune('a' + i)),
			VectorScore: float64(n-i) / float64(n), // descending vector scores
			Authority:   AuthorityLevel((i % 4) + 1),
		}
	}
	return chunks
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
