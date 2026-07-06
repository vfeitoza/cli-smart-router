package domain

import "testing"

func candidates() []Candidate {
	return []Candidate{
		CandidateFromConfig(CandidateConfig{Provider: "claude", Model: "opus", Cost: "very_high", Quality: "highest"}),
		CandidateFromConfig(CandidateConfig{Provider: "claude", Model: "sonnet", Cost: "high", Quality: "high"}),
		CandidateFromConfig(CandidateConfig{Provider: "codex", Model: "gpt-high", Cost: "high", Quality: "high"}),
		CandidateFromConfig(CandidateConfig{Provider: "claude", Model: "haiku", Cost: "low", Quality: "medium"}),
	}
}

func TestSelectCandidateCostPreferencePicksCheapest(t *testing.T) {
	got, ok := SelectCandidateForPrompt(candidates(), []string{"claude", "codex"}, "", PreferenceCost)
	if !ok || got.Model != "haiku" {
		t.Fatalf("cost preference = %#v, want haiku", got)
	}
}

func TestSelectCandidateQualityPreferencePicksHighest(t *testing.T) {
	got, ok := SelectCandidateForPrompt(candidates(), []string{"claude", "codex"}, "", PreferenceQuality)
	if !ok || got.Model != "opus" {
		t.Fatalf("quality preference = %#v, want opus", got)
	}
}

func TestApplyPreferenceTiebreakQualityPromotesSameCostTier(t *testing.T) {
	// Classifier picked a high-cost model; quality preference should keep the highest
	// quality within the same cost tier (both sonnet and gpt-high are cost=high/quality=high,
	// so no change), while opus is a different cost tier and must not be selected.
	chosen := CandidateFromConfig(CandidateConfig{Provider: "codex", Model: "gpt-high", Cost: "high", Quality: "high"})
	got := ApplyPreferenceTiebreak(chosen, candidates(), []string{"claude", "codex"}, PreferenceQuality)
	if costRank(got.Cost) != costRank("high") {
		t.Fatalf("tiebreak crossed cost tier: %#v", got)
	}
}

func TestApplyPreferenceTiebreakCostPromotesCheaperSameQuality(t *testing.T) {
	list := []Candidate{
		CandidateFromConfig(CandidateConfig{Provider: "claude", Model: "sonnet", Cost: "high", Quality: "high"}),
		CandidateFromConfig(CandidateConfig{Provider: "codex", Model: "gpt-med", Cost: "medium", Quality: "high"}),
	}
	chosen := list[0] // sonnet, cost high, quality high
	got := ApplyPreferenceTiebreak(chosen, list, []string{"claude", "codex"}, PreferenceCost)
	if got.Model != "gpt-med" {
		t.Fatalf("cost tiebreak = %#v, want gpt-med (same quality, cheaper)", got)
	}
}

func TestApplyPreferenceTiebreakBalancedIsNoop(t *testing.T) {
	chosen := CandidateFromConfig(CandidateConfig{Provider: "claude", Model: "sonnet", Cost: "high", Quality: "high"})
	got := ApplyPreferenceTiebreak(chosen, candidates(), []string{"claude", "codex"}, PreferenceBalanced)
	if got.Model != "sonnet" {
		t.Fatalf("balanced tiebreak = %#v, want unchanged sonnet", got)
	}
}

// capabilityCandidates mirrors the configured models.yaml example so scoring tests
// exercise realistic capability tags rather than synthetic ones.
func capabilityCandidates() []Candidate {
	return []Candidate{
		CandidateFromConfig(CandidateConfig{
			Provider: "claude", Model: "claude-opus-4-8", Cost: "very_high", Quality: "highest",
			Capabilities: []string{"reasoning", "architecture", "planning", "analysis", "writing", "long_context", "high_quality"},
		}),
		CandidateFromConfig(CandidateConfig{
			Provider: "claude", Model: "claude-sonnet-5", Cost: "high", Quality: "high",
			Capabilities: []string{"coding", "reasoning", "review", "refactor", "tools", "architecture", "writing"},
		}),
		CandidateFromConfig(CandidateConfig{
			Provider: "claude", Model: "claude-haiku-4-5", Cost: "low", Quality: "medium",
			Capabilities: []string{"classify", "summarize", "translate", "fast", "low_cost", "routing"},
		}),
		CandidateFromConfig(CandidateConfig{
			Provider: "codex", Model: "gpt-5.5", Cost: "very_high", Quality: "highest",
			Capabilities: []string{"coding", "reasoning", "agents", "tools", "architecture", "analysis", "high_quality"},
		}),
		CandidateFromConfig(CandidateConfig{
			Provider: "codex", Model: "gpt-5.4", Cost: "high", Quality: "high",
			Capabilities: []string{"coding", "reasoning", "tools", "review", "writing", "documentation", "tests", "scripts", "general"},
		}),
		CandidateFromConfig(CandidateConfig{
			Provider: "codex", Model: "gpt-5.4-mini", Cost: "low", Quality: "medium",
			Capabilities: []string{"classify", "summarize", "translate", "simple_coding", "fast", "low_cost", "routing"},
		}),
	}
}

