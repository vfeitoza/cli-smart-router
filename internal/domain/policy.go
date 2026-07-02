package domain

import "strings"

// SelectCandidate returns the cheapest configured candidate with an available provider.
func SelectCandidate(candidates []Candidate, availableProviders []string) (Candidate, bool) {
	return SelectCandidateForPrompt(candidates, availableProviders, "", DefaultPreference)
}

// SelectCandidateForPrompt picks a model strong enough for the prompt, biased by preference.
// quality prefers the highest-quality eligible model; cost/balanced prefer the cheapest.
func SelectCandidateForPrompt(candidates []Candidate, availableProviders []string, prompt string, preference string) (Candidate, bool) {
	available := make(map[string]struct{}, len(availableProviders))
	for _, provider := range normalizeStrings(availableProviders) {
		available[provider] = struct{}{}
	}
	minCost := minimumCostForPrompt(prompt)
	var selected Candidate
	found := false
	for _, candidate := range candidates {
		if !candidate.Valid() {
			continue
		}
		if len(available) > 0 {
			if _, ok := available[candidate.Provider]; !ok {
				continue
			}
		}
		if costRank(candidate.Cost) < minCost {
			continue
		}
		if !found || preferBetter(candidate, selected, preference) {
			selected = candidate
			found = true
		}
	}
	return selected, found
}

// preferBetter reports whether candidate should replace current under the given preference.
func preferBetter(candidate, current Candidate, preference string) bool {
	if preference == PreferenceQuality {
		if qualityRank(candidate.Quality) != qualityRank(current.Quality) {
			return qualityRank(candidate.Quality) > qualityRank(current.Quality)
		}
		// Tie on quality: prefer the cheaper one.
		return costRank(candidate.Cost) < costRank(current.Cost)
	}
	if costRank(candidate.Cost) != costRank(current.Cost) {
		return costRank(candidate.Cost) < costRank(current.Cost)
	}
	// Tie on cost: prefer the higher quality one.
	return qualityRank(candidate.Quality) > qualityRank(current.Quality)
}

func minimumCostForPrompt(prompt string) int {
	prompt = strings.ToLower(prompt)
	// ponytail: naive text signals; replace with classifier scoring if real traces demand it.
	if strings.Contains(prompt, "2+3") || strings.Contains(prompt, "quanto é") || strings.Contains(prompt, "classifique") || strings.Contains(prompt, "resuma") {
		return costRank("low")
	}
	if strings.Contains(prompt, "detalhes") || strings.Contains(prompt, "como funciona") || strings.Contains(prompt, "explique") || strings.Contains(prompt, "código") || strings.Contains(prompt, "codigo") {
		return costRank("high")
	}
	return costRank("low")
}

func costRank(cost string) int {
	switch cost {
	case "low":
		return 0
	case "medium":
		return 1
	case "high":
		return 2
	case "very_high":
		return 3
	default:
		return 1
	}
}

func qualityRank(quality string) int {
	switch quality {
	case "low":
		return 0
	case "medium":
		return 1
	case "high":
		return 2
	case "highest":
		return 3
	default:
		return 1
	}
}

// ApplyPreferenceTiebreak nudges the chosen candidate toward the preference among models
// the chosen one is effectively tied with, so preference "wins" real ties without
// overriding the complexity judgment (cost tier for quality; quality tier for cost).
func ApplyPreferenceTiebreak(chosen Candidate, candidates []Candidate, availableProviders []string, preference string) Candidate {
	if preference == PreferenceBalanced || !chosen.Valid() {
		return chosen
	}
	available := make(map[string]struct{}, len(availableProviders))
	for _, provider := range normalizeStrings(availableProviders) {
		available[provider] = struct{}{}
	}
	best := chosen
	for _, candidate := range candidates {
		if !candidate.Valid() {
			continue
		}
		if len(available) > 0 {
			if _, ok := available[candidate.Provider]; !ok {
				continue
			}
		}
		switch preference {
		case PreferenceQuality:
			// Same cost tier: prefer the highest quality.
			if costRank(candidate.Cost) == costRank(chosen.Cost) &&
				qualityRank(candidate.Quality) > qualityRank(best.Quality) {
				best = candidate
			}
		case PreferenceCost:
			// Same quality tier: prefer the cheapest.
			if qualityRank(candidate.Quality) == qualityRank(chosen.Quality) &&
				costRank(candidate.Cost) < costRank(best.Cost) {
				best = candidate
			}
		}
	}
	return best
}
