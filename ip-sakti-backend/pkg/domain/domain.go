// Package domain defines the core IP-SAKTI domain types shared across all
// internal packages. Having them in pkg/domain (rather than internal/api)
// breaks the import cycle between internal/api (handlers) and the pipeline
// packages (classifier, retriever, synthesizer, confidence) which also need
// these types.
package domain

// ── Formulation types ─────────────────────────────────────────────────────────

// FormulationType categorises the Ayurvedic product formulation.
type FormulationType string

const (
	FormulationClassical         FormulationType = "classical"
	FormulationModifiedClassical FormulationType = "modified_classical"
	FormulationProprietary       FormulationType = "proprietary"
	FormulationNewDrug           FormulationType = "new_drug"
	FormulationFood              FormulationType = "food_nutraceutical"
	FormulationCosmetic          FormulationType = "cosmetic"
	FormulationUnknown           FormulationType = "unknown"
)

// ── IP / regulatory domains ───────────────────────────────────────────────────

// Domain identifies an IP or regulatory domain relevant to a formulation.
type Domain string

const (
	DomainPatent     Domain = "patent"
	DomainTrademark  Domain = "trademark"
	DomainABS        Domain = "biodiversity_abs"
	DomainRegulatory Domain = "regulatory"
	DomainTK         Domain = "traditional_knowledge"
)

// ── Classification ────────────────────────────────────────────────────────────

// ClassificationResult is the output of the classifier pipeline.
type ClassificationResult struct {
	FormulationType     FormulationType `json:"formulation_type"`
	IndianBioResources  bool            `json:"indian_bio_resources"`
	TKInvolved          bool            `json:"tk_involved"`
	TargetMarkets       []string        `json:"target_markets"`
	RelevantDomains     []Domain        `json:"relevant_domains"`
	ClarifyingQuestions []string        `json:"clarifying_questions"`
	Confidence          float64         `json:"confidence"`
	RawDescription      string          `json:"raw_description"`
}

// ── Citations ─────────────────────────────────────────────────────────────────

// Citation links a piece of analysis back to a source document.
type Citation struct {
	ChunkID     string `json:"chunk_id"`
	DocTitle    string `json:"doc_title"`
	Section     string `json:"section"`
	SourceURL   string `json:"source_url"`
	RetrievedAt string `json:"retrieved_at"`
}

// ── Domain analysis ───────────────────────────────────────────────────────────

// DomainStatus represents the relevance verdict for a single IP domain.
type DomainStatus string

const (
	StatusRelevant             DomainStatus = "relevant"
	StatusInsufficientEvidence DomainStatus = "insufficient_evidence"
	StatusNotApplicable        DomainStatus = "not_applicable"
)

// DomainAnalysis is a structured finding for one IP/regulatory domain.
type DomainAnalysis struct {
	Domain          Domain       `json:"domain"`
	Status          DomainStatus `json:"status"`
	Finding         string       `json:"finding"`
	KeyRisks        []string     `json:"key_risks"`
	Citations       []Citation   `json:"citations"`
	Confidence      float64      `json:"confidence"`
	NeedsEscalation bool         `json:"needs_escalation"`
}

// ── Escalation ────────────────────────────────────────────────────────────────

// EscalationItem recommends a human professional review for a specific issue.
type EscalationItem struct {
	Reason   string `json:"reason"`
	ProfType string `json:"prof_type"`
	Urgency  string `json:"urgency"`
}

// ── Roadmap ───────────────────────────────────────────────────────────────────

// IPRoadmap is the synthesised IP strategy delivered to the user.
type IPRoadmap struct {
	ProductSummary    string           `json:"product_summary"`
	Classification    string           `json:"classification"`
	Domains           []DomainAnalysis `json:"domains"`
	NextSteps         []string         `json:"next_steps"`
	HumanEscalation   []EscalationItem `json:"human_escalation"`
	OverallConfidence float64          `json:"overall_confidence"`
	Disclaimer        string           `json:"disclaimer"`
}


