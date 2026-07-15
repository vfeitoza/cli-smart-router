package domain

import "testing"

func TestEvaluateRoutesMatchesExampleRule(t *testing.T) {
	// routes:
	//   - when: {task: coding, language: go, complexity: low}
	//     model: kimi
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding", Language: "go", Complexity: "low"}, Model: "kimi"},
	}
	facts := RouteFacts{Task: IntentCoding, Language: "go", ComplexityTier: 0, ComplexityScore: 10}
	got := EvaluateRoutes(rules, facts, routeTestCandidates(), nil)
	if !got.Matched || got.Model != "kimi" || got.Provider != "codex" {
		t.Fatalf("expected kimi/codex, got %+v", got)
	}
	if got.Specificity != 3 {
		t.Fatalf("expected specificity 3, got %d", got.Specificity)
	}
}

func TestEvaluateRoutesNoMatchWhenConditionFails(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding", Language: "go"}, Model: "kimi"},
	}
	facts := RouteFacts{Task: IntentCoding, Language: "python"}
	got := EvaluateRoutes(rules, facts, routeTestCandidates(), nil)
	if got.Matched {
		t.Fatalf("expected no match for language mismatch, got %+v", got)
	}
}

func TestEvaluateRoutesPrefersMostSpecificRule(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding"}, Model: "gpt-5.4-mini"},                            // specificity 1
		{When: RouteCondition{Task: "coding", Language: "go", Complexity: "low"}, Model: "kimi"}, // specificity 3
	}
	facts := RouteFacts{Task: IntentCoding, Language: "go", ComplexityTier: 0}
	got := EvaluateRoutes(rules, facts, routeTestCandidates(), nil)
	if got.Model != "kimi" {
		t.Fatalf("expected most specific rule (kimi), got %q", got.Model)
	}
}

func TestEvaluateRoutesTieBrokenByOrder(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding"}, Model: "gpt-5.4-mini"},
		{When: RouteCondition{Language: "go"}, Model: "kimi"},
	}
	facts := RouteFacts{Task: IntentCoding, Language: "go"}
	got := EvaluateRoutes(rules, facts, routeTestCandidates(), nil)
	// Both specificity 1; first wins.
	if got.Model != "gpt-5.4-mini" {
		t.Fatalf("expected first rule on tie, got %q", got.Model)
	}
}

func TestEvaluateRoutesCatchAll(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{}, Model: "claude-sonnet-5"},
	}
	got := EvaluateRoutes(rules, RouteFacts{Task: IntentDebug}, routeTestCandidates(), nil)
	if !got.Matched || got.Model != "claude-sonnet-5" {
		t.Fatalf("expected catch-all match, got %+v", got)
	}
	if got.Specificity != 0 {
		t.Fatalf("expected specificity 0 for catch-all, got %d", got.Specificity)
	}
}

func TestEvaluateRoutesNumericRanges(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{ComplexityMin: intPtr(75)}, Model: "claude-opus-4-8"},
	}
	if got := EvaluateRoutes(rules, RouteFacts{ComplexityScore: 80}, routeTestCandidates(), nil); !got.Matched {
		t.Fatalf("expected match for score >= 75")
	}
	if got := EvaluateRoutes(rules, RouteFacts{ComplexityScore: 40}, routeTestCandidates(), nil); got.Matched {
		t.Fatalf("expected no match for score < 75")
	}
}

func TestEvaluateRoutesMinFilesAndDiffAndStream(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{MinFiles: intPtr(3), HasDiff: boolPtr(true), Stream: boolPtr(false)}, Model: "claude-sonnet-5"},
	}
	match := RouteFacts{FileCount: 5, HasDiff: true, Stream: false}
	if got := EvaluateRoutes(rules, match, routeTestCandidates(), nil); !got.Matched {
		t.Fatalf("expected match, got %+v", got)
	}
	noMatch := RouteFacts{FileCount: 5, HasDiff: false, Stream: false}
	if got := EvaluateRoutes(rules, noMatch, routeTestCandidates(), nil); got.Matched {
		t.Fatalf("expected no match when has_diff false")
	}
}

func TestEvaluateRoutesSkipsInvalidModel(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding", Language: "go"}, Model: "nonexistent"}, // more specific but invalid
		{When: RouteCondition{Task: "coding"}, Model: "gpt-5.4-mini"},                // valid fallback
	}
	facts := RouteFacts{Task: IntentCoding, Language: "go"}
	got := EvaluateRoutes(rules, facts, routeTestCandidates(), nil)
	if !got.Matched || got.Model != "gpt-5.4-mini" {
		t.Fatalf("expected fallback to valid rule, got %+v", got)
	}
}

func TestEvaluateRoutesRespectsProviderAvailability(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding"}, Model: "claude-sonnet-5"},
	}
	facts := RouteFacts{Task: IntentCoding}
	// claude not available → rule cannot resolve → no match.
	got := EvaluateRoutes(rules, facts, routeTestCandidates(), []string{"codex"})
	if got.Matched {
		t.Fatalf("expected no match when provider unavailable, got %+v", got)
	}
}

func TestEvaluateRoutesProviderDisambiguation(t *testing.T) {
	candidates := []Candidate{
		{Provider: "codex", Model: "shared", Cost: "low", Quality: "medium"},
		{Provider: "claude", Model: "shared", Cost: "high", Quality: "high"},
	}
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding"}, Model: "shared", Provider: "claude"},
	}
	got := EvaluateRoutes(rules, RouteFacts{Task: IntentCoding}, candidates, nil)
	if got.Provider != "claude" {
		t.Fatalf("expected claude provider, got %q", got.Provider)
	}
}

func TestEvaluateRoutesEmptyRulesNoMatch(t *testing.T) {
	got := EvaluateRoutes(nil, RouteFacts{Task: IntentCoding}, routeTestCandidates(), nil)
	if got.Matched || got.RuleIndex != -1 {
		t.Fatalf("expected no match for empty rules, got %+v", got)
	}
}
