# Smart Model Router Configuration

This document describes the `smart-model-router` plugin configuration shown in `configs/smart-model-router.yaml`.

The plugin is configured under the CLIProxyAPI plugin section:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    smart-model-router:
      enabled: true
      virtual_model: router:auto
      strategy: hybrid
      preference: balanced
```

## Overview

`smart-model-router` registers one virtual model and routes requests for that virtual model to one configured real provider/model.

The default route target is `TargetKind: provider`, so CLIProxyAPI keeps responsibility for provider authentication, request execution, logging, retries, and usage accounting.

The configured `models` list is authoritative for provider, cost, quality, and capability metadata. `/v1/models` is only used to validate or enrich availability state; it is not used to infer provider mappings.

## Top-Level CLIProxyAPI Plugin Fields

### `plugins.enabled`

Type: `bool`

Enables CLIProxyAPI plugin loading globally. If this is `false`, no plugin is loaded.

Example:

```yaml
plugins:
  enabled: true
```

### `plugins.dir`

Type: `string`

Directory where the native plugin shared library is installed.

Expected plugin artifact names depend on the platform:

- Linux: `smart-model-router.so`
- macOS: `smart-model-router.dylib`
- Windows: `smart-model-router.dll`

Example:

```yaml
plugins:
  dir: plugins
```

### `plugins.configs.smart-model-router`

This block contains plugin-owned configuration. The key must match the plugin identifier / dynamic library name: `smart-model-router`.

## Core Plugin Fields

### `enabled`

Type: `bool`

Enables this plugin instance.

Default in code: `true`

Example:

```yaml
enabled: true
```

### `priority`

Type: `int`

Plugin priority is handled by CLIProxyAPI. Higher-priority routers run first. Keep this high when the virtual model should be routed before other routers.

Example:

```yaml
priority: 100
```

### `virtual_model`

Type: `string`

The model name clients should request. The router only handles requests where the requested model exactly matches this value. All other model requests return `Handled: false` and are left to CLIProxyAPI or other routers.

Default in code: `router:auto`

Examples:

```yaml
virtual_model: router:auto
```

```yaml
virtual_model: claude-auto
```

### `strategy`

Type: `string`

Available values exposed by plugin metadata:

- `capability`
- `benchmark`
- `llm`
- `hybrid`

Default in code: `capability`

Current behavior:

- `capability`: uses the local capability-aware deterministic decision only. No classifier call.
- `benchmark`: currently uses the same local deterministic decision path; benchmark-specific scoring fields are kept as policy metadata for future scoring.
- `llm`: tries the configured classifier models first on every request, then falls back to the local deterministic decision if classification fails.
- `hybrid`: local-first. Computes the local deterministic decision first (no host call); if that decision is confident (the prompt matched a capability declared by the winning candidate; see `models[].capabilities` below), it is used directly and the classifier is skipped. Otherwise, behaves like `llm` for that request: classifier first, local decision as fallback.

The local deterministic decision never calls a classifier and keeps the router operational if the classifier fails.

Example:

```yaml
strategy: hybrid
```

### `preference`

Type: `string`

Available values:

- `cost`
- `balanced`
- `quality`

Default in code: `balanced`

Invalid or empty values are normalized to `balanced`.

This field influences model selection in three places:

- In local capability-aware scoring (used by every strategy), it ranks the pool of candidates gated by inferred capabilities.
- In `llm`, and in `hybrid` when the local decision is not confident, it biases the classifier prompt.
- In all modes, it breaks real ties among equivalent-tier models after classifier selection.

Behavior:

- `cost`: prefer the cheapest acceptable model. In tiebreaks, if models have the same quality tier, choose the lower-cost model.
- `balanced`: balance cost and quality. Current post-classifier tiebreak is a no-op; deterministic fallback prefers cheaper eligible models and uses quality as a secondary tie-break.
- `quality`: prefer the highest-quality model. In tiebreaks, if models have the same cost tier, choose the higher-quality model.

The preference does not blindly override task complexity. For example, `quality` should not force every trivial arithmetic request to the most expensive model.

Example:

```yaml
preference: balanced
```

### `state_path`

Type: `string`

Path to the non-sensitive runtime state file.

The state may contain:

- catalog model IDs
- pricing fetch metadata
- usage aggregates
- route cache entries
- session route pins
- counters

The state must not contain prompts, request bodies, credentials, API keys, auth records, or response bodies.

When empty, state loading/saving is skipped.

Example:

```yaml
state_path: smart-model-router-state.json
```

## `debug`

Controls JSONL route decision logs.

Example:

```yaml
debug:
  enabled: true
  log_path: smart-model-router-decisions.jsonl
