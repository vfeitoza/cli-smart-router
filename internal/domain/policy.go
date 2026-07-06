package domain

import (
	"fmt"
	"sort"
	"strings"
)

// RouteScore is one scored routing candidate produced by local capability-aware
// selection. It captures enough detail for callers to log the decision and to
// decide whether the local decision is confident enough to skip a classifier.
type RouteScore struct {
	Candidate     Candidate
	Score         float64
	MatchedCount  int
	InferredCount int
	Reason        string
}

// LocalConfident reports whether this score represents a decision the local
// capability scoring is confident about, without needing a classifier second
// opinion. Confidence requires that the prompt matched at least one known
// capability signal (InferredCount > 0) and that this candidate actually
// satisfies at least one of those signals (MatchedCount > 0). An ambiguous
// prompt that matches no known signal is never considered confident, even
// though a candidate is still deterministically selected via preference/tier
// ranking.
func (r RouteScore) LocalConfident() bool {
	return r.InferredCount > 0 && r.MatchedCount > 0
}

// SelectCandidate returns the cheapest configured candidate with an available provider.
func SelectCandidate(candidates []Candidate, availableProviders []string) (Candidate, bool) {
	return SelectCandidateForPrompt(candidates, availableProviders, "", DefaultPreference)
}

// SelectCandidateForPrompt picks the best-scoring eligible candidate for the prompt.
// See ScoreCandidates for the ranking rules and SelectCandidateWithConfidence for
// the scored result including local confidence.
func SelectCandidateForPrompt(candidates []Candidate, availableProviders []string, prompt string, preference string) (Candidate, bool) {
	scores := ScoreCandidates(candidates, availableProviders, prompt, preference)
	if len(scores) == 0 {
		return Candidate{}, false
	}
	return scores[0].Candidate, true
}

// SelectCandidateWithConfidence scores every eligible candidate for the prompt and
// returns the best one, including whether the local decision is confident enough
// to skip a classifier call. Callers such as the hybrid strategy use this to decide
// whether a classifier second opinion is worth its latency and cost.
func SelectCandidateWithConfidence(candidates []Candidate, availableProviders []string, prompt string, preference string) (RouteScore, bool) {
	scores := ScoreCandidates(candidates, availableProviders, prompt, preference)
	if len(scores) == 0 {
		return RouteScore{}, false
	}
	return scores[0], true
}

