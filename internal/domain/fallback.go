package domain

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultCooldown is the fallback cooldown duration when no Retry-After is specified.
	DefaultCooldown = 30 * time.Second
	// MaxCooldown bounds the maximum cooldown duration.
	MaxCooldown = 10 * time.Minute
	// MinCooldown bounds the minimum cooldown duration.
	MinCooldown = 1 * time.Second
)

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
		if containsAny(lowerErr, "token limit", "max tokens", "maximum tokens", "too many tokens", "rate limit", "rate_limit", "quota") {
			return FailureTokenLimit
		}
		if containsAny(lowerErr, "unavailable", "no healthy", "connection refused", "no such host", "overloaded", "model_cooldown", "cooling down", "cooling_down", "cooldown", "auth_unavailable", "auth unavailable") {
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

// FallbackKey builds the dedupe key for a provider/model pair.
func FallbackKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.TrimSpace(model)
}

// fallbackKey builds the dedupe key for a provider/model pair.
func fallbackKey(provider, model string) string {
	return FallbackKey(provider, model)
}

// CandidateTarget represents a provider/model target for fallback ordering.
type CandidateTarget struct {
	Provider string
	Model    string
}

// OrderCandidatesByCooldown orders healthy targets before cooling targets,
// sorting cooling targets by earliest expiry while preserving original relative
// order for healthy targets and ties, then truncating to maxAttempts.
func OrderCandidatesByCooldown(
	targets []CandidateTarget,
	cooldownExpiry func(provider, model string) (time.Time, bool),
	maxAttempts int,
) []CandidateTarget {
	if len(targets) == 0 {
		return nil
	}
	type coolingTarget struct {
		target CandidateTarget
		until  time.Time
		index  int
	}

	healthy := make([]CandidateTarget, 0, len(targets))
	cooling := make([]coolingTarget, 0, len(targets))

	for i, t := range targets {
		if until, isCooling := cooldownExpiry(t.Provider, t.Model); isCooling {
			cooling = append(cooling, coolingTarget{target: t, until: until, index: i})
		} else {
			healthy = append(healthy, t)
		}
	}

	sort.SliceStable(cooling, func(i, j int) bool {
		if cooling[i].until.Equal(cooling[j].until) {
			return cooling[i].index < cooling[j].index
		}
		return cooling[i].until.Before(cooling[j].until)
	})

	ordered := make([]CandidateTarget, 0, len(targets))
	ordered = append(ordered, healthy...)
	for _, c := range cooling {
		ordered = append(ordered, c.target)
	}

	if maxAttempts > 0 && len(ordered) > maxAttempts {
		ordered = ordered[:maxAttempts]
	}
	return ordered
}

// ParseRetryAfter parses an HTTP Retry-After header value, which may be either
// a delta in seconds (e.g. "120") or an HTTP-date.
func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(v, 10, 64); err == nil {
		if seconds <= 0 {
			return 0, false
		}
		// Saturate before multiplication so untrusted delta-seconds cannot wrap.
		if seconds > int64((1<<63-1)/time.Second) {
			return time.Duration(1<<63 - 1), true
		}
		return time.Duration(seconds) * time.Second, true
	}
	for _, layout := range []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC850,
		time.ANSIC,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, v); err == nil {
			diff := t.Sub(now)
			if diff <= 0 {
				return 0, false
			}
			return diff, true
		}
	}
	return 0, false
}

// CalculateCooldown determines the cooldown duration from a Retry-After header,
// applying default short cooldown and bounding between MinCooldown and MaxCooldown.
func CalculateCooldown(retryAfterHeader string, now time.Time) time.Duration {
	if d, ok := ParseRetryAfter(retryAfterHeader, now); ok {
		if d > MaxCooldown {
			return MaxCooldown
		}
		if d < MinCooldown {
			return MinCooldown
		}
		return d
	}
	return DefaultCooldown
}

// IsRetryableRPCError reports whether an RPC error from the host is retryable.
// Retryable errors include model_cooldown, cooling down, quota/rate limit,
// auth_unavailable, timeouts, and availability errors. Nonretryable errors
// (e.g. invalid requests, unknown methods, arbitrary client errors) must fail fast.
func IsRetryableRPCError(errText string) bool {
	if strings.TrimSpace(errText) == "" {
		return false
	}
	lower := strings.ToLower(errText)
	patterns := []string{
		"model_cooldown",
		"cooling down",
		"cooling_down",
		"cooldown",
		"quota",
		"rate limit",
		"rate_limit",
		"rate-limit",
		"too many requests",
		"auth_unavailable",
		"auth unavailable",
		"timeout",
		"timed out",
		"deadline exceeded",
		"unavailable",
		"overloaded",
		"no healthy",
		"connection refused",
		"connection reset",
		"no such host",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// IsCooldownError reports whether an error or status indicates the target should
// be placed into cooldown (cooldown notice, quota, rate limit, auth unavailable).
func IsCooldownError(errText string) bool {
	if strings.TrimSpace(errText) == "" {
		return false
	}
	lower := strings.ToLower(errText)
	patterns := []string{
		"model_cooldown",
		"cooling down",
		"cooling_down",
		"cooldown",
		"quota",
		"rate limit",
		"rate_limit",
		"rate-limit",
		"too many requests",
		"auth_unavailable",
		"auth unavailable",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
