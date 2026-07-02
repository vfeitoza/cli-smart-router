# Smart Model Router Business Rules

This document describes the business rules implemented by `smart-model-router`.

The rules below describe product behavior, not low-level implementation details.

## Core Rule: Route Only the Configured Virtual Model

The plugin only handles requests whose requested model exactly matches `virtual_model`.

If the request model does not match `virtual_model`, the plugin must return `Handled: false`.

Default virtual model:

```yaml
virtual_model: router:auto
```

A deployment may use a different alias, for example:

```yaml
virtual_model: claude-auto
```

## Provider Execution Rule

The default routing path must return a provider route:

```text
TargetKind: provider
Target: <provider>
TargetModel: <model>
```

This keeps provider authentication, request execution, retries, logging, streaming, and usage accounting inside CLIProxyAPI.

The plugin should not execute the final user request itself unless `executor_fallback.enabled` is explicitly enabled.

## Authoritative Model Metadata Rule

The configured `models` list is authoritative for:

- provider mapping
- model ID
- capabilities
- cost tier
- quality tier

`/v1/models` may be used to validate or enrich availability state, but it must not be used to infer provider or capability metadata for unmapped models.

## Candidate Validity Rule

A candidate model is routable only when it has at least:

- non-empty `provider`
- non-empty `model`

If CLIProxyAPI provides an available provider list, the candidate provider must be present in that list.

## Strategy Rule

Supported strategy values are:

- `capability`
- `benchmark`
- `llm`
- `hybrid`

Current behavior:

- `capability`: deterministic fallback only.
- `benchmark`: currently behaves like deterministic fallback; benchmark fields are reserved for future scoring.
- `llm`: classifier first, deterministic fallback on classifier failure.
- `hybrid`: same operational behavior as `llm` today.

Every strategy must keep deterministic fallback available so the router remains operational when advanced features fail.

## Preference Rule

Supported preference values are:
- `cost`
- `balanced`
- `quality`

Default: `balanced`

Invalid or empty values must normalize to `balanced`.

Preference influences decisions but does not blindly override task complexity.

Business meaning:

- `cost`: choose the cheapest acceptable model.
- `balanced`: balance cost and quality.
- `quality`: choose the highest-quality model that is appropriate for the task.

## Classifier Rule

When `strategy` is `llm` or `hybrid` and `classifier.enabled` is true, the plugin should attempt semantic classification before deterministic fallback.

The classifier must choose a model from the configured `models` catalog.

Classifier models are tried in configured order up to `classifier.max_attempts`.

The classifier must receive an isolated routing prompt, not the original full request body as a chat continuation.

The classifier receives:

- configured provider/model catalog
- model capabilities
- cost tiers
- quality tiers
- current `preference`
- extracted last user message

The classifier is expected to return compact JSON:

```json
{"selected_model":"<id>","confidence":0.9,"reason":"short reason"}
```

## Classifier Failure Rule

Classifier output must be rejected when:

- the host callback fails
- the classifier response is non-2xx
- the classifier response is not valid JSON or does not contain a JSON object
- `selected_model` is empty
- `selected_model` is not present in configured `models`
- selected provider is unavailable

If one classifier attempt fails, the next configured classifier may be tried.

If all classifier attempts fail, deterministic fallback must be used.

## Last User Message Rule

Routing classification and cache keys must use the extracted last user message, not the full conversation history.

Reason:

- The same user prompt should route deterministically even when prior conversation history differs.
- Historical context from tools or previous messages should not dominate model selection for the current request.

The plugin must support common request body shapes such as OpenAI/Claude `messages` and Gemini `contents.parts`.

## Route Cache Rule

When `cache.enabled` is true, repeated identical prompts should reuse the same route decision.

Cache key material:

- configured virtual model
- extracted last user message

The raw prompt must not be persisted. Only a hash is used as the route cache key.

Cache entries must be bounded by `cache.max_entries`.

When full, the least recently used entry must be evicted.

If `cache.ttl` is set, entries older than the TTL must be re-evaluated.

## Session Affinity Rule

When `routing.keep_same_model_per_session` is true, a known session route should be reused before cache or classifier selection.

Session ID source:

```text
metadata.execution_session_id
```

Session routes are non-sensitive because they store only provider/model/reason metadata.

## Decision Priority Rule

For matching virtual-model requests, route selection order is:

1. Existing session route.
2. Existing cache route.
3. Classifier route for `llm` or `hybrid`.
4. Deterministic fallback.

Newly selected routes should be cached when cache is enabled and pinned to the session when session affinity is enabled.

## Deterministic Fallback Rule

Deterministic fallback chooses from configured candidates without calling any LLM.

It must:

- filter invalid candidates
- respect available providers
- estimate the minimum required cost tier from simple prompt signals
- apply `preference`

Current prompt signals:

- Simple arithmetic/classification/summary requests can use `low` cost models.
- Prompts containing `detalhes`, `como funciona`, `explique`, `código`, or `codigo` require at least `high` cost models.

Ranking by preference:

- `quality`: highest quality first, cost as secondary tie-break.
- `cost`: lowest cost first, quality as secondary tie-break.
- `balanced`: lowest cost first, quality as secondary tie-break.

## Preference Tiebreak Rule

After classifier selection, preference may apply a conservative tiebreak among equivalent-tier candidates.

Rules:

- `balanced`: do not alter the classifier pick.
- `quality`: within the same cost tier as the classifier pick, prefer the highest quality.
- `cost`: within the same quality tier as the classifier pick, prefer the cheapest.

The tiebreak must not cross tiers because it should not override the classifier's complexity judgment.

When a tiebreak changes the model, the route reason must show the change:

```text
preference_tiebreak:old-model->new-model
```

## Cost Tier Rule

Recognized cost tiers:

- `low`
- `medium`
- `high`
- `very_high`

Unknown or empty cost values rank as `medium`.

## Quality Tier Rule

Recognized quality tiers:

- `low`
- `medium`
- `high`
- `highest`

Unknown or empty quality values rank as `medium`.

## Debug Logging Rule

When debug logging is enabled, the plugin writes one JSON line per routed request.

Logs may include:

- timestamp
- decision source
- virtual model
- source format
- stream flag
- strategy
- preference
- target provider/model
- reason
- classifier trace

Logs must not include:

- prompts
- full request bodies
- credentials
- API keys
- auth records
- response bodies

## Runtime State Rule

Runtime state may persist only non-sensitive data:

- catalog IDs
- pricing metadata
- aggregate usage
- route cache
- session routes
- counters

Runtime state must not persist:

- prompts
- request bodies
- response bodies
- credentials
- API keys
- auth records

## Pricing Rule

Pricing metadata fetches are optional.

If pricing refresh fails, routing must continue.

Current routing does not enforce `routing.max_cost_per_request` as a hard limit.

## Catalog Rule

Catalog refresh from `/v1/models` is optional.

If catalog refresh fails, routing must continue.

Configured `models` remain authoritative even when catalog data is available.

## Executor Fallback Rule

Executor fallback is opt-in.

When disabled, the plugin returns provider routes.

When enabled, non-streaming requests may be routed to the plugin itself for same-request fallback attempts.

Streaming executor fallback is not implemented.

## Management Rule

The plugin exposes a management status route:

```text
GET /plugins/smart-model-router/status
```

It reports non-sensitive status, usage snapshots, and runtime state snapshots.

## Safety Rule

The router must remain useful even when optional features fail.

Failures in classifier, catalog, pricing, cache lookup, or state persistence must not prevent deterministic routing unless no valid configured candidate exists.