// ScoreCandidates scores every eligible candidate (valid, available provider, and
// meeting the minimum cost tier required by the prompt) and returns them sorted
// from highest to lowest score. Configuration order breaks exact score ties
// because sort.SliceStable preserves the input order of equal elements.
//
// Selection happens in two stages:
//
//  1. Capability gate: capabilities are inferred from the prompt via
//     InferCapabilities. If at least one candidate declares at least one of
//     those inferred capabilities, only candidates that declare at least one
//     matching capability remain eligible. This narrows the pool to models the
//     configuration actually advertises as fit for the task (e.g. a prompt
//     implying "architecture" excludes models that only declare "summarize").
//     If the prompt has no inferred capabilities, or no configured candidate
//     declares any of them, the gate is a no-op and every eligible candidate
//     from stage zero remains in the pool. This keeps behavior unchanged for
//     candidates that omit `capabilities` in configuration.
//  2. Preference-driven tier ranking within the gated pool: `quality` prefers
//     the highest quality tier, breaking ties toward the cheaper candidate;
//     `cost` and `balanced` prefer the lowest cost tier, breaking ties toward
//     the higher quality candidate. This is the same deterministic tier logic
//     used before capability-aware scoring existed, so a prompt matching zero
//     or many candidates at the capability stage still degrades gracefully to
//     the original tier ranking behavior.
//
// The capability gate intentionally does not rank by *how many* capabilities a
// candidate matches: a model matching two inferred capabilities is not
// automatically preferred over one matching a single, more specific
// capability. Ranking within the gated pool is always driven by preference so
// that `balanced`/`cost` do not silently escalate to the most capable (and
// usually priciest) model just because it happens to declare more tags.
func ScoreCandidates(candidates []Candidate, availableProviders []string, prompt string, preference string) []RouteScore {
	available := make(map[string]struct{}, len(availableProviders))
	for _, provider := range normalizeStrings(availableProviders) {
		available[provider] = struct{}{}
	}
	minCost := minimumCostForPrompt(prompt)
	inferred := InferCapabilities(prompt)

	type eligible struct {
		candidate Candidate
		matched   int
	}
	pool := make([]eligible, 0, len(candidates))
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
		pool = append(pool, eligible{candidate: candidate, matched: matchCount(candidate, inferred)})
	}

	gated := pool
	if len(inferred) > 0 {
		matchedOnly := make([]eligible, 0, len(pool))
		for _, item := range pool {
			if item.matched > 0 {
				matchedOnly = append(matchedOnly, item)
			}
		}
		if len(matchedOnly) > 0 {
			gated = matchedOnly
		}
	}

	out := make([]RouteScore, 0, len(gated))
	for _, item := range gated {
		out = append(out, RouteScore{
			Candidate:     item.candidate,
			Score:         tierScore(item.candidate, preference),
			MatchedCount:  item.matched,
			InferredCount: len(inferred),
			Reason:        scoreReason(item.matched, len(inferred), preference),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// tierScore ranks a candidate by cost/quality tier according to preference.
// The multiplier of 10 keeps the primary dimension (quality for `quality`,
// cost for `cost`/`balanced`) dominant over the secondary tie-break dimension,
// whose rank spread never exceeds 3.
func tierScore(candidate Candidate, preference string) float64 {
	if preference == PreferenceQuality {
		return float64(qualityRank(candidate.Quality))*10 - float64(costRank(candidate.Cost))
	}
	// cost and balanced share the same tier ranking: cheapest first, quality as tie-break.
	return float64(3-costRank(candidate.Cost))*10 + float64(qualityRank(candidate.Quality))
}

// matchCount counts how many inferred capabilities the candidate satisfies.
func matchCount(candidate Candidate, inferred []string) int {
	count := 0
	for _, cap := range inferred {
		if containsString(candidate.Capabilities, cap) {
			count++
		}
	}
	return count
}

func scoreReason(matched, inferredCount int, preference string) string {
	if inferredCount == 0 {
		return fmt.Sprintf("preference:%s", preference)
	}
	return fmt.Sprintf("capabilities:%d/%d preference:%s", matched, inferredCount, preference)
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func minimumCostForPrompt(prompt string) int {
	prompt = strings.ToLower(prompt)
	// ponytail: keyword tiers keep deterministic fallback useful without building a rules engine.
	if containsAny(prompt,
		"arquitetura", "architecture", "system design", "design system",
		"migração", "migracao", "migration", "monorepo",
		"produção", "producao", "production",
		"segurança", "seguranca", "security",
		"critical", "crítica", "critica",
	) {
		return costRank("very_high")
	}
	if containsAny(prompt,
		"plano", "planeje", "planejar", "planning", "strategy", "estratégia", "estrategia",
		"detalhes", "details", "como funciona", "how does it work",
		"explique", "explain", "analise", "analyze", "compare",
		"código", "codigo", "code", "coding",
		"implemente", "implement", "implementation",
		"refatore", "refactor", "debug", "bug", "erro", "error",
		"teste", "test", "review", "revisão", "revisao",
	) {
		return costRank("high")
	}
	if containsAny(prompt,
		"script", "regex", "sql", "query",
		"documente", "document", "documentation",
		"formate", "format", "rewrite", "reword",
	) {
		return costRank("medium")
	}
	if containsAny(prompt, "2+3", "quanto é", "quanto e", "classifique", "classify", "resuma", "summarize", "traduza", "translate") {
		return costRank("low")
	}
	return costRank("low")
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
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
