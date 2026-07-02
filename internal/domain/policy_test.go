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
