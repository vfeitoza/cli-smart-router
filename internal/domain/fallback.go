package domain

import "strings"

// FailureReason classifies why an attempt against a model failed. The Fallback
// Engine reacts to any of these by moving to the next policy-allowed model.
type FailureReason string

const (
	// FailureNone means the attempt did not fail.
	FailureNone FailureReason = ""
	// FailureTimeout means the model call timed out.
	FailureTimeout FailureReason = "timeout"
	// FailureHTTPError means the model returned a non-2xx HTTP status.
	FailureHTTPError FailureReason = "http_error"
	// FailureContextExceeded means the request exceeded the model context window.
	FailureContextExceeded FailureReason = "context_exceeded"
	// FailureTokenLimit means the request hit a token limit.
	FailureTokenLimit FailureReason = "token_limit"
	// FailureUnavailable means the model/provider was unavailable.
	FailureUnavailable FailureReason = "unavailable"
)

// AttemptOutcome describes the result of one model attempt for classification. It
// is pure data so the domain can classify failures without importing host types.
type AttemptOutcome struct {
	TimedOut   bool   // transport/deadline timeout
	StatusCode int    // HTTP status returned (0 when none)
	Err        string // error text, used for heuristic classification
}

// Fallbackable reports whether the outcome is a failure the Fallback Engine should
// react to by trying the next model.
func (r FailureReason) Fallbackable() bool {
	return r != FailureNone
}

// ClassifyFailure maps an attempt outcome to a FailureReason. It checks, in order:
// timeout, context-exceeded, token-limit, unavailability, then any other non-2xx
// HTTP status as a generic HTTP error. A 2xx status with no timeout/error is
// FailureNone (success).
func ClassifyFailure(outcome AttemptOutcome) FailureReason {
	if outcome.TimedOut {
		return FailureTimeout
	}
	lowerErr := strings.ToLower(outcome.Err)
	if outcome.Err != "" {
		if containsAny(lowerErr, "timeout", "timed out", "deadline exceeded") {
			return FailureTimeout
		}
		if containsAny(lowerErr, "context length", "context window", "maximum context", "too long", "context exceeded") {
			return FailureContextExceeded
		}
		if containsAny(lowerErr, "token limit", "max tokens", "maximum tokens", "too many tokens") {
			return FailureTokenLimit
		}
		if containsAny(lowerErr, "unavailable", "no healthy", "connection refused", "no such host", "overloaded") {
			return FailureUnavailable
		}
	}
	if isContextStatus(outcome.StatusCode, lowerErr) {
		return FailureContextExceeded
	}
	switch outcome.StatusCode {
	case 429:
		return FailureTokenLimit
	case 408, 504:
		return FailureTimeout
	case 502, 503:
		return FailureUnavailable
	}
	if outcome.StatusCode != 0 && (outcome.StatusCode < 200 || outcome.StatusCode >= 300) {
		return FailureHTTPError
	}
	if outcome.Err != "" {
		return FailureHTTPError
	}
	return FailureNone
}

// isContextStatus detects context-window rejections that some providers surface as
// a 400 with a descriptive body.
func isContextStatus(status int, lowerErr string) bool {
	if status != 400 {
		return false
	}
	return containsAny(lowerErr, "context", "too long", "token")
}

// FallbackChain is the ordered list of policy-allowed targets to try, produced by
// EvaluateRoutesRanked. The Fallback Engine walks it, skipping already-failed
// models, to pick the next allowed model after a failure.
type FallbackChain []PolicyDecision

// FallbackSelection is the Fallback Engine result: the next model to try (if any)
// and the reason that triggered the fallback.
type FallbackSelection struct {
	HasNext  bool
	Decision PolicyDecision
	Reason   FailureReason
	Attempt  int // 0-based index of the selected target within the chain
}

// SelectNext picks the next policy-allowed target after a failure. It returns the
// first chain entry whose provider/model has not already failed. When the outcome
// is not a failure, or no allowed model remains, HasNext is false.
//
// This is the whole behavior requested: on timeout, HTTP error, context exceeded,
// token limit, or unavailability, automatically select the next model permitted by
// the policy chain. The chain itself comes from the Policy Engine, so the Fallback
// Engine never picks a model the policy did not allow.
func SelectNext(chain FallbackChain, failed map[string]struct{}, outcome AttemptOutcome) FallbackSelection {
	reason := ClassifyFailure(outcome)
	if !reason.Fallbackable() {
		return FallbackSelection{HasNext: false, Reason: reason}
	}
	for index, decision := range chain {
		key := fallbackKey(decision.Provider, decision.Model)
		if _, done := failed[key]; done {
			continue
		}
		return FallbackSelection{HasNext: true, Decision: decision, Reason: reason, Attempt: index}
	}
	return FallbackSelection{HasNext: false, Reason: reason}
}

// MarkFailed records a provider/model as failed so SelectNext skips it next time.
func MarkFailed(failed map[string]struct{}, provider, model string) {
	if failed == nil {
		return
	}
	failed[fallbackKey(provider, model)] = struct{}{}
}

// fallbackKey builds the dedupe key for a provider/model pair.
func fallbackKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.TrimSpace(model)
}