```

### `debug.enabled`

Type: `bool`

Enables route decision logging.

### `debug.log_path`

Type: `string`

Path to the JSON Lines log file. If empty, debug logging is skipped.

Each log line includes non-sensitive route metadata such as:

- time
- source: `selected`, `cache`, or `session`
- virtual model
- source format
- stream flag
- strategy
- preference
- target provider/model
- route reason
- classifier trace when a classifier was attempted

Debug logs intentionally do not include prompts, request bodies, credentials, API keys, auth records, or response bodies.

## `catalog`

Controls live catalog refresh from CLIProxyAPI `/v1/models`.

Example:

```yaml
catalog:
  source: cli_proxy_api
  base_url: http://127.0.0.1:8317
  api_key: ""
  refresh_interval: 10m
  include_router_model: false
```

### `catalog.source`

Type: `string`

Informational source label. The current implementation expects CLIProxyAPI-compatible behavior when `base_url` is configured.

Example:

```yaml
source: cli_proxy_api
```

### `catalog.base_url`

Type: `string`

Base URL used to fetch `GET /v1/models` through the host HTTP callback.

If empty, catalog refresh is skipped.

### `catalog.api_key`

Type: `string`

Optional API key used as `Authorization: Bearer <api_key>` when fetching the catalog.

Do not commit real keys.

### `catalog.refresh_interval`

Type: Go duration string, for example `10m`, `1h`, `30s`

Controls how often the catalog is refreshed.

If invalid or empty, the runtime fallback is `10m`.

### `catalog.include_router_model`

Type: `bool`

When `false`, the configured virtual model is removed from the stored catalog model list.

## `pricing`

Controls external pricing metadata fetches.

Example:

```yaml
pricing:
  enabled: true
  url: https://raw.githubusercontent.com/ENTERPILOT/ai-model-price-list/refs/heads/main/sources/llm_prices_current.json
  refresh_interval: 6h
```

### `pricing.enabled`

Type: `bool`

Enables pricing metadata fetches.

Current routing does not depend on successful pricing fetches. Routing continues if the URL is unavailable.

### `pricing.url`

Type: `string`

URL fetched through the host HTTP callback. The runtime state stores only fetch metadata such as byte count and errors, not the full pricing body.

### `pricing.refresh_interval`

Type: Go duration string

If invalid or empty, the runtime fallback is `6h`.

## `cache`

Controls route decision caching.

Example:

```yaml
cache:
  enabled: true
  max_entries: 1024
  ttl: 24h
```

### `cache.enabled`

Type: `bool`

Enables decision caching.

When enabled, repeated prompts can reuse the same route decision without calling the classifier again.

### `cache.max_entries`

Type: `int`

Maximum number of route decisions retained in memory/state.

Default in code when `<= 0`: `1024`

When full, the least recently used entry is evicted.

### `cache.ttl`

Type: Go duration string

Optional time-to-live for cache entries.

Examples:

- `24h`
- `6h`
- `30m`

If empty or invalid, entries do not expire by age and are removed only by LRU capacity pressure.

Cache keys are hashes of the last user message plus the virtual model. This makes identical prompts deterministic even when surrounding conversation history changes.

## `executor_fallback`

Controls optional same-request execution fallback.

Example:

```yaml
executor_fallback:
  enabled: false
  max_attempts: 3
