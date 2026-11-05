package domain

import (
	"testing"
	"time"
)

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

func TestOrderCandidatesByCooldown(t *testing.T) {
	now := time.Now()
	targets := []CandidateTarget{
		{Provider: "p1", Model: "m1"},
		{Provider: "p2", Model: "m2"},
		{Provider: "p3", Model: "m3"},
	}

	// 1. All healthy: preserved order, capped by maxAttempts
	got := OrderCandidatesByCooldown(targets, func(provider, model string) (time.Time, bool) {
		return time.Time{}, false
	}, 2)
	if len(got) != 2 || got[0].Model != "m1" || got[1].Model != "m2" {
		t.Fatalf("expected [m1, m2], got %+v", got)
	}

	// 2. Primary m1 cooling: m2 and m3 are healthy, so m3 beyond maxAttempts (pos 2) becomes reachable
	cooldowns := map[string]time.Time{
		"p1/m1": now.Add(30 * time.Second),
	}
	got = OrderCandidatesByCooldown(targets, func(provider, model string) (time.Time, bool) {
		until, ok := cooldowns[FallbackKey(provider, model)]
		return until, ok
	}, 2)
	if len(got) != 2 || got[0].Model != "m2" || got[1].Model != "m3" {
		t.Fatalf("expected healthy beyond max_attempts to be reachable: want [m2, m3], got %+v", got)
	}

	// 3. Multiple cooling targets: ordered by earliest expiry
	cooldowns = map[string]time.Time{
		"p1/m1": now.Add(40 * time.Second),
		"p2/m2": now.Add(10 * time.Second),
	}
	got = OrderCandidatesByCooldown(targets, func(provider, model string) (time.Time, bool) {
		until, ok := cooldowns[FallbackKey(provider, model)]
		return until, ok
	}, 3)
	if len(got) != 3 || got[0].Model != "m3" || got[1].Model != "m2" || got[2].Model != "m1" {
		t.Fatalf("expected healthy m3 first, then m2 (earliest), then m1: got %+v", got)
	}
}

func TestParseRetryAfterAndCalculateCooldown(t *testing.T) {
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	// Delta seconds
	dur, ok := ParseRetryAfter("60", now)
	if !ok || dur != 60*time.Second {
		t.Fatalf("expected 60s, got %v ok=%v", dur, ok)
	}

	// HTTP-date RFC1123
	dateStr := now.Add(45 * time.Second).Format(time.RFC1123)
	dur, ok = ParseRetryAfter(dateStr, now)
	if !ok || dur != 45*time.Second {
		t.Fatalf("expected 45s, got %v ok=%v", dur, ok)
	}

	// Invalid / empty
	if _, ok := ParseRetryAfter("", now); ok {
		t.Fatal("expected empty to fail")
	}
	if _, ok := ParseRetryAfter("not-a-date", now); ok {
		t.Fatal("expected invalid to fail")
	}

	// CalculateCooldown: default when no header
	if got := CalculateCooldown("", now); got != DefaultCooldown {
		t.Fatalf("expected DefaultCooldown %v, got %v", DefaultCooldown, got)
	}

	// CalculateCooldown: capped to MaxCooldown
	if got := CalculateCooldown("999999", now); got != MaxCooldown {
		t.Fatalf("expected MaxCooldown %v, got %v", MaxCooldown, got)
	}
}

func TestRetryAfterOverflowSaturates(t *testing.T) {
	now := time.Now()
	for _, value := range []string{"9223372036854775807", "18446744074"} {
		duration, ok := ParseRetryAfter(value, now)
		if !ok || duration <= 0 {
			t.Fatalf("Retry-After %q wrapped: %v, %v", value, duration, ok)
		}
		if got := CalculateCooldown(value, now); got != MaxCooldown {
			t.Fatalf("Retry-After %q: got %v, want %v", value, got, MaxCooldown)
		}
	}
}

func TestIsRetryableRPCErrorAndIsCooldownError(t *testing.T) {
	retryableCases := []string{
		"model_cooldown: claude is cooling down",
		"model is cooling down",
		"rate_limit_exceeded: too many requests",
		"quota_exceeded: insufficient quota",
		"auth_unavailable: no token",
		"context deadline exceeded",
		"upstream connection refused",
		"server overloaded",
	}
	for _, text := range retryableCases {
		if !IsRetryableRPCError(text) {
			t.Fatalf("expected %q to be retryable", text)
		}
	}

	nonretryableCases := []string{
		"invalid_argument: bad model",
		"unknown_method: foo",
		"unsupported_protocol: grpc",
		"bad_request: prompt cannot be empty",
	}
	for _, text := range nonretryableCases {
		if IsRetryableRPCError(text) {
			t.Fatalf("expected %q to be nonretryable", text)
		}
	}

	if !IsCooldownError("model_cooldown: cooling down") {
		t.Fatal("expected cooldown error")
	}
	if !IsCooldownError("rate limit reached") {
		t.Fatal("expected rate limit cooldown error")
	}
	if IsCooldownError("upstream connection refused") {
		t.Fatal("connection refused should not be a rate-limit/quota cooldown error")
	}
}
