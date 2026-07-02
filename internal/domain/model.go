package domain

import "strings"

// Candidate is one normalized model route candidate.
type Candidate struct {
	Provider     string
	Model        string
	Capabilities []string
	Cost         string
	Quality      string
}

// CandidateFromConfig converts user configuration to a normalized candidate.
func CandidateFromConfig(cfg CandidateConfig) Candidate {
	return Candidate{
		Provider:     strings.ToLower(strings.TrimSpace(cfg.Provider)),
		Model:        strings.TrimSpace(cfg.Model),
		Capabilities: normalizeStrings(cfg.Capabilities),
		Cost:         strings.ToLower(strings.TrimSpace(cfg.Cost)),
		Quality:      strings.ToLower(strings.TrimSpace(cfg.Quality)),
	}
}

// Valid reports whether a candidate can produce a provider route.
func (c Candidate) Valid() bool {
	return strings.TrimSpace(c.Provider) != "" && strings.TrimSpace(c.Model) != ""
}

// RouteDecision is the domain result of a routing use case.
type RouteDecision struct {
	Handled        bool
	TargetProvider string
	TargetModel    string
	Reason         string
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
