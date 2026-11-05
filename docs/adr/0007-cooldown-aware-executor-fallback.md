# ADR 0007: Cooldown-Aware Executor Fallback

## Status

Proposed

## Context

The optional non-streaming executor can receive host RPC errors without an HTTP response. Treating those errors as empty responses can produce false success. Repeatedly attempting a quota-exhausted primary also wastes the limited candidate budget.

## Decision

Classify retryable host RPC errors explicitly. Advance within the eligible candidate chain for transient failures; return an error immediately for non-retryable RPC failures. Track quota, cooldown, and unavailable-auth failures in a mutex-protected, bounded, ephemeral provider/model map.

Use a 30-second default cooldown. Accept `Retry-After` delta seconds or HTTP dates, saturate integer overflow, and bound durations to 1 second through 10 minutes. Order healthy targets first and cooling targets by earliest recovery, then apply `max_attempts`. Clear target cooldown after success.

Preserve cached policy-chain membership. An explicit execution session with no cached chain fails closed; requests without an execution session retain configured-model fallback.

## Consequences

- Exhausted targets no longer consume the attempt budget ahead of healthy targets.
- Cooldowns are advisory and process-local; cooling targets remain available as recovery probes when necessary.
- The host retains authentication and provider execution responsibility.
- Streaming, the native ABI, and configuration defaults remain unchanged.
- No new dependencies, persistent cooldown records, prompts, or credentials are introduced.
