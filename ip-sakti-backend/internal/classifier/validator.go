package classifier

import (
	"fmt"
	"strings"
)

var validFormulationTypes = map[string]struct{}{
	"classical":          {},
	"modified_classical": {},
	"proprietary":        {},
	"new_drug":           {},
	"food_nutraceutical": {},
	"cosmetic":           {},
	"unknown":            {},
}

var validDomains = map[string]struct{}{
	"patent":              {},
	"traditional_knowledge": {},
	"biodiversity_abs":    {},
	"regulatory":          {},
	"trademark":           {},
}

// validateInput checks the raw description before making any API calls.
func validateInput(description string) error {
	trimmed := strings.TrimSpace(description)
	if trimmed == "" {
		return fmt.Errorf("description cannot be empty")
	}
	if len(trimmed) < 20 {
		return fmt.Errorf("description too short (minimum 20 characters)")
	}
	if len(description) > 2000 {
		return fmt.Errorf("description too long (maximum 2000 characters)")
	}
	return nil
}

// validateOutput checks the parsed Gemini response for semantic correctness.
func validateOutput(raw classificationRaw) error {
	if _, ok := validFormulationTypes[raw.FormulationType]; !ok {
		return fmt.Errorf("invalid formulation_type: %s", raw.FormulationType)
	}

	if raw.RelevantDomains == nil {
		return fmt.Errorf("relevant_domains must not be nil")
	}

	for _, d := range raw.RelevantDomains {
		if _, ok := validDomains[d]; !ok {
			return fmt.Errorf("invalid domain: %s", d)
		}
	}

	if raw.Confidence < 0.0 || raw.Confidence > 1.0 {
		return fmt.Errorf("confidence out of range: %f", raw.Confidence)
	}

	return nil
}