func TestScoreCandidatesSummarizePromptPicksLowCostSummarizer(t *testing.T) {
	score, ok := SelectCandidateWithConfidence(capabilityCandidates(), []string{"claude", "codex"}, "Resuma este texto em 5 bullets", PreferenceBalanced)
	if !ok {
		t.Fatal("SelectCandidateWithConfidence() ok = false, want true")
	}
	if score.Candidate.Model != "claude-haiku-4-5" && score.Candidate.Model != "gpt-5.4-mini" {
		t.Fatalf("summarize prompt = %#v, want claude-haiku-4-5 or gpt-5.4-mini", score.Candidate)
	}
	if !score.LocalConfident() {
		t.Fatalf("summarize prompt score = %#v, want LocalConfident() true", score)
	}
}

func TestScoreCandidatesArchitecturePromptExcludesSummarizers(t *testing.T) {
	score, ok := SelectCandidateWithConfidence(capabilityCandidates(), []string{"claude", "codex"}, "Planeje a arquitetura de um sistema multi-tenant com filas e retries", PreferenceBalanced)
	if !ok {
		t.Fatal("SelectCandidateWithConfidence() ok = false, want true")
	}
	if score.Candidate.Model == "claude-haiku-4-5" || score.Candidate.Model == "gpt-5.4-mini" {
		t.Fatalf("architecture prompt = %#v, want a coding/architecture-capable model, not a summarizer", score.Candidate)
	}
	if !score.LocalConfident() {
		t.Fatalf("architecture prompt score = %#v, want LocalConfident() true", score)
	}
}

func TestScoreCandidatesArchitecturePromptQualityPrefersHighestTier(t *testing.T) {
	score, ok := SelectCandidateWithConfidence(capabilityCandidates(), []string{"claude", "codex"}, "Planeje a arquitetura de um sistema multi-tenant com filas e retries", PreferenceQuality)
	if !ok {
		t.Fatal("SelectCandidateWithConfidence() ok = false, want true")
	}
	if qualityRank(score.Candidate.Quality) != qualityRank("highest") {
		t.Fatalf("architecture prompt with quality preference = %#v, want highest quality tier", score.Candidate)
	}
}

func TestScoreCandidatesAmbiguousPromptIsNotLocalConfident(t *testing.T) {
	score, ok := SelectCandidateWithConfidence(capabilityCandidates(), []string{"claude", "codex"}, "Me ajude a melhorar isso", PreferenceBalanced)
	if !ok {
		t.Fatal("SelectCandidateWithConfidence() ok = false, want true")
	}
	if score.LocalConfident() {
		t.Fatalf("ambiguous prompt score = %#v, want LocalConfident() false", score)
	}
}

func TestScoreCandidatesNoCapabilitiesConfiguredFallsBackToTierRanking(t *testing.T) {
	// When configured candidates omit `capabilities` entirely, the gate is a
	// no-op and behavior matches the original deterministic tier ranking.
	score, ok := SelectCandidateWithConfidence(candidates(), []string{"claude", "codex"}, "explique como funciona o sistema", PreferenceCost)
	if !ok {
		t.Fatal("SelectCandidateWithConfidence() ok = false, want true")
	}
	if score.Candidate.Model != "sonnet" && score.Candidate.Model != "gpt-high" {
		t.Fatalf("no-capabilities detailed prompt = %#v, want a high-cost tier candidate", score.Candidate)
	}
	if score.LocalConfident() {
		t.Fatalf("no-capabilities score = %#v, want LocalConfident() false (no inferred capability matched any candidate)", score)
	}
}
