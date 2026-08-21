package classifier

import (
	"strings"
	"testing"
)

// ── validateInput tests ───────────────────────────────────────────────────────

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   \t\n  ",
			wantErr: true,
		},
		{
			name:    "too short (5 chars)",
			input:   "short",
			wantErr: true,
		},
		{
			name:    "exactly 19 chars (one under minimum)",
			input:   "nineteen chars here",
			wantErr: true,
		},
		{
			name:    "exactly 20 chars (at minimum)",
			input:   "twenty characters ok",
			wantErr: false,
		},
		{
			name:    "valid 50-char description",
			input:   "A valid Ayurvedic formulation with named ingredients",
			wantErr: false,
		},
		{
			name:    "2000 chars (at maximum)",
			input:   strings.Repeat("a", 2000),
			wantErr: false,
		},
		{
			name:    "2001 chars (over maximum)",
			input:   strings.Repeat("a", 2001),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateInput(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// ── validateOutput tests ──────────────────────────────────────────────────────

func makeValidRaw() classificationRaw {
	return classificationRaw{
		FormulationType:     "proprietary",
		IndianBioResources:  true,
		TKInvolved:          true,
		TargetMarkets:       []string{"india"},
		RelevantDomains:     []string{"patent", "regulatory"},
		ClarifyingQuestions: []string{},
		Confidence:          0.82,
	}
}

func TestValidateOutput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*classificationRaw)
		wantErr bool
	}{
		{
			name:    "all valid fields",
			mutate:  func(r *classificationRaw) {},
			wantErr: false,
		},
		{
			name:    "unknown formulation_type 'tablet'",
			mutate:  func(r *classificationRaw) { r.FormulationType = "tablet" },
			wantErr: true,
		},
		{
			name:    "all valid formulation types",
			mutate:  func(r *classificationRaw) { r.FormulationType = "classical" },
			wantErr: false,
		},
		{
			name:    "confidence 1.5 (above 1.0)",
			mutate:  func(r *classificationRaw) { r.Confidence = 1.5 },
			wantErr: true,
		},
		{
			name:    "confidence -0.1 (below 0.0)",
			mutate:  func(r *classificationRaw) { r.Confidence = -0.1 },
			wantErr: true,
		},
		{
			name:    "confidence 0.0 (boundary, valid)",
			mutate:  func(r *classificationRaw) { r.Confidence = 0.0 },
			wantErr: false,
		},
		{
			name:    "confidence 1.0 (boundary, valid)",
			mutate:  func(r *classificationRaw) { r.Confidence = 1.0 },
			wantErr: false,
		},
		{
			name:    "nil relevant_domains",
			mutate:  func(r *classificationRaw) { r.RelevantDomains = nil },
			wantErr: true,
		},
		{
			name:    "invalid domain 'copyright' in list",
			mutate:  func(r *classificationRaw) { r.RelevantDomains = []string{"patent", "copyright"} },
			wantErr: true,
		},
		{
			name:    "empty relevant_domains slice (valid)",
			mutate:  func(r *classificationRaw) { r.RelevantDomains = []string{} },
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := makeValidRaw()
			tc.mutate(&raw)
			err := validateOutput(raw)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// ── buildClassificationPrompt tests ──────────────────────────────────────────

func TestBuildClassificationPrompt(t *testing.T) {
	const desc = "Ashwagandha and Shallaki joint-pain formulation sourced from India."

	t.Run("empty clarifications contains only description", func(t *testing.T) {
		out := buildClassificationPrompt(desc, map[string]string{})
		if !strings.Contains(out, "Product description:") {
			t.Error("expected 'Product description:' header")
		}
		if !strings.Contains(out, desc) {
			t.Error("expected description in output")
		}
		if strings.Contains(out, "clarifications") {
			t.Error("did not expect 'clarifications' section for empty map")
		}
	})

	t.Run("non-empty clarifications appear in output", func(t *testing.T) {
		clarifications := map[string]string{
			"0": "Ingredients sourced entirely from India.",
			"1": "Not based on a classical text.",
		}
		out := buildClassificationPrompt(desc, clarifications)
		if !strings.Contains(out, "Product description:") {
			t.Error("expected 'Product description:' header")
		}
		if !strings.Contains(out, "Additional clarifications") {
			t.Error("expected clarifications section")
		}
		if !strings.Contains(out, "Ingredients sourced entirely from India.") {
			t.Error("expected first answer in output")
		}
		if !strings.Contains(out, "Not based on a classical text.") {
			t.Error("expected second answer in output")
		}
	})
}

// ── extractJSON tests ─────────────────────────────────────────────────────────

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean JSON passthrough",
			input: `{"confidence":0.8}`,
			want:  `{"confidence":0.8}`,
		},
		{
			name:  "strips leading markdown fence",
			input: "```json\n{\"confidence\":0.8}\n```",
			want:  `{"confidence":0.8}`,
		},
		{
			name:  "strips preamble text",
			input: `Here is the result: {"confidence":0.8}`,
			want:  `{"confidence":0.8}`,
		},
		{
			name:  "no braces returns original",
			input: "no json here",
			want:  "no json here",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractJSON(tc.input)
			if got != tc.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
