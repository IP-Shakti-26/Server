package synthesizer

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
)

// testLogger returns a slog.Logger that discards output during tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // suppress info/warn noise during tests
	}))
}

// ── validateAndCleanCitations tests ──────────────────────────────────────────

func TestValidateAndCleanCitations_ValidCitationSurvives(t *testing.T) {
	t.Parallel()

	evidenceIndex := map[string]retriever.Chunk{
		"chunk_001": {ID: "chunk_001", DocTitle: "Patents Act 1970"},
	}

	roadmap := &domain.IPRoadmap{
		Domains: []domain.DomainAnalysis{
			{
				Domain: domain.DomainPatent,
				Status: domain.StatusRelevant,
				Citations: []domain.Citation{
					{ChunkID: "chunk_001"},
				},
			},
		},
		NextSteps: []string{"step"},
	}

	validateAndCleanCitations(roadmap, evidenceIndex, testLogger())

	d := roadmap.Domains[0]
	if len(d.Citations) != 1 {
		t.Fatalf("expected 1 citation, got %d", len(d.Citations))
	}
	if d.Citations[0].ChunkID != "chunk_001" {
		t.Errorf("expected chunk_001, got %s", d.Citations[0].ChunkID)
	}
	if d.Status != domain.StatusRelevant {
		t.Errorf("status should remain relevant, got %s", d.Status)
	}
}

func TestValidateAndCleanCitations_HallucinatedCitationDropped(t *testing.T) {
	t.Parallel()

	evidenceIndex := map[string]retriever.Chunk{
		"chunk_001": {ID: "chunk_001"},
	}

	roadmap := &domain.IPRoadmap{
		Domains: []domain.DomainAnalysis{
			{
				Domain: domain.DomainPatent,
				Status: domain.StatusRelevant,
				Citations: []domain.Citation{
					{ChunkID: "chunk_FAKE"},
				},
			},
		},
		NextSteps: []string{"step"},
	}

	validateAndCleanCitations(roadmap, evidenceIndex, testLogger())

	d := roadmap.Domains[0]
	// chunk_FAKE should be dropped, leaving 0 citations.
	if len(d.Citations) != 0 {
		t.Errorf("expected 0 citations after dropping hallucinated one, got %d", len(d.Citations))
	}
	// Domain should be downgraded because all citations were hallucinated.
	if d.Status != domain.StatusInsufficientEvidence {
		t.Errorf("expected insufficient_evidence after hallucination, got %s", d.Status)
	}
	if !d.NeedsEscalation {
		t.Error("expected NeedsEscalation=true after hallucination")
	}
}

func TestValidateAndCleanCitations_AllCitationsHallucinated_DomainDowngraded(t *testing.T) {
	t.Parallel()

	// Empty evidenceIndex — nothing is real.
	evidenceIndex := map[string]retriever.Chunk{}

	roadmap := &domain.IPRoadmap{
		Domains: []domain.DomainAnalysis{
			{
				Domain: domain.DomainPatent,
				Status: domain.StatusRelevant,
				Citations: []domain.Citation{
					{ChunkID: "chunk_FAKE"},
				},
			},
		},
		NextSteps: []string{"step"},
	}

	validateAndCleanCitations(roadmap, evidenceIndex, testLogger())

	d := roadmap.Domains[0]
	if d.Status != domain.StatusInsufficientEvidence {
		t.Errorf("expected insufficient_evidence, got %s", d.Status)
	}
	if !d.NeedsEscalation {
		t.Error("expected NeedsEscalation=true")
	}
	if d.Finding == "" {
		t.Error("expected Finding to be set after downgrade")
	}
}

