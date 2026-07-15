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

// AvailableSet builds a lookup of lowercased, trimmed provider names. An empty
// input yields an empty set, which callers treat as "no restriction". It is the
// single availability-set builder shared by the Policy Engine and Router so
// provider-availability checks stay identical across layers.
func AvailableSet(providers []string) map[string]struct{} {
	set := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider != "" {
			set[provider] = struct{}{}
		}
	}
	return set
}

// ProviderAvailable reports whether provider is in the available list. An empty
// list means no restriction (everything is available).
func ProviderAvailable(provider string, available []string) bool {
	if len(available) == 0 {
		return true
	}
	if _, ok := AvailableSet(available)[strings.ToLower(strings.TrimSpace(provider))]; ok {
		return true
	}
	return false
}

// RouteDecision is the domain result of a routing use case.
type RouteDecision struct {
	Handled        bool
	TargetProvider string
	TargetModel    string
	Reason         string
	// Confident reports whether local capability-aware scoring matched at least
	// one capability signal inferred from the prompt. Callers implementing the
	// hybrid strategy use this to skip the classifier when the local decision
	// is already well-supported by configured `models.capabilities`.
	Confident bool
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