```

### `executor_fallback.enabled`

Type: `bool`

When `false`, routing returns `TargetKind: provider`, and CLIProxyAPI executes the selected provider/model.

When `true`, the plugin declares executor capability and non-streaming routed requests can target the plugin itself (`TargetKind: self`) for same-request fallback attempts.

Streaming executor fallback is not implemented. Streaming requests should use the provider route path.

Keep this disabled unless same-request retry inside the plugin is explicitly needed.

### `executor_fallback.max_attempts`

Type: `int`

Maximum number of configured candidates the executor fallback tries.

Default in code when `<= 0`: `3`

## `classifier`

Controls optional LLM-based semantic routing.

Example:

```yaml
classifier:
  enabled: true
  models:
    - provider: codex
      model: gpt-5.4-mini
    - provider: claude
      model: claude-haiku-4-5-20251001
  timeout: 8s
  max_attempts: 2
```

### `classifier.enabled`

Type: `bool`

Enables classifier routing attempts.

Behavior by strategy:

- `llm`: the classifier is attempted on every request when enabled.
- `hybrid`: the classifier is attempted only when the local capability-aware decision is not confident (see `strategy` above and `models[].capabilities` below).
- `capability` / `benchmark`: the classifier is never attempted regardless of this setting.

If disabled, or if the strategy is not `llm`/`hybrid`, the local deterministic decision is used directly.

### `classifier.models`

Type: list of classifier model entries

Classifier models are tried in order up to `classifier.max_attempts`.

Each classifier entry has:

- `provider`: provider key, normalized to lowercase
- `model`: model ID

Classifier models should also exist in the configured `models` list. This keeps provider/model metadata explicit.

The classifier request is isolated from the original user request. It receives a routing prompt, the configured model catalog, and only the extracted last user message. It must return compact JSON:

```json
{"selected_model":"<id>","confidence":0.9,"reason":"short reason"}
```

If the classifier fails, returns invalid JSON, selects an unknown model, or selects a provider that is not available, the plugin falls back to deterministic routing.

### `classifier.timeout`

Type: Go duration string

Currently parsed/normalized but not enforced by the plugin's host callback call path.

### `classifier.max_attempts`

Type: `int`

Maximum number of classifier entries to try.

If `<= 0` or greater than the number of configured classifier models, all configured classifier models may be tried.

## `routing`

Policy knobs for scoring and route stability.

Example:

```yaml
routing:
  prefer_low_cost: true
  prefer_low_latency: false
  prefer_high_quality: true
  max_cost_per_request: 0.05
  max_input_tokens: 100000
  keep_same_model_per_session: true
  allow_fallback: true
  switch_threshold: 0.15
  benchmark_weight: 0.4
  llm_router_weight: 0.3
  capability_weight: 0.3
```

### `routing.prefer_low_cost`

Type: `bool`

Policy metadata for cost-aware routing. The active cost/quality bias is currently controlled by `preference` and candidate `cost` values.

### `routing.prefer_low_latency`

Type: `bool`

Policy metadata reserved for latency-aware scoring. Current routing does not compute latency scores from this field.

### `routing.prefer_high_quality`

Type: `bool`

Policy metadata for quality-aware routing. The active quality bias is currently controlled by `preference` and candidate `quality` values.

### `routing.max_cost_per_request`

Type: `float`

Policy metadata reserved for future cost ceilings. Current routing does not enforce this as a hard limit.

### `routing.max_input_tokens`

Type: `int`

Policy metadata reserved for future token-limit scoring. Current routing does not count prompt tokens from this value.

### `routing.keep_same_model_per_session`

Type: `bool`

When enabled, the plugin pins a session ID to the selected route.

Session IDs are read from request metadata key `execution_session_id`.

If a session route exists, it is used before cache and classifier selection.

### `routing.allow_fallback`

Type: `bool`

Policy metadata. Current routing always keeps deterministic fallback available when classifier selection fails.

### `routing.switch_threshold`

Type: `float`

Policy metadata reserved for future score-based switching.

### `routing.benchmark_weight`

Type: `float`

Policy metadata reserved for future weighted benchmark scoring.

### `routing.llm_router_weight`

Type: `float`

Policy metadata reserved for future weighted classifier scoring.

### `routing.capability_weight`

Type: `float`

Policy metadata reserved for future weighted capability scoring.

## `models`

Authoritative provider/model matrix.

Example:

```yaml
models:
  - provider: claude
    model: claude-sonnet-5
    capabilities: [coding, reasoning, review, refactor, tools, architecture, writing]
    cost: high
    quality: high