func TestValidateAndCleanCitations_MixedValidAndInvalid(t *testing.T) {
	t.Parallel()

	evidenceIndex := map[string]retriever.Chunk{
		"chunk_001": {ID: "chunk_001"},
		"chunk_002": {ID: "chunk_002"},
	}

	roadmap := &domain.IPRoadmap{
		Domains: []domain.DomainAnalysis{
			{
				Domain: domain.DomainPatent,
				Status: domain.StatusRelevant,
				Citations: []domain.Citation{
					{ChunkID: "chunk_001"},
					{ChunkID: "chunk_FAKE"},
					{ChunkID: "chunk_002"},
				},
			},
		},
		NextSteps: []string{"step"},
	}

	validateAndCleanCitations(roadmap, evidenceIndex, testLogger())

	d := roadmap.Domains[0]
	if len(d.Citations) != 2 {
		t.Errorf("expected 2 surviving citations, got %d", len(d.Citations))
	}
	// Domain should still be relevant (has valid citations).
	if d.Status != domain.StatusRelevant {
		t.Errorf("expected status to remain relevant, got %s", d.Status)
	}
	// Verify the surviving IDs are the real ones.
	ids := make(map[string]bool)
	for _, c := range d.Citations {
		ids[c.ChunkID] = true
	}
	if !ids["chunk_001"] || !ids["chunk_002"] {
		t.Errorf("surviving citations should be chunk_001 and chunk_002, got %v", ids)
	}
}

func TestValidateAndCleanCitations_EmptyCitationsOnRelevantDomain_IsDowngraded(t *testing.T) {
	t.Parallel()
	// Per spec Section 9 clarification: the downgrade condition is
	// status == relevant AND len(citations after cleaning) == 0.
	// This includes the case where citations was ALREADY empty from the LLM.
	// The LLM should always cite if status is relevant and evidence exists.
	// An empty list is treated as a soft failure.

	evidenceIndex := map[string]retriever.Chunk{
		"chunk_001": {ID: "chunk_001"},
	}

	roadmap := &domain.IPRoadmap{
		Domains: []domain.DomainAnalysis{
			{
				Domain:    domain.DomainTK,
				Status:    domain.StatusRelevant,
				Citations: []domain.Citation{}, // LLM chose not to cite
			},
		},
		NextSteps: []string{"step"},
	}

	validateAndCleanCitations(roadmap, evidenceIndex, testLogger())

	d := roadmap.Domains[0]
	if d.Status != domain.StatusInsufficientEvidence {
		t.Errorf("expected downgrade to insufficient_evidence for empty citations on relevant domain, got %s", d.Status)
	}
	if !d.NeedsEscalation {
		t.Error("expected NeedsEscalation=true after downgrade")
	}
}

// ── buildEvidenceContext tests ─────────────────────────────────────────────────

func TestBuildEvidenceContext_EmptyEvidence(t *testing.T) {
	t.Parallel()

	result := buildEvidenceContext([]retriever.RetrievalResult{})

	if !strings.Contains(result, "=== EVIDENCE CONTEXT ===") {
		t.Error("missing EVIDENCE CONTEXT header")
	}
	if !strings.Contains(result, "=== END EVIDENCE CONTEXT ===") {
		t.Error("missing END EVIDENCE CONTEXT footer")
	}
	if !strings.Contains(result, "Total domains with evidence: 0") {
		t.Error("missing domain count")
	}
}

func TestBuildEvidenceContext_DomainWithNoChunks(t *testing.T) {
	t.Parallel()

	evidence := []retriever.RetrievalResult{
		{Domain: "biodiversity_abs", Chunks: []retriever.Chunk{}},
	}

	result := buildEvidenceContext(evidence)

	if !strings.Contains(result, "NO EVIDENCE RETRIEVED FOR THIS DOMAIN") {
		t.Error("expected no-evidence marker for domain with empty chunks")
	}
	if !strings.Contains(result, "--- DOMAIN: biodiversity_abs ---") {
		t.Error("expected domain header even with no chunks")
	}
}

