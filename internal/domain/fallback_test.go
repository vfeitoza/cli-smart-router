package domain

import "testing"

func TestClassifyFailureReasons(t *testing.T) {
	cases := []struct {
		name    string
		outcome AttemptOutcome
		want    FailureReason
	}{
		{"timeout flag", AttemptOutcome{TimedOut: true}, FailureTimeout},
		{"timeout err", AttemptOutcome{Err: "context deadline exceeded"}, FailureTimeout},
		{"context window err", AttemptOutcome{Err: "maximum context length exceeded"}, FailureContextExceeded},
		{"token limit err", AttemptOutcome{Err: "max tokens reached"}, FailureTokenLimit},
		{"unavailable err", AttemptOutcome{Err: "upstream connection refused"}, FailureUnavailable},
		{"429 token", AttemptOutcome{StatusCode: 429}, FailureTokenLimit},
		{"408 timeout", AttemptOutcome{StatusCode: 408}, FailureTimeout},
		{"504 timeout", AttemptOutcome{StatusCode: 504}, FailureTimeout},
		{"503 unavailable", AttemptOutcome{StatusCode: 503}, FailureUnavailable},
		{"502 unavailable", AttemptOutcome{StatusCode: 502}, FailureUnavailable},
		{"400 context", AttemptOutcome{StatusCode: 400, Err: "prompt too long for context"}, FailureContextExceeded},
		{"500 http error", AttemptOutcome{StatusCode: 500}, FailureHTTPError},
		{"401 http error", AttemptOutcome{StatusCode: 401}, FailureHTTPError},
		{"success", AttemptOutcome{StatusCode: 200}, FailureNone},
		{"no signal", AttemptOutcome{}, FailureNone},
	}
	for _, c := range cases {
		if got := ClassifyFailure(c.outcome); got != c.want {
			t.Fatalf("%s: expected %q, got %q", c.name, c.want, got)
		}
	}
}

func TestFallbackableReason(t *testing.T) {
	if FailureNone.Fallbackable() {
		t.Fatalf("none must not be fallbackable")
	}
	for _, r := range []FailureReason{FailureTimeout, FailureHTTPError, FailureContextExceeded, FailureTokenLimit, FailureUnavailable} {
		if !r.Fallbackable() {
			t.Fatalf("%q must be fallbackable", r)
		}
	}
}

func fallbackTestChain() FallbackChain {
	return FallbackChain{
		{Matched: true, Provider: "codex", Model: "gpt-5.4-mini"},
		{Matched: true, Provider: "claude", Model: "claude-sonnet-5"},
		{Matched: true, Provider: "claude", Model: "claude-opus-4-8"},
	}
}

func TestSelectNextPicksFirstUnfailed(t *testing.T) {
	failed := map[string]struct{}{}
	MarkFailed(failed, "codex", "gpt-5.4-mini")
	got := SelectNext(fallbackTestChain(), failed, AttemptOutcome{StatusCode: 500})
	if !got.HasNext {
		t.Fatalf("expected a next model")
	}
	if got.Decision.Model != "claude-sonnet-5" {
		t.Fatalf("expected claude-sonnet-5, got %q", got.Decision.Model)
	}
	if got.Reason != FailureHTTPError {
		t.Fatalf("expected http_error reason, got %q", got.Reason)
	}
	if got.Attempt != 1 {
		t.Fatalf("expected attempt index 1, got %d", got.Attempt)
	}
}

func TestSelectNextNoFallbackOnSuccess(t *testing.T) {
	got := SelectNext(fallbackTestChain(), map[string]struct{}{}, AttemptOutcome{StatusCode: 200})
	if got.HasNext {
		t.Fatalf("expected no fallback on success")
	}
	if got.Reason != FailureNone {
		t.Fatalf("expected none reason, got %q", got.Reason)
	}
}

func TestSelectNextExhaustedChain(t *testing.T) {
	failed := map[string]struct{}{}
	for _, d := range fallbackTestChain() {
		MarkFailed(failed, d.Provider, d.Model)
	}
	got := SelectNext(fallbackTestChain(), failed, AttemptOutcome{TimedOut: true})
	if got.HasNext {
		t.Fatalf("expected exhausted chain to have no next")
	}
	if got.Reason != FailureTimeout {
		t.Fatalf("expected timeout reason, got %q", got.Reason)
	}
}

func TestSelectNextWalksEntireChain(t *testing.T) {
	chain := fallbackTestChain()
	failed := map[string]struct{}{}
	order := []string{}
	// Simulate: every model fails with a different reason; collect the walk order.
	for i := 0; i < len(chain)+1; i++ {
		sel := SelectNext(chain, failed, AttemptOutcome{StatusCode: 503})
		if !sel.HasNext {
			break
		}
		order = append(order, sel.Decision.Model)
		MarkFailed(failed, sel.Decision.Provider, sel.Decision.Model)
	}
	want := []string{"gpt-5.4-mini", "claude-sonnet-5", "claude-opus-4-8"}
	if len(order) != len(want) {
		t.Fatalf("expected %d attempts, got %d (%v)", len(want), len(order), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("attempt %d: expected %q, got %q", i, want[i], order[i])
		}
	}
}

func TestEvaluateRoutesRankedOrdersBySpecificity(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding"}, Model: "gpt-5.4-mini"},                    // spec 1
		{When: RouteCondition{Task: "coding", Language: "go"}, Model: "claude-sonnet-5"}, // spec 2
		{When: RouteCondition{}, Model: "claude-opus-4-8"},                               // catch-all spec 0
	}
	facts := RouteFacts{Task: IntentCoding, Language: "go"}
	chain := EvaluateRoutesRanked(rules, facts, routeTestCandidates(), nil)
	if len(chain) != 3 {
		t.Fatalf("expected 3 chain entries, got %d", len(chain))
	}
	if chain[0].Model != "claude-sonnet-5" || chain[1].Model != "gpt-5.4-mini" || chain[2].Model != "claude-opus-4-8" {
		t.Fatalf("unexpected chain order: %v", chain)
	}
}

func TestEvaluateRoutesRankedDeduplicates(t *testing.T) {
	rules := []RouteRule{
		{When: RouteCondition{Task: "coding", Language: "go"}, Model: "gpt-5.4-mini"},
		{When: RouteCondition{Task: "coding"}, Model: "gpt-5.4-mini"}, // same target, less specific
	}
	facts := RouteFacts{Task: IntentCoding, Language: "go"}
	chain := EvaluateRoutesRanked(rules, facts, routeTestCandidates(), nil)
	if len(chain) != 1 {
		t.Fatalf("expected deduped chain of 1, got %d", len(chain))
	}
}

func TestEvaluateRoutesRankedEmptyWhenNoMatch(t *testing.T) {
	rules := []RouteRule{{When: RouteCondition{Language: "rust"}, Model: "kimi"}}
	chain := EvaluateRoutesRanked(rules, RouteFacts{Task: IntentCoding, Language: "go"}, routeTestCandidates(), nil)
	if len(chain) != 0 {
		t.Fatalf("expected empty chain, got %d", len(chain))
	}
}
