# ADR 0002: Routing Determinism, Preferences, and Cache Management

## Status

Accepted

## Context

`smart-model-router` initially routed through deterministic configuration and later gained optional LLM classification. Runtime testing showed several practical issues:

- The classifier was receiving the original request body and sometimes answered the user's prompt instead of returning a routing decision.
- The classifier used the full conversation history, causing prior messages to dominate current model selection.
- Repeated identical prompts could select different models when the classifier considered equivalent candidates such as `gpt-5.4` and `claude-sonnet-5`.
- The cache used whole request-body hashing, which reduced cache hits when surrounding history changed.
- The cache eviction behavior was effectively arbitrary because map iteration removed an unspecified entry.
- Users needed a simple configuration knob to influence cost-vs-quality tradeoffs without rewriting all model metadata.

## Decision

Implement the following routing decisions:

1. The classifier receives an isolated routing prompt instead of the original request body as a chat continuation.
2. The classifier prompt includes configured candidate metadata: provider, model, cost, quality, and capabilities.
3. The classifier uses only the extracted last user message for classification.
4. The route cache key uses the virtual model and extracted last user message, not the full request body.
5. Route cache eviction uses LRU with a bounded `cache.max_entries` size.
6. Route cache supports optional TTL through `cache.ttl`.
7. Runtime state writes are throttled to reduce full JSON rewrites.
8. Add `preference: cost|balanced|quality` as a routing influencer.
9. Apply `preference` in three places:
   - classifier prompt bias;
   - deterministic fallback ranking;
   - conservative post-classifier tiebreaking among equivalent-tier candidates.
10. Include `preference` in debug decision logs.

## Rationale

- An isolated classifier prompt keeps the classifier focused on model selection instead of task execution.
- Using only the last user message makes repeated prompts deterministic even when conversation history differs.
- Hashing the extracted prompt protects privacy while preserving useful cache hits.
- LRU keeps the JSON state file bounded and predictable without adding SQLite or another dependency.
- TTL gives deployments a way to re-evaluate stale decisions after model catalog or routing preferences change.
- `preference` is simpler and safer than adding numeric scoring weights. It covers the user-facing need: cheaper, balanced, or higher-quality behavior.
- Conservative tiebreaking avoids promoting trivial tasks to expensive models just because `preference: quality` is set.

## Implementation Summary

Implemented changes:

- Added `preference` to plugin config with default `balanced`.
- Added supported preferences: `cost`, `balanced`, `quality`.
- Added `preferenceInstruction` to bias classifier prompts.
- Added `ApplyPreferenceTiebreak` to enforce conservative tiebreaks.
- Updated deterministic fallback to honor preference.
- Changed route cache key to hash the extracted last user message plus virtual model.
- Added `LastUsed` to route cache entries.
- Added LRU eviction for route cache.
- Added `cache.ttl` support.
- Added throttled runtime state saving.
- Added debug log field `preference`.
- Added documentation for configuration, architecture, and business rules.

## Consequences

- Repeated identical prompts should route to the same model after the first selection.
- Cache size is bounded by `cache.max_entries`.
- Cache entries may be refreshed by TTL when configured.
- The classifier is more reliable because it is asked to classify, not answer.
- Preference influences model selection without becoming an unsafe hard override.
- No SQLite dependency was added; JSON state remains sufficient for the current cache size.

## Tradeoffs

- `preference` is an influencer, not an absolute command. This avoids extreme behavior but means it may not force every request to the cheapest or highest-quality model.
- `classifier.timeout` is currently normalized but not enforced by the host callback path.
- `benchmark` strategy still behaves like deterministic fallback; benchmark scoring remains future work.
- Several `routing` weights remain metadata for future scoring and are not currently used as active weighted calculations.

## Rejected Alternatives

### SQLite Cache

SQLite was considered for route cache persistence.

It was rejected for now because:

- current cache size is small and bounded;
- JSON with LRU and TTL is simpler;
- adding a database dependency would increase operational complexity;
- the plugin already writes only non-sensitive state.

SQLite may become appropriate if deployments need tens of thousands of cache entries, cross-process sharing, or advanced cache queries.

### Full Numeric Weighted Scoring

Numeric scoring based on benchmark, classifier, capability, latency, and cost weights was deferred.

It was rejected for now because current behavior only needs simple cost/balanced/quality influence, and unproven weights would add complexity without clear runtime benefit.

### Provider Locking for `claude-auto`

Restricting `claude-auto` to Claude models was considered.

It was not implemented because the chosen behavior is provider-agnostic optimization: the classifier may choose between Claude and GPT/Codex candidates based on cost and quality.

## Follow-Up Work

- Enforce classifier timeout if CLIProxyAPI host callback support allows it.
- Implement real benchmark scoring only when measured runtime data is available.
- Consider SQLite only if the bounded JSON cache becomes insufficient.
- Consider documenting provider-specific virtual models if deployments want `claude-auto` to prefer or restrict Claude candidates.