```

Each configured model entry has the following fields.

### `models[].provider`

Type: `string`

Provider key used by CLIProxyAPI when routing with `TargetKind: provider`.

Known provider keys in the example:

- `claude`: Anthropic/Claude provider path
- `codex`: OpenAI/Codex/GPT-compatible provider path

Provider names are normalized to lowercase.

### `models[].model`

Type: `string`

Target model ID sent to the selected provider.

Examples:

- `claude-opus-4-8`
- `claude-sonnet-5`
- `claude-haiku-4-5-20251001`
- `gpt-5.5`
- `gpt-5.4`
- `gpt-5.4-mini`

### `models[].capabilities`

Type: list of strings

Capability tags normalized to lowercase. These tags are used in two places:

- classifier context, as before;
- local capability-aware scoring (every strategy): the router infers capability tags from the prompt using a small keyword matcher (for example, "resuma"/"summarize" implies `summarize`; "arquitetura"/"architecture" implies `architecture`/`planning`; "erro"/"bug" implies `coding`/`review`). If the prompt matches an inferred capability and at least one candidate declares it, only candidates declaring a matching capability are eligible for that request; `preference` then ranks the remaining pool by cost/quality tier.

Candidates that omit `capabilities` are never excluded by this gate on their own account, but they can be excluded by *other* candidates' matching capabilities when the prompt clearly implies a capability none of them declare. If no configured candidate declares any inferred capability, the gate has no effect for that request and ranking falls back to plain cost/quality tier ranking, same as when `capabilities` is omitted entirely.

Configuring accurate `capabilities` is what allows `strategy: hybrid` to skip the classifier for prompts the configuration already answers clearly. See `docs/adr/0003-local-first-hybrid-routing.md`.

Common tags in the example:

- `reasoning`
- `architecture`
- `planning`
- `analysis`
- `writing`
- `long_context`
- `high_quality`
- `coding`
- `review`
- `refactor`
- `tools`
- `classify`
- `summarize`
- `translate`
- `fast`
- `low_cost`
- `routing`
- `agents`
- `documentation`
- `tests`
- `scripts`
- `general`
- `simple_coding`

### `models[].cost`

Type: `string`

Recognized cost tiers:

- `low`
- `medium`
- `high`
- `very_high`

Unknown or empty values are treated as `medium` by the routing ranker.

Cost rank affects deterministic fallback and preference tiebreaking.

### `models[].quality`

Type: `string`

Recognized quality tiers:

- `low`
- `medium`
- `high`
- `highest`

Unknown or empty values are treated as `medium` by the routing ranker.

Quality rank affects deterministic fallback and preference tiebreaking.

## Decision Order

For a request matching `virtual_model`, route selection currently happens in this order:

1. Session route, when `routing.keep_same_model_per_session` is enabled and a session route exists.
2. Cache route, when `cache.enabled` is true and a valid cache entry exists.
3. Local capability-aware deterministic decision, computed for every strategy (no classifier call).
4. `hybrid`: use the local decision directly if it is confident; otherwise call the classifier. `llm`: always call the classifier. `capability`/`benchmark`: always use the local decision from step 3.
5. If the classifier is called and fails, use the local decision from step 3.

After classifier selection, `preference` may apply a conservative tiebreak among equivalent-tier candidates.

The selected route is stored in cache and optionally pinned to the session.

## Local Capability-Aware Decision

The local decision selects an available configured candidate that is strong enough for the prompt, using local text signals plus configured `capabilities`. It is computed for every strategy and used directly by `capability`/`benchmark`, as the direct decision for confident `hybrid` requests, and as the fallback when a classifier call fails or is skipped.

Current local signals for minimum cost tier:

- Simple arithmetic/classification/summary requests can use `low` cost candidates.
- Prompts containing signals such as `detalhes`, `como funciona`, `explique`, `código`, or `codigo` require at least `high` cost candidates.

Current local signals for capability gating (see `models[].capabilities` above for the full list): `resuma`/`summarize` implies `summarize`; `traduza`/`translate` implies `translate`; `classifique`/`classify` implies `classify`; `erro`/`bug`/`falha` implies `coding`/`review`; `refatora`/`refactor` implies `refactor`/`coding`; `arquitetura`/`design`/`planeje` implies `architecture`/`planning`; `explique`/`analise`/`compare` implies `reasoning`/`analysis`; `teste`/`coverage` implies `tests`/`coding`; `agente`/`ferramenta`/`tool` implies `agents`/`tools`; long prompts imply `long_context`.

If the prompt matches an inferred capability and at least one eligible candidate declares it, only candidates declaring a matching capability remain eligible. Otherwise this gate has no effect.

Then `preference` decides the ranking of the gated pool:

- `quality`: highest quality first, cheaper model as secondary tie-break.
- `cost` or `balanced`: lowest cost first, higher quality as secondary tie-break.

The decision is considered "locally confident" only when the prompt matched an inferred capability and the winning candidate declares it. `hybrid` uses this to decide whether to call the classifier.

## Classifier Behavior

The classifier is a small model call used only to choose a target model. It does not answer the user's request.

`llm` calls the classifier on every request. `hybrid` calls the classifier only when the local decision above is not confident.

The classifier receives:

- configured model catalog
- cost/quality/capability metadata
- the extracted last user message
- the configured `preference` instruction

It returns a selected model ID, confidence, and short reason. The selected model must exist in `models` and its provider must be available.

If anything fails, routing falls back to the local capability-aware decision.

## Cache Behavior

The route cache is deterministic for repeated prompts.

Cache key material:

- `virtual_model`
- extracted last user message

The raw prompt is not written to logs or state; only a hash is used as the cache key.

Cache storage:

- in memory during runtime
- persisted in `state_path` when configured
- written with throttling to avoid rewriting the state file on every request
- LRU eviction when `max_entries` is reached
- optional TTL expiration

## Debug Log Example

Example JSONL decision log using the classifier (`llm`, or `hybrid` with a non-confident local decision):

```json
{"time":"2026-07-02T07:29:55.089425433-03:00","source":"selected","virtual_model":"claude-auto","source_format":"openai","stream":true,"strategy":"llm","preference":"balanced","target_provider":"claude","target_model":"claude-sonnet-5","reason":"classifier:Pedido envolve arquitetura, implementação em Go, JWT e plano de performance; Sonnet atende bem sem subir ao modelo mais caro.","classifier":{"enabled":true,"used":true,"model":"gpt-5.4-mini","response":"{\"selected_model\":\"claude-sonnet-5\",\"confidence\":0.94,\"reason\":\"...\"}"}}
```

Example JSONL decision log for a `hybrid` request that skipped the classifier because the local decision was confident:

```json
{"time":"2026-07-02T07:31:10.221004112-03:00","source":"selected","virtual_model":"router:auto","source_format":"openai","stream":false,"strategy":"hybrid","preference":"balanced","target_provider":"codex","target_model":"gpt-5.4-mini","reason":"local_confident deterministic_fallback strategy:hybrid capabilities:1/1 preference:balanced provider:codex cost:low"}
```

Possible `source` values:

- `selected`: freshly selected by classifier or the local deterministic decision
- `cache`: reused from route cache
- `session`: reused from session affinity

A `reason` prefixed with `local_confident` indicates `hybrid` used the local decision and skipped the classifier for that request.

## Security Notes

Do not commit real values for:

- `catalog.api_key`
- CLIProxyAPI `api-keys`
- provider credentials
- auth records

The plugin is designed not to persist or log prompts, request bodies, credentials, API keys, auth records, or response bodies.