func TestBuildEvidenceContext_LongChunkTextIsTruncated(t *testing.T) {
	t.Parallel()

	longText := strings.Repeat("A", 1200) // 1200 chars, exceeds 800-char limit

	evidence := []retriever.RetrievalResult{
		{
			Domain: "patent",
			Chunks: []retriever.Chunk{
				{
					ID:           "chunk_abc",
					DocTitle:     "Patents Act",
					Section:      "Section 3",
					AuthorityStr: "statute",
					Text:         longText,
				},
			},
		},
	}

	result := buildEvidenceContext(evidence)

	// The text in the output should be truncated to 800 chars + "..."
	if strings.Contains(result, longText) {
		t.Error("expected long text to be truncated, but full text found in output")
	}
	if !strings.Contains(result, "...") {
		t.Error("expected '...' ellipsis after truncated text")
	}
	// Verify the truncated portion is 800 chars.
	truncated := longText[:800] + "..."
	if !strings.Contains(result, truncated) {
		t.Error("expected truncated text + ellipsis in output")
	}
}

func TestBuildEvidenceContext_ChunkIDInExactFormat(t *testing.T) {
	t.Parallel()

	evidence := []retriever.RetrievalResult{
		{
			Domain: "patent",
			Chunks: []retriever.Chunk{
				{
					ID:           "abc-123",
					DocTitle:     "Test Doc",
					Section:      "Section 1",
					AuthorityStr: "statute",
					Text:         "Some legal text.",
				},
			},
		},
	}

	result := buildEvidenceContext(evidence)

	expected := "[CHUNK ID: abc-123]"
	if !strings.Contains(result, expected) {
		t.Errorf("expected chunk ID format %q in output, got:\n%s", expected, result)
	}
}

// ── validateRoadmapStructure tests ────────────────────────────────────────────

func validRoadmap() *domain.IPRoadmap {
	return &domain.IPRoadmap{
		ProductSummary: "Test product",
		Classification: "Proprietary",
		Domains: []domain.DomainAnalysis{
			{
				Domain:     domain.DomainPatent,
				Status:     domain.StatusRelevant,
				Confidence: 0.75,
			},
		},
		NextSteps:         []string{"Do something"},
		HumanEscalation:   []domain.EscalationItem{},
		JurisdictionNotes: []domain.JurisdictionNote{},
		OverallConfidence: 0.75,
	}
}

func TestValidateRoadmapStructure_ValidRoadmap(t *testing.T) {
	t.Parallel()

	if err := validateRoadmapStructure(validRoadmap()); err != nil {
		t.Errorf("expected nil error for valid roadmap, got: %v", err)
	}
}

func TestValidateRoadmapStructure_NilRoadmap(t *testing.T) {
	t.Parallel()

	if err := validateRoadmapStructure(nil); err == nil {
		t.Error("expected error for nil roadmap, got nil")
	}
}

func TestValidateRoadmapStructure_EmptyDomains(t *testing.T) {
	t.Parallel()

	r := validRoadmap()
	r.Domains = []domain.DomainAnalysis{}

	if err := validateRoadmapStructure(r); err == nil {
		t.Error("expected error for empty domains, got nil")
	}
}

func TestValidateRoadmapStructure_EmptyNextSteps(t *testing.T) {
	t.Parallel()

	r := validRoadmap()
	r.NextSteps = []string{}

	if err := validateRoadmapStructure(r); err == nil {
		t.Error("expected error for empty next steps, got nil")
	}
}

func TestValidateRoadmapStructure_InvalidDomainStatus(t *testing.T) {
	t.Parallel()

	r := validRoadmap()
	r.Domains[0].Status = "totally_made_up"

	if err := validateRoadmapStructure(r); err == nil {
		t.Error("expected error for invalid domain status, got nil")
	}
}

func TestValidateRoadmapStructure_ConfidenceOutOfRange(t *testing.T) {
	t.Parallel()

	r := validRoadmap()
	r.Domains[0].Confidence = 1.5

	if err := validateRoadmapStructure(r); err == nil {
		t.Error("expected error for confidence > 1.0, got nil")
	}
}

func TestValidateRoadmapStructure_EmptyDomainField(t *testing.T) {
	t.Parallel()

	r := validRoadmap()
	r.Domains[0].Domain = ""

	if err := validateRoadmapStructure(r); err == nil {
		t.Error("expected error for empty domain field, got nil")
	}
}
